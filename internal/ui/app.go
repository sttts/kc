package ui

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
    kccluster "github.com/sttts/kc/internal/cluster"
    navui "github.com/sttts/kc/internal/navigation"
    "github.com/sttts/kc/internal/overlay"
    _ "github.com/sttts/kc/internal/ui/view"
    "github.com/sttts/kc/pkg/appconfig"
	"github.com/sttts/kc/pkg/kubeconfig"
	metamapper "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	yaml "sigs.k8s.io/yaml"
)

// EscTimeoutMsg is sent when the escape sequence times out
type EscTimeoutMsg struct{}

// FolderTickMsg triggers periodic folder refresh (debounced to ~1s).
type FolderTickMsg struct{}

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
    lastClickTime time.Time
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
    toastLogger *ToastLogger
    pendingCmds []tea.Cmd
}

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
        cfg:          appconfig.Default(),
    }
    app.toastLogger = NewToastLogger(app, 2*time.Second)

	// Register modals
	app.setupModals()

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
    if cmd == nil { return }
    a.pendingCmds = append(a.pendingCmds, cmd)
}

// viewOptions returns a snapshot of current resource view options for folders.
func (a *App) favSet() map[string]bool {
    fav := map[string]bool{}
    if a.cfg != nil {
        for _, r := range a.cfg.Resources.Favorites { if r != "" { fav[strings.ToLower(r)] = true } }
    }
    return fav
}
func (a *App) leftViewOptions() navui.ViewOptions {
    show, order := a.leftPanel.ResourceViewOptions()
    return navui.ViewOptions{ShowNonEmptyOnly: show, Order: order, Favorites: a.favSet(), Columns: a.leftPanel.ColumnsMode(), ObjectsOrder: a.leftPanel.ObjectOrder()}
}
func (a *App) rightViewOptions() navui.ViewOptions {
    show, order := a.rightPanel.ResourceViewOptions()
    return navui.ViewOptions{ShowNonEmptyOnly: show, Order: order, Favorites: a.favSet(), Columns: a.rightPanel.ColumnsMode(), ObjectsOrder: a.rightPanel.ObjectOrder()}
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
				if winW < 40 { winW = 40 }
				if winW > a.width-2 { winW = a.width - 2 }
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
                if a.cfg == nil { a.cfg = appconfig.Default() }
                a.cfg.Resources.ShowNonEmptyOnly = m.ShowNonEmptyOnly
                a.cfg.Resources.Order = appconfig.ResourcesViewOrder(m.Order)
                _ = appconfig.Save(a.cfg)
            }
            if m.Accept {
                // Apply to active panel only; do not persist
                if a.activePanel == 0 {
                    a.leftPanel.SetResourceViewOptions(m.ShowNonEmptyOnly, m.Order)
                } else {
                    a.rightPanel.SetResourceViewOptions(m.ShowNonEmptyOnly, m.Order)
                }
                // Refresh only the active panel's folder
                if a.activePanel == 0 && a.leftNav != nil {
                    if rf, ok := a.leftNav.Current().(interface{ Refresh() }); ok { rf.Refresh() }
                    a.leftPanel.SetFolder(a.leftNav.Current(), a.leftNav.HasBack())
                    a.leftPanel.SetCurrentPath(a.leftNav.Path())
                    a.leftPanel.RefreshFolder()
                }
                if a.activePanel == 1 && a.rightNav != nil {
                    if rf, ok := a.rightNav.Current().(interface{ Refresh() }); ok { rf.Refresh() }
                    a.rightPanel.SetFolder(a.rightNav.Current(), a.rightNav.HasBack())
                    a.rightPanel.SetCurrentPath(a.rightNav.Path())
                    a.rightPanel.RefreshFolder()
                }
            }
            if m.Close { a.modalManager.Hide() }
            return a, nil
        case ObjectOptionsChangedMsg:
            if m.SaveDefault {
                if a.cfg == nil { a.cfg = appconfig.Default() }
                // Save table mode
                switch strings.ToLower(m.TableMode) { case "fit": a.cfg.Panel.Table.Mode = appconfig.TableModeFit; default: a.cfg.Panel.Table.Mode = appconfig.TableModeScroll }
                // Save columns mode to objects.columns
                if strings.EqualFold(m.Columns, "wide") { a.cfg.Objects.Columns = "wide" } else { a.cfg.Objects.Columns = "normal" }
                // Save objects order
                a.cfg.Objects.Order = m.ObjectsOrder
                _ = appconfig.Save(a.cfg)
            }
            if a.activePanel == 0 {
                a.leftPanel.SetTableMode(m.TableMode)
                a.leftPanel.SetColumnsMode(m.Columns)
                a.leftPanel.SetObjectOrder(m.ObjectsOrder)
                if a.leftNav != nil { if rf, ok := a.leftNav.Current().(interface{ Refresh() }); ok { rf.Refresh() }; a.leftPanel.RefreshFolder() }
            } else {
                a.rightPanel.SetTableMode(m.TableMode)
                a.rightPanel.SetColumnsMode(m.Columns)
                a.rightPanel.SetObjectOrder(m.ObjectsOrder)
                if a.rightNav != nil { if rf, ok := a.rightNav.Current().(interface{ Refresh() }); ok { rf.Refresh() }; a.rightPanel.RefreshFolder() }
            }
            if m.Close { a.modalManager.Hide() }
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
        if msg.token == a.busyToken { a.busyActive = false }
        return a, nil
    case busyDoneMsg:
        if msg.token == a.busyToken { a.busyActive = false; a.busyToken++ }
        // Re-dispatch the original message for normal handling
        return a, func() tea.Msg { return msg.msg }
    case showToastMsg:
        a.toastActive = true
        a.toastText = msg.text
        a.toastUntil = time.Now().Add(msg.ttl)
        return a, tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return toastTickMsg{} })
    case toastTickMsg:
        if a.toastActive {
            if time.Now().After(a.toastUntil) { a.toastActive = false } else {
                return a, tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return toastTickMsg{} })
            }
        }
        return a, nil
    case EscTimeoutMsg:
		// Escape sequence timed out
		a.escPressed = false
		return a, nil
    case FolderTickMsg:
        // Refresh only when current folders report dirty to avoid unnecessary redraws.
        if a.leftNav != nil && a.leftPanel != nil {
            if d, ok := a.leftNav.Current().(interface{ IsDirty() bool }); ok && d.IsDirty() {
                a.leftPanel.RefreshFolder()
            }
        }
        if a.rightNav != nil && a.rightPanel != nil {
            if d, ok := a.rightNav.Current().(interface{ IsDirty() bool }); ok && d.IsDirty() {
                a.rightPanel.RefreshFolder()
            }
        }
        // Schedule next tick (lightweight check)
        return a, tea.Tick(time.Second, func(time.Time) tea.Msg { return FolderTickMsg{} })

        case tea.KeyMsg:
		// Handle global shortcuts first
		switch msg.String() {
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
				return a, a.showResourceSelector() // Esc 2 = F2
			case "3":
				a.escPressed = false
				return a, a.viewItem() // Esc 3 = F3
			case "4":
				a.escPressed = false
				return a, a.editItem() // Esc 4 = F4
			case "5":
				a.escPressed = false
				return a, a.copyItem() // Esc 5 = F5
			case "6":
				a.escPressed = false
				return a, a.renameMoveItem() // Esc 6 = F6
			case "7":
				a.escPressed = false
				return a, a.createNamespace() // Esc 7 = F7
			case "8":
				a.escPressed = false
				return a, a.deleteResource() // Esc 8 = F8
			case "9":
				a.escPressed = false
				return a, a.showContextMenu() // Esc 9 = F9
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
            // Intercept F2/F3/F4 for app-level dialogs/viewers/editors
            if msg.String() == "f2" {
                return a, a.showResourceSelector()
            }
            if msg.String() == "ctrl+w" {
                // Toggle columns mode (Normal/Wide) on active panel
                if a.activePanel == 0 {
                    if a.leftPanel.ColumnsMode() == "wide" { a.leftPanel.SetColumnsMode("normal") } else { a.leftPanel.SetColumnsMode("wide") }
                    if a.leftNav != nil { if rf, ok := a.leftNav.Current().(interface{ Refresh() }); ok { rf.Refresh() }; a.leftPanel.RefreshFolder() }
                } else {
                    if a.rightPanel.ColumnsMode() == "wide" { a.rightPanel.SetColumnsMode("normal") } else { a.rightPanel.SetColumnsMode("wide") }
                    if a.rightNav != nil { if rf, ok := a.rightNav.Current().(interface{ Refresh() }); ok { rf.Refresh() }; a.rightPanel.RefreshFolder() }
                }
                return a, nil
            }
            if msg.String() == "f3" {
                return a, a.openYAMLForSelection()
            }
            if msg.String() == "f4" {
                return a, a.editSelection()
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
		// Mouse and other messages
        if mm, ok := msg.(tea.MouseMsg); ok {
            // In fullscreen terminal mode, intercept clicks on the toggle message
            if a.showTerminal {
                // Never forward mouse events that occur on the bottom toggle line
                m := mm.Mouse()
                if m.Y == a.height-1 {
                    if rel, ok := mm.(tea.MouseReleaseMsg); ok && rel.Mouse().Button == tea.MouseLeft {
                        // Toggle back to panels on release
                        a.showTerminal = false
                        a.terminal.SetShowPanels(true)
                    }
                    // Swallow all mouse events on the toggle line
                    return a, nil
                }
                // Forward all other mouse events to the terminal in fullscreen
                model, cmd := a.terminal.Update(mm)
                a.terminal = model.(*Terminal)
                cmds = append(cmds, cmd)
                return a, tea.Batch(cmds...)
            }
            // Panel mode: only act on specific mouse messages; do NOT forward
            // any mouse events to the terminal while panels are visible to
            // avoid escape sequences leaking into the 2‑line terminal view.
            switch e := mm.(type) {
            case tea.MouseWheelMsg:
                m := e.Mouse()
                target := a.leftPanel
                if a.activePanel == 1 { target = a.rightPanel }
                switch m.Button {
                case tea.MouseWheelUp:
                    target.moveUp()
                case tea.MouseWheelDown:
                    target.moveDown()
                }
                return a, tea.Batch(cmds...)
            case tea.MouseClickMsg:
                m := e.Mouse()
                x, y := m.X, m.Y
                panelHeight := a.height - 3
                panelWidth := a.width / 2
                if y < panelHeight {
                    // Click inside panels area
                    if x >= panelWidth { a.activePanel = 1 } else { a.activePanel = 0 }
                    contentY := y - 2 // 1 for frame top, 1 for header
                    if contentY < 0 { contentY = 0 }
                    target := a.leftPanel
                    if a.activePanel == 1 { target = a.rightPanel }
                    // Right-click: open context menu (future wiring)
                    if m.Button == tea.MouseRight {
                        return a, a.showContextMenu()
                    }
                    // Left-click: select row under cursor
                    var clickedID string
                    if target.useFolder && target.folder != nil && target.bt != nil {
                        if id, ok := target.bt.VisibleRowID(contentY); ok {
                            clickedID = id
                            target.SelectByRowID(id)
                        }
                    } else {
                        idx := target.scrollTop + contentY
                        if idx < 0 { idx = 0 }
                        if idx >= len(target.items) { idx = len(target.items) - 1 }
                        if idx >= 0 && len(target.items) > 0 {
                            target.selected = idx
                            target.adjustScroll()
                        }
                    }
                    // Double-click detection
                    if clickedID != "" {
                        now := time.Now()
                        timeout := a.cfg.Input.Mouse.DoubleClickTimeout.Duration
                        if a.lastClickRowID == clickedID && a.lastClickPanel == a.activePanel && now.Sub(a.lastClickTime) <= timeout {
                            a.lastClickTime = time.Time{}
                            a.lastClickRowID = ""
                            if cmd := target.enterItem(); cmd != nil { return a, cmd }
                        } else {
                            a.lastClickTime = now
                            a.lastClickPanel = a.activePanel
                            a.lastClickRowID = clickedID
                        }
                    } else {
                        a.lastClickTime = time.Now()
                        a.lastClickPanel = a.activePanel
                        a.lastClickRowID = ""
                    }
                    return a, tea.Batch(cmds...)
                }
                // Ignore click on function key bar; act on release instead (below)
                if y == a.height-1 { return a, tea.Batch(cmds...) }
                return a, tea.Batch(cmds...)
            case tea.MouseReleaseMsg:
                m := e.Mouse()
                x, y := m.X, m.Y
                // Function key bar (act on release only)
                if y == a.height-1 {
                    if cmd := a.handleFunctionKeyClick(x); cmd != nil { return a, cmd }
                    return a, tea.Batch(cmds...)
                }
                // Swallow click messages not on bars/panels
                return a, tea.Batch(cmds...)
            default:
                // Mouse motion/release and any other mouse-related events are
                // swallowed in panel mode.
                return a, tea.Batch(cmds...)
            }
        }
		// Pass other messages to terminal (e.g., process exit)
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
    // Reserve space for: terminal (2) + function keys (1) + optional toast (1)
    reserved := 3
    if a.toastActive { reserved++ }
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
	a.leftPanel.SetDimensions(contentWidth, contentHeight-2)
	a.rightPanel.SetDimensions(contentWidth, contentHeight-2)
	leftContentView := a.leftPanel.ViewContentOnlyFocused(a.activePanel == 0)
	rightContentView := a.rightPanel.ViewContentOnlyFocused(a.activePanel == 1)

	// Calculate heights for frame and footer
	footerHeight := 2
	frameHeight := panelHeight - footerHeight

	// Create frames with proper dimensions, passing focus state
	leftFramed := a.createFrameWithOverlayTitle(leftContentView, a.leftPanel.GetCurrentPath(), panelWidth, frameHeight, a.activePanel == 0)
	rightFramed := a.createFrameWithOverlayTitle(rightContentView, a.rightPanel.GetCurrentPath(), panelWidth, frameHeight, a.activePanel == 1)

	// Create framed footers with T-junction connection
	leftFooter := a.createFramedFooter(a.leftPanel.GetFooter(), panelWidth)
	rightFooter := a.createFramedFooter(a.rightPanel.GetFooter(), panelWidth)

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
    // Optional toast line above function keys
    toastLine := ""
    if a.toastActive {
        st := lipgloss.NewStyle().Background(lipgloss.Color("196")).Foreground(lipgloss.White).Bold(true)
        // Ensure message fits the width; truncate with … if needed
        msg := a.toastText
        maxw := a.width
        if lipgloss.Width(msg) > maxw {
            if maxw > 1 { msg = sliceANSIColsRaw(msg, 0, maxw-1) + "…" } else { msg = sliceANSIColsRaw(msg, 0, maxw) }
        }
        toastLine = st.Width(a.width).Render(msg)
    }
    // spinner is added inside renderFunctionKeys at the far left; nothing to do here

    if a.toastActive {
        combinedView := lipgloss.JoinVertical(
            lipgloss.Left,
            panels,
            terminalView,
            toastLine,
            functionKeys,
        )
        // Adjust cursor position
        if terminalCursor != nil {
            offsetY := panelHeight
            adjustedCursor := tea.NewCursor(terminalCursor.X, terminalCursor.Y+offsetY)
            adjustedCursor.Blink = terminalCursor.Blink
            adjustedCursor.Color = terminalCursor.Color
            adjustedCursor.Shape = terminalCursor.Shape
            return combinedView, adjustedCursor
        }
        return combinedView, nil
    }
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
    if maxTerm < 1 { maxTerm = 1 }
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
        if cy >= maxTerm { cy = maxTerm - 1 }
        if cy < 0 { cy = 0 }
        adjusted := tea.NewCursor(terminalCursor.X, cy)
        adjusted.Blink = terminalCursor.Blink
        adjusted.Color = terminalCursor.Color
        adjusted.Shape = terminalCursor.Shape
        return combinedView, adjusted
    }
    return combinedView, nil
}

