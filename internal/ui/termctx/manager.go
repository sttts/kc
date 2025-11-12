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

	clientcmd "k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const hashPrefix = "# kc overlay hash:"

type Mode string

const (
	ModeOverlay Mode = "overlay"
	ModeCopy    Mode = "copy"
)

// Manager owns the kubeconfig material for a PTY session.
type Manager struct {
	dir      string
	envValue string
	mode     Mode
	basePath string
	overlay  string
	copyPath string
	lastHash string
	template *clientcmdapi.Config
}

// NewManager creates a manager under a per-user temp root and cleans stale dirs.
func NewManager(basePath string, template *clientcmdapi.Config, mode Mode) (*Manager, error) {
	root := filepath.Join(os.TempDir(), fmt.Sprintf("kc-%d", os.Getuid()))
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	reap(root)
	dir, err := os.MkdirTemp(root, fmt.Sprintf("%d-", os.Getpid()))
	if err != nil {
		return nil, err
	}
	m := &Manager{
		dir:      dir,
		mode:     mode,
		basePath: basePath,
	}
	switch mode {
	case ModeCopy:
		cp := filepath.Join(dir, "config.yaml")
		if template != nil {
			if err := clientcmd.WriteToFile(*template.DeepCopy(), cp); err != nil {
				return nil, err
			}
		} else {
			data, err := os.ReadFile(basePath)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(cp, data, 0o600); err != nil {
				return nil, err
			}
		}
		m.copyPath = cp
		m.envValue = cp
	default:
		overlay := filepath.Join(dir, "overlay.yaml")
		m.overlay = overlay
		m.envValue = fmt.Sprintf("%s:%s", overlay, basePath)
	}
	if template != nil {
		m.template = template.DeepCopy()
	}
	return m, nil
}

// EnvValue returns the string to use for KUBECONFIG.
func (m *Manager) EnvValue() string { return m.envValue }

// OverlayPath returns the overlay file path (overlay mode only).
func (m *Manager) OverlayPath() string { return m.overlay }

// Update rewrites the config with the given context+namespace.
func (m *Manager) Update(ctxName, namespace string) error {
	switch m.mode {
	case ModeCopy:
		return m.updateCopy(ctxName, namespace)
	default:
		return m.updateOverlay(ctxName, namespace)
	}
}

func (m *Manager) updateOverlay(ctxName, namespace string) error {
	var cluster, user string
	if namespace != "" {
		ctx, err := m.lookupContext(ctxName)
		if err != nil {
			return err
		}
		cluster = ctx.Cluster
		user = ctx.AuthInfo
	}
	body := renderOverlay(ctxName, namespace, cluster, user)
	h := sha256.Sum256([]byte(body))
	hashLine := fmt.Sprintf("%s %s\n", hashPrefix, hex.EncodeToString(h[:]))
	contents := hashLine + body
	if err := os.WriteFile(m.overlay, []byte(contents), 0o600); err != nil {
		return err
	}
	m.lastHash = hex.EncodeToString(h[:])
	return nil
}

func (m *Manager) updateCopy(ctxName, namespace string) error {
	cfg, err := clientcmd.LoadFromFile(m.copyPath)
	if err != nil {
		if m.template == nil {
			return err
		}
		cfg = m.template.DeepCopy()
	}
	cfg.CurrentContext = ctxName
	ctx := cfg.Contexts[ctxName]
	if ctx == nil {
		if m.template != nil && m.template.Contexts[ctxName] != nil {
			ctx = m.template.Contexts[ctxName].DeepCopy()
		} else {
			return fmt.Errorf("context %s not found in kubeconfig", ctxName)
		}
		cfg.Contexts[ctxName] = ctx
	}
	ctx.Namespace = namespace
	return clientcmd.WriteToFile(*cfg, m.copyPath)
}

func (m *Manager) lookupContext(ctxName string) (*clientcmdapi.Context, error) {
	if m.template != nil {
		if ctx := m.template.Contexts[ctxName]; ctx != nil {
			return ctx, nil
		}
	}
	if m.basePath == "" {
		return nil, fmt.Errorf("context %s not found in kubeconfig", ctxName)
	}
	cfg, err := clientcmd.LoadFromFile(m.basePath)
	if err != nil {
		return nil, err
	}
	ctx := cfg.Contexts[ctxName]
	if ctx == nil {
		return nil, fmt.Errorf("context %s not found in kubeconfig", ctxName)
	}
	return ctx, nil
}

// DetectExternalChange reports when the overlay hash changes outside kc.
func (m *Manager) DetectExternalChange() (changed bool, ctxName, namespace string, err error) {
	if m.mode != ModeOverlay {
		return false, "", "", nil
	}
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

// Close removes the temp dir.
func (m *Manager) Close() error {
	return os.RemoveAll(m.dir)
}

func renderOverlay(ctxName, namespace, cluster, user string) string {
	if namespace == "" {
		return fmt.Sprintf("apiVersion: v1\nkind: Config\ncurrent-context: %s\n", ctxName)
	}
	return fmt.Sprintf(`apiVersion: v1
kind: Config
contexts:
- name: %[1]s
  context:
    cluster: %[3]s
    user: %[4]s
    namespace: %[2]s
current-context: %[1]s
`, ctxName, namespace, cluster, user)
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
