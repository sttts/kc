package termctx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	clientcmd "k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func writeTestConfig(t *testing.T) (string, *clientcmdapi.Config) {
	t.Helper()
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["cluster-a"] = &clientcmdapi.Cluster{Server: "https://cluster-a"}
	cfg.AuthInfos["user-a"] = &clientcmdapi.AuthInfo{Token: "fake"}
	cfg.Contexts["ctx-a"] = &clientcmdapi.Context{
		Cluster:  "cluster-a",
		AuthInfo: "user-a",
	}
	cfg.CurrentContext = "ctx-a"
	base := filepath.Join(t.TempDir(), "base.yaml")
	if err := clientcmd.WriteToFile(*cfg, base); err != nil {
		t.Fatalf("write base config: %v", err)
	}
	return base, cfg
}

func TestManagerUpdate(t *testing.T) {
	base, cfg := writeTestConfig(t)
	mgr, err := NewManager(base, cfg, ModeOverlay)
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
	base, cfg := writeTestConfig(t)
	mgr, err := NewManager(base, cfg, ModeOverlay)
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
	if !strings.Contains(content, "cluster: cluster-a") {
		t.Fatalf("overlay missing cluster: %s", content)
	}
	if !strings.Contains(content, "user: user-a") {
		t.Fatalf("overlay missing user: %s", content)
	}
}

func TestDetectExternalChange(t *testing.T) {
	base, cfg := writeTestConfig(t)
	mgr, err := NewManager(base, cfg, ModeOverlay)
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
	if err := os.WriteFile(mgr.overlay, []byte(body), 0o600); err != nil {
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

func TestManagerCopyMode(t *testing.T) {
	base, cfg := writeTestConfig(t)
	mgr, err := NewManager(base, cfg, ModeCopy)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	if err := mgr.Update("ctx-a", "team-a"); err != nil {
		t.Fatalf("update copy: %v", err)
	}
	copyCfg, err := clientcmd.LoadFromFile(mgr.EnvValue())
	if err != nil {
		t.Fatalf("load copy: %v", err)
	}
	if copyCfg.Contexts["ctx-a"].Namespace != "team-a" {
		t.Fatalf("expected namespace team-a, got %s", copyCfg.Contexts["ctx-a"].Namespace)
	}
}
