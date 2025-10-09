package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	kccluster "github.com/sttts/kc/internal/cluster"
	models "github.com/sttts/kc/internal/models"
	navui "github.com/sttts/kc/internal/navigation"
	"github.com/sttts/kc/internal/overlay"
	"github.com/sttts/kc/pkg/appconfig"
	"github.com/sttts/kc/pkg/kubeconfig"
	corev1 "k8s.io/api/core/v1"
	metamapper "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientcmd "k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// EscTimeoutMsg is sent when the escape sequence times out
type EscTimeoutMsg struct{}

// FolderTickMsg triggers periodic folder refresh (debounced to ~1s).
type FolderTickMsg struct{}

// kubectlEditFinishedMsg notifies that a kubectl edit invocation exited.
type kubectlEditFinishedMsg struct {
	err         error
	panelIndex  int
	resourceRef string
	tempConfig  string
}

type namespaceCreatedMsg struct {
	name string
	err  error
}

type deleteTarget struct {
	panelIdx  int
	gvr       schema.GroupVersionResource
	namespace string
	name      string
}

type resourceDeletedMsg struct {
	target deleteTarget
	err    error
}

// App represents the main application state
type App struct {
	leftPanel    *Panel
	rightPanel   *Panel
	terminal     *Terminal
	modalManager *ModalManager
	width        int
	height       int
	activePanel  int // 0 = left, 1 = right
	showTerminal bool
	allResources []schema.GroupVersionKind
	// Esc sequence tracking
	escPressed bool
	// Data providers
	kubeMgr    *kubeconfig.Manager
	cl         *kccluster.Cluster
	clPool     *kccluster.Pool
	ctx        context.Context
	cancel     context.CancelFunc
	currentCtx *kubeconfig.Context
	viewConfig *ViewConfig
	cfg        *appconfig.Config
	// Theme dialog state
	prevTheme           string
	suppressThemeRevert bool
	// New navigation (folder-backed) using a Navigator
	leftNav  *navui.Navigator
	rightNav *navui.Navigator
	// Mouse double-click detection
	lastClickTime  time.Time
	lastClickPanel int
	lastClickRowID string
	// Suppress forwarding of mouse to terminal immediately after toggling fullscreen
	suppressMouseUntil time.Time
	// Resources options dialog state
	prevResShowNonEmpty bool
	prevResOrder        string
	resOptsChanged      bool
	resOptsConfirmed    bool
	// Busy spinner state (lightweight, non-intrusive)
	busyActive bool
	busyLabel  string
	busyFrame  int
	busyToken  int
	// Toast notification state (auto-dismiss)
	toastActive bool
	toastText   string
	toastUntil  time.Time
	// Logger that emits toasts on errors with rate limiting
	toastLogger          *ToastLogger
	pendingCmds          []tea.Cmd
	leftConfig           *appconfig.Config
	rightConfig          *appconfig.Config
	namespaceInput       *NamespaceCreateModel
	deleteConfirm        *DeleteConfirmModel
	pendingDelete        *deleteTarget
	namespaceCreatePanel int
}

const requestTimeout = 10 * time.Second

// Invariant: a.cfg is always non-nil. NewApp initializes it with defaults and
// Init() loads and overwrites with persisted config, never leaving it nil.

// NewApp creates a new application instance
func NewApp() *App {
	app := &App{
		leftPanel:    NewPanel(""),
		rightPanel:   NewPanel(""),
		terminal:     NewTerminal(),
		modalManager: NewModalManager(),
		activePanel:  0,
		showTerminal: false,
		allResources: make([]schema.GroupVersionKind, 0),
		escPressed:   false,
		viewConfig:   NewViewConfig(),
		// Invariant: cfg is always non-nil; initialize with defaults
		cfg:                  appconfig.Default(),
		namespaceCreatePanel: -1,
	}
	app.ctx, app.cancel = context.WithCancel(context.Background())
	app.terminal.SetLogger(ctrllog.Log.WithName("terminal"))
	app.toastLogger = NewToastLogger(app, 2*time.Second)

	// Register modals
	app.setupModals()
	app.setupPanelInputs()

	return app
}

// Init initializes the application
func (a *App) Init() tea.Cmd {
	// Load config (best-effort)
	cfg, err := appconfig.Load()
	if err != nil {
		cfg = appconfig.Default()
	}
	a.cfg = cfg
	a.leftConfig = cloneConfig(cfg)
	a.rightConfig = cloneConfig(cfg)
	return tea.Batch(
		a.leftPanel.Init(),
		a.rightPanel.Init(),
		a.terminal.Init(),
		func() tea.Msg {
			// Focus the terminal initially since it's the main input area
			a.terminal.Focus()
			return nil
		},
		tea.Tick(time.Second, func(time.Time) tea.Msg { return FolderTickMsg{} }),
	)
}

// enqueueCmd appends a command to be executed on the next Update cycle.
func (a *App) enqueueCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	a.pendingCmds = append(a.pendingCmds, cmd)
}

func cloneConfig(cfg *appconfig.Config) *appconfig.Config {
	if cfg == nil {
		return appconfig.Default()
	}
	clone := *cfg
	clone.Resources.Favorites = append([]string(nil), cfg.Resources.Favorites...)
	return &clone
}

func (a *App) ensurePanelConfig(panel *Panel) *appconfig.Config {
	if panel == a.leftPanel {
		if a.leftConfig == nil {
			a.leftConfig = cloneConfig(a.cfg)
		}
		return a.leftConfig
	}
	if a.rightConfig == nil {
		a.rightConfig = cloneConfig(a.cfg)
	}
	return a.rightConfig
}

// activePanelRef returns the currently focused panel.
func (a *App) activePanelRef() *Panel {
	if a.activePanel == 1 {
		return a.rightPanel
	}
	return a.leftPanel
}

// panelIndex returns 0 for the left panel and 1 for the right panel.
func (a *App) panelIndex(panel *Panel) int {
	if panel == nil {
		return -1
	}
	if panel == a.rightPanel {
		return 1
	}
	return 0
}

func (a *App) navigatorForPanel(panel *Panel) *navui.Navigator {
	if panel == a.rightPanel {
		return a.rightNav
	}
	return a.leftNav
}

func (a *App) panelByIndex(idx int) *Panel {
	if idx == 1 {
		return a.rightPanel
	}
	return a.leftPanel
}

func (a *App) panelAreaMetrics() (panelWidth int, panelHeight int, headerOffset int) {
	reserved := 3
	if a.toastActive {
		reserved++
	}
	panelHeight = a.height - reserved
	if panelHeight < 0 {
		panelHeight = 0
	}
	panelWidth = a.width / 2
	if panelWidth < 0 {
		panelWidth = 0
	}
	headerOffset = 2
	return
}

func (a *App) syncPanelConfig(panel *Panel) {
	cfg := a.ensurePanelConfig(panel)
	cfg.Resources.ShowNonEmptyOnly = panel.resShowNonEmpty
	cfg.Resources.Order = appconfig.ResourcesViewOrder(panel.resOrder)
	cfg.Resources.Columns = panel.columnsMode
	cfg.Objects.Order = panel.objOrder
	cfg.Objects.Columns = panel.columnsMode
	cfg.Panel.Table.Mode = appconfig.TableMode(panel.TableMode())
}

func (a *App) aggregatedKubeConfig(current string) clientcmdapi.Config {
	contexts := make(map[string]*clientcmdapi.Context)
	if a.kubeMgr != nil {
		for _, ctx := range a.kubeMgr.GetContexts() {
			if ctx == nil {
				continue
			}
			contexts[ctx.Name] = &clientcmdapi.Context{
				Cluster:   ctx.Cluster,
				AuthInfo:  ctx.User,
				Namespace: ctx.Namespace,
			}
		}
	}
	return clientcmdapi.Config{
		CurrentContext: current,
		Contexts:       contexts,
	}
}

func (a *App) makeDeps(cl *kccluster.Cluster, cfg *appconfig.Config, current string) models.Deps {
	if cfg == nil {
		cfg = a.cfg
	}
	return models.Deps{
		Cl:         cl,
		Ctx:        a.ctx,
		CtxName:    current,
		KubeConfig: a.aggregatedKubeConfig(current),
		AppConfig:  cfg,
	}
}

func (a *App) navigatorPath(nav *navui.Navigator) string {
	if nav == nil {
		return "/"
	}
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	defer cancel()
	return nav.Path(ctx)
}

func (a *App) makeEnterContextFunc(cfg *appconfig.Config) func(string, []string) (models.Folder, error) {
	return func(name string, basePath []string) (models.Folder, error) {
		if a.kubeMgr == nil {
			return nil, fmt.Errorf("no kubeconfig manager available")
		}
		target := a.kubeMgr.GetContextByName(name)
		if target == nil {
			return nil, fmt.Errorf("context %q not found", name)
		}
		if target.Kubeconfig == nil {
			return nil, fmt.Errorf("context %q has no kubeconfig", name)
		}
		key := kccluster.Key{KubeconfigPath: target.Kubeconfig.Path, ContextName: target.Name}
		cl, err := a.clPool.Get(a.ctx, key)
		if err != nil {
			return nil, err
		}
		deps := a.makeDeps(cl, cfg, name)
		return models.NewContextRootFolder(deps, basePath), nil
	}
}

