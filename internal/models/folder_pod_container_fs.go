package models

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sttts/kc/internal/podfs"
	table "github.com/sttts/kc/internal/table"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	fsDefaultTimeout     = 10 * time.Second
	fsFileViewMaxBytes   = 512 * 1024
	fsUnavailableMessage = "Pod filesystem browsing unavailable"
)

// PodContainerDetailFolder exposes container-level virtual entries (logs, root filesystem, etc.).
type PodContainerDetailFolder struct {
	*BaseFolder
	Namespace string
	Pod       string
	Container string

	session *containerSessionHandle
}

func NewPodContainerDetailFolder(deps Deps, path []string, namespace, pod, container string) *PodContainerDetailFolder {
	base := NewBaseFolder(deps, []table.Column{{Title: " Name"}}, path)
	handle := newContainerSessionHandle(deps.PodFSFactory, namespace, pod, container)
	folder := &PodContainerDetailFolder{
		BaseFolder: base,
		Namespace:  namespace,
		Pod:        pod,
		Container:  container,
		session:    handle,
	}
	base.SetPopulate(folder.buildRows)
	return folder
}

func (f *PodContainerDetailFolder) buildRows(context.Context) ([]table.Row, error) {
	rows := make([]table.Row, 0, 2)
	logSpec := LogsSpec{
		Namespace: f.Namespace,
		Pod:       f.Pod,
		Container: f.Container,
		Follow:    true,
		TailLines: DefaultLogsTailLines,
	}
	logPath := append(append([]string{}, f.Path()...), "logs", "latest")
	logItem := NewContainerLogItem("logs_latest", []string{"/logs"}, logPath, logSpec)
	logItem.RowItem.details = "Stream container logs"
	rows = append(rows, logItem)

	rootPath := append(append([]string{}, f.Path()...), "root")
	if f.session == nil {
		unavailable := NewSimpleItem("root_unavailable", []string{"/root"}, rootPath, DimStyle())
		unavailable.RowItem.details = fsUnavailableMessage
		rows = append(rows, unavailable)
		return rows, nil
	}
	rootItem := NewContainerSectionItem("root", []string{"/root"}, rootPath, WhiteStyle(), func() (Folder, error) {
		return NewPodContainerFSFolder(f.Deps, rootPath, "/", f.session), nil
	})
	if f.session != nil && f.session.HelperUsed() {
		rootItem.RowItem.details = "Browse container filesystem (debug helper)"
	} else {
		rootItem.RowItem.details = "Browse container filesystem"
	}
	rows = append(rows, rootItem)
	return rows, nil
}

// PodContainerFSFolder lists filesystem entries for a container under a given directory.
type PodContainerFSFolder struct {
	*BaseFolder
	dir     string
	session *containerSessionHandle
}

func NewPodContainerFSFolder(deps Deps, path []string, dir string, session *containerSessionHandle) *PodContainerFSFolder {
	if dir == "" {
		dir = "/"
	}
	base := NewBaseFolder(deps, []table.Column{
		{Title: " Name"},
		{Title: " Type"},
		{Title: " Size"},
		{Title: " Modified"},
	}, path)
	folder := &PodContainerFSFolder{
		BaseFolder: base,
		dir:        sanitizeContainerPath(dir),
		session:    session,
	}
	base.SetPopulate(folder.buildRows)
	return folder
}

func (f *PodContainerFSFolder) buildRows(ctx context.Context) ([]table.Row, error) {
	if f.session == nil {
		item := NewSimpleItem("root_unavailable", []string{"unavailable"}, f.Path(), DimStyle())
		item.RowItem.details = fsUnavailableMessage
		item.WithViewContent(errorViewContent("Pod filesystem unavailable", fsUnavailableMessage))
		return []table.Row{item}, nil
	}
	listCtx, cancel := deriveContext(ctx, f.Deps.Ctx, fsDefaultTimeout)
	defer cancel()
	sess, err := f.session.Session(listCtx)
	if err != nil {
		log := ctrllog.FromContext(listCtx).WithName("podfs_folder").
			WithValues("namespace", f.session.spec.Namespace, "pod", f.session.spec.Pod, "container", f.session.spec.Container)
		log.Error(err, "Session establishment failed")
		item := NewSimpleItem("root_error", []string{"error"}, f.Path(), DimStyle())
		detail := describeFSError("Open filesystem", err)
		item.RowItem.details = detail
		item.WithViewContent(errorViewContent("Pod filesystem", detail))
		return []table.Row{item}, nil
	}
	entries, err := sess.List(listCtx, f.dir)
	if err != nil {
		log := ctrllog.FromContext(listCtx).WithName("podfs_folder").
			WithValues("namespace", f.session.spec.Namespace, "pod", f.session.spec.Pod, "container", f.session.spec.Container, "dir", f.dir)
		log.Error(err, "List failed")
		item := NewSimpleItem("root_error", []string{"error"}, f.Path(), DimStyle())
		detail := describeFSError("List directory", err)
		item.RowItem.details = detail
		item.WithViewContent(errorViewContent("Pod filesystem", detail))
		return []table.Row{item}, nil
	}
	rows := make([]table.Row, 0, len(entries))
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Type == entries[j].Type {
			return entries[i].Name < entries[j].Name
		}
		if entries[i].Type == podfs.EntryTypeDir {
			return true
		}
		if entries[j].Type == podfs.EntryTypeDir {
			return false
		}
		return entries[i].Name < entries[j].Name
	})
	for _, entry := range entries {
		rowPath := append(append([]string{}, f.Path()...), entry.Name)
		displaySize := formatBytes(entry.Size)
		mod := formatModTime(entry.UpdatedAt)
		cells := []string{entry.Name, string(entry.Type), displaySize, mod}
		entryPath := joinContainerPath(f.dir, entry.Name)
		switch entry.Type {
		case podfs.EntryTypeDir:
			item := NewPodFSDirItem(entryPath, cells, rowPath, WhiteStyle(), func(nextDir string) func() (Folder, error) {
				return func() (Folder, error) {
					return NewPodContainerFSFolder(f.Deps, rowPath, nextDir, f.session), nil
				}
			}(entryPath))
			item.RowItem.details = fmt.Sprintf("Directory (%s)", entryPath)
			rows = append(rows, item)
		case podfs.EntryTypeSymlink:
			fileItem := newPodFSFileItem(entryPath, cells, rowPath, entryPath, entry.Size, entry.Target, f)
			fileItem.RowItem.details = fmt.Sprintf("Symlink → %s", entry.Target)
			rows = append(rows, fileItem)
		default:
			fileItem := newPodFSFileItem(entryPath, cells, rowPath, entryPath, entry.Size, "", f)
			fileItem.RowItem.details = fmt.Sprintf("%s bytes", displaySize)
			rows = append(rows, fileItem)
		}
	}
	return rows, nil
}

