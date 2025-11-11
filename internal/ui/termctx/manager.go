package termctx

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const hashPrefix = "# kc overlay hash:"

// Manager owns the overlay kubeconfig for a PTY session.
type Manager struct {
	dir        string
	overlay    string
	baseConfig string
	lastHash   string
}

// NewManager creates a manager under a per-user temp root and cleans stale dirs.
func NewManager(baseConfig string) (*Manager, error) {
	root := filepath.Join(os.TempDir(), fmt.Sprintf("kc-%d", os.Getuid()))
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	reap(root)
	dir, err := os.MkdirTemp(root, fmt.Sprintf("%d-", os.Getpid()))
	if err != nil {
		return nil, err
	}
	overlay := filepath.Join(dir, "overlay.yaml")
	return &Manager{dir: dir, overlay: overlay, baseConfig: baseConfig}, nil
}

// OverlayPath returns the overlay kubeconfig path.
func (m *Manager) OverlayPath() string { return m.overlay }

// BaseConfig returns the underlying kubeconfig.
func (m *Manager) BaseConfig() string { return m.baseConfig }

// Update rewrites the overlay with the given context+namespace.
func (m *Manager) Update(ctxName, namespace string) error {
	body := renderOverlay(ctxName, namespace)
	h := sha256.Sum256([]byte(body))
	hashLine := fmt.Sprintf("%s %s\n", hashPrefix, hex.EncodeToString(h[:]))
	contents := hashLine + body
	if err := os.WriteFile(m.overlay, []byte(contents), 0o600); err != nil {
		return err
	}
	m.lastHash = hex.EncodeToString(h[:])
	return nil
}

// DetectExternalChange reports when the overlay hash changes outside kc.
func (m *Manager) DetectExternalChange() (changed bool, ctxName, namespace string, err error) {
	data, err := os.ReadFile(m.overlay)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", "", nil
		}
		return false, "", "", err
	}
	sections := strings.SplitN(string(data), "\n", 2)
	if len(sections) < 2 {
		return false, "", "", nil
	}
	line := sections[0]
	if !strings.HasPrefix(line, hashPrefix) {
		ctx, ns := parseOverlay(sections[1])
		return true, ctx, ns, nil
	}
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return false, "", "", nil
	}
	hash := fields[len(fields)-1]
	if hash == m.lastHash {
		return false, "", "", nil
	}
	ctxName, ns := parseOverlay(sections[1])
	return true, ctxName, ns, nil
}

// Close removes the overlay temp dir.
func (m *Manager) Close() error {
	return os.RemoveAll(m.dir)
}

func renderOverlay(ctxName, namespace string) string {
	if namespace == "" {
		return fmt.Sprintf("apiVersion: v1\nkind: Config\ncurrent-context: %s\n", ctxName)
	}
	return fmt.Sprintf(`apiVersion: v1
kind: Config
contexts:
- name: %[1]s
  context:
    cluster: PLACEHOLDER_CLUSTER
    user: PLACEHOLDER_USER
    namespace: %[2]s
current-context: %[1]s
`, ctxName, namespace)
}

func parseOverlay(body string) (ctxName, namespace string) {
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "current-context:") {
			ctxName = strings.TrimSpace(strings.TrimPrefix(line, "current-context:"))
		}
		if strings.HasPrefix(line, "namespace:") {
			namespace = strings.TrimSpace(strings.TrimPrefix(line, "namespace:"))
		}
	}
	return ctxName, namespace
}

func reap(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !info.IsDir() {
			continue
		}
		name := entry.Name()
		parts := strings.Split(name, "-")
		if len(parts) == 0 {
			continue
		}
		pid, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		if err := syscall.Kill(pid, 0); err == nil || err == syscall.EPERM {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, name))
	}
}
