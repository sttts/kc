package ui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/go-logr/logr"
	"github.com/sttts/kc/internal/models"
	"github.com/sttts/kc/internal/ui/panelcontent"
	"github.com/sttts/kc/pkg/appconfig"
	bubbleterm "github.com/taigrr/bubbleterm"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

type CommandWidget struct {
	panelDeps panelcontent.WidgetDeps
	config    appconfig.CommandConfig
	terminal  *bubbleterm.Model
	width     int
	height    int
	focused   bool
	cmd       *exec.Cmd
	running   bool
	output    string // To show output after exit if keep-open
	err       error
	log       logr.Logger
	// Debounce state
	debounceTimer *time.Timer
	pendingItems  []models.Item
	pendingGVR    schema.GroupVersionResource
}

func NewCommandWidget(deps panelcontent.WidgetDeps, config appconfig.CommandConfig) *CommandWidget {
	return &CommandWidget{
		panelDeps: deps,
		config:    config,
	}
}

func (w *CommandWidget) Init(ctx context.Context) tea.Cmd {
	return nil
}

func (w *CommandWidget) Teardown(ctx context.Context) {
	if w.cmd != nil && w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
	w.running = false
}

func (w *CommandWidget) Update(ctx context.Context, msg tea.Msg) (tea.Cmd, bool) {
	// Handle debounce timer
	if _, ok := msg.(debounceMsg); ok {
		return w.startPendingCommand(), true
	}

	if w.terminal != nil {
		// Special case: Tab should remain a panel navigation key even for interactive commands.
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "tab" {
			return nil, false
		}
		model, cmd := w.terminal.Update(msg)
		if term, ok := model.(*bubbleterm.Model); ok {
			w.terminal = term
		}
		// Always signal handled so the panel can re-render with the latest terminal frame.
		return cmd, true
	}

	return nil, false
}

func (w *CommandWidget) View(ctx context.Context, frame panelcontent.Frame) string {
	if w.err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(fmt.Sprintf("Error: %v", w.err))
	}

	if w.terminal != nil {
		view := w.terminal.View()
		if view.Content != nil {
			return fmt.Sprint(view.Content)
		}
		return ""
	}

	if w.output != "" {
		return w.output
	}

	return "Starting..."
}

func (w *CommandWidget) Resize(ctx context.Context, size panelcontent.Size) {
	w.width = size.Width
	w.height = size.Height
	if w.terminal != nil {
		msg := tea.WindowSizeMsg{Width: size.Width, Height: size.Height}
		model, _ := w.terminal.Update(msg)
		w.terminal = model.(*bubbleterm.Model)
	}
}

func (w *CommandWidget) SetFocus(ctx context.Context, focused bool) {
	w.focused = focused
	if w.terminal != nil {
		if focused {
			w.terminal.Focus()
		} else {
			w.terminal.Blur()
		}
	}
}

func (w *CommandWidget) CurrentSelectionID(ctx context.Context) string {
	return ""
}

// StartCommand initiates the command.
// For selector type, it respects debounce.
func (w *CommandWidget) StartCommand(ctx context.Context, items []models.Item, gvr schema.GroupVersionResource) tea.Cmd {
	w.log = ctrllog.FromContext(ctx).WithName("command")
	if w.config.Type == appconfig.CommandTypeSelector {
		// Stop existing timer
		if w.debounceTimer != nil {
			w.debounceTimer.Stop()
		}

		w.pendingItems = items
		w.pendingGVR = gvr

		// If debounce is 0, start immediately
		if w.config.Debounce.Duration == 0 {
			return w.startPendingCommand()
		}

		// Schedule start
		return tea.Tick(w.config.Debounce.Duration, func(t time.Time) tea.Msg {
			return debounceMsg{}
		})
	}

	// For other types, start immediately
	w.pendingItems = items
	w.pendingGVR = gvr
	return w.startPendingCommand()
}

func IsCommandApplicable(cmd appconfig.CommandConfig, item models.Item) bool {
	selectedCount := 0
	if item != nil {
		selectedCount = 1
	}
	return isCommandApplicable(cmd, item, selectedCount, namespaceFromItem(item))
}