// Update handles messages and updates the application state
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if len(a.pendingCmds) > 0 {
		cmds = append(cmds, a.pendingCmds...)
		a.pendingCmds = nil
	}

	// Always adapt size
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		msg.Width = max(40, msg.Width)
		msg.Height = max(5, msg.Height)

		a.width = msg.Width
		a.height = msg.Height

		// Ensure active modal scales with terminal size
		if a.modalManager != nil && a.modalManager.IsModalVisible() {
			if m := a.modalManager.GetActiveModal(); m != nil {
				m.SetDimensions(a.width, a.height)
				// If the resources dialog is open, re-center and resnapshot background
				// to keep proportions and stack drawing correct on resize.
				// Window size: clamp to 60% of width, minimum 40, height fixed to content+frame.
				// This avoids sprawling on very large terminals but scales on smaller ones.
				winW := a.width * 6 / 10
				if winW < 40 {
					winW = 40
				}
				if winW > a.width-2 {
					winW = a.width - 2
				}
				winH := 6
				bg, _ := a.renderMainView()
				m.SetWindowed(winW, winH, bg)
			}
		}

		if a.terminal != nil {
			// Reserve space for status bar (1 line)
			// Terminal gets the remaining space
			terminalMsg := tea.WindowSizeMsg{
				Width:  msg.Width,
				Height: msg.Height - 1,
			}
			model, cmd := a.terminal.Update(terminalMsg)
			a.terminal = model.(*Terminal)
			cmds = append(cmds, cmd)
		}
	}

	// Handle modals first
	if a.modalManager.IsModalVisible() {
		// Intercept resource options changes even while modal is visible
		switch m := msg.(type) {
		case ResourcesOptionsChangedMsg:
			if m.SaveDefault {
				// Persist current dialog values to config defaults
				if a.cfg == nil {
					a.cfg = appconfig.Default()
				}
				if m.HasInclude {
					a.cfg.Resources.ShowNonEmptyOnly = m.ShowNonEmptyOnly
				}
				if m.HasOrder {
					a.cfg.Resources.Order = appconfig.ResourcesViewOrder(m.Order)
				}
				a.cfg.Panel.Table.Mode = appconfig.TableMode(m.TableMode)
				_ = appconfig.Save(a.cfg)
			}
			if m.Accept {
				// Apply to active panel only; do not persist
				if a.activePanel == 0 {
					ctxTable, cancelTable := context.WithTimeout(a.ctx, panelContextTimeout)
					a.leftPanel.SetTableMode(ctxTable, m.TableMode)
					cancelTable()
					if m.HasInclude || m.HasOrder {
						a.leftPanel.SetResourceViewOptions(m.ShowNonEmptyOnly, m.Order)
					}
					a.syncPanelConfig(a.leftPanel)
				} else {
					ctxTable, cancelTable := context.WithTimeout(a.ctx, panelContextTimeout)
					a.rightPanel.SetTableMode(ctxTable, m.TableMode)
					cancelTable()
					if m.HasInclude || m.HasOrder {
						a.rightPanel.SetResourceViewOptions(m.ShowNonEmptyOnly, m.Order)
					}
					a.syncPanelConfig(a.rightPanel)
				}
				// Refresh only the active panel's folder
				if a.activePanel == 0 && a.leftNav != nil {
					ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
					if rf, ok := a.leftNav.Current().(interface{ Refresh() }); ok {
						rf.Refresh()
					}
					a.leftPanel.SetFolder(ctx, a.leftNav.Current(), a.leftNav.HasBack())
					a.leftPanel.SetCurrentPath(a.navigatorPath(a.leftNav))
					cancel()
					ctxRefresh, cancelRefresh := context.WithTimeout(a.ctx, panelContextTimeout)
					a.leftPanel.RefreshFolder(ctxRefresh)
					cancelRefresh()
				}
				if a.activePanel == 1 && a.rightNav != nil {
					ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
					if rf, ok := a.rightNav.Current().(interface{ Refresh() }); ok {
						rf.Refresh()
					}
					a.rightPanel.SetFolder(ctx, a.rightNav.Current(), a.rightNav.HasBack())
					a.rightPanel.SetCurrentPath(a.navigatorPath(a.rightNav))
					cancel()
					ctxRefresh, cancelRefresh := context.WithTimeout(a.ctx, panelContextTimeout)
					a.rightPanel.RefreshFolder(ctxRefresh)
					cancelRefresh()
				}
			}
			if m.Close {
				a.modalManager.Hide()
			}
			return a, nil
		case ObjectOptionsChangedMsg:
			if m.SaveDefault {
				if a.cfg == nil {
					a.cfg = appconfig.Default()
				}
				// Save table mode
				switch strings.ToLower(m.TableMode) {
				case "fit":
					a.cfg.Panel.Table.Mode = appconfig.TableModeFit
				default:
					a.cfg.Panel.Table.Mode = appconfig.TableModeScroll
				}
				// Save columns mode to objects.columns
				if strings.EqualFold(m.Columns, "wide") {
					a.cfg.Objects.Columns = "wide"
				} else {
					a.cfg.Objects.Columns = "normal"
				}
				// Save objects order
				a.cfg.Objects.Order = m.ObjectsOrder
				_ = appconfig.Save(a.cfg)
			}
			if a.activePanel == 0 {
				ctxPanel, cancelPanel := context.WithTimeout(a.ctx, panelContextTimeout)
				a.leftPanel.SetTableMode(ctxPanel, m.TableMode)
				a.leftPanel.SetColumnsMode(ctxPanel, m.Columns)
				a.leftPanel.SetObjectOrder(ctxPanel, m.ObjectsOrder)
				a.syncPanelConfig(a.leftPanel)
				cancelPanel()
				if a.leftNav != nil {
					if rf, ok := a.leftNav.Current().(interface{ Refresh() }); ok {
						rf.Refresh()
					}
					ctxRefresh, cancelRefresh := context.WithTimeout(a.ctx, panelContextTimeout)
					a.leftPanel.RefreshFolder(ctxRefresh)
					cancelRefresh()
				}
			} else {
				ctxPanel, cancelPanel := context.WithTimeout(a.ctx, panelContextTimeout)
				a.rightPanel.SetTableMode(ctxPanel, m.TableMode)
				a.rightPanel.SetColumnsMode(ctxPanel, m.Columns)
				a.rightPanel.SetObjectOrder(ctxPanel, m.ObjectsOrder)
				a.syncPanelConfig(a.rightPanel)
				cancelPanel()
				if a.rightNav != nil {
					if rf, ok := a.rightNav.Current().(interface{ Refresh() }); ok {
						rf.Refresh()
					}
					ctxRefresh, cancelRefresh := context.WithTimeout(a.ctx, panelContextTimeout)
					a.rightPanel.RefreshFolder(ctxRefresh)
					cancelRefresh()
				}
			}
			if m.Close {
				a.modalManager.Hide()
			}
			return a, nil
		}
		model, cmd := a.modalManager.Update(msg)
		a.modalManager = model.(*ModalManager)
		cmds = append(cmds, cmd)
		// While a modal is open, still forward non-key messages to the
		// terminal (process output, window size). Background is snapshotted,
		// so this stays light and keeps the 2-line terminal fresh.
		if _, isKey := msg.(tea.KeyMsg); !isKey && a.terminal != nil {
			tmodel, tcmd := a.terminal.Update(msg)
			a.terminal = tmodel.(*Terminal)
			cmds = append(cmds, tcmd)
		}
		return a, tea.Batch(cmds...)
	}

	// Check if terminal process has exited (check on every message)
	if a.terminal.IsProcessExited() {
		return a, tea.Quit
	}

	switch msg := msg.(type) {
	case BusyShowMsg:
		if msg.token == a.busyToken {
			a.busyActive = true
			a.busyFrame = 0
			return a, tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return BusyTickMsg{} })
		}
		return a, nil
	case BusyTickMsg:
		if a.busyActive {
			a.busyFrame = (a.busyFrame + 1) % 10
			return a, tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return BusyTickMsg{} })
		}
		return a, nil
	case BusyHideMsg:
		if msg.token == a.busyToken {
			a.busyActive = false
		}
		return a, nil
	case busyDoneMsg:
		if msg.token == a.busyToken {
			a.busyActive = false
			a.busyToken++
		}
		// Re-dispatch the original message for normal handling
		return a, func() tea.Msg { return msg.msg }
	case showToastMsg:
		a.toastActive = true
		a.toastText = msg.text
		a.toastUntil = time.Now().Add(msg.ttl)
		return a, tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return toastTickMsg{} })
	case toastTickMsg:
		if a.toastActive {
			if time.Now().After(a.toastUntil) {
				a.toastActive = false
			} else {
				return a, tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return toastTickMsg{} })
			}
		}
		return a, nil
	case kubectlEditFinishedMsg:
		if msg.tempConfig != "" {
			_ = os.Remove(msg.tempConfig)
		}
		if msg.err != nil {
			if a.toastLogger != nil {
				a.enqueueCmd(a.toastLogger.Errorf("kubectl edit %s failed: %v", msg.resourceRef, msg.err))
			} else {
				a.enqueueCmd(a.ShowToast(fmt.Sprintf("kubectl edit failed: %v", msg.err), 5*time.Second))
			}
			return a, nil
		}
		a.refreshPanelAfterEdit(msg.panelIndex)
		return a, nil
	case NamespaceCreateResultMsg:
		if msg.Close {
			a.modalManager.Hide()
			if a.namespaceInput != nil {
				a.namespaceInput.Reset()
			}
			if !msg.Confirm {
				a.namespaceCreatePanel = -1
			}
		}
		if msg.Confirm {
			if a.cl == nil {
				if a.toastLogger != nil {
					a.enqueueCmd(a.toastLogger.Errorf("Cluster not ready for namespace creation"))
				}
				return a, nil
			}
			return a, a.createNamespaceWithName(msg.Name)
		}
		return a, nil
	case namespaceCreatedMsg:
		if msg.err != nil {
			if a.toastLogger != nil {
				a.enqueueCmd(a.toastLogger.Errorf("Create namespace %s failed: %v", msg.name, msg.err))
			} else {
				a.enqueueCmd(a.ShowToast(fmt.Sprintf("Create namespace failed: %v", msg.err), 5*time.Second))
			}
			a.namespaceCreatePanel = -1
			return a, nil
		}
		a.enqueueCmd(a.ShowToast(fmt.Sprintf("Namespace %s created", msg.name), 3*time.Second))
		if a.namespaceCreatePanel == 0 || a.namespaceCreatePanel == 1 {
			a.refreshPanelAfterEdit(a.namespaceCreatePanel)
			other := 1 - a.namespaceCreatePanel
			if other == 0 || other == 1 {
				var otherPanel *Panel
				if other == 0 {
					otherPanel = a.leftPanel
				} else {
					otherPanel = a.rightPanel
				}
				if otherPanel != nil && otherPanel.GetCurrentPath() == "/namespaces" {
					a.refreshPanelAfterEdit(other)
				}
			}
		} else {
			a.refreshPanelAfterEdit(0)
			a.refreshPanelAfterEdit(1)
		}
		a.namespaceCreatePanel = -1
		return a, nil
	case DeleteConfirmMsg:
		if msg.Close {
			a.modalManager.Hide()
		}
		target := a.pendingDelete
		a.pendingDelete = nil
		if target != nil && msg.Confirm {
			return a, a.performDelete(*target)
		}
		return a, nil
	case PanelSelectionChangedMsg:
		return a, nil
	case PanelModeSelectedMsg:
		if panel := a.panelByIndex(msg.PanelIndex); panel != nil {
			ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
			modeCmd := panel.SetMode(ctx, msg.Mode)
			cancel()
			if modeCmd != nil {
				cmds = append(cmds, modeCmd)
			}
			a.syncPanelConfig(panel)
			if nav := a.navigatorForPanel(panel); nav != nil {
				if rf, ok := nav.Current().(interface{ Refresh() }); ok {
					rf.Refresh()
				}
			}
		}
		a.modalManager.Hide()
		return a, tea.Batch(cmds...)
	case resourceDeletedMsg:
		if msg.err != nil {
			if a.toastLogger != nil {
				a.enqueueCmd(a.toastLogger.Errorf("Delete %s failed: %v", kubectlResourceRef(msg.target.gvr, msg.target.name), msg.err))
			} else {
				a.enqueueCmd(a.ShowToast(fmt.Sprintf("Delete failed: %v", msg.err), 5*time.Second))
			}
			return a, nil
		}
		a.enqueueCmd(a.ShowToast(fmt.Sprintf("Deleted %s", kubectlResourceRef(msg.target.gvr, msg.target.name)), 3*time.Second))
		a.refreshPanelAfterEdit(msg.target.panelIdx)
		return a, nil
	case EscTimeoutMsg:
		// Escape sequence timed out
		a.escPressed = false
		return a, nil
	case FolderTickMsg:
		// Refresh only when current folders report dirty to avoid unnecessary redraws.
		if a.leftNav != nil && a.leftPanel != nil {
			if d, ok := a.leftNav.Current().(interface{ IsDirty() bool }); ok && d.IsDirty() {
				ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
				a.leftPanel.RefreshFolder(ctx)
				cancel()
			}
		}
		if a.rightNav != nil && a.rightPanel != nil {
			if d, ok := a.rightNav.Current().(interface{ IsDirty() bool }); ok && d.IsDirty() {
				ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
				a.rightPanel.RefreshFolder(ctx)
				cancel()
			}
		}
		// Schedule next tick (lightweight check)
		return a, tea.Tick(time.Second, func(time.Time) tea.Msg { return FolderTickMsg{} })

	case tea.KeyMsg:
		// Handle global shortcuts first
		switch msg.String() {
		case "alt+f1", "ctrl+1":
			return a, a.showPanelModeModal(0)
		case "alt+f2", "ctrl+2":
			return a, a.showPanelModeModal(1)
		case "ctrl+o":
			// Toggle terminal mode
			a.showTerminal = !a.showTerminal
			a.terminal.SetShowPanels(!a.showTerminal)
			// Always keep terminal focused for typing
			a.terminal.Focus()
			return a, nil

		case "tab":
			// Switch between panels
			a.activePanel = (a.activePanel + 1) % 2
			return a, nil

		case "f10":
			// F10 only quits kc when not in fullscreen mode
			// In fullscreen mode, F10 should go to terminal (for shell commands)
			if !a.showTerminal {
				return a, tea.Quit
			}
			// In fullscreen mode, don't handle F10 here - let it go to terminal
		case "ctrl+q":
			return a, tea.Quit
		}

		// Handle Esc+number escape sequences (Esc then number)
		keyStr := msg.String()
		if keyStr == "esc" {
			// Esc key pressed - start escape sequence with timeout
			a.escPressed = true
			return a, tea.Tick(time.Second, func(time.Time) tea.Msg {
				return EscTimeoutMsg{}
			})
		} else if a.escPressed {
			// We're in an escape sequence, check for numbers
			panel := a.activePanelRef()
			caps := a.capabilitiesForPanel(panel)
			switch keyStr {
			case "0":
				a.escPressed = false
				// Esc 0 = F10, only quit when not in fullscreen mode
				if !a.showTerminal {
					return a, tea.Quit
				}
				// In fullscreen mode, let Esc+0 go to terminal
			case "1":
				a.escPressed = false
				return a, a.showHelp() // Esc 1 = F1
			case "2":
				a.escPressed = false
				if panel != nil {
					ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
					cmd := panel.invokeActionIfAllowed(ctx, PanelActionOptions)
					cancel()
					return a, cmd
				}
				return a, nil
			case "3":
				a.escPressed = false
				if panel != nil && caps.CanView {
					ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
					cmd := panel.invokeActionIfAllowed(ctx, PanelActionView)
					cancel()
					return a, cmd
				}
				return a, nil
			case "4":
				a.escPressed = false
				if panel != nil && caps.CanEdit {
					ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
					cmd := panel.invokeActionIfAllowed(ctx, PanelActionEdit)
					cancel()
					return a, cmd
				}
				return a, nil
			case "5":
				a.escPressed = false
				return a, a.copyItem() // Esc 5 = F5
			case "6":
				a.escPressed = false
				return a, a.renameMoveItem() // Esc 6 = F6
			case "7":
				a.escPressed = false
				if panel != nil && caps.CanCreateNS {
					ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
					cmd := panel.invokeActionIfAllowed(ctx, PanelActionCreateNamespace)
					cancel()
					return a, cmd
				}
				return a, nil
			case "8":
				a.escPressed = false
				if panel != nil && caps.CanDelete {
					ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
					cmd := panel.invokeActionIfAllowed(ctx, PanelActionDelete)
					cancel()
					return a, cmd
				}
				return a, nil
			case "9":
				a.escPressed = false
				if panel != nil {
					ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
					cmd := panel.invokeActionIfAllowed(ctx, PanelActionMenu)
					cancel()
					return a, cmd
				}
				return a, nil
			default:
				// Not a number, cancel escape sequence
				a.escPressed = false
				// Continue with normal key handling
			}
		}

		// In terminal mode, all input goes to terminal except Ctrl-O to return
		if a.showTerminal {
			// Only handle Ctrl-O to return to panel mode
			if msg.String() == "ctrl+o" {
				a.showTerminal = false
				return a, nil
			}
			// Everything else goes to the terminal
			model, cmd := a.terminal.Update(msg)
			a.terminal = model.(*Terminal)
			cmds = append(cmds, cmd)
		} else {
			// In panel mode, use smart key routing based on terminal state
			// If user typed in the 2-line terminal, Enter and Ctrl+C must be SENT to the terminal,
			// then reset typed state to return focus to the panels.
			if (msg.String() == "enter" || msg.String() == "ctrl+c") && a.terminal != nil && a.terminal.HasInput() {
				model, cmd := a.terminal.Update(msg) // deliver to terminal
				a.terminal = model.(*Terminal)
				a.terminal.ClearTyped() // reset typed; next keys route to panels
				return a, cmd
			}
			if a.shouldRouteToPanel(msg.String()) {
				// Handle panel-specific keys
				if a.activePanel == 0 {
					model, cmd := a.leftPanel.Update(msg)
					a.leftPanel = model.(*Panel)
					cmds = append(cmds, cmd)
				} else {
					model, cmd := a.rightPanel.Update(msg)
					a.rightPanel = model.(*Panel)
					cmds = append(cmds, cmd)
				}
			} else {
				// Route to terminal
				model, cmd := a.terminal.Update(msg)
				a.terminal = model.(*Terminal)
				cmds = append(cmds, cmd)
			}
		}

	default:
		if mm, ok := msg.(tea.MouseMsg); ok {
			if a.showTerminal {
				m := mm.Mouse()
				if m.Y == a.height-1 {
					if rel, ok := mm.(tea.MouseReleaseMsg); ok && rel.Mouse().Button == tea.MouseLeft {
						a.showTerminal = false
						a.terminal.SetShowPanels(true)
					}
					return a, nil
				}
				model, cmd := a.terminal.Update(mm)
				a.terminal = model.(*Terminal)
				cmds = append(cmds, cmd)
				return a, tea.Batch(cmds...)
			}
			m := mm.Mouse()
			if m.Y == a.height-1 {
				if rel, ok := mm.(tea.MouseReleaseMsg); ok && rel.Mouse().Button == tea.MouseLeft {
					if cmd := a.handleFunctionKeyClick(m.X); cmd != nil {
						return a, cmd
					}
				}
				return a, nil
			}
			if cmd, panel, panelMsg, panelIdx, handled := a.dispatchPanelMouse(mm); handled {
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				if panelMsg.Type == PanelMouseClick && panelMsg.Button == tea.MouseLeft && panel != nil {
					ctxSel, cancelSel := context.WithTimeout(a.ctx, panelContextTimeout)
					selectionID := panel.currentSelectionID(ctxSel)
					cancelSel()
					if selectionID != "" {
						now := time.Now()
						timeout := a.cfg.Input.Mouse.DoubleClickTimeout.Duration
						if timeout <= 0 {
							timeout = 300 * time.Millisecond
						}
						if a.lastClickRowID == selectionID && a.lastClickPanel == panelIdx && now.Sub(a.lastClickTime) <= timeout {
							a.lastClickTime = time.Time{}
							a.lastClickRowID = ""
							ctxEnter, cancelEnter := context.WithTimeout(a.ctx, panelContextTimeout)
							enterCmd := panel.enterItem(ctxEnter)
							cancelEnter()
							if enterCmd != nil {
								return a, enterCmd
							}
						} else {
							a.lastClickTime = now
							a.lastClickPanel = panelIdx
							a.lastClickRowID = selectionID
						}
					}
				}
				return a, tea.Batch(cmds...)
			}
			return a, tea.Batch(cmds...)
		}
		model, cmd := a.terminal.Update(msg)
		a.terminal = model.(*Terminal)
		cmds = append(cmds, cmd)

	}

	return a, tea.Batch(cmds...)
}

