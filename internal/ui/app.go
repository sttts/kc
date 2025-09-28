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
	"github.com/sttts/kc/pkg/appconfig"
	"github.com/sttts/kc/pkg/kubeconfig"
	"github.com/sttts/kc/pkg/navigation"
	"github.com/sttts/kc/pkg/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	yaml "sigs.k8s.io/yaml"
)

// EscTimeoutMsg is sent when the escape sequence times out
type EscTimeoutMsg struct{}

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
	kubeMgr        *kubeconfig.Manager
	resMgr         *resources.Manager
	navMgr         *navigation.Manager
	storePool      *resources.ClusterPool
	storeProvider  resources.StoreProvider
	currentCtx     *kubeconfig.Context
	viewConfig     *ViewConfig
	resCatalog     map[string]schema.GroupVersionKind
	genericFactory func(schema.GroupVersionKind) *GenericDataSource
	cfg            *appconfig.Config
	// Theme dialog state
	prevTheme           string
	suppressThemeRevert bool
}

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
		resCatalog:   make(map[string]schema.GroupVersionKind),
	}

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
	)
}

// Update handles messages and updates the application state
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Always adapt size
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		msg.Width = max(40, msg.Width)
		msg.Height = max(5, msg.Height)

		a.width = msg.Width
		a.height = msg.Height

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
		model, cmd := a.modalManager.Update(msg)
		a.modalManager = model.(*ModalManager)
		cmds = append(cmds, cmd)
		// While a modal is open, still forward non-key messages to the terminal
		// (process output, window size, timers) so the 2-line terminal stays fresh.
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
	case EscTimeoutMsg:
		// Escape sequence timed out
		a.escPressed = false
		return a, nil

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
			// Intercept F3/F4 to open viewers/editors
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
		"left", "right", // Always go to terminal
		"space", // Never go to panels
	}

	for _, termKey := range terminalKeys {
		if key == termKey {
			return false
		}
	}

	// Always route these keys to panels
	// Always route specific keys to panels regardless of terminal state
	panelKeys := []string{
		// Navigation keys
		"up", "down", "left", "right", // Navigate items
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
	// Reserve space for: terminal (2) + function keys (1) = 3 lines
	panelHeight := a.height - 3
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

	a.leftPanel.SetDimensions(contentWidth, contentHeight-1)  // -1 for footer space
	a.rightPanel.SetDimensions(contentWidth, contentHeight-1) // -1 for footer space
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

	combinedView := lipgloss.JoinVertical(
		lipgloss.Left,
		panels,
		terminalView,
		functionKeys,
	)

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

// renderTerminalArea renders the 2-line terminal area in main view
func (a *App) renderTerminalArea() (string, *tea.Cursor) {
	terminalView, terminalCursor := a.terminal.View()
	return terminalView, terminalCursor
}

// renderTerminalView renders the full-screen terminal view
func (a *App) renderTerminalView() (string, *tea.Cursor) {
	// Get terminal view
	terminalView, terminalCursor := a.terminal.View()

	// Add toggle message
	toggleMsg := a.renderToggleMessage()

	// Combine terminal and toggle message
	combinedView := lipgloss.JoinVertical(
		lipgloss.Left,
		terminalView,
		toggleMsg,
	)

	// Adjust cursor position for the combined view
	if terminalCursor != nil {
		// Cursor position doesn't need adjustment since toggle message is below terminal
		return combinedView, terminalCursor
	}

	return combinedView, nil
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
			renderKey("F2", "Resources", true),
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

	// Add spaces between keys
	renderedKeys := make([]string, 0, len(keys)*2-1)
	for _, key := range keys {
		renderedKeys = append(renderedKeys, key)
	}

	// Join all elements (keys + spaces)
	joined := lipgloss.JoinHorizontal(lipgloss.Left, renderedKeys...)

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
	// Resource selector modal
	resourceSelector := NewResourceSelector(a.allResources)
	resourceModal := NewModal("Resource Selection", resourceSelector)
	resourceModal.SetOnClose(func() tea.Cmd {
		// TODO: Apply selected resources to panels
		return nil
	})
	a.modalManager.Register("resource_selector", resourceModal)

	// Theme selector modal; content is set dynamically when opened
	themeSelector := NewThemeSelector(nil)
	themeModal := NewModal("YAML Theme", themeSelector)
	a.modalManager.Register("theme_selector", themeModal)
}

// Message handlers for function keys
func (a *App) showResourceSelector() tea.Cmd {
	a.modalManager.Show("resource_selector")
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
			var err error
			gvk, err = a.resMgr.ResourceToGVK(res)
			if err != nil {
				return nil
			}
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
		viewer := NewYAMLViewer(titleName, body, theme, func() tea.Cmd { return a.editSelection() }, nil, func() tea.Cmd { a.modalManager.Hide(); return nil })
		viewer.SetOnTheme(func() tea.Cmd { return a.showThemeSelector(viewer) })
		title := path
		if !strings.HasSuffix(path, "/"+titleName) {
			title = path + "/" + titleName
		}
		modal := NewModal(title, viewer)
		modal.SetDimensions(a.width, a.height)
		modal.SetCloseOnSingleEsc(false)
		a.modalManager.Register("yaml_viewer", modal)
		a.modalManager.Show("yaml_viewer")
		return nil
	}
	// Fetch object via GenericDataSource
	ds := a.genericFactory(gvk)
	obj, err := ds.Get(ns, name)
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
	viewer := NewYAMLViewer(titleName, body, theme, func() tea.Cmd { return a.editSelection() }, nil, func() tea.Cmd {
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
func (a *App) showThemeSelector(v *YAMLViewer) tea.Cmd {
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
		if app.storePool != nil {
			app.storePool.Stop()
		}
		if app.resMgr != nil {
			app.resMgr.Stop()
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
		return fmt.Errorf("discover kubeconfigs: %w", err)
	}
	// Select current context (prefer env KUBECONFIG first path)
	a.currentCtx = a.selectCurrentContext()
	if a.currentCtx == nil {
		return fmt.Errorf("no current context found")
	}
	// Build resources manager and start cluster/cache
	cfg, err := a.kubeMgr.CreateClientConfig(a.currentCtx)
	if err != nil {
		return fmt.Errorf("client config: %w", err)
	}
	a.resMgr, err = resources.NewManager(cfg)
	if err != nil {
		return fmt.Errorf("resources manager: %w", err)
	}
	if err := a.resMgr.Start(); err != nil {
		// Non-fatal; continue without cache warm
		fmt.Printf("Warning: start resources manager: %v\n", err)
	}
	// Store provider and pool (2m TTL)
	a.storeProvider, a.storePool = resources.NewStoreProviderForContext(a.currentCtx, 2*time.Minute)
	// Navigation manager and store wiring
	a.navMgr = navigation.NewManager(a.kubeMgr, a.resMgr)
	a.navMgr.SetStoreProvider(a.storeProvider)
	// Build hierarchy and load context resources
	if err := a.navMgr.BuildHierarchy(); err != nil {
		return fmt.Errorf("build hierarchy: %w", err)
	}
	if err := a.navMgr.LoadContextResources(a.currentCtx); err != nil {
		// Non-fatal; still allow UI
		fmt.Printf("Warning: load context resources: %v\n", err)
	}
	// Wire panel data sources
	tableFn := func(ctx context.Context, gvr schema.GroupVersionResource, ns string) (*metav1.Table, error) {
		return a.resMgr.ListTableByGVR(ctx, gvr, ns)
	}
	nsDS := NewNamespacesDataSource(a.storeProvider, a.resMgr.GVKToGVR, tableFn)
	a.leftPanel.SetNamespacesDataSource(nsDS)
	a.rightPanel.SetNamespacesDataSource(nsDS)
	// Discovery-backed catalog
	if infos, err := a.resMgr.GetResourceInfos(); err == nil {
		a.leftPanel.SetResourceCatalog(infos)
		a.rightPanel.SetResourceCatalog(infos)
		// populate app-level resource catalog for lookups
		a.resCatalog = make(map[string]schema.GroupVersionKind)
		for _, info := range infos {
			if info.Namespaced {
				a.resCatalog[info.Resource] = info.GVK
			}
		}
	} else {
		fmt.Printf("Warning: discovery resources: %v\n", err)
	}
	// Generic data source factory (per-GVK)
	factory := func(gvk schema.GroupVersionKind) *GenericDataSource {
		return NewGenericDataSource(a.storeProvider, a.resMgr.GVKToGVR, tableFn, gvk)
	}
	a.leftPanel.SetGenericDataSourceFactory(factory)
	a.rightPanel.SetGenericDataSourceFactory(factory)
	a.genericFactory = factory
	a.leftPanel.SetViewConfig(a.viewConfig)
	a.rightPanel.SetViewConfig(a.viewConfig)
	// Provide contexts count to panels for root display
	a.leftPanel.SetContextCountProvider(func() int { return len(a.kubeMgr.GetContexts()) })
	a.rightPanel.SetContextCountProvider(func() int { return len(a.kubeMgr.GetContexts()) })
	return nil
}

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

// Implement view.Context to support modular viewers.
func (a *App) ResourceToGVK(resource string) (schema.GroupVersionKind, error) {
	return a.resMgr.ResourceToGVK(resource)
}

func (a *App) GetObject(gvk schema.GroupVersionKind, namespace, name string) (map[string]interface{}, error) {
	ds := a.genericFactory(gvk)
	if ds == nil {
		return nil, fmt.Errorf("no datasource for %s", gvk.String())
	}
	obj, err := ds.Get(namespace, name)
	if err != nil {
		return nil, err
	}
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	return obj.Object, nil
}

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
