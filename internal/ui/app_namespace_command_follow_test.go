package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sttts/kc/pkg/appconfig"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// flattenCmd runs a tea.Cmd and flattens BatchMsg into individual messages.
func flattenCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	switch m := msg.(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, sub := range m {
			if sub == nil {
				continue
			}
			out = append(out, sub)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

func TestNamespaceCommandRestartImmediate(t *testing.T) {
	app := NewApp()
	cfg := appconfig.CommandConfig{
		Name: "Top Pods",
		Type: appconfig.CommandTypeNamespace,
	}

	// Arm panel 1 with a namespaced command placeholder and set a non-empty current target.
	if cmd := app.startNamespaceCommand(1, cfg, ""); cmd != nil {
		_ = cmd()
	}
	app.namespaceCommandTarget[1] = "default"

	if cmd := app.maybeRestartNamespaceCommand(1, ""); cmd != nil {
		// Should restart immediately (no debounce) and update target to empty.
		for _, msg := range flattenCmd(cmd) {
			if r, ok := msg.(restartNamespaceCommandMsg); ok {
				// Apply the restart path.
				if _, restartCmd := app.Update(r); restartCmd != nil {
					_ = restartCmd()
				}
			}
		}
	}

	if got := app.namespaceCommandTarget[1]; got != "" {
		t.Fatalf("expected namespace target cleared, got %q", got)
	}
}

func TestNamespaceCommandRestartDebounced(t *testing.T) {
	app := NewApp()
	cfg := appconfig.CommandConfig{
		Name:     "Top Pods",
		Type:     appconfig.CommandTypeNamespace,
		Debounce: metav1.Duration{Duration: 2 * time.Millisecond},
	}

	if cmd := app.startNamespaceCommand(1, cfg, ""); cmd != nil {
		_ = cmd()
	}
	app.namespaceCommandTarget[1] = "default"

	cmd := app.maybeRestartNamespaceCommand(1, "")
	if cmd == nil {
		t.Fatalf("expected debounce restart command")
	}

	msgs := flattenCmd(cmd)
	if len(msgs) == 0 {
		t.Fatalf("expected debounce tick message")
	}
	var restart restartNamespaceCommandMsg
	found := false
	for _, m := range msgs {
		if r, ok := m.(restartNamespaceCommandMsg); ok {
			restart = r
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected restartNamespaceCommandMsg in debounce output, got %v", msgs)
	}
	if restart.Namespace != "" {
		t.Fatalf("expected restart namespace \"\", got %q", restart.Namespace)
	}

	if _, restartCmd := app.Update(restart); restartCmd != nil {
		_ = restartCmd()
	}
	if got := app.namespaceCommandTarget[1]; got != "" {
		t.Fatalf("expected namespace target cleared after restart, got %q", got)
	}
}

func TestNamespaceCommandPlaceholderDisablesWatch(t *testing.T) {
	app := NewApp()
	cfg := appconfig.CommandConfig{
		Name:          "Top Pods",
		Type:          appconfig.CommandTypeNamespace,
		WatchInterval: metav1.Duration{Duration: 5 * time.Second},
	}

	// Pretend an existing watch was active.
	app.commandWatchInterval[0] = cfg.WatchInterval.Duration
	app.commandWatchToken[0] = 7

	oldToken := app.commandWatchToken[0]
	if cmd := app.startNamespaceCommand(0, cfg, ""); cmd != nil {
		_ = cmd()
	}

	if got := app.commandWatchInterval[0]; got != 0 {
		t.Fatalf("expected watch interval cleared on placeholder, got %v", got)
	}
	if got := app.commandWatchToken[0]; got != oldToken+1 {
		t.Fatalf("expected watch token bumped, got %d want %d", got, oldToken+1)
	}

	// A stale tick with the old token must be ignored.
	app.namespaceCommandTarget[0] = "default"
	if model, cmd := app.handleCommandWatchTick(commandWatchTickMsg{PanelIdx: 0, Token: oldToken}); cmd != nil {
		t.Fatalf("expected no command from stale tick, got %v (model=%T)", cmd, model)
	}
}
