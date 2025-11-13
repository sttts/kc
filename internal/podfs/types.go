package podfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

var (
	// ErrShellMissing indicates the container image lacks a usable /bin/sh.
	ErrShellMissing = errors.New("pod filesystem: shell missing")
)

// MissingCommandError reports that a required binary is absent in the container image.
type MissingCommandError struct {
	Command string
}

func (e MissingCommandError) Error() string {
	return fmt.Sprintf("pod filesystem: missing required command %s", e.Command)
}

// EntryType classifies filesystem entries returned by ExecSession.List.
type EntryType string

const (
	EntryTypeFile    EntryType = "file"
	EntryTypeDir     EntryType = "dir"
	EntryTypeSymlink EntryType = "symlink"
	EntryTypeSocket  EntryType = "socket"
	EntryTypePipe    EntryType = "pipe"
	EntryTypeDevice  EntryType = "device"
	EntryTypeOther   EntryType = "other"
)

// FileEntry is a portable description of a remote filesystem entry.
type FileEntry struct {
	Name      string
	Path      string
	Type      EntryType
	Size      int64
	Mode      uint32
	UpdatedAt time.Time
	Target    string // symlink target if available
}

// SessionSpec identifies the container to exec into.
type SessionSpec struct {
	Namespace string
	Pod       string
	Container string
}

// ExecSession provides filesystem operations backed by a long-lived exec stream.
type ExecSession interface {
	List(ctx context.Context, path string) ([]FileEntry, error)
	ReadFile(ctx context.Context, path string, limit int64) (io.ReadCloser, error)
	Close() error
}

// Factory constructs ExecSessions on demand.
type Factory interface {
	NewSession(ctx context.Context, spec SessionSpec) (ExecSession, error)
}