// refreshFoldersAfterViewChange reapplies the current folders to panels so that
// folder population re-reads the latest ViewOptions.
func (a *App) refreshFoldersAfterViewChange() {
    if a.leftNav != nil {
        cur := a.leftNav.Current()
        a.leftPanel.SetFolder(cur, a.leftNav.HasBack())
        a.leftPanel.SetCurrentPath(a.leftNav.Path())
        a.leftPanel.RefreshFolder()
    }
    if a.rightNav != nil {
        cur := a.rightNav.Current()
        a.rightPanel.SetFolder(cur, a.rightNav.HasBack())
        a.rightPanel.SetCurrentPath(a.rightNav.Path())
        a.rightPanel.RefreshFolder()
    }
}

// renderFunctionKeys renders the function key bar
func (a *App) renderFunctionKeys() string {
	var keys []string

	if a.showTerminal {
		keys = []string{FunctionKeyStyle.Render("Ctrl+O") + FunctionKeyDescriptionStyle.Render("Return to panels")}
	} else {
		// Determine capabilities from active panel
		p := a.leftPanel
		if a.activePanel == 1 {
			p = a.rightPanel
		}
		path := p.GetCurrentPath()
		cur := p.GetCurrentItem()
		// Defaults
		canView, canEdit, canCreateNS, canDelete := false, false, false, false
		// Location-based rules
		if path == "/namespaces" {
			canCreateNS = true
			// viewing a namespace YAML is allowed when an item selected
			if cur != nil && cur.Type == ItemTypeNamespace {
				canView, canEdit, canDelete = true, true, true
			}
		} else if strings.HasPrefix(path, "/namespaces/") {
			parts := strings.Split(path, "/")
			if len(parts) == 3 { // /namespaces/<ns>
				// resource folders only; F3/F4/F8 disabled
			} else if len(parts) == 4 { // /namespaces/<ns>/<resource>
				// viewing/editing/deleting objects is possible when an object row is selected (not ".." and not a folder)
				if cur != nil && cur.Name != ".." && !cur.Enterable {
					canView, canEdit, canDelete = true, true, true
				}
			} else if len(parts) >= 5 { // object or deeper
				if cur != nil && cur.Name != ".." {
					canView = true
					canEdit = true
					canDelete = true
				}
			}
		}

		// Helper to render enabled/disabled key
		renderKey := func(key, label string, enabled bool) string {
			desc := FunctionKeyDescriptionStyle
			if !enabled {
				desc = FunctionKeyDisabledStyle
			}
			return FunctionKeyStyle.Render(key) + desc.Render(label)
		}

		keys = []string{
			renderKey("F1", "Help", true),
                // Always label F2 as Options; it opens the appropriate dialog
                // (Object Options or Resources Options) based on context.
                renderKey("F2", "Options", true),
			renderKey("F3", "View", canView),
			renderKey("F4", "Edit", canEdit),
			renderKey("F5", "Copy", false),
			renderKey("F6", "Rename/Move", false),
			renderKey("F7", "Namespace", canCreateNS),
			renderKey("F8", "Delete", canDelete),
			renderKey("F9", "Menu", true),
			FunctionKeyStyle.Render("F10") + FunctionKeyDescriptionStyle.Render("Quit"),
			FunctionKeyStyle.Render("Ctrl+O") + FunctionKeyDescriptionStyle.Render("Fullscreen"),
		}
	}

    // Join keys
    joined := lipgloss.JoinHorizontal(lipgloss.Left, keys...)

	// Always add "Kubernetes Commander" right-aligned
	title := " Kubernetes Commander "

	// Create a full-width container with left-aligned keys and right-aligned title
	fullWidthStyle := FunctionKeyBarStyle.
		Width(a.width).
		Align(lipgloss.Left)

	// Create a full-width container with left-aligned keys and right-aligned title
	titleStyle := FunctionKeyTitleStyle.
		Align(lipgloss.Center).
		Width(a.width - lipgloss.Width(joined) - 1)

	// Calculate the exact spacing needed to push title to the right edge
	titleRendered := titleStyle.Render(title)

    return fullWidthStyle.Render(joined + " " + titleRendered)
}