// shouldRouteToPanel determines if a key should be routed to the panel based on terminal state
func (a *App) shouldRouteToPanel(key string) bool {
	// Always route these keys to terminal
	terminalKeys := []string{
		"space", // Never go to panels
	}

	for _, termKey := range terminalKeys {
		if key == termKey {
			return false
		}
	}

	// Always route these keys to panels (others handled below)
	panelKeys := []string{
		// Navigation keys
		"up", "down", // Navigate items (left/right handled conditionally below)
		"home", "end", // Navigate to beginning/end
		"pgup", "pgdown", // Page up/down
		// Panel control keys
		"tab",    // Switch panels
		"ctrl+o", // Toggle fullscreen
		// Selection keys
		"ctrl+t", "insert", // Toggle selection
		"*",      // Invert selection
		"ctrl+a", // Select all
		"ctrl+w",
		// Function keys (F10 handled separately for fullscreen vs panel mode)
		"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f11", "f12",
		// Other panel actions
		"ctrl+r", // Refresh
		"ctrl+s", // Search
		"esc",    // Cancel
	}

	for _, panelKey := range panelKeys {
		if key == panelKey {
			return true
		}
	}

	// Special handling for Enter key
	if key == "enter" {
		// Check if terminal has non-empty input
		if a.terminal != nil && a.terminal.HasInput() {
			return false // Route Enter to terminal if user is typing
		}
		return true // Route Enter to panels if terminal is empty
	}

	// Special handling for Left/Right: route to panels only when terminal input is empty
	if key == "left" || key == "right" {
		if a.terminal != nil && a.terminal.HasInput() {
			return false // typing → keep in terminal
		}
		return true
	}

	// Special handling for + and - keys (glob patterns)
	if key == "+" || key == "-" {
		// Only route to panels if terminal is empty
		if a.terminal != nil && a.terminal.HasInput() {
			return false // Route to terminal if user is typing
		}
		return true // Route to panels if terminal is empty
	}

	// Special handling for F10 key
	if key == "f10" {
		// In fullscreen mode, F10 goes to terminal (for shell commands)
		// In panel mode, F10 quits kc (handled in main switch statement)
		return false // Always route to terminal
	}

	// Default: route to terminal for typing
	return false
}

// View renders the application
func (a *App) View() (string, *tea.Cursor) {
	// In fullscreen terminal mode, only show terminal
	if a.showTerminal {
		terminalView, terminalCursor := a.renderTerminalView()
		return terminalView, terminalCursor
	}

	// In normal mode, show main view
	mainView, mainCursor := a.renderMainView()

	// Overlay modal if visible
	if a.modalManager.IsModalVisible() {
		// Render modal as an overlay covering the UI for clarity
		return a.modalManager.View(), nil
	}

	return mainView, mainCursor
}

// renderMainView renders the main two-panel view
func (a *App) renderMainView() (string, *tea.Cursor) {
	// Calculate dimensions
	// Reserve space for: terminal (2) + function keys (1)
	reserved := 3
	panelHeight := a.height - reserved
	panelWidth := a.width / 2 // No separator needed

	// Set dimensions for panel content (accounting for borders)
	// Each panel needs 2 characters width and 2 lines height for borders
	contentWidth := panelWidth - 2
	contentHeight := panelHeight - 2
	if contentWidth < 0 {
		contentWidth = 1
	}
	if contentHeight < 0 {
		contentHeight = 1
	}

	// Panel content height must match the frame interior (frameHeight-2).
	// contentHeight already equals (panelHeight-2), so subtract one more to account for the frame's top/bottom.
	ctxLeft, cancelLeft := context.WithTimeout(a.ctx, panelContextTimeout)
	a.leftPanel.SetDimensions(ctxLeft, contentWidth, contentHeight-2)
	leftContentView := a.leftPanel.ViewContentOnlyFocused(ctxLeft, a.activePanel == 0)
	leftFooterView := a.leftPanel.GetFooter(ctxLeft)
	cancelLeft()
	ctxRight, cancelRight := context.WithTimeout(a.ctx, panelContextTimeout)
	a.rightPanel.SetDimensions(ctxRight, contentWidth, contentHeight-2)
	rightContentView := a.rightPanel.ViewContentOnlyFocused(ctxRight, a.activePanel == 1)
	rightFooterView := a.rightPanel.GetFooter(ctxRight)
	cancelRight()

	// Calculate heights for frame and footer
	footerHeight := 2
	frameHeight := panelHeight - footerHeight

	// Create frames with proper dimensions, passing focus state
	leftFramed := a.createFrameWithOverlayTitle(leftContentView, a.leftPanel.GetCurrentPath(), panelWidth, frameHeight, a.activePanel == 0)
	rightFramed := a.createFrameWithOverlayTitle(rightContentView, a.rightPanel.GetCurrentPath(), panelWidth, frameHeight, a.activePanel == 1)

	// Create framed footers with T-junction connection
	leftFooter := a.createFramedFooter(leftFooterView, panelWidth)
	rightFooter := a.createFramedFooter(rightFooterView, panelWidth)

	// Combine frame and footer for each panel
	leftPanel := lipgloss.JoinVertical(lipgloss.Top, leftFramed, leftFooter)
	rightPanel := lipgloss.JoinVertical(lipgloss.Top, rightFramed, rightFooter)

	// Combine panels without separator
	panels := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPanel,
		rightPanel,
	)

	// Add terminal (2 lines)
	terminalView, terminalCursor := a.renderTerminalArea()

	// Add function key bar
	functionKeys := a.renderFunctionKeys()
	combinedView := lipgloss.JoinVertical(
		lipgloss.Left,
		panels,
		terminalView,
		functionKeys,
	)

	// Busy overlay: show a small 2x2 ASCII animation centered over the main view
	if a.busyActive {
		ov := a.renderBusyOverlay()
		if ov != "" {
			combinedView = overlay.Composite(ov, combinedView, overlay.Center, overlay.Center, 0, 0)
		}
	}

	// Adjust cursor position for the combined view
	// The cursor needs to be offset by the height of panels
	if terminalCursor != nil {
		// Calculate the offset: panels height
		offsetY := panelHeight
		adjustedCursor := tea.NewCursor(terminalCursor.X, terminalCursor.Y+offsetY)
		adjustedCursor.Blink = terminalCursor.Blink
		adjustedCursor.Color = terminalCursor.Color
		adjustedCursor.Shape = terminalCursor.Shape
		return combinedView, adjustedCursor
	}

	return combinedView, nil
}