func (w *CommandWidget) startPendingCommand() tea.Cmd {
	// Stop current command if running
	if w.cmd != nil && w.cmd.Process != nil {
		// TODO: How to kill? SIGTERM or SIGKILL?
		// User said: "kill the previous process immediately"
		if err := w.cmd.Process.Kill(); err != nil {
			w.log.Error(err, "killing previous command")
		}
		w.cmd = nil
		w.running = false
	}

	// Render command template
	cmdStr, err := RenderCommand(w.config.Command, w.pendingItems, w.pendingGVR)
	if err != nil {
		w.err = err
		w.log.Error(err, "render command template failed", "name", w.config.Name)
		return nil
	}
	w.log.V(1).Info("starting command", "name", w.config.Name, "type", w.config.Type, "cmd", cmdStr, "interactive", w.config.Interactive, "width", w.width, "height", w.height)

	// Create terminal
	term, err := bubbleterm.New(w.width, w.height)
	if err != nil {
		w.err = err
		return nil
	}
	term.SetAutoPoll(true)
	term.SetPollInterval(100 * time.Millisecond)
	w.terminal = term

	// Prepare command
	// Use sh -c to execute the command string
	shell := "sh"
	w.cmd = exec.Command(shell, "-c", cmdStr)

	// TODO: Inject KUBECONFIG env var
	// w.cmd.Env = append(os.Environ(), "KUBECONFIG=...")

	// Setup exit handler
	term.GetEmulator().SetOnExit(func(code string) {
		w.running = false
		// Handle OnExit behavior
		// If keep-open, we just leave the terminal view as is (it shows output)
		// If close, we should notify panel to switch back
	})

	w.running = true
	w.err = nil

	envVars := []string{}
	for _, kv := range w.cmd.Env {
		if strings.HasPrefix(kv, "KUBECONFIG=") {
			envVars = append(envVars, "KUBECONFIG")
		}
	}
	w.log.Info("exec command",
		"name", w.config.Name,
		"type", w.config.Type,
		"shell", shell,
		"args", w.cmd.Args,
		"dir", w.cmd.Dir,
		"env", envVars,
	)

	return tea.Batch(
		w.terminal.Init(),
		w.terminal.StartCommand(w.cmd),
	)
}

type debounceMsg struct{}

// Implement other Widget interface methods...
func (w *CommandWidget) SelectedNavItem(ctx context.Context) (models.Item, bool) {
	return nil, false
}
func (w *CommandWidget) SetResourceViewOptions(showNonEmpty bool, order string)       {}
func (w *CommandWidget) ResourceViewOptions() (bool, string)                          { return false, "" }
func (w *CommandWidget) ResetSelection(ctx context.Context)                           {}
func (w *CommandWidget) SetFolder(ctx context.Context, f models.Folder, hasBack bool) {}
func (w *CommandWidget) UseFolder(on bool)                                            {}
func (w *CommandWidget) ClearFolder()                                                 {}
func (w *CommandWidget) Folder() models.Folder                                        { return nil }
func (w *CommandWidget) Enter(ctx context.Context) tea.Cmd                            { return nil }
func (w *CommandWidget) SetTableMode(ctx context.Context, mode string)                {}
func (w *CommandWidget) TableMode() string                                            { return "" }
func (w *CommandWidget) SetColumnsMode(ctx context.Context, mode string)              {}
func (w *CommandWidget) ColumnsMode() string                                          { return "" }
func (w *CommandWidget) SetObjectOrder(ctx context.Context, order string)             {}
func (w *CommandWidget) ToggleColumnsMode(ctx context.Context) tea.Cmd                { return nil }
func (w *CommandWidget) ObjectOrder() string                                          { return "" }
func (w *CommandWidget) SelectByRowID(ctx context.Context, id string)                 {}
func (w *CommandWidget) SelectRowIDs(ctx context.Context, ids []string)               {}
func (w *CommandWidget) SetFolderNavHandler(h func(back bool, selID string, next models.Folder)) {
}
func (w *CommandWidget) RefreshFolder(ctx context.Context) {}
func (w *CommandWidget) FrameInfo(ctx context.Context, req panelcontent.FrameInfoRequest) panelcontent.FrameInfo {
	return panelcontent.FrameInfo{
		Breadcrumb:     w.config.Name,
		HeaderStatus:   "",
		SuppressFooter: true,
	}
}
