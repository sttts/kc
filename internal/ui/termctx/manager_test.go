package termctx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerUpdate(t *testing.T) {
	base := "/home/user/.kube/config"
	mgr, err := NewManager(base)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	if err := mgr.Update("ctx-a", ""); err != nil {
		t.Fatalf("update: %v", err)
	}
	data, err := os.ReadFile(mgr.OverlayPath())
	if err != nil {
		t.Fatalf("read overlay: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "current-context: ctx-a") {
		t.Fatalf("overlay missing ctx: %s", content)
	}
	if !strings.Contains(content, hashPrefix) {
		t.Fatalf("overlay missing hash comment")
	}
}

func TestManagerNamespaceOverlay(t *testing.T) {
	mgr, err := NewManager("/home/user/.kube/config")
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	if err := mgr.Update("ctx-a", "team-a"); err != nil {
		t.Fatalf("update: %v", err)
	}
	data, err := os.ReadFile(mgr.OverlayPath())
	if err != nil {
		t.Fatalf("read overlay: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "namespace: team-a") {
		t.Fatalf("overlay missing namespace: %s", content)
	}
}

func TestDetectExternalChange(t *testing.T) {
	mgr, err := NewManager("/home/user/.kube/config")
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	if err := mgr.Update("ctx-a", ""); err != nil {
		t.Fatalf("update: %v", err)
	}
	changed, _, _, err := mgr.DetectExternalChange()
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if changed {
		t.Fatalf("expected no change")
	}
	// simulate external change
	body := "apiVersion: v1\nkind: Config\ncurrent-context: ctx-b\n"
	if err := os.WriteFile(mgr.OverlayPath(), []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	changed, ctxName, _, err := mgr.DetectExternalChange()
	if err != nil {
		t.Fatalf("detect external: %v", err)
	}
	if !changed || ctxName != "ctx-b" {
		t.Fatalf("expected external ctx-b change, got changed=%v ctx=%s", changed, ctxName)
	}
}

func TestReapRemovesStale(t *testing.T) {
	root := filepath.Join(os.TempDir(), "kc-test")
	_ = os.MkdirAll(root, 0o755)
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	dir := filepath.Join(root, "1234-stale")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	reap(root)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected stale dir removed")
	}
}