type PodFSDirItem struct {
	*RowItem
	enter func() (Folder, error)
}

func NewPodFSDirItem(id string, cells []string, path []string, style *lipgloss.Style, enter func() (Folder, error)) *PodFSDirItem {
	return &PodFSDirItem{RowItem: NewRowItem(id, cells, path, style), enter: enter}
}

func (d *PodFSDirItem) Enter() (Folder, error) {
	if d.enter == nil {
		return nil, nil
	}
	return d.enter()
}

func newPodFSFileItem(id string, cells []string, path []string, fsPath string, size int64, target string, folder *PodContainerFSFolder) *SimpleItem {
	item := NewSimpleItem(id, cells, path, WhiteStyle())
	detailsSuffix := ""
	if target != "" {
		detailsSuffix = fmt.Sprintf(" → %s", target)
	}
	item.RowItem.details = fmt.Sprintf("%d bytes%s", size, detailsSuffix)
	item.WithViewContent(podFSFileViewContent(folder, fsPath, size))
	return item
}

func podFSFileViewContent(folder *PodContainerFSFolder, fsPath string, size int64) ViewContentFunc {
	return func() (string, string, string, string, string, error) {
		if folder == nil || folder.session == nil {
			return "", "", "", "", "", fmt.Errorf("pod filesystem unavailable")
		}
		readCtx, cancel := deriveContext(nil, folder.Deps.Ctx, fsDefaultTimeout)
		defer cancel()
		sess, err := folder.session.Session(readCtx)
		if err != nil {
			return "", "", "", "", "", err
		}
		limit := size
		if limit <= 0 || limit > fsFileViewMaxBytes {
			limit = fsFileViewMaxBytes
		}
		reader, err := sess.ReadFile(readCtx, fsPath, limit)
		if err != nil {
			return "", "", "", "", "", err
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		if err != nil {
			return "", "", "", "", "", err
		}
		body := string(data)
		filename := path.Base(fsPath)
		return fsPath, body, "", "", filename, nil
	}
}

type containerSessionHandle struct {
	factory podfs.Factory
	spec    podfs.SessionSpec

	mu      sync.Mutex
	session podfs.ExecSession
	helper  bool
}

func newContainerSessionHandle(factory podfs.Factory, namespace, pod, container string) *containerSessionHandle {
	if factory == nil {
		return nil
	}
	handle := &containerSessionHandle{
		factory: factory,
		spec: podfs.SessionSpec{
			Namespace: namespace,
			Pod:       pod,
			Container: container,
		},
	}
	return handle
}

func (h *containerSessionHandle) Session(ctx context.Context) (podfs.ExecSession, error) {
	if h == nil {
		return nil, fmt.Errorf("pod filesystem unavailable")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.session != nil {
		return h.session, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	session, err := h.factory.NewSession(ctx, h.spec)
	if err != nil {
		return nil, err
	}
	if usage, ok := session.(podfs.HelperUsage); ok {
		h.helper = usage.HelperUsed()
	} else {
		h.helper = false
	}
	h.session = session
	return session, nil
}

func (h *containerSessionHandle) Close() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.session == nil {
		return nil
	}
	err := h.session.Close()
	h.session = nil
	h.helper = false
	return err
}

func (h *containerSessionHandle) HelperUsed() bool {
	if h == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.helper
}

func formatBytes(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	const unit = 1024
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	value := float64(size) / float64(div)
	return fmt.Sprintf("%.1f %ciB", value, "KMGTPE"[exp])
}

func formatModTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Local().Format("2006-01-02 15:04")
}

func sanitizeContainerPath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}

func joinContainerPath(base, name string) string {
	base = sanitizeContainerPath(base)
	return sanitizeContainerPath(path.Join(base, name))
}

func describeFSError(action string, err error) string {
	if err == nil {
		return ""
	}
	var missing podfs.MissingCommandError
	switch {
	case errors.Is(err, podfs.ErrShellMissing):
		return "Filesystem browsing unavailable: container image lacks /bin/sh."
	case errors.As(err, &missing):
		return fmt.Sprintf("Filesystem browsing unavailable: missing required command %s", missing.Command)
	default:
		return fmt.Sprintf("%s failed: %v", action, err)
	}
}

func errorViewContent(title, detail string) ViewContentFunc {
	return func() (string, string, string, string, string, error) {
		return title, detail + "\n", "", "text/plain", "error.txt", nil
	}
}

func deriveContext(ctx context.Context, fallback context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = fallback
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}