// handleFunctionKeyClick maps an x coordinate on the function key bar to a key action.
func (a *App) handleFunctionKeyClick(x int) tea.Cmd {
    // Recompute the keys exactly like renderFunctionKeys does and record spans.
    // Build the list but capture text lengths to map x.
    var keys []struct{ label string; enabled bool; action func() tea.Cmd }
    if a.showTerminal {
        keys = []struct{ label string; enabled bool; action func() tea.Cmd }{
            {label: FunctionKeyStyle.Render("Ctrl+O") + FunctionKeyDescriptionStyle.Render("Return to panels"), enabled: true, action: func() tea.Cmd {
                a.showTerminal = false
                a.terminal.SetShowPanels(true)
                return nil
            }},
        }
    } else {
        p := a.leftPanel
        if a.activePanel == 1 { p = a.rightPanel }
        path := p.GetCurrentPath()
        cur := p.GetCurrentItem()
        canView, canEdit, canCreateNS, canDelete := false, false, false, false
        if path == "/namespaces" {
            canCreateNS = true
            if cur != nil && cur.Type == ItemTypeNamespace { canView, canEdit, canDelete = true, true, true }
        } else if strings.HasPrefix(path, "/namespaces/") {
            parts := strings.Split(path, "/")
            if len(parts) == 4 { /* list */ } else if len(parts) == 5 {
                if cur != nil && cur.Name != ".." && !cur.Enterable { canView, canEdit, canDelete = true, true, true }
            } else if len(parts) >= 5 { if cur != nil && cur.Name != ".." { canView, canEdit, canDelete = true, true, true } }
        }
        makeLbl := func(key, label string, enabled bool) string {
            desc := FunctionKeyDescriptionStyle
            if !enabled { desc = FunctionKeyDisabledStyle }
            return FunctionKeyStyle.Render(key) + desc.Render(label)
        }
        keys = []struct{ label string; enabled bool; action func() tea.Cmd }{
            {makeLbl("F1", "Help", true), true, a.showHelp},
            {makeLbl("F2", "Options", true), true, a.showResourceSelector},
            {makeLbl("F3", "View", canView), canView, a.openYAMLForSelection},
            {makeLbl("F4", "Edit", canEdit), canEdit, a.editSelection},
            {makeLbl("F5", "Copy", false), false, a.copyItem},
            {makeLbl("F6", "Rename/Move", false), false, a.renameMoveItem},
            {makeLbl("F7", "Namespace", canCreateNS), canCreateNS, a.createNamespace},
            {makeLbl("F8", "Delete", canDelete), canDelete, a.deleteResource},
            {FunctionKeyStyle.Render("F9") + FunctionKeyDescriptionStyle.Render("Menu"), true, a.showContextMenu},
            {FunctionKeyStyle.Render("F10") + FunctionKeyDescriptionStyle.Render("Quit"), true, func() tea.Cmd { return tea.Quit }},
            {FunctionKeyStyle.Render("Ctrl+O") + FunctionKeyDescriptionStyle.Render("Fullscreen"), true, func() tea.Cmd {
                a.showTerminal = true
                a.terminal.SetShowPanels(false)
                a.terminal.Focus()
                // Suppress trailing mouse events from this click to avoid
                // sending them to the PTY immediately after toggling.
                a.suppressMouseUntil = time.Now().Add(150 * time.Millisecond)
                return nil
            }},
        }
    }
    // Map x to index by accumulating rendered widths
    acc := 0
    for _, k := range keys {
        w := lipgloss.Width(k.label)
        if x >= acc && x < acc+w {
            if k.enabled && k.action != nil { return k.action() }
            return nil
        }
        acc += w
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
    opts := NewResourcesOptionsModel(false, "favorites")
    resModal := NewModal("Resources", opts)
    a.modalManager.Register("resources_options", resModal)

	// Theme selector modal; content is set dynamically when opened
	themeSelector := NewThemeSelector(nil)
	themeModal := NewModal("YAML Theme", themeSelector)
	a.modalManager.Register("theme_selector", themeModal)
}

// Message handlers for function keys
func (a *App) showResourceSelector() tea.Cmd {
    // Depending on current view, open resources or objects options.
    curPanel := a.leftPanel; if a.activePanel == 1 { curPanel = a.rightPanel }
    // Heuristic: if folder is an object-list, open object options; else resources.
    isObjects := false
    switch curPanel.folder.(type) {
    case *navui.NamespacedObjectsFolder, *navui.ClusterObjectsFolder:
        isObjects = true
    }
    if isObjects {
        mode := curPanel.TableMode()
        cols := curPanel.ColumnsMode()
        order := curPanel.ObjectOrder()
        o := NewObjectOptionsModel(mode, cols, order)
        // Prepare modal
        modal := a.modalManager.modals["object_options"]
        if modal == nil {
            modal = NewModal("Options", o)
            a.modalManager.Register("object_options", modal)
        } else {
            modal.SetContent(o)
        }
        winW, winH := 50, 6
        bg, _ := a.renderMainView()
        modal.SetWindowed(winW, winH, bg)
        modal.SetOnClose(func() tea.Cmd { return nil })
        modal.SetDimensions(a.width, a.height)
        a.modalManager.Show("object_options")
        return nil
    }
    // Resources options
    showNonEmpty, order := curPanel.ResourceViewOptions()
    content := NewResourcesOptionsModel(showNonEmpty, order)
    modal := a.modalManager.modals["resources_options"]
    if modal == nil { modal = NewModal("Options", content); a.modalManager.Register("resources_options", modal) } else { modal.SetContent(content); modal.title = "Options" }
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
	// TODO: Implement namespace creation
	return nil
}

func (a *App) deleteResource() tea.Cmd {
	// TODO: Implement resource deletion
	return nil
}

func (a *App) showContextMenu() tea.Cmd {
	// TODO: Implement context menu
	return nil
}

// Function key action methods
func (a *App) showHelp() tea.Cmd {
	// TODO: Implement help dialog
	return nil
}

func (a *App) viewItem() tea.Cmd {
	return a.openYAMLForSelection()
}

func (a *App) editItem() tea.Cmd {
	return a.editSelection()
}

// openYAMLForSelection fetches the selected object and opens a YAML viewer modal.
func (a *App) openYAMLForSelection() tea.Cmd {
	// Determine active panel and current selection
	p := a.leftPanel
	if a.activePanel == 1 {
		p = a.rightPanel
	}
    // Folder-backed path: object YAML or key value depending on folder type
    if p.useFolder && p.folder != nil {
        // Use the selected index directly; folder rows include the back row at index 0
        if p.selected < 0 || p.selected >= p.folder.Len() { return nil }
        rows := p.folder.Lines(0, p.folder.Len())
        id, _, _, ok := rows[p.selected].Columns()
        if ok && id == "__back__" { return nil }
        if !ok {
            return nil
        }
        // Key folders (configmaps/secrets data): show key value, not whole object
        if kf, ok := p.folder.(navui.KeyFolder); ok {
            gvr, ns, name := kf.Parent()
            obj, err := a.cl.GetByGVR(a.ctx, gvr, ns, name)
            if err != nil || obj == nil { return nil }
            // Extract data value
            var body string
            if data, found, _ := unstructured.NestedMap(obj.Object, "data"); found {
                if val, ok2 := data[id]; ok2 {
                    switch v := val.(type) {
                    case string:
                        if gvr.Resource == "secrets" {
                            if b, err := base64.StdEncoding.DecodeString(v); err == nil {
                                if isProbablyText(b) { body = string(b) } else { body = v }
                            } else { body = v }
                        } else {
                            body = v
                        }
                    default:
                        yb, _ := yaml.Marshal(v)
                        body = string(yb)
                    }
                }
            }
            if body == "" { body = "" }
            theme := "dracula"
            if a.cfg != nil && a.cfg.Viewer.Theme != "" { theme = a.cfg.Viewer.Theme }
            // Title from item.Path() when available
            title := ""
            type pathAware interface{ Path() []string }
            if pa, ok := rows[p.selected].(pathAware); ok {
                segs := pa.Path(); if len(segs) > 0 { title = "/" + strings.Join(segs, "/") }
            }
            if title == "" {
                // Fallback: /namespaces/<ns>/<res>/<name>/data/<key>
                title = "/" + gvr.Resource
                if ns != "" { title = "/namespaces/" + ns + "/" + gvr.Resource }
                title = title + "/" + name + "/data/" + id
            }
            viewer := NewTextViewer(id, body, "text", "text/plain", id, theme, func() tea.Cmd { return a.editSelection() }, nil, func() tea.Cmd { a.modalManager.Hide(); return nil })
            viewer.SetOnTheme(func() tea.Cmd { return a.showThemeSelector(viewer) })
            modal := NewModal(title, viewer)
            modal.SetDimensions(a.width, a.height)
            modal.SetCloseOnSingleEsc(false)
            a.modalManager.Register("yaml_viewer", modal)
            a.modalManager.Show("yaml_viewer")
            return nil
        }
        // Extract object-list metadata
        type metaProv interface {
            ObjectListMeta() (schema.GroupVersionResource, string, bool)
        }
        if mp, ok := p.folder.(metaProv); ok {
			gvr, ns, mok := mp.ObjectListMeta()
			if mok {
				obj, err := a.cl.GetByGVR(a.ctx, gvr, ns, id)
				if err != nil || obj == nil {
					return nil
				}
				unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
				yb, _ := yaml.Marshal(obj.Object)
				theme := "dracula"
				if a.cfg != nil && a.cfg.Viewer.Theme != "" {
					theme = a.cfg.Viewer.Theme
				}
            viewer := NewTextViewer(id, string(yb), "yaml", "application/yaml", id, theme, func() tea.Cmd { return a.editSelection() }, nil, func() tea.Cmd { a.modalManager.Hide(); return nil })
            viewer.SetOnTheme(func() tea.Cmd { return a.showThemeSelector(viewer) })
            // Build full breadcrumbs from item.Path() when available, else fallback to meta
            title := ""
            type pathAware interface{ Path() []string }
            if pa, ok := rows[p.selected].(pathAware); ok {
                segs := pa.Path();
                if len(segs) > 0 { title = "/" + strings.Join(segs, "/") }
            }
            if title == "" {
                title = "/" + gvr.Resource
                if ns != "" { title = "/namespaces/" + ns + "/" + gvr.Resource }
                title = title + "/" + id
            }
            modal := NewModal(title, viewer)
				modal.SetDimensions(a.width, a.height)
				modal.SetCloseOnSingleEsc(false)
				a.modalManager.Register("yaml_viewer", modal)
				a.modalManager.Show("yaml_viewer")
				return nil
			}
		}
		return nil
	}
	item := p.GetCurrentItem()
	if item == nil || item.Name == ".." {
		return nil
	}
	path := p.GetCurrentPath()
	// Resolve namespace/resource/name
	ns, res, name, ok := parseNamespacedObjectPath(path, item.Name)
	var gvk schema.GroupVersionKind
	if !ok {
		// Special case: viewing a namespace YAML at /namespaces
		if path == "/namespaces" && item.Type == ItemTypeNamespace {
			gvk = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
			name = item.Name
		} else {
			return nil
		}
	} else {
		// Prefer typed GVK carried by item; otherwise resolve via RESTMapper for current cluster
		if item.TypedGVK.Group != "" || item.TypedGVK.Kind != "" || item.TypedGVK.Version != "" {
			gvk = item.TypedGVK
		} else {
			// Resolve via RESTMapper from resource plural
			k, err := a.cl.RESTMapper().KindFor(schema.GroupVersionResource{Resource: res})
			if err != nil {
				return nil
			}
			gvk = k
		}
	}
	// If item provides its own view, delegate to it
	if item != nil && item.Viewer != nil {
		titleName, body, err := item.Viewer.BuildView(a)
		if err != nil {
			return nil
		}
        theme := "dracula"
        if a.cfg != nil && a.cfg.Viewer.Theme != "" {
            theme = a.cfg.Viewer.Theme
        }
        // Let the viewer decide how it prefers to be displayed; fallback to autodetect.
        lang, mime, filename := "", "", titleName
        if hp, ok := item.Viewer.(interface{ DisplayHints() (string, string, string) }); ok {
            lang, mime, filename = hp.DisplayHints()
        }
        viewer := NewTextViewer(titleName, body, lang, mime, filename, theme, func() tea.Cmd { return a.editSelection() }, nil, func() tea.Cmd { a.modalManager.Hide(); return nil })
        viewer.SetOnTheme(func() tea.Cmd { return a.showThemeSelector(viewer) })
        // Prefer breadcrumbs from parsed path when available
        title := path
        if ns != "" && res != "" {
            title = "/namespaces/" + ns + "/" + res + "/" + titleName
        } else if !strings.HasSuffix(path, "/"+titleName) {
            title = path + "/" + titleName
        }
        modal := NewModal(title, viewer)
		modal.SetDimensions(a.width, a.height)
		modal.SetCloseOnSingleEsc(false)
		a.modalManager.Register("yaml_viewer", modal)
		a.modalManager.Show("yaml_viewer")
		return nil
	}
	// Build GVR from current folder meta when available; fallback to typed GVK + resource plural
	var gvr schema.GroupVersionResource
	if path == "/namespaces" && item.Type == ItemTypeNamespace {
		gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
    } else {
        // Prefer folder meta (authoritative) from the active panel's navigator
        nav := a.leftNav
        if a.activePanel == 1 { nav = a.rightNav }
        if nav != nil {
            if cur := nav.Current(); cur != nil {
                type metaProv interface{ ObjectListMeta() (schema.GroupVersionResource, string, bool) }
                if mp, ok := cur.(metaProv); ok {
                    if mgvr, _, ok2 := mp.ObjectListMeta(); ok2 { gvr = mgvr }
                }
            }
        }
		// Fallback: derive from typed GVK + resource plural
		if gvr.Resource == "" {
			gvr = schema.GroupVersionResource{Group: gvk.Group, Version: gvk.Version, Resource: res}
		}
	}
	obj, err := a.cl.GetByGVR(a.ctx, gvr, ns, name)
	if err != nil {
		return nil
	}
	// Strip managedFields
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	// Determine sub-view context (container specs and data keys)
	parts := strings.Split(path, "/")
	content := interface{}(obj.Object)
	// pods: container spec when selecting a container folder
	if len(parts) >= 4 && parts[3] == "pods" {
		if len(parts) == 5 && item != nil && item.Type == ItemTypeDirectory { // container item under a pod
			cname := item.Name
			found := false
			if arr, foundCont, _ := unstructured.NestedSlice(obj.Object, "spec", "containers"); foundCont {
				for _, c := range arr {
					if m, ok := c.(map[string]interface{}); ok {
						if n, _ := m["name"].(string); n == cname {
							content = m
							found = true
							break
						}
					}
				}
			}
			if !found {
				if arr, foundInit, _ := unstructured.NestedSlice(obj.Object, "spec", "initContainers"); foundInit {
					for _, c := range arr {
						if m, ok := c.(map[string]interface{}); ok {
							if n, _ := m["name"].(string); n == cname {
								content = m
								break
							}
						}
					}
				}
			}
		}
	}
	// configmaps | secrets: data value when selecting a key
	if len(parts) >= 4 && (parts[3] == "configmaps" || parts[3] == "secrets") && len(parts) == 5 && item != nil && item.Type == ItemTypeFile {
		key := item.Name
		if data, found, _ := unstructured.NestedMap(obj.Object, "data"); found {
			if val, ok := data[key]; ok {
				content = map[string]interface{}{"key": key, "value": val}
			}
		}
	}
	// Marshal to YAML unless we computed a rawText value (key view below)
	var body string
	// configmaps | secrets: plain value or base64
	if len(parts) >= 4 && (parts[3] == "configmaps" || parts[3] == "secrets") && len(parts) == 5 && item != nil && item.Type == ItemTypeFile {
		key := item.Name
		if data, found, _ := unstructured.NestedMap(obj.Object, "data"); found {
			if val, ok := data[key]; ok {
				switch v := val.(type) {
				case string:
					if parts[3] == "secrets" {
						if b, err := base64.StdEncoding.DecodeString(v); err == nil {
							if isProbablyText(b) {
								body = string(b)
							} else {
								body = v
							}
						} else {
							body = v
						}
					} else {
						body = v
					}
				default:
					yb, _ := yaml.Marshal(v)
					body = string(yb)
				}
			}
		}
	}
	if body == "" {
		yb, _ := yaml.Marshal(content)
		body = string(yb)
	}
	theme := "dracula"
	if a.cfg != nil && a.cfg.Viewer.Theme != "" {
		theme = a.cfg.Viewer.Theme
	}
	titleName := name
	if item != nil && item.Type == ItemTypeDirectory && len(parts) >= 4 && parts[3] == "pods" {
		titleName = name + "/" + item.Name
	}
	if item != nil && item.Type == ItemTypeFile && (len(parts) >= 4 && (parts[3] == "configmaps" || parts[3] == "secrets")) {
		titleName = name + ":" + item.Name
	}
    viewer := NewTextViewer(titleName, body, "yaml", "application/yaml", titleName, theme, func() tea.Cmd { return a.editSelection() }, nil, func() tea.Cmd {
		// Close the topmost modal (the YAML viewer itself)
		a.modalManager.Hide()
		return nil
	})
	viewer.SetOnTheme(func() tea.Cmd { return a.showThemeSelector(viewer) })
	// Title: full breadcrumb of the object
	title := path
	if ok {
		// If path doesn't already contain the object name (list level), append it
		if !strings.HasSuffix(path, "/"+name) {
			title = path + "/" + name
		}
	} else if path == "/namespaces" {
		title = "/namespaces/" + name
	}
	modal := NewModal(title, viewer)
	modal.SetDimensions(a.width, a.height)
	// In the YAML viewer we disable single-Esc close to avoid breaking Esc+digit
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

// editSelection triggers kubectl edit for the selected object (stub wiring; full logic in later step).
func (a *App) editSelection() tea.Cmd {
	// Placeholder: actual kubectl edit integration will be added in the Edit task.
	return nil
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
func Run() error {
	app := NewApp()

	// Initialize data model (best-effort; UI can still run without it)
	if err := app.initData(); err != nil {
		fmt.Printf("Data init warning: %v\n", err)
	}

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
		if app.cancel != nil {
			app.cancel()
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running application: %v\n", err)
		return err
	}

	return nil
}

// initData discovers kubeconfigs, selects current context, starts cluster/cache and wires navigation.
func (a *App) initData() error {
    // Kubeconfig manager and discovery
	a.kubeMgr = kubeconfig.NewManager()
    if err := a.kubeMgr.DiscoverKubeconfigs(); err != nil {
        // Log and show toast
        if a.toastLogger != nil { a.enqueueCmd(a.toastLogger.Errorf("Kubeconfig discovery failed: %v", err)) }
        return fmt.Errorf("discover kubeconfigs: %w", err)
    }
	// Select current context (prefer env KUBECONFIG first path)
	a.currentCtx = a.selectCurrentContext()
	if a.currentCtx == nil {
		return fmt.Errorf("no current context found")
	}
	// Prepare app context and cluster pool; cluster will be started via pool.Get
	a.ctx, a.cancel = context.WithCancel(context.TODO())
	a.clPool = kccluster.NewPool(2 * time.Minute)
	a.clPool.Start()
	k := kccluster.Key{KubeconfigPath: a.currentCtx.Kubeconfig.Path, ContextName: a.currentCtx.Name}
	cl, err := a.clPool.Get(a.ctx, k)
	if err != nil {
		return fmt.Errorf("cluster pool get: %w", err)
	}
	a.cl = cl
    // Discovery-backed catalog (for panel displays)
    if infos, err := a.cl.GetResourceInfos(); err == nil {
        a.leftPanel.SetResourceCatalog(infos)
        a.rightPanel.SetResourceCatalog(infos)
    } else {
        if a.toastLogger != nil { a.enqueueCmd(a.toastLogger.Errorf("Discovery resources failed: %v", err)) }
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
        a.leftPanel.SetTableMode(string(a.cfg.Panel.Table.Mode))
        a.rightPanel.SetTableMode(string(a.cfg.Panel.Table.Mode))
        // Initialize columns mode and objects order from config defaults
        a.leftPanel.SetColumnsMode(a.cfg.Objects.Columns)
        a.rightPanel.SetColumnsMode(a.cfg.Objects.Columns)
        a.leftPanel.SetObjectOrder(a.cfg.Objects.Order)
        a.rightPanel.SetObjectOrder(a.cfg.Objects.Order)
    }
    // Preview: Use folder-backed rendering starting at root (not contexts listing)
	{
		// Programmatic navigation to current namespace for both panels
		ns := "default"
		if a.currentCtx != nil && a.currentCtx.Namespace != "" {
			ns = a.currentCtx.Namespace
		}
		a.goToNamespace(ns)
	}
	return nil
}

// Legacy builder helpers removed (replaced by self-sufficient folders).

// goToNamespace programmatically navigates to /namespaces/<ns> and updates panels.
// If ns is empty, uses "default". If the namespace does not exist, navigates to root.
func (a *App) goToNamespace(ns string) {
	if ns == "" {
		ns = "default"
	}
    // Build separate deps for left and right so each panel can have independent view options
    depsLeft := navui.Deps{
        Cl: a.cl, Ctx: a.ctx, CtxName: a.currentCtx.Name,
        ListContexts: func() []string {
            out := make([]string, 0, len(a.kubeMgr.GetContexts()))
            for _, c := range a.kubeMgr.GetContexts() { out = append(out, c.Name) }
            return out
        },
        ViewOptions: func() navui.ViewOptions { return a.leftViewOptions() },
    }
    depsRight := navui.Deps{
        Cl: a.cl, Ctx: a.ctx, CtxName: a.currentCtx.Name,
        ListContexts: func() []string {
            out := make([]string, 0, len(a.kubeMgr.GetContexts()))
            for _, c := range a.kubeMgr.GetContexts() { out = append(out, c.Name) }
            return out
        },
        ViewOptions: func() navui.ViewOptions { return a.rightViewOptions() },
    }
	// set EnterContext after deps to avoid forward reference
    depsLeft.EnterContext = func(name string, basePath []string) (navui.Folder, error) {
        var target *kubeconfig.Context
        for _, c := range a.kubeMgr.GetContexts() {
            if c.Name == name {
                target = c
                break
            }
        }
        if target == nil {
            return nil, fmt.Errorf("context %q not found", name)
        }
        key := kccluster.Key{KubeconfigPath: target.Kubeconfig.Path, ContextName: target.Name}
        cl, err := a.clPool.Get(a.ctx, key)
        if err != nil {
            return nil, err
        }
        ndeps := navui.Deps{Cl: cl, Ctx: a.ctx, CtxName: target.Name, ListContexts: depsLeft.ListContexts, ViewOptions: func() navui.ViewOptions { return a.leftViewOptions() }}
        return navui.NewContextRootFolder(ndeps, basePath), nil
    }
    depsRight.EnterContext = func(name string, basePath []string) (navui.Folder, error) {
        var target *kubeconfig.Context
        for _, c := range a.kubeMgr.GetContexts() {
            if c.Name == name { target = c; break }
        }
        if target == nil { return nil, fmt.Errorf("context %q not found", name) }
        key := kccluster.Key{KubeconfigPath: target.Kubeconfig.Path, ContextName: target.Name}
        cl, err := a.clPool.Get(a.ctx, key)
        if err != nil { return nil, err }
        ndeps := navui.Deps{Cl: cl, Ctx: a.ctx, CtxName: target.Name, ListContexts: depsRight.ListContexts, ViewOptions: func() navui.ViewOptions { return a.rightViewOptions() }}
        return navui.NewContextRootFolder(ndeps, basePath), nil
    }
    rootLeft := navui.NewRootFolder(depsLeft)
    rootRight := navui.NewRootFolder(depsRight)
    a.leftNav = navui.NewNavigator(rootLeft)
    a.rightNav = navui.NewNavigator(rootRight)
    if a.namespaceExists(ns) {
        // Left panel: remember selection when entering
        a.leftNav.SetSelectionID("namespaces")
        leftNS := navui.NewClusterObjectsFolder(depsLeft, schema.GroupVersionResource{Group:"", Version:"v1", Resource:"namespaces"}, []string{"namespaces"})
        a.enqueueCmd(a.withBusy("Namespaces", 800*time.Millisecond, func() tea.Msg { _ = leftNS.Len(); return nil }))
        a.leftNav.Push(leftNS)
        a.leftNav.SetSelectionID(ns)
        leftGroups := navui.NewNamespacedGroupsFolder(depsLeft, ns, []string{"namespaces", ns})
        a.enqueueCmd(a.withBusy("Resources", 800*time.Millisecond, func() tea.Msg { _ = leftGroups.Len(); return nil }))
        a.leftNav.Push(leftGroups)
        // Right panel: same
        a.rightNav.SetSelectionID("namespaces")
        rightNS := navui.NewClusterObjectsFolder(depsRight, schema.GroupVersionResource{Group:"", Version:"v1", Resource:"namespaces"}, []string{"namespaces"})
        a.enqueueCmd(a.withBusy("Namespaces", 800*time.Millisecond, func() tea.Msg { _ = rightNS.Len(); return nil }))
        a.rightNav.Push(rightNS)
        a.rightNav.SetSelectionID(ns)
        rightGroups := navui.NewNamespacedGroupsFolder(depsRight, ns, []string{"namespaces", ns})
        a.enqueueCmd(a.withBusy("Resources", 800*time.Millisecond, func() tea.Msg { _ = rightGroups.Len(); return nil }))
        a.rightNav.Push(rightGroups)
    }
    curL := a.leftNav.Current(); hasBackL := a.leftNav.HasBack()
    curR := a.rightNav.Current(); hasBackR := a.rightNav.HasBack()
    a.leftPanel.SetFolder(curL, hasBackL)
    a.rightPanel.SetFolder(curR, hasBackR)
    // Use navigator paths for breadcrumbs
    if a.leftNav != nil { a.leftPanel.SetCurrentPath(a.leftNav.Path()) }
    if a.rightNav != nil { a.rightPanel.SetCurrentPath(a.rightNav.Path()) }
	a.leftPanel.UseFolder(true)
	a.rightPanel.UseFolder(true)
    a.leftPanel.SetFolderNavHandler(func(back bool, selID string, next navui.Folder) { a.activePanel = 0; a.handleFolderNav(back, selID, next) })
    a.rightPanel.SetFolderNavHandler(func(back bool, selID string, next navui.Folder) { a.activePanel = 1; a.handleFolderNav(back, selID, next) })
	a.leftPanel.ResetSelectionTop()
	a.rightPanel.ResetSelectionTop()
}

// handleFolderNav processes back/forward navigation from panels and updates both panels.
// currentNav returns the navigator for the active panel (left=0, right=1).
func (a *App) currentNav() *navui.Navigator {
    if a.activePanel == 0 { return a.leftNav }
    return a.rightNav
}

func (a *App) handleFolderNav(back bool, selID string, next navui.Folder) {
    // Use navigator for current active panel
    ensure := func() *navui.Navigator {
        // Build deps bound to the current active panel
        vo := func() navui.ViewOptions {
            if a.activePanel == 0 { return a.leftViewOptions() }
            return a.rightViewOptions()
        }
        deps := navui.Deps{Cl: a.cl, Ctx: a.ctx, CtxName: a.currentCtx.Name,
            ListContexts: func() []string {
                out := make([]string, 0, len(a.kubeMgr.GetContexts()))
                for _, c := range a.kubeMgr.GetContexts() { out = append(out, c.Name) }
                return out
            },
            ViewOptions: vo,
        }
        return navui.NewNavigator(navui.NewRootFolder(deps))
    }
    var nav *navui.Navigator
    var panelSet func(navui.Folder, bool)
    var panelSelectByID func(string)
    var panelReset func()
    if a.activePanel == 0 {
        if a.leftNav == nil { a.leftNav = ensure() }
        nav = a.leftNav
        panelSet = a.leftPanel.SetFolder
        panelSelectByID = a.leftPanel.SelectByRowID
        panelReset = func(){ a.leftPanel.ResetSelectionTop() }
    } else {
        if a.rightNav == nil { a.rightNav = ensure() }
        nav = a.rightNav
        panelSet = a.rightPanel.SetFolder
        panelSelectByID = a.rightPanel.SelectByRowID
        panelReset = func(){ a.rightPanel.ResetSelectionTop() }
    }
    if back {
        nav.Back()
    } else if next != nil {
        // Pre-warm the next folder in background to trigger informer/lister start.
        // This shows a spinner if it takes longer than the delay and avoids UI freeze.
        if a.activePanel == 0 {
            a.enqueueCmd(a.withBusy("Loading", 800*time.Millisecond, func() tea.Msg { _ = next.Len(); return nil }))
        } else {
            a.enqueueCmd(a.withBusy("Loading", 800*time.Millisecond, func() tea.Msg { _ = next.Len(); return nil }))
        }
        nav.SetSelectionID(selID)
        nav.Push(next)
    }
    cur := nav.Current()
    hasBack := nav.HasBack()
    panelSet(cur, hasBack)
    // Update breadcrumbs from navigator state
    if a.activePanel == 0 {
        a.leftPanel.SetCurrentPath(nav.Path())
    } else {
        a.rightPanel.SetCurrentPath(nav.Path())
    }
    if back {
        id := nav.CurrentSelectionID()
        if id != "" { panelSelectByID(id) } else { panelReset() }
    } else {
        panelReset()
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

// isProbablyText returns true if the byte slice looks like readable UTF-8
// with a low proportion of control bytes.
func isProbablyText(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	if !utf8.Valid(b) {
		return false
	}
	ctrl := 0
	for _, r := range string(b) {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < 0x20 {
			ctrl++
		}
	}
	return ctrl*10 < len(b)
}