// renderBusyOverlay returns a small 2x2 ASCII animation based on busyFrame.
func (a *App) renderBusyOverlay() string {
	// 2x2 ASCII frames: cross and bar alternation
	frames := []string{
		"\\/\n/\\", // star
		"||\n||",
		"/\\\n\\/",
		"--\n--",
	}
	f := frames[a.busyFrame%len(frames)]
	// Add a faint box/spacing around for visibility (optional)
	st := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.White).Background(lipgloss.Color("238")).Padding(0, 1)
	return st.Render(f)
}

// renderTerminalArea renders the 2-line terminal area in main view
func (a *App) renderTerminalArea() (string, *tea.Cursor) {
	terminalView, terminalCursor := a.terminal.View()
	return terminalView, terminalCursor
}

// renderTerminalView renders the full-screen terminal view
func (a *App) renderTerminalView() (string, *tea.Cursor) {
	// Get terminal view
	terminalView, terminalCursor := a.terminal.View()

	// Compose with a one-line toggle message at the bottom. To ensure it's visible,
	// clamp the terminal content to a.height-1 lines.
	toggleMsg := a.renderToggleMessage()
	lines := strings.Split(terminalView, "\n")
	maxTerm := a.height - 1
	if maxTerm < 1 {
		maxTerm = 1
	}
	if len(lines) > maxTerm {
		lines = lines[:maxTerm]
	} else if len(lines) < maxTerm {
		// pad with empty lines to keep layout stable
		pad := make([]string, maxTerm-len(lines))
		lines = append(lines, pad...)
	}
	clamped := strings.Join(lines, "\n")
	combinedView := lipgloss.JoinVertical(lipgloss.Left, clamped, toggleMsg)

	// Adjust cursor position so it never overlaps the toggle message
	if terminalCursor != nil {
		cy := terminalCursor.Y
		if cy >= maxTerm {
			cy = maxTerm - 1
		}
		if cy < 0 {
			cy = 0
		}
		adjusted := tea.NewCursor(terminalCursor.X, cy)
		adjusted.Blink = terminalCursor.Blink
		adjusted.Color = terminalCursor.Color
		adjusted.Shape = terminalCursor.Shape
		return combinedView, adjusted
	}
	return combinedView, nil
}

// refreshFoldersAfterViewChange reapplies the current folders to panels so that
// folder population re-reads the latest panel config.
func (a *App) refreshFoldersAfterViewChange() {
	if a.leftNav != nil {
		cur := a.leftNav.Current()
		ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
		a.leftPanel.SetFolder(ctx, cur, a.leftNav.HasBack())
		a.leftPanel.SetCurrentPath(a.navigatorPath(a.leftNav))
		a.leftPanel.RefreshFolder(ctx)
		cancel()
	}
	if a.rightNav != nil {
		cur := a.rightNav.Current()
		ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
		a.rightPanel.SetFolder(ctx, cur, a.rightNav.HasBack())
		a.rightPanel.SetCurrentPath(a.navigatorPath(a.rightNav))
		a.rightPanel.RefreshFolder(ctx)
		cancel()
	}
}

// renderFunctionKeys renders the function key bar
func (a *App) renderFunctionKeys() string {
	if a.toastActive {
		msg := a.toastText
		maxw := a.width
		if lipgloss.Width(msg) > maxw {
			if maxw > 1 {
				msg = sliceANSIColsRaw(msg, 0, maxw-1) + "…"
			} else {
				msg = sliceANSIColsRaw(msg, 0, maxw)
			}
		}
		toastStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("196")).
			Foreground(lipgloss.White).
			Bold(true)
		return toastStyle.Width(a.width).Render(msg)
	}

	var keys []string

	if a.showTerminal {
		keys = []string{FunctionKeyStyle.Render("Ctrl+O") + FunctionKeyDescriptionStyle.Render("Return to panels")}
	} else {
		panel := a.activePanelRef()
		ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
		caps := PanelCapabilities{}
		if panel != nil {
			caps = panel.Capabilities(ctx)
		}
		cancel()
		renderKey := func(key, label string, enabled bool) string {
			desc := FunctionKeyDescriptionStyle
			if !enabled {
				desc = FunctionKeyDisabledStyle
			}
			return FunctionKeyStyle.Render(key) + desc.Render(label)
		}

		keys = []string{
			renderKey("F1", "Help", caps.HasHelp),
			renderKey("F2", "Options", caps.HasOptions),
			renderKey("F3", "View", caps.CanView),
			renderKey("F4", "Edit", caps.CanEdit),
			renderKey("F5", "Copy", false),
			renderKey("F6", "Rename/Move", false),
			renderKey("F7", "Namespace", caps.CanCreateNS),
			renderKey("F8", "Delete", caps.CanDelete),
			renderKey("F9", "Menu", caps.HasContextMenu),
			FunctionKeyStyle.Render("F10") + FunctionKeyDescriptionStyle.Render("Quit"),
			FunctionKeyStyle.Render("Ctrl+O") + FunctionKeyDescriptionStyle.Render("Fullscreen"),
		}
	}

	joined := lipgloss.JoinHorizontal(lipgloss.Left, keys...)
	title := " Kubernetes Commander "
	fullWidthStyle := FunctionKeyBarStyle.Width(a.width).Align(lipgloss.Left)
	titleStyle := FunctionKeyTitleStyle.Align(lipgloss.Center).Width(a.width - lipgloss.Width(joined) - 1)
	titleRendered := titleStyle.Render(title)
	return fullWidthStyle.Render(joined + " " + titleRendered)
}

// handleFunctionKeyClick maps an x coordinate on the function key bar to a key action.
func (a *App) handleFunctionKeyClick(x int) tea.Cmd {
	if a.toastActive {
		return nil
	}
	var keys []struct {
		label   string
		enabled bool
		action  func() tea.Cmd
	}
	if a.showTerminal {
		keys = []struct {
			label   string
			enabled bool
			action  func() tea.Cmd
		}{
			{label: FunctionKeyStyle.Render("Ctrl+O") + FunctionKeyDescriptionStyle.Render("Return to panels"), enabled: true, action: func() tea.Cmd {
				a.showTerminal = false
				a.terminal.SetShowPanels(true)
				return nil
			}},
		}
	} else {
		panel := a.activePanelRef()
		ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
		caps := PanelCapabilities{}
		if panel != nil {
			caps = panel.Capabilities(ctx)
		}
		cancel()
		makeLbl := func(key, label string, enabled bool) string {
			desc := FunctionKeyDescriptionStyle
			if !enabled {
				desc = FunctionKeyDisabledStyle
			}
			return FunctionKeyStyle.Render(key) + desc.Render(label)
		}
		invoke := func(action PanelAction) func() tea.Cmd {
			return func() tea.Cmd {
				if panel == nil {
					return nil
				}
				ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
				cmd := panel.invokeActionIfAllowed(ctx, action)
				cancel()
				return cmd
			}
		}
		keys = []struct {
			label   string
			enabled bool
			action  func() tea.Cmd
		}{
			{makeLbl("F1", "Help", caps.HasHelp), caps.HasHelp, invoke(PanelActionHelp)},
			{makeLbl("F2", "Options", caps.HasOptions), caps.HasOptions, invoke(PanelActionOptions)},
			{makeLbl("F3", "View", caps.CanView), caps.CanView, invoke(PanelActionView)},
			{makeLbl("F4", "Edit", caps.CanEdit), caps.CanEdit, invoke(PanelActionEdit)},
			{makeLbl("F5", "Copy", false), false, a.copyItem},
			{makeLbl("F6", "Rename/Move", false), false, a.renameMoveItem},
			{makeLbl("F7", "Namespace", caps.CanCreateNS), caps.CanCreateNS, invoke(PanelActionCreateNamespace)},
			{makeLbl("F8", "Delete", caps.CanDelete), caps.CanDelete, invoke(PanelActionDelete)},
			{FunctionKeyStyle.Render("F9") + FunctionKeyDescriptionStyle.Render("Menu"), caps.HasContextMenu, invoke(PanelActionMenu)},
			{FunctionKeyStyle.Render("F10") + FunctionKeyDescriptionStyle.Render("Quit"), true, func() tea.Cmd { return tea.Quit }},
			{FunctionKeyStyle.Render("Ctrl+O") + FunctionKeyDescriptionStyle.Render("Fullscreen"), true, func() tea.Cmd {
				a.showTerminal = true
				a.terminal.SetShowPanels(false)
				return nil
			}},
		}
	}

	spans := make([]int, len(keys)+1)
	acc := 0
	for i, k := range keys {
		spans[i] = acc
		acc += lipgloss.Width(k.label)
	}
	spans[len(keys)] = acc

	for i := 0; i < len(keys); i++ {
		if x >= spans[i] && x < spans[i+1] {
			if keys[i].enabled && keys[i].action != nil {
				return keys[i].action()
			}
			return nil
		}
	}
	return nil
}

// renderToggleMessage renders the toggle message for fullscreen mode
func (a *App) renderToggleMessage() string {
	// Create the same layout as function keys
	key := FunctionKeyStyle.Render("Ctrl+O") + FunctionKeyDescriptionStyle.Render("Return to panels")
	title := FunctionKeyTitleStyle.Render("Kubernetes Commander")

	// Calculate the exact spacing needed to push title to the right edge
	spacing := a.width - len(key) - len(title)
	if spacing < 0 {
		spacing = 1 // minimum spacing
	}

	content := key + strings.Repeat(" ", spacing) + title

	// Create a full-width container
	fullWidthStyle := FunctionKeyBarStyle.
		Width(a.width).
		Align(lipgloss.Left)

	return fullWidthStyle.Render(content)
}

// setupModals sets up the modal dialogs

