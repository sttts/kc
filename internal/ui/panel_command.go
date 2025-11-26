package ui

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/go-logr/logr"
	"github.com/sttts/kc/internal/models"
	"github.com/sttts/kc/internal/ui/panelcontent"
	uistyles "github.com/sttts/kc/internal/ui/styles"
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
	// Watch state
	watchInterval time.Duration
	watchToken    int
	// Heartbeat / status
	heartbeatToken int
	heartbeatPhase int
	heartbeatUntil time.Time
	heartbeatOn    bool
	exitKnown      bool
	exitCode       int
}

func NewCommandWidget(deps panelcontent.WidgetDeps, config appconfig.CommandConfig) *CommandWidget {
	return &CommandWidget{
		panelDeps: deps,
		config:    config,
		log:       ctrllog.Log.WithName("command"),
	}
}

func (w *CommandWidget) Init(ctx context.Context) tea.Cmd {
	return nil
}

func (w *CommandWidget) Teardown(ctx context.Context) {
	w.watchToken++
	w.heartbeatToken++
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
	if tick, ok := msg.(commandWatchMsg); ok {
		if tick.token != w.watchToken || w.watchInterval <= 0 {
			return nil, false
		}
		w.armHeartbeat()
		w.log.Info("restarting command on watch interval", "name", w.config.Name, "interval", w.watchInterval)
		return w.startPendingCommand(), true
	}
	if tick, ok := msg.(heartbeatMsg); ok {
		if tick.token != w.heartbeatToken || !w.heartbeatOn {
			return nil, false
		}
		if time.Now().After(w.heartbeatUntil) {
			w.heartbeatOn = false
			return nil, true
		}
		w.heartbeatPhase = (w.heartbeatPhase + 1) % len(heartbeatFrames)
		return tea.Tick(heartbeatInterval, func(time.Time) tea.Msg {
			return heartbeatMsg{token: tick.token}
		}), true
	}

	if w.terminal != nil {
		if key, ok := msg.(tea.KeyMsg); ok {
			// Non-interactive commands should not consume key input.
			if !w.config.Interactive {
				return nil, false
			}
			// Special case: Tab should remain a panel navigation key even for interactive commands.
			if key.String() == "tab" {
				return nil, false
			}
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
	if w.log.GetSink() != nil {
		w.log.V(1).Info("command widget resized", "width", w.width, "height", w.height)
	}
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
		w.exitKnown = false
		if c, err := strconv.Atoi(strings.TrimSpace(code)); err == nil {
			w.exitCode = c
			w.exitKnown = true
		}
		// Handle OnExit behavior
		// If keep-open, we just leave the terminal view as is (it shows output)
		// If close, we should notify panel to switch back
	})

	w.running = true
	w.err = nil
	w.exitKnown = false

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
		w.restartWatchTimer(),
		w.triggerHeartbeat(),
	)
}

type debounceMsg struct{}
type commandWatchMsg struct {
	token int
}
type heartbeatMsg struct {
	token int
}

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
		HeaderStatus:   w.heartbeatStatus(),
		SuppressFooter: true,
	}
}

// WatchInterval reports the active watch interval.
func (w *CommandWidget) WatchInterval() time.Duration {
	return w.watchInterval
}

// SetWatchInterval updates the watch interval and schedules the next tick.
func (w *CommandWidget) SetWatchInterval(ctx context.Context, interval time.Duration) tea.Cmd {
	if interval == w.watchInterval {
		return nil
	}
	w.watchInterval = interval
	return w.restartWatchTimer()
}

func (w *CommandWidget) restartWatchTimer() tea.Cmd {
	// Skip auto-restart for interactive commands to avoid killing user sessions.
	if w.config.Interactive {
		return nil
	}
	w.watchToken++
	if w.watchInterval <= 0 {
		return nil
	}
	token := w.watchToken
	interval := w.watchInterval
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return commandWatchMsg{token: token}
	})
}

func (w *CommandWidget) triggerHeartbeat() tea.Cmd {
	w.armHeartbeat()
	token := w.heartbeatToken
	return tea.Tick(heartbeatInterval, func(time.Time) tea.Msg {
		return heartbeatMsg{token: token}
	})
}

func (w *CommandWidget) armHeartbeat() {
	w.heartbeatToken++
	w.heartbeatPhase = 0
	w.heartbeatOn = true
	w.heartbeatUntil = time.Now().Add(heartbeatBurst)
}

func (w *CommandWidget) heartbeatStatus() string {
	frame := heartbeatFrames[w.heartbeatPhase%len(heartbeatFrames)]
	if !w.heartbeatOn {
		frame = heartbeatFrames[0]
	}
	if !w.running && !w.exitKnown && !w.heartbeatOn {
		return ""
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(uistyles.ColorWhite)).Bold(true)
	if w.exitKnown && w.exitCode != 0 {
		style = style.Foreground(lipgloss.Color("1"))
	}
	return style.Render(frame)
}

var heartbeatFrames = []string{"<3", "<3", "<3", "<<3", "<3<", "<3"}

const heartbeatInterval = 150 * time.Millisecond
const heartbeatBurst = 900 * time.Millisecond
