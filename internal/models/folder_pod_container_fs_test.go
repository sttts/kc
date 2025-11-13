package models

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sttts/kc/internal/podfs"
)

type fakePodFSFactory struct {
	session podfs.ExecSession
	err     error
}

func (f fakePodFSFactory) NewSession(context.Context, podfs.SessionSpec) (podfs.ExecSession, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.session, nil
}

type fakeExecSession struct {
	entries  []podfs.FileEntry
	fileData map[string]string
}

func (f *fakeExecSession) List(context.Context, string) ([]podfs.FileEntry, error) {
	return f.entries, nil
}

func (f *fakeExecSession) ReadFile(_ context.Context, path string, limit int64) (io.ReadCloser, error) {
	if data, ok := f.fileData[path]; ok {
		if limit > 0 && int(limit) < len(data) {
			data = data[:limit]
		}
		return io.NopCloser(strings.NewReader(data)), nil
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *fakeExecSession) Close() error { return nil }

func TestPodContainerFSFolderListsEntries(t *testing.T) {
	ctx := context.Background()
	session := &fakeExecSession{
		entries: []podfs.FileEntry{
			{Name: "etc", Type: podfs.EntryTypeDir, Size: 0, UpdatedAt: time.Unix(1, 0)},
			{Name: "readme.txt", Type: podfs.EntryTypeFile, Size: 12, UpdatedAt: time.Unix(2, 0)},
		},
		fileData: map[string]string{"/readme.txt": "hello world\n"},
	}
	deps := Deps{
		Ctx:          ctx,
		PodFSFactory: fakePodFSFactory{session: session},
	}
	handle := newContainerSessionHandle(deps.PodFSFactory, "ns", "pod", "container")
	folder := NewPodContainerFSFolder(deps, []string{"containers", "pod", "root"}, "/", handle)

	rows := folder.Lines(ctx, 0, 10)
	if len(rows) < 3 { // Back + two entries
		t.Fatalf("expected at least 3 rows, got %d", len(rows))
	}

	fileItem, ok := rows[2].(*SimpleItem)
	if !ok {
		t.Fatalf("expected simple item for file, got %T", rows[2])
	}
	title, body, _, _, _, err := fileItem.ViewContent()
	if err != nil {
		t.Fatalf("ViewContent error: %v", err)
	}
	if title != "/readme.txt" {
		t.Fatalf("title = %q, want /readme.txt", title)
	}
	if !strings.Contains(body, "hello world") {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestPodContainerFSFolderMissingShellMessage(t *testing.T) {
	ctx := context.Background()
	deps := Deps{
		Ctx:          ctx,
		PodFSFactory: fakePodFSFactory{err: fmt.Errorf("%w", podfs.ErrShellMissing)},
	}
	handle := newContainerSessionHandle(deps.PodFSFactory, "ns", "pod", "container")
	folder := NewPodContainerFSFolder(deps, []string{"containers", "pod", "root"}, "/", handle)

	rows := folder.Lines(ctx, 0, 5)
	if len(rows) < 2 {
		t.Fatalf("expected rows")
	}
	errRow, ok := rows[1].(*SimpleItem)
	if !ok {
		t.Fatalf("expected SimpleItem, got %T", rows[1])
	}
	if !strings.Contains(errRow.Details(), "lacks /bin/sh") {
		t.Fatalf("details = %q, want mention of missing shell", errRow.Details())
	}
}