func (a *App) setupModals() {
	// Resources options modal (content set dynamically on open)
	opts := NewResourcesOptionsModel(false, false, "scroll", false, "favorites")
	resModal := NewModal("Resources", opts)
	a.modalManager.Register("resources_options", resModal)

	// Theme selector modal; content is set dynamically when opened
	themeSelector := NewThemeSelector(nil)
	themeModal := NewModal("YAML Theme", themeSelector)
	a.modalManager.Register("theme_selector", themeModal)

	// Namespace creation modal (configured on open)
	nsModel := NewNamespaceCreateModel()
	nsModal := NewModal("Create Namespace", nsModel)
	nsModal.SetCloseOnSingleEsc(true)
	a.modalManager.Register("namespace_create", nsModal)
	a.namespaceInput = nsModel

	// Delete confirmation modal
	delModel := NewDeleteConfirmModel()
	delModal := NewModal("Confirm Delete", delModel)
	delModal.SetCloseOnSingleEsc(true)
	a.modalManager.Register("delete_confirm", delModal)
	a.deleteConfirm = delModel

	for idx := 0; idx < 2; idx++ {
		modeModel := NewPanelModeModel(idx, []PanelViewMode{PanelModeList}, PanelModeList)
		modeModal := NewModal("Panel Mode", modeModel)
		modeModal.SetCloseOnSingleEsc(true)
		a.modalManager.Register(panelModeModalKey(idx), modeModal)
	}
}

func (a *App) setupPanelInputs() {
	envSupplier := func() PanelEnvironment { return a.panelEnvironment() }
	registerModes := func(panel *Panel, name string) {
		if panel == nil {
			return
		}
		panel.RegisterMode(PanelModeDescribe, func(p *Panel) PanelWidget {
			return newPlaceholderWidget(p, fmt.Sprintf("%s describe view coming soon", name))
		})
		panel.RegisterMode(PanelModeManifest, func(p *Panel) PanelWidget {
			return newPlaceholderWidget(p, fmt.Sprintf("%s manifest view coming soon", name))
		})
		panel.RegisterMode(PanelModeFile, func(p *Panel) PanelWidget {
			return newPlaceholderWidget(p, fmt.Sprintf("%s file view coming soon", name))
		})
	}
	if a.leftPanel != nil {
		a.leftPanel.SetEnvironmentSupplier(envSupplier)
		a.leftPanel.SetActionHandlers(a.panelActionHandlers())
		registerModes(a.leftPanel, "Left panel")
	}
	if a.rightPanel != nil {
		a.rightPanel.SetEnvironmentSupplier(envSupplier)
		a.rightPanel.SetActionHandlers(a.panelActionHandlers())
		registerModes(a.rightPanel, "Right panel")
	}
}

func (a *App) panelActionHandlers() PanelActionHandlers {
	return PanelActionHandlers{
		PanelActionHelp: func(*Panel) tea.Cmd {
			return a.showHelp()
		},
		PanelActionOptions: func(p *Panel) tea.Cmd {
			return a.showViewOptionsModalForPanel(p)
		},
		PanelActionView: func(p *Panel) tea.Cmd {
			return a.openViewerForPanel(p)
		},
		PanelActionEdit: func(p *Panel) tea.Cmd {
			return a.editSelectionForPanel(p)
		},
		PanelActionCreateNamespace: func(p *Panel) tea.Cmd {
			return a.createNamespaceForPanel(p)
		},
		PanelActionDelete: func(p *Panel) tea.Cmd {
			return a.deleteResourceForPanel(p)
		},
		PanelActionMenu: func(p *Panel) tea.Cmd {
			return a.showContextMenuForPanel(p)
		},
	}
}

func (a *App) panelEnvironment() PanelEnvironment {
	env := PanelEnvironment{}
	if a.currentCtx != nil {
		env.AllowCreateNamespaces = true
		if a.currentCtx.Kubeconfig != nil {
			env.AllowEditObjects = true
		}
	}
	if a.cl != nil {
		env.AllowDeleteObjects = true
	}
	return env
}

func (a *App) capabilitiesForPanel(panel *Panel) PanelCapabilities {
	if panel == nil {
		return PanelCapabilities{}
	}
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	defer cancel()
	return panel.Capabilities(ctx)
}

func (a *App) cyclePanelMode(idx int) tea.Cmd {
	panel := a.panelByIndex(idx)
	if panel == nil {
		return nil
	}
	next := NextPanelMode(panel.Mode())
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	defer cancel()
	return panel.SetMode(ctx, next)
}

func panelModeModalKey(idx int) string {
	if idx == 0 {
		return "panel_mode_left"
	}
	return "panel_mode_right"
}

func (a *App) showPanelModeModal(panelIdx int) tea.Cmd {
	panel := a.panelByIndex(panelIdx)
	if panel == nil {
		return nil
	}
	modes := panel.AvailableModes()
	if len(modes) == 0 {
		modes = []PanelViewMode{PanelModeList}
	}
	model := NewPanelModeModel(panelIdx, modes, panel.Mode())
	key := panelModeModalKey(panelIdx)
	modal := a.modalManager.modals[key]
	if modal == nil {
		modal = NewModal("Panel Mode", model)
		modal.SetCloseOnSingleEsc(true)
		a.modalManager.Register(key, modal)
	} else {
		modal.SetContent(model)
	}
	panelWidth, panelHeight, headerOffset := a.panelAreaMetrics()
	if panelWidth <= 0 {
		panelWidth = max(20, a.width/2)
	}
	width := panelWidth / 2
	if width < 24 {
		width = 24
	}
	width = min(width, panelWidth-2)
	width = min(width, a.width-4)
	maxContentHeight := max(len(modes)+4, 6)
	targetHeight := panelHeight / 2
	if targetHeight < 6 {
		targetHeight = 6
	}
	height := min(maxContentHeight, targetHeight)
	if height < 6 {
		height = 6
	}
	model.SetDimensions(max(1, width-4), max(1, height-3))
	modal.SetDimensions(a.width, a.height)
	bg, _ := a.renderMainView()
	modal.SetWindowed(width, height, bg)
	offsetX := panelIdx*panelWidth + max(0, (panelWidth-width)/2)
	offsetY := headerOffset
	modal.SetWindowOffset(offsetX, offsetY)
	modal.SetOnClose(func() tea.Cmd { return nil })
	a.modalManager.Show(key)
	return nil
}

func (a *App) dispatchPanelMouse(msg tea.MouseMsg) (tea.Cmd, *Panel, PanelMouseMsg, int, bool) {
	panelWidth, panelHeight, headerOffset := a.panelAreaMetrics()
	m := msg.Mouse()
	if m.Y >= panelHeight {
		return nil, nil, PanelMouseMsg{}, 0, false
	}
	panelIdx := 0
	if m.X >= panelWidth {
		panelIdx = 1
	}
	panel := a.panelByIndex(panelIdx)
	if panel == nil {
		return nil, nil, PanelMouseMsg{}, panelIdx, false
	}
	relRow := m.Y - headerOffset
	if relRow < 0 {
		relRow = 0
	}
	var panelMsg PanelMouseMsg
	switch mm := msg.(type) {
	case tea.MouseWheelMsg:
		delta := 0
		switch mm.Button {
		case tea.MouseWheelUp:
			delta = -1
		case tea.MouseWheelDown:
			delta = 1
		default:
			return nil, nil, PanelMouseMsg{}, panelIdx, false
		}
		panelMsg = PanelMouseMsg{
			Type:   PanelMouseWheel,
			DeltaY: delta,
		}
	case tea.MouseClickMsg:
		panelMsg = PanelMouseMsg{
			Type:   PanelMouseClick,
			Row:    relRow,
			Button: mm.Button,
		}
	default:
		return nil, nil, PanelMouseMsg{}, panelIdx, false
	}
	if panelIdx != a.activePanel {
		a.activePanel = panelIdx
	}
	model, cmd := panel.Update(panelMsg)
	if model != nil {
		if panelIdx == 0 {
			a.leftPanel = model.(*Panel)
		} else {
			a.rightPanel = model.(*Panel)
		}
	}
	return cmd, panel, panelMsg, panelIdx, true
}

// Message handlers for function keys
// showViewOptionsModal opens the appropriate View Options dialog (Resources or Objects)
// depending on the active view context.
func (a *App) showViewOptionsModal() tea.Cmd {
	return a.showViewOptionsModalForPanel(a.activePanelRef())
}

func (a *App) showViewOptionsModalForPanel(panel *Panel) tea.Cmd {
	if panel == nil {
		panel = a.activePanelRef()
	}
	if panel == nil {
		return nil
	}

	// Determine folder context for contextual options.
	var curFolder models.Folder
	if nav := a.navigatorForPanel(panel); nav != nil {
		curFolder = nav.Current()
	}
	if curFolder == nil {
		curFolder = panel.folder
	}

	showInclude := false
	showOrder := false
	if curFolder != nil {
		switch curFolder.(type) {
		case *models.RootFolder,
			*models.ClusterResourcesFolder,
			*models.NamespacedResourcesFolder,
			*models.ContextRootFolder,
			*models.ResourcesFolder:
			showInclude = true
			showOrder = true
		}
	}

	showNonEmpty, order := panel.ResourceViewOptions()
	tableMode := panel.TableMode()
	content := NewResourcesOptionsModel(showInclude, showOrder, tableMode, showNonEmpty, order)
	modal := a.modalManager.modals["resources_options"]
	if modal == nil {
		modal = NewModal("Resources View Options", content)
		a.modalManager.Register("resources_options", modal)
	} else {
		modal.SetContent(content)
		modal.title = "Resources View Options"
	}
	winW, winH := 50, 6
	bg, _ := a.renderMainView()
	modal.SetWindowed(winW, winH, bg)
	modal.SetOnClose(func() tea.Cmd { return nil })
	modal.SetDimensions(a.width, a.height)
	a.modalManager.Show("resources_options")
	return nil
}

func (a *App) viewResource() tea.Cmd {
	// TODO: Implement resource viewer
	return nil
}

func (a *App) editResource() tea.Cmd {
	// TODO: Implement resource editor
	return nil
}

func (a *App) createNamespace() tea.Cmd {
	return a.createNamespaceForPanel(a.activePanelRef())
}

func (a *App) createNamespaceForPanel(panel *Panel) tea.Cmd {
	if a.currentCtx == nil {
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("No active context for namespace creation"))
		}
		return nil
	}
	if panel == nil {
		panel = a.activePanelRef()
	}
	if panel == nil {
		return nil
	}
	if panel.GetCurrentPath() != "/namespaces" {
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("Create namespace is only available at /namespaces"))
		}
		return nil
	}
	a.namespaceCreatePanel = a.panelIndex(panel)
	modal := a.modalManager.modals["namespace_create"]
	if modal == nil {
		a.namespaceCreatePanel = -1
		return nil
	}
	if a.namespaceInput == nil {
		if model, ok := modal.content.(*NamespaceCreateModel); ok {
			a.namespaceInput = model
		} else {
			return nil
		}
	}
	a.namespaceInput.Reset()
	a.namespaceInput.SetDimensions(max(20, a.width-4), max(5, a.height-6))
	modal.SetContent(a.namespaceInput)
	modal.SetDimensions(a.width, a.height)
	bg, _ := a.renderMainView()
	winW := min(max(40, a.width/2), a.width-4)
	winH := min(10, a.height-4)
	if winW < 30 {
		winW = 30
	}
	if winH < 8 {
		winH = 8
	}
	modal.SetWindowed(winW, winH, bg)
	modal.SetOnClose(func() tea.Cmd {
		a.namespaceInput.Reset()
		return nil
	})
	a.modalManager.Show("namespace_create")
	return nil
}

func (a *App) deleteResource() tea.Cmd {
	return a.deleteResourceForPanel(a.activePanelRef())
}

func (a *App) deleteResourceForPanel(panel *Panel) tea.Cmd {
	panelIdx := a.activePanel
	if panel == nil {
		panel = a.activePanelRef()
	} else {
		if idx := a.panelIndex(panel); idx >= 0 {
			panelIdx = idx
		}
	}
	if panel == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	item, ok := panel.SelectedNavItem(ctx)
	cancel()
	if !ok || item == nil {
		return nil
	}
	obj, ok := item.(models.ObjectItem)
	if !ok {
		return nil
	}
	gvr := obj.GVR()
	name := obj.Name()
	namespace := obj.Namespace()
	target := deleteTarget{
		panelIdx:  panelIdx,
		gvr:       gvr,
		namespace: namespace,
		name:      name,
	}
	a.pendingDelete = &target

	modal := a.modalManager.modals["delete_confirm"]
	if modal == nil {
		return nil
	}
	if a.deleteConfirm == nil {
		if model, ok := modal.content.(*DeleteConfirmModel); ok {
			a.deleteConfirm = model
		} else {
			return nil
		}
	}
	resourceLabel := kubectlResourceRef(gvr, name)
	a.deleteConfirm.Configure(resourceLabel, namespace)
	a.deleteConfirm.SetDimensions(max(20, a.width-4), max(5, a.height-6))
	modal.SetContent(a.deleteConfirm)
	modal.SetDimensions(a.width, a.height)
	bg, _ := a.renderMainView()
	winW := min(max(50, a.width/2), a.width-4)
	winH := min(8, a.height-4)
	if winW < 40 {
		winW = 40
	}
	if winH < 6 {
		winH = 6
	}
	modal.SetWindowed(winW, winH, bg)
	modal.SetOnClose(func() tea.Cmd {
		a.pendingDelete = nil
		return nil
	})
	a.modalManager.Show("delete_confirm")
	return nil
}

func (a *App) showContextMenu() tea.Cmd {
	return a.showContextMenuForPanel(a.activePanelRef())
}

func (a *App) showContextMenuForPanel(_ *Panel) tea.Cmd {
	// TODO: Implement context menu
	return nil
}

func (a *App) createNamespaceWithName(name string) tea.Cmd {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	return a.withBusy("Create namespace", 300*time.Millisecond, func() tea.Msg {
		if a.cl == nil {
			return namespaceCreatedMsg{name: name, err: fmt.Errorf("cluster not ready")}
		}
		ctx, cancel := context.WithTimeout(a.ctx, requestTimeout)
		defer cancel()
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
		if err := a.cl.GetClient().Create(ctx, ns); err != nil {
			return namespaceCreatedMsg{name: name, err: err}
		}
		return namespaceCreatedMsg{name: name}
	})
}

func (a *App) performDelete(target deleteTarget) tea.Cmd {
	return a.withBusy("Delete", 300*time.Millisecond, func() tea.Msg {
		if a.cl == nil {
			return resourceDeletedMsg{target: target, err: fmt.Errorf("cluster not ready")}
		}
		ctx, cancel := context.WithTimeout(a.ctx, requestTimeout)
		defer cancel()
		kind, err := a.cl.RESTMapper().KindFor(target.gvr)
		if err != nil {
			return resourceDeletedMsg{target: target, err: err}
		}
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(kind)
		obj.SetName(target.name)
		if target.namespace != "" {
			obj.SetNamespace(target.namespace)
		}
		if err := a.cl.GetClient().Delete(ctx, obj); err != nil {
			return resourceDeletedMsg{target: target, err: err}
		}
		return resourceDeletedMsg{target: target}
	})
}

// Function key action methods
func (a *App) showHelp() tea.Cmd {
	// TODO: Implement help dialog
	return nil
}

func (a *App) viewItem() tea.Cmd {
	return a.openViewerForPanel(a.activePanelRef())
}

func (a *App) editItem() tea.Cmd {
	return a.editSelectionForPanel(a.activePanelRef())
}

// openViewerForSelection opens the focused item's viewer when available.
func (a *App) openViewerForSelection() tea.Cmd {
	return a.openViewerForPanel(a.activePanelRef())
}

func (a *App) openViewerForPanel(panel *Panel) tea.Cmd {
	if panel == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	defer cancel()
	item, ok := panel.SelectedNavItem(ctx)
	if !ok || item == nil {
		return nil
	}
	if _, isBack := item.(models.Back); isBack {
		return nil
	}
	viewable, ok := item.(models.Viewable)
	if !ok {
		type vc interface {
			ViewContent() (string, string, string, string, string, error)
		}
		if alt, okAlt := item.(vc); okAlt {
			viewable = alt
		} else {
			return nil
		}
	}
	title, body, lang, mime, filename, err := viewable.ViewContent()
	if err != nil {
		if errors.Is(err, models.ErrNoViewContent) {
			return nil
		}
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("View failed: %v", err))
		}
		return nil
	}
	if filename == "" {
		filename = title
	}
	theme := "dracula"
	if a.cfg != nil && a.cfg.Viewer.Theme != "" {
		theme = a.cfg.Viewer.Theme
	}
	var onEdit func() tea.Cmd
	if _, ok := item.(models.ObjectItem); ok {
		onEdit = func() tea.Cmd { return a.editSelectionForPanel(panel) }
	}
	viewer := NewTextViewer(title, body, lang, mime, filename, theme, onEdit, nil, func() tea.Cmd {
		a.modalManager.Hide()
		return nil
	})
	viewer.SetOnTheme(func() tea.Cmd { return a.showThemeSelector(viewer) })
	modalTitle := ""
	if pa, ok := item.(interface{ Path() []string }); ok {
		if segs := pa.Path(); len(segs) > 0 {
			modalTitle = "/" + strings.Join(segs, "/")
		}
	}
	if modalTitle == "" {
		modalTitle = "/" + title
	}
	modal := NewModal(modalTitle, viewer)
	modal.SetDimensions(a.width, a.height)
	modal.SetCloseOnSingleEsc(false)
	a.modalManager.Register("yaml_viewer", modal)
	a.modalManager.Show("yaml_viewer")
	return nil
}

// showThemeSelector opens the theme selector modal and wires selection to save
// config and re-highlight the currently open YAML viewer.
func (a *App) showThemeSelector(v *TextViewer) tea.Cmd {
	modal := a.modalManager.modals["theme_selector"]
	if modal == nil {
		return nil
	}
	// Remember previous theme to restore on cancel
	a.prevTheme = a.cfg.Viewer.Theme
	a.suppressThemeRevert = false

	selector := NewThemeSelector(func(name string) tea.Cmd {
		if name == "" {
			return nil
		}
		if a.cfg == nil {
			a.cfg = appconfig.Default()
		}
		a.cfg.Viewer.Theme = name
		_ = appconfig.Save(a.cfg)
		v.SetTheme(name)
		a.suppressThemeRevert = true
		a.modalManager.Hide()
		return nil
	})
	selector.SetDimensions(a.width-2, a.height-6)
	// Preselect current theme if available
	if a.cfg != nil {
		selector.SetSelectedByName(a.cfg.Viewer.Theme)
	}
	// Live preview on selection change
	selector.SetOnChange(func(name string) tea.Cmd { v.SetTheme(name); return nil })
	modal.SetContent(selector)
	modal.SetDimensions(a.width, a.height)
	// Configure as centered window overlay so YAML viewer remains visible
	winW := min(max(40, a.width*2/3), a.width-4)
	winH := min(max(10, a.height*2/3), a.height-4)
	bg := ""
	if y := a.modalManager.modals["yaml_viewer"]; y != nil {
		bg = y.View()
	}
	modal.SetWindowed(winW, winH, bg)
	// onClose not needed; Esc handling hides the top modal and reveals viewer beneath
	modal.SetOnClose(func() tea.Cmd {
		if !a.suppressThemeRevert {
			if a.prevTheme != "" {
				v.SetTheme(a.prevTheme)
			}
		}
		a.suppressThemeRevert = false
		return nil
	})
	a.modalManager.Show("theme_selector")
	return nil
}

// editSelection triggers kubectl edit for the selected object.
func (a *App) editSelection() tea.Cmd {
	return a.editSelectionForPanel(a.activePanelRef())
}

func (a *App) editSelectionForPanel(panel *Panel) tea.Cmd {
	panelIdx := a.activePanel
	if panel == nil {
		panel = a.activePanelRef()
	} else {
		if idx := a.panelIndex(panel); idx >= 0 {
			panelIdx = idx
		}
	}
	if panel == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
	item, ok := panel.SelectedNavItem(ctx)
	cancel()
	if !ok || item == nil {
		return nil
	}
	obj, ok := item.(models.ObjectItem)
	if !ok {
		return nil
	}

	path := panel.GetCurrentPath()
	return a.runKubectlEdit(panelIdx, path, obj)
}

func (a *App) runKubectlEdit(panelIdx int, panelPath string, obj models.ObjectItem) tea.Cmd {
	if a.currentCtx == nil || a.currentCtx.Kubeconfig == nil {
		if a.toastLogger != nil {
			return a.toastLogger.Errorf("kubectl edit: no active kubeconfig")
		}
		return a.ShowToast("kubectl edit unavailable: no active kubeconfig", 5*time.Second)
	}

	log := ctrllog.FromContext(a.ctx).WithName("kubectl_edit")

	kubeconfigPath := a.currentCtx.Kubeconfig.Path
	tempConfigPath := ""
	if kubeconfigPath == "" {
		if a.currentCtx.Kubeconfig.Config == nil {
			if a.toastLogger != nil {
				return a.toastLogger.Errorf("kubectl edit: kubeconfig has no backing file")
			}
			return a.ShowToast("kubectl edit failed: kubeconfig missing backing file", 5*time.Second)
		}
		tmpFile, err := os.CreateTemp("", "kc-kubeconfig-*.yaml")
		if err != nil {
			if a.toastLogger != nil {
				return a.toastLogger.Errorf("kubectl edit: temp kubeconfig: %v", err)
			}
			return a.ShowToast(fmt.Sprintf("kubectl edit failed: %v", err), 5*time.Second)
		}
		if err := tmpFile.Close(); err != nil {
			_ = os.Remove(tmpFile.Name())
			if a.toastLogger != nil {
				return a.toastLogger.Errorf("kubectl edit: temp kubeconfig close: %v", err)
			}
			return a.ShowToast(fmt.Sprintf("kubectl edit failed: %v", err), 5*time.Second)
		}
		if err := clientcmd.WriteToFile(*a.currentCtx.Kubeconfig.Config, tmpFile.Name()); err != nil {
			_ = os.Remove(tmpFile.Name())
			if a.toastLogger != nil {
				return a.toastLogger.Errorf("kubectl edit: write kubeconfig: %v", err)
			}
			return a.ShowToast(fmt.Sprintf("kubectl edit failed: %v", err), 5*time.Second)
		}
		kubeconfigPath = tmpFile.Name()
		tempConfigPath = tmpFile.Name()
	}

	contextName := a.currentCtx.Name
	if contextName == "" {
		if a.toastLogger != nil {
			return a.toastLogger.Errorf("kubectl edit: context name empty")
		}
		return a.ShowToast("kubectl edit failed: active context missing name", 5*time.Second)
	}

	namespace := strings.TrimSpace(obj.Namespace())
	if namespace == "" && panelPath != "" {
		if ns, _, _, ok := parseNamespacedObjectPath(panelPath, obj.Name()); ok && ns != "" {
			namespace = ns
		}
	}
	if namespace == "" {
		namespace = strings.TrimSpace(a.currentCtx.Namespace)
	}

	resourceRef := kubectlResourceRef(obj.GVR(), obj.Name())

	commandStr := strings.Join(append([]string{"kubectl", "edit", resourceRef, "--context", contextName, "--kubeconfig", kubeconfigPath}), " ")
	args := []string{"edit", resourceRef, "--context", contextName, "--kubeconfig", kubeconfigPath}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
		commandStr = commandStr + " --namespace " + namespace
	}

	log.Info("launching kubectl edit", "command", commandStr)
	a.logCommandToTerminal(commandStr)

	// Ensure the UI returns to the primary panel view before launching the editor.
	if a.modalManager != nil {
		for a.modalManager.IsModalVisible() {
			a.modalManager.Hide()
		}
	}
	if a.showTerminal {
		a.showTerminal = false
		if a.terminal != nil {
			a.terminal.SetShowPanels(true)
		}
	}

	cmd := exec.Command("kubectl", args...)
	env := os.Environ()
	if kubeconfigPath != "" {
		env = append(env, "KUBECONFIG="+kubeconfigPath)
	}
	cmd.Env = env

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return kubectlEditFinishedMsg{
			err:         err,
			panelIndex:  panelIdx,
			resourceRef: resourceRef,
			tempConfig:  tempConfigPath,
		}
	})
}

func kubectlResourceRef(gvr schema.GroupVersionResource, name string) string {
	resource := strings.Join([]string{gvr.Resource, gvr.Version, gvr.Group}, ".")
	return fmt.Sprintf("%s/%s", resource, name)
}

func (a *App) logCommandToTerminal(command string) {
	if command == "" {
		return
	}
	fmt.Fprintf(os.Stdout, "\n[kc] %s\n", command)
}

func (a *App) refreshPanelAfterEdit(panelIdx int) {
	var panel *Panel
	var nav *navui.Navigator
	if panelIdx == 0 {
		panel = a.leftPanel
		nav = a.leftNav
	} else {
		panel = a.rightPanel
		nav = a.rightNav
	}

	if nav != nil {
		if rf, ok := nav.Current().(interface{ Refresh() }); ok {
			rf.Refresh()
		}
	}

	if panel != nil {
		ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
		panel.RefreshFolder(ctx)
		cancel()
	}
}

func parseNamespacedObjectPath(path, currentName string) (ns, res, name string, ok bool) {
	// /namespaces/<ns>/<res>[/<name>]
	if strings.HasPrefix(path, "/namespaces/") {
		parts := strings.Split(path, "/")
		if len(parts) == 4 { // object list level
			return parts[2], parts[3], currentName, true
		}
		if len(parts) >= 5 { // object level
			return parts[2], parts[3], parts[4], true
		}
	}
	return "", "", "", false
}

func (a *App) copyItem() tea.Cmd {
	// TODO: Implement copy functionality (F5)
	return nil
}

func (a *App) renameMoveItem() tea.Cmd {
	// TODO: Implement rename/move functionality (F6)
	return nil
}

// createFrameWithOverlayTitle creates a frame with title overlaid on the top border
// Based on the approach from https://gist.github.com/meowgorithm/1777377a43373f563476a2bcb7d89306
func (a *App) createFrameWithOverlayTitle(content, title string, width, height int, isFocused bool) string {
	if title == "" {
		// No title, just return regular frame
		return lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.White).
			Background(lipgloss.Blue).
			Width(width).
			Height(height).
			Render(content)
	}

	// Create the box style
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.White).
		BorderBackground(lipgloss.Blue).
		Background(lipgloss.Blue).
		Width(width).
		Height(height)

	// Create label style for the title based on focus state
	var labelStyle lipgloss.Style
	if isFocused {
		// Focused panel: grey background, black text
		labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Black).
			Background(lipgloss.White).
			Padding(0, 1)
	} else {
		// Unfocused panel: dark blue background, grey text
		labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.White).
			Background(lipgloss.Blue).
			Padding(0, 1)
	}

	// Get border properties
	border := boxStyle.GetBorderStyle()
	topBorderStyler := lipgloss.NewStyle().
		Foreground(boxStyle.GetBorderTopForeground()).
		Background(boxStyle.GetBorderTopBackground()).
		Render

	topLeft := topBorderStyler(border.TopLeft)
	topRight := topBorderStyler(border.TopRight)
	// Ellipsize breadcrumb-style titles from the left (replace leading
	// components with ".../") until it fits between the top corners.
	// Measure using the rendered width (accounts for padding).
	ellipsize := func(text string, maxW int) string {
		// Fast path: fits
		if lipgloss.Width(labelStyle.Render(text)) <= maxW {
			return text
		}
		// Minimal fallback
		if maxW <= lipgloss.Width(labelStyle.Render("...")) {
			return "..."
		}
		parts := strings.Split(text, "/")
		segs := make([]string, 0, len(parts))
		for _, p := range parts {
			if p != "" {
				segs = append(segs, p)
			}
		}
		acc := ""
		for i := len(segs) - 1; i >= 0; i-- {
			candidate := "/" + segs[i] + acc
			test := "..." + candidate
			if lipgloss.Width(labelStyle.Render(test)) <= maxW {
				acc = candidate
			} else {
				break
			}
		}
		if acc == "" {
			return "..."
		}
		return "..." + acc
	}

	// Calculate centered positioning for the title
	availableSpace := width - lipgloss.Width(topLeft+topRight)
	title = ellipsize(title, availableSpace)
	renderedLabel := labelStyle.Render(title)
	labelWidth := lipgloss.Width(renderedLabel)

	var top string
	if labelWidth >= availableSpace {
		// Title exactly fills or equals available space; position flush-left between corners
		gap := strings.Repeat(border.Top, max(0, availableSpace-labelWidth))
		top = topLeft + renderedLabel + topBorderStyler(gap) + topRight
	} else {
		// Center the title
		totalBorderNeeded := availableSpace - labelWidth
		leftBorder := totalBorderNeeded / 2
		rightBorder := totalBorderNeeded - leftBorder

		leftGap := topBorderStyler(strings.Repeat(border.Top, leftBorder))
		rightGap := topBorderStyler(strings.Repeat(border.Top, rightBorder))
		top = topLeft + leftGap + renderedLabel + rightGap + topRight
	}

	// Render the rest of the box without the top border
	bottom := boxStyle.Copy().
		BorderTop(false).
		Width(width).
		Height(height - 1). // Subtract 1 since we're manually adding the top
		Render(content)

	// Replace the two corner characters at the TOP of the footer with T-junction characters
	lines := strings.Split(bottom, "\n")
	if len(lines) >= 2 {
		// The bottom border line (last line) - replace └ with ├ and ┘ with ┤
		bottomLine := lines[len(lines)-1]
		bottomLine = strings.Replace(bottomLine, "└", "├", 1)
		bottomLine = strings.Replace(bottomLine, "┘", "┤", 1)
		lines[len(lines)-1] = bottomLine
	}

	// Combine the custom top with the box
	return top + "\n" + strings.Join(lines, "\n")
}

// createFramedFooter creates a framed footer with T-junction characters at the top
func (a *App) createFramedFooter(content string, width int) string {
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderTop(false). // No top border since it connects to the main frame
		BorderForeground(lipgloss.White).
		BorderBackground(lipgloss.Blue).
		Background(lipgloss.Blue).
		Foreground(lipgloss.Color(ColorWhite)).
		Width(width).
		Render(content)
}

// Run starts the application
func Run(ctx context.Context) error {
	ctx = ctrllog.IntoContext(ctx, ctrllog.Log.WithName("startup"))
	log := ctrllog.FromContext(ctx)
	app := NewApp()

	// Initialize data model (best-effort; UI can still run without it)
	log.Info("initializing data")
	if err := app.initData(ctx); err != nil {
		log.Error(err, "initialization warning")
		fmt.Printf("Data init warning: %v\n", err)
	}
	log.Info("initialization complete, launching UI")

	// Create program with proper options
	p := tea.NewProgram(
		app,
		tea.WithAltScreen(),        // Use alternate screen buffer
		tea.WithMouseCellMotion(),  // Enable mouse support
		tea.WithoutSignalHandler(), // Handle signals ourselves
	)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		// Send quit message to the program
		p.Quit()
	}()

	// Ensure terminal is reset on exit
	defer func() {
		// Reset terminal to normal state
		fmt.Print("\033[?1049l") // Exit alternate screen
		fmt.Print("\033[?25h")   // Show cursor
		fmt.Print("\033[0m")     // Reset all attributes
		// Stop background resources
		if app.clPool != nil {
			app.clPool.Stop()
		}
		app.cancel()
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running application: %v\n", err)
		return err
	}

	return nil
}

// initData discovers kubeconfigs, selects current context, starts cluster/cache and wires navigation.
func (a *App) initData(ctx context.Context) error {
	log := ctrllog.FromContext(ctx).WithName("init")
	// Kubeconfig manager and discovery
	a.kubeMgr = kubeconfig.NewManager()
	log.Info("discovering kubeconfigs")
	if err := a.kubeMgr.DiscoverKubeconfigs(); err != nil {
		// Log and show toast
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("Kubeconfig discovery failed: %v", err))
		}
		log.Error(err, "failed to discover kubeconfigs")
		return fmt.Errorf("discover kubeconfigs: %w", err)
	}
	log.Info("kubeconfigs discovered", "count", len(a.kubeMgr.GetKubeconfigs()), "contexts", len(a.kubeMgr.GetContexts()))
	// Select current context (prefer env KUBECONFIG first path)
	a.currentCtx = a.selectCurrentContext()
	if a.currentCtx == nil {
		log.Error(nil, "no current context found")
		return fmt.Errorf("no current context found")
	}
	ctxNamespace := a.currentCtx.Namespace
	if ctxNamespace == "" {
		ctxNamespace = "default"
	}
	log.Info("selected context", "name", a.currentCtx.Name, "cluster", a.currentCtx.Cluster, "namespace", ctxNamespace)
	// Prepare app context and cluster pool; cluster will be started via pool.Get
	a.cancel()
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.clPool = kccluster.NewPool(2 * time.Minute)
	log.Info("starting cluster pool")
	a.clPool.Start()
	k := kccluster.Key{KubeconfigPath: a.currentCtx.Kubeconfig.Path, ContextName: a.currentCtx.Name}
	log.Info("acquiring cluster", "key", k)
	cl, err := a.clPool.Get(a.ctx, k)
	if err != nil {
		log.Error(err, "cluster acquisition failed")
		return fmt.Errorf("cluster pool get: %w", err)
	}
	a.cl = cl
	log.Info("cluster ready, fetching resource info")
	// Discovery-backed catalog (for panel displays)
	if infos, err := a.cl.GetResourceInfos(); err == nil {
		log.Info("resource infos fetched", "count", len(infos))
		a.leftPanel.SetResourceCatalog(infos)
		a.rightPanel.SetResourceCatalog(infos)
	} else {
		if a.toastLogger != nil {
			a.enqueueCmd(a.toastLogger.Errorf("Discovery resources failed: %v", err))
		}
		log.Error(err, "failed to fetch resource infos")
	}
	// Legacy generic data sources removed; folders provide data directly
	a.leftPanel.SetViewConfig(a.viewConfig)
	a.rightPanel.SetViewConfig(a.viewConfig)
	// Provide contexts count to panels for root display
	a.leftPanel.SetContextCountProvider(func() int { return len(a.kubeMgr.GetContexts()) })
	a.rightPanel.SetContextCountProvider(func() int { return len(a.kubeMgr.GetContexts()) })
	// Initialize per-panel view options from config defaults
	if a.cfg != nil {
		a.leftPanel.SetResourceViewOptions(a.cfg.Resources.ShowNonEmptyOnly, string(a.cfg.Resources.Order))
		a.rightPanel.SetResourceViewOptions(a.cfg.Resources.ShowNonEmptyOnly, string(a.cfg.Resources.Order))
		// Initialize table mode from config defaults
		ctxLeft, cancelLeft := context.WithTimeout(a.ctx, panelContextTimeout)
		a.leftPanel.SetTableMode(ctxLeft, string(a.cfg.Panel.Table.Mode))
		a.leftPanel.SetColumnsMode(ctxLeft, a.cfg.Objects.Columns)
		a.leftPanel.SetObjectOrder(ctxLeft, a.cfg.Objects.Order)
		cancelLeft()
		ctxRight, cancelRight := context.WithTimeout(a.ctx, panelContextTimeout)
		a.rightPanel.SetTableMode(ctxRight, string(a.cfg.Panel.Table.Mode))
		a.rightPanel.SetColumnsMode(ctxRight, a.cfg.Objects.Columns)
		a.rightPanel.SetObjectOrder(ctxRight, a.cfg.Objects.Order)
		cancelRight()
		// Initialize columns mode and objects order from config defaults
		a.syncPanelConfig(a.leftPanel)
		a.syncPanelConfig(a.rightPanel)
	}
	// Preview: Use folder-backed rendering starting at root (not contexts listing)
	{
		// Programmatic navigation to current namespace for both panels
		ns := "default"
		if a.currentCtx != nil && a.currentCtx.Namespace != "" {
			ns = a.currentCtx.Namespace
		}
		log.Info("initial navigation", "namespace", ns)
		a.goToNamespace(ns)
	}
	log.Info("panel initialization complete")
	return nil
}

// Legacy builder helpers removed (replaced by self-sufficient folders).

// goToNamespace programmatically navigates to /namespaces/<ns> and updates panels.
// If ns is empty, uses "default". If the namespace does not exist, navigates to root.
func (a *App) goToNamespace(ns string) {
	if ns == "" {
		ns = "default"
	}
	leftCfg := a.ensurePanelConfig(a.leftPanel)
	rightCfg := a.ensurePanelConfig(a.rightPanel)
	a.syncPanelConfig(a.leftPanel)
	a.syncPanelConfig(a.rightPanel)
	currentName := ""
	if a.currentCtx != nil {
		currentName = a.currentCtx.Name
	}
	depsLeft := a.makeDeps(a.cl, leftCfg, currentName)
	depsRight := a.makeDeps(a.cl, rightCfg, currentName)
	enterLeft := a.makeEnterContextFunc(leftCfg)
	enterRight := a.makeEnterContextFunc(rightCfg)
	rootLeft := models.NewRootFolder(depsLeft, enterLeft)
	rootRight := models.NewRootFolder(depsRight, enterRight)
	a.leftNav = navui.NewNavigator(rootLeft)
	a.rightNav = navui.NewNavigator(rootRight)
	if a.namespaceExists(ns) {
		// Left panel: remember selection when entering
		a.leftNav.SetSelectionID("namespaces")
		leftNSPath := append(append([]string{}, rootLeft.Path()...), "namespaces")
		leftNS := models.NewClusterObjectsFolder(depsLeft, schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}, leftNSPath)
		a.enqueueCmd(a.withBusy("Namespaces", 800*time.Millisecond, func() tea.Msg {
			ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
			defer cancel()
			_ = leftNS.Len(ctx)
			return nil
		}))
		a.leftNav.Push(leftNS)
		a.leftNav.SetSelectionID(ns)
		leftGroupsPath := append(append([]string{}, leftNSPath...), ns)
		leftGroups := models.NewNamespacedResourcesFolder(depsLeft, ns, leftGroupsPath)
		a.enqueueCmd(a.withBusy("Resources", 800*time.Millisecond, func() tea.Msg {
			ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
			defer cancel()
			_ = leftGroups.Len(ctx)
			return nil
		}))
		a.leftNav.Push(leftGroups)
		// Right panel: same
		a.rightNav.SetSelectionID("namespaces")
		rightNSPath := append(append([]string{}, rootRight.Path()...), "namespaces")
		rightNS := models.NewClusterObjectsFolder(depsRight, schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}, rightNSPath)
		a.enqueueCmd(a.withBusy("Namespaces", 800*time.Millisecond, func() tea.Msg {
			ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
			defer cancel()
			_ = rightNS.Len(ctx)
			return nil
		}))
		a.rightNav.Push(rightNS)
		a.rightNav.SetSelectionID(ns)
		rightGroupsPath := append(append([]string{}, rightNSPath...), ns)
		rightGroups := models.NewNamespacedResourcesFolder(depsRight, ns, rightGroupsPath)
		a.enqueueCmd(a.withBusy("Resources", 800*time.Millisecond, func() tea.Msg {
			ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
			defer cancel()
			_ = rightGroups.Len(ctx)
			return nil
		}))
		a.rightNav.Push(rightGroups)
	}
	curL := a.leftNav.Current()
	hasBackL := a.leftNav.HasBack()
	curR := a.rightNav.Current()
	hasBackR := a.rightNav.HasBack()
	ctxLeft, cancelLeft := context.WithTimeout(a.ctx, panelContextTimeout)
	a.leftPanel.SetFolder(ctxLeft, curL, hasBackL)
	cancelLeft()
	ctxRight, cancelRight := context.WithTimeout(a.ctx, panelContextTimeout)
	a.rightPanel.SetFolder(ctxRight, curR, hasBackR)
	cancelRight()
	// Use navigator paths for breadcrumbs
	a.leftPanel.SetCurrentPath(a.navigatorPath(a.leftNav))
	a.rightPanel.SetCurrentPath(a.navigatorPath(a.rightNav))
	a.leftPanel.UseFolder(true)
	a.rightPanel.UseFolder(true)
	a.leftPanel.SetFolderNavHandler(func(back bool, selID string, next models.Folder) {
		a.activePanel = 0
		a.handleFolderNav(back, selID, next)
	})
	a.rightPanel.SetFolderNavHandler(func(back bool, selID string, next models.Folder) {
		a.activePanel = 1
		a.handleFolderNav(back, selID, next)
	})
	ctxResetL, cancelResetL := context.WithTimeout(a.ctx, panelContextTimeout)
	a.leftPanel.ResetSelectionTop(ctxResetL)
	cancelResetL()
	ctxResetR, cancelResetR := context.WithTimeout(a.ctx, panelContextTimeout)
	a.rightPanel.ResetSelectionTop(ctxResetR)
	cancelResetR()
}

// handleFolderNav processes back/forward navigation from panels and updates both panels.
// currentNav returns the navigator for the active panel (left=0, right=1).
func (a *App) currentNav() *navui.Navigator {
	if a.activePanel == 0 {
		return a.leftNav
	}
	return a.rightNav
}

func (a *App) handleFolderNav(back bool, selID string, next models.Folder) {
	currentName := ""
	if a.currentCtx != nil {
		currentName = a.currentCtx.Name
	}
	var nav *navui.Navigator
	var panelSet func(context.Context, models.Folder, bool)
	var panelSelectByID func(context.Context, string)
	var panelReset func(context.Context)
	if a.activePanel == 0 {
		cfg := a.ensurePanelConfig(a.leftPanel)
		a.syncPanelConfig(a.leftPanel)
		if a.leftNav == nil {
			deps := a.makeDeps(a.cl, cfg, currentName)
			enter := a.makeEnterContextFunc(cfg)
			a.leftNav = navui.NewNavigator(models.NewRootFolder(deps, enter))
		}
		nav = a.leftNav
		panelSet = func(ctx context.Context, folder models.Folder, hasBack bool) {
			a.leftPanel.SetFolder(ctx, folder, hasBack)
		}
		panelSelectByID = func(ctx context.Context, id string) { a.leftPanel.SelectByRowID(ctx, id) }
		panelReset = func(ctx context.Context) { a.leftPanel.ResetSelectionTop(ctx) }
	} else {
		cfg := a.ensurePanelConfig(a.rightPanel)
		a.syncPanelConfig(a.rightPanel)
		if a.rightNav == nil {
			deps := a.makeDeps(a.cl, cfg, currentName)
			enter := a.makeEnterContextFunc(cfg)
			a.rightNav = navui.NewNavigator(models.NewRootFolder(deps, enter))
		}
		nav = a.rightNav
		panelSet = func(ctx context.Context, folder models.Folder, hasBack bool) {
			a.rightPanel.SetFolder(ctx, folder, hasBack)
		}
		panelSelectByID = func(ctx context.Context, id string) { a.rightPanel.SelectByRowID(ctx, id) }
		panelReset = func(ctx context.Context) { a.rightPanel.ResetSelectionTop(ctx) }
	}
	if back {
		nav.Back()
	} else if next != nil {
		// Pre-warm the next folder in background to trigger informer/lister start.
		// This shows a spinner if it takes longer than the delay and avoids UI freeze.
		a.enqueueCmd(a.withBusy("Loading", 800*time.Millisecond, func() tea.Msg {
			ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
			defer cancel()
			_ = next.Len(ctx)
			return nil
		}))
		nav.SetSelectionID(selID)
		nav.Push(next)
	}
	cur := nav.Current()
	hasBack := nav.HasBack()
	ctxPanel, cancelPanel := context.WithTimeout(a.ctx, panelContextTimeout)
	panelSet(ctxPanel, cur, hasBack)
	cancelPanel()
	// Update breadcrumbs from navigator state
	if a.activePanel == 0 {
		a.leftPanel.SetCurrentPath(a.navigatorPath(nav))
	} else {
		a.rightPanel.SetCurrentPath(a.navigatorPath(nav))
	}
	if back {
		id := nav.CurrentSelectionID()
		if id != "" {
			ctxSel, cancelSel := context.WithTimeout(a.ctx, panelContextTimeout)
			panelSelectByID(ctxSel, id)
			cancelSel()
		} else {
			ctxReset, cancelReset := context.WithTimeout(a.ctx, panelContextTimeout)
			panelReset(ctxReset)
			cancelReset()
		}
	} else {
		ctxReset, cancelReset := context.WithTimeout(a.ctx, panelContextTimeout)
		panelReset(ctxReset)
		cancelReset()
	}
}

// namespaceExists returns true if the namespace exists in the current cluster.
func (a *App) namespaceExists(ns string) bool {
	if ns == "" {
		return false
	}
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
	gvr, err := a.cl.GVKToGVR(gvk)
	if err != nil {
		return false
	}
	lst, err := a.cl.ListByGVR(a.ctx, gvr, "")
	if err != nil {
		return false
	}
	for i := range lst.Items {
		if lst.Items[i].GetName() == ns {
			return true
		}
	}
	return false
}

//

// selectCurrentContext prefers $KUBECONFIG current-context, else any current-context, else first discovered.
func (a *App) selectCurrentContext() *kubeconfig.Context {
	if env := os.Getenv("KUBECONFIG"); env != "" {
		for _, p := range strings.Split(env, string(os.PathListSeparator)) {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			for _, kc := range a.kubeMgr.GetKubeconfigs() {
				if sameFilepath(kc.Path, p) {
					if ctx := a.kubeMgr.GetCurrentContext(kc); ctx != nil {
						return ctx
					}
				}
			}
		}
	}
	for _, kc := range a.kubeMgr.GetKubeconfigs() {
		if ctx := a.kubeMgr.GetCurrentContext(kc); ctx != nil {
			return ctx
		}
	}
	cs := a.kubeMgr.GetContexts()
	if len(cs) > 0 {
		return cs[0]
	}
	return nil
}

func sameFilepath(a, b string) bool {
	ap, err1 := filepath.Abs(a)
	bp, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	return ap == bp
}

func (a *App) GetObject(gvk schema.GroupVersionKind, namespace, name string) (map[string]interface{}, error) {
	gvr, err := a.cl.GVKToGVR(gvk)
	if err != nil {
		return nil, err
	}
	obj, err := a.cl.GetByGVR(a.ctx, gvr, namespace, name)
	if err != nil {
		return nil, err
	}
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	return obj.Object, nil
}

// RESTMapper exposes the app's RESTMapper to viewers for resource→GVK resolution.
func (a *App) RESTMapper() metamapper.RESTMapper { return a.cl.RESTMapper() }
