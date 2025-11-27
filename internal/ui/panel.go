package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	kccluster "github.com/sttts/kc/internal/cluster"
	models "github.com/sttts/kc/internal/models"
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
	listwidget "github.com/sttts/kc/internal/ui/panelcontent/list"
	manifestwidget "github.com/sttts/kc/internal/ui/panelcontent/manifest"
	uistyles "github.com/sttts/kc/internal/ui/styles"
	"github.com/sttts/kc/pkg/appconfig"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// Panel represents a file/resource panel
type Panel struct {
	title                string
	width                int
	height               int
	currentPath          string
	pathHistory          []string
	viewConfig           *ViewConfig
	contextCountProvider func() int
	actionHandlers       PanelActionHandlers
	envSupplier          PanelEnvironmentSupplier
	mode                 PanelViewMode
	widgets              map[PanelViewMode]PanelWidget
	widgetFactories      map[PanelViewMode]PanelWidgetFactory
	commandInteractive   bool
	commandFocused       bool
	messagePoster        func(tea.Msg)
	commandFocusHandler  func(bool) tea.Cmd
	lastSelectionID      string
	lastSelection        models.Item
	renderCache          string
	renderCacheWidth     int
	renderCacheHeight    int
	renderCacheFocused   bool
	renderCacheValid     bool
}

const panelContextTimeout = 250 * time.Millisecond

// Item represents an item in the panel (file, directory, resource, etc.)
type Item = listwidget.Item

// NewPanel creates a new panel
func NewPanel(title string) *Panel {
	p := &Panel{
		title:           title,
		currentPath:     "/",
		pathHistory:     make([]string, 0),
		mode:            PanelModeList,
		widgets:         make(map[PanelViewMode]PanelWidget),
		widgetFactories: make(map[PanelViewMode]PanelWidgetFactory),
	}
	p.RegisterMode(PanelModeList, func(panel *Panel) panelcontent.Widget {
		return listwidget.New(panel.listWidgetDeps())
	})
	p.RegisterMode(PanelModeDescribe, func(panel *Panel) panelcontent.Widget {
		return newPlaceholderWidget(panel, "Describe mode placeholder")
	})
	p.RegisterMode(PanelModeManifest, func(panel *Panel) panelcontent.Widget {
		return manifestwidget.New(panel.manifestWidgetDeps())
	})
	// Command widget is registered dynamically when StartCommand is called.
	return p
}

// StartCommand starts a custom command in the panel using an optional frame size.
// When frameWidth/frameHeight are >0, the panel is resized for the command mode
// before launching the widget to ensure bubbleterm dimensions are correct.
func (p *Panel) StartCommand(ctx context.Context, config appconfig.CommandConfig, items []models.Item, gvr schema.GroupVersionResource, frameWidth, frameHeight int) tea.Cmd {
	// Create widget if not exists or if config changed (simplified: always create new for now)
	widget := NewCommandWidget(p.listWidgetDeps(), config)
	widget.SetInteractive(config.Interactive)
	widget.SetFocusChangedHandler(p.setCommandFocus)

	// Register it as the widget for PanelModeCommand
	p.RegisterMode(PanelModeCommand, func(panel *Panel) panelcontent.Widget {
		return widget
	})

	// Switch mode
	cmd := p.SetMode(ctx, PanelModeCommand)

	// Resize to the provided frame size now that the command mode is active.
	if frameWidth > 0 && frameHeight > 0 {
		p.SetFrameDimensions(ctx, frameWidth, frameHeight)
	}

	// Start command
	startCmd := widget.StartCommand(ctx, items, gvr)

	return tea.Batch(cmd, startCmd)
}

func (p *Panel) invalidateRenderCache() {
	p.renderCacheValid = false
}

func (p *Panel) renderCacheMatches(width, height int, focused bool) bool {
	return p.renderCacheValid &&
		p.renderCacheWidth == width &&
		p.renderCacheHeight == height &&
		p.renderCacheFocused == focused
}

// HasCachedFrame reports whether the last render can be reused for the given size/focus.
func (p *Panel) HasCachedFrame(width, height int, focused bool) bool {
	return p.renderCacheMatches(width, height, focused)
}

// CachedFrame returns the cached frame for the given dimensions/focus when valid.
func (p *Panel) CachedFrame(width, height int, focused bool) (string, bool) {
	if p.renderCacheMatches(width, height, focused) {
		return p.renderCache, true
	}
	return "", false
}

func (p *Panel) listWidgetDeps() panelcontent.WidgetDeps {
	return panelcontent.WidgetDeps{
		InvokeAction:     p.invokeWidgetAction,
		Path:             func() string { return p.currentPath },
		SelectionChanged: p.widgetSelectionChanged,
		Post:             p.messagePoster,
	}
}

func (p *Panel) manifestWidgetDeps() panelcontent.WidgetDeps {
	return panelcontent.WidgetDeps{
		InvokeAction: p.invokeWidgetAction,
		Path:         func() string { return p.currentPath },
		SelectedItem: func(ctx context.Context) (models.Item, bool) {
			return p.SelectedNavItem(ctx)
		},
	}
}

func (p *Panel) listWidget(ctx context.Context) *listwidget.Widget {
	widget := p.ensureWidget(ctx, PanelModeList)
	if lw, ok := widget.(*listwidget.Widget); ok {
		return lw
	}
	return nil
}

func (p *Panel) commandWidget(ctx context.Context) *CommandWidget {
	widget := p.ensureWidget(ctx, PanelModeCommand)
	if cw, ok := widget.(*CommandWidget); ok {
		return cw
	}
	return nil
}

func (p *Panel) invokeWidgetAction(ctx context.Context, action panelcontent.Action) tea.Cmd {
	switch action {
	case panelcontent.ActionHelp:
		return p.invokeActionIfAllowed(ctx, PanelActionHelp)
	case panelcontent.ActionOptions:
		return p.invokeActionIfAllowed(ctx, PanelActionOptions)
	case panelcontent.ActionView:
		return p.invokeActionIfAllowed(ctx, PanelActionView)
	case panelcontent.ActionEdit:
		return p.invokeActionIfAllowed(ctx, PanelActionEdit)
	case panelcontent.ActionCreate:
		return p.invokeActionIfAllowed(ctx, PanelActionCreateNamespace)
	case panelcontent.ActionDelete:
		return p.invokeActionIfAllowed(ctx, PanelActionDelete)
	case panelcontent.ActionMenu:
		return p.invokeActionIfAllowed(ctx, PanelActionMenu)
	default:
		return nil
	}
}

func (p *Panel) widgetSelectionChanged(ctx context.Context, sel panelcontent.Selection) tea.Cmd {
	if sel.ID == "" {
		if lw := p.listWidget(ctx); lw != nil {
			sel.ID = lw.CurrentSelectionID(ctx)
		}
	}
	if sel.Path == "" {
		sel.Path = p.currentPath
	}
	if sel.Item == nil {
		if lw := p.listWidget(ctx); lw != nil {
			if item, ok := lw.SelectedNavItem(ctx); ok {
				sel.Item = item
			}
		}
	}
	changed := sel.ID != "" && sel.ID != p.lastSelectionID
	if sel.ID != "" {
		p.lastSelectionID = sel.ID
	}
	p.lastSelection = sel.Item
	p.notifySelectionListeners(ctx, sel)
	if changed || sel.Force {
		p.invalidateRenderCache()
	}
	if !changed && !sel.Force {
		return nil
	}
	panel := p
	selection := sel
	return func() tea.Msg {
		return PanelSelectionChangedMsg{
			Panel:     panel,
			Selection: selection,
		}
	}
}

// RegisterMode registers a widget factory for a panel view mode.
func (p *Panel) RegisterMode(mode PanelViewMode, factory PanelWidgetFactory) {
	if p.widgetFactories == nil {
		p.widgetFactories = make(map[PanelViewMode]PanelWidgetFactory)
	}
	p.widgetFactories[mode] = factory
}

// SetMessagePoster sets a callback to post messages back to the app.
func (p *Panel) SetMessagePoster(f func(tea.Msg)) { p.messagePoster = f }

// SetMode switches the active view mode and ensures the widget is initialized.
func (p *Panel) SetMode(ctx context.Context, mode PanelViewMode) tea.Cmd {
	if current := p.ensureActiveWidget(ctx); current != nil && p.mode != mode {
		current.SetFocus(ctx, false)
	}
	if p.mode != mode {
		p.invalidateRenderCache()
	}
	p.mode = mode
	w := p.ensureActiveWidget(ctx)
	if w == nil {
		return nil
	}
	w.Resize(ctx, panelcontent.Size{Width: p.width, Height: p.height})
	w.SetFocus(ctx, true)
	var cmds []tea.Cmd
	if initCmd := w.Init(ctx); initCmd != nil {
		cmds = append(cmds, initCmd)
	}
	if mode == PanelModeList {
		var selItem models.Item
		if item, ok := p.SelectedNavItem(ctx); ok {
			selItem = item
		}
		if selCmd := p.widgetSelectionChanged(ctx, panelcontent.Selection{
			ID:    p.currentSelectionID(ctx),
			Path:  p.currentPath,
			Item:  selItem,
			Force: true,
		}); selCmd != nil {
			cmds = append(cmds, selCmd)
		}
	} else {
		p.notifySelectionListeners(ctx, panelcontent.Selection{Path: p.currentPath})
	}
	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	default:
		return tea.Batch(cmds...)
	}
}

func (p *Panel) ensureActiveWidget(ctx context.Context) PanelWidget {
	return p.ensureWidget(ctx, p.mode)
}

func (p *Panel) ensureWidget(ctx context.Context, mode PanelViewMode) PanelWidget {
	if p.widgets == nil {
		p.widgets = make(map[PanelViewMode]PanelWidget)
	}
	if widget, ok := p.widgets[mode]; ok && widget != nil {
		return widget
	}
	factory := p.widgetFactories[mode]
	if factory == nil && mode == PanelModeList {
		factory = func(panel *Panel) panelcontent.Widget {
			return listwidget.New(panel.listWidgetDeps())
		}
	}
	if factory == nil {
		return nil
	}
	widget := factory(p)
	if widget == nil {
		return nil
	}
	p.widgets[mode] = widget
	widget.Resize(ctx, panelcontent.Size{Width: p.width, Height: p.height})
	return widget
}

func (p *Panel) widgetCursor(ctx context.Context) *tea.Cursor {
	if widget := p.ensureActiveWidget(ctx); widget != nil {
		if cp, ok := widget.(interface{ Cursor() *tea.Cursor }); ok {
			return cp.Cursor()
		}
	}
	return nil
}

// Mode returns the currently active view mode.
func (p *Panel) Mode() PanelViewMode { return p.mode }

// AvailableModes reports registered modes in deterministic order.
func (p *Panel) AvailableModes() []PanelViewMode {
	order := PanelModeOrder()
	result := make([]PanelViewMode, 0, len(order))
	for _, mode := range order {
		if p.widgetFactories == nil {
			if mode == PanelModeList {
				result = append(result, mode)
			}
			continue
		}
		if _, ok := p.widgetFactories[mode]; ok {
			result = append(result, mode)
		}
	}
	if len(result) == 0 {
		result = append(result, PanelModeList)
	}
	return result
}

// SetResourceViewOptions sets the per-panel view toggles for resource groups.
func (p *Panel) SetResourceViewOptions(showNonEmpty bool, order string) {
	p.invalidateRenderCache()
	if lw := p.listWidget(nil); lw != nil {
		lw.SetResourceViewOptions(showNonEmpty, order)
	}
}

// ResourceViewOptions returns current per-panel options.
func (p *Panel) ResourceViewOptions() (bool, string) {
	if lw := p.listWidget(nil); lw != nil {
		return lw.ResourceViewOptions()
	}
	return false, "favorites"
}

// ResetSelectionTop moves the cursor to the top and resets scrolling.
func (p *Panel) ResetSelectionTop(ctx context.Context) {
	if lw := p.listWidget(ctx); lw != nil {
		lw.ResetSelection(ctx)
	}
	p.widgetSelectionChanged(ctx, panelcontent.Selection{ID: p.currentSelectionID(ctx), Path: p.currentPath})
}

// SetFolder enables folder-backed rendering using the new navigation package.
// This does not alter legacy behaviors beyond rendering headers/rows from the
// folder for preview purposes. Selection/enter logic remains unchanged.
func (p *Panel) SetFolder(ctx context.Context, f models.Folder, hasBack bool) {
	if lw := p.listWidget(ctx); lw != nil {
		lw.SetFolder(ctx, f, hasBack)
	}
	p.invalidateRenderCache()
	p.widgetSelectionChanged(ctx, panelcontent.Selection{ID: p.currentSelectionID(ctx), Path: p.currentPath})
}

// UseFolder toggles folder-backed rendering.
func (p *Panel) UseFolder(ctx context.Context, on bool) {
	if lw := p.listWidget(ctx); lw != nil {
		lw.UseFolder(on)
	}
	p.invalidateRenderCache()
	p.widgetSelectionChanged(ctx, panelcontent.Selection{ID: p.lastSelectionID, Path: p.currentPath})
}

// ClearFolder disables folder-backed rendering and clears current folder.
func (p *Panel) ClearFolder(ctx context.Context) {
	if lw := p.listWidget(ctx); lw != nil {
		lw.ClearFolder()
	}
	p.invalidateRenderCache()
	p.widgetSelectionChanged(ctx, panelcontent.Selection{ID: "", Path: p.currentPath})
}

// Folder returns the active folder if the list widget is folder-backed.
func (p *Panel) Folder() models.Folder {
	if lw := p.listWidget(nil); lw != nil {
		return lw.Folder()
	}
	return nil
}

// Enter triggers the list widget's enter behaviour for the current selection.
func (p *Panel) Enter(ctx context.Context) tea.Cmd {
	if lw := p.listWidget(ctx); lw != nil {
		return lw.Enter(ctx)
	}
	return nil
}

// SetTableMode updates the panel's table rendering mode ("scroll" or "fit").
func (p *Panel) SetTableMode(ctx context.Context, mode string) {
	p.invalidateRenderCache()
	if lw := p.listWidget(ctx); lw != nil {
		lw.SetTableMode(ctx, mode)
	}
}

// TableMode returns the current mode label ("scroll" or "fit").
func (p *Panel) TableMode() string {
	if lw := p.listWidget(nil); lw != nil {
		return lw.TableMode()
	}
	return "scroll"
}

// SetColumnsMode updates which server-side table columns to show (normal or wide).
func (p *Panel) SetColumnsMode(ctx context.Context, mode string) {
	p.invalidateRenderCache()
	if lw := p.listWidget(ctx); lw != nil {
		lw.SetColumnsMode(ctx, mode)
	}
}

// ColumnsMode returns the current columns mode label.
func (p *Panel) ColumnsMode() string {
	if lw := p.listWidget(nil); lw != nil {
		return lw.ColumnsMode()
	}
	return "normal"
}

// SetObjectOrder updates object list ordering mode.
func (p *Panel) SetObjectOrder(ctx context.Context, order string) {
	p.invalidateRenderCache()
	if lw := p.listWidget(ctx); lw != nil {
		lw.SetObjectOrder(ctx, order)
	}
}

func (p *Panel) toggleColumnsMode(ctx context.Context) tea.Cmd {
	if lw := p.listWidget(ctx); lw != nil {
		return lw.ToggleColumnsMode(ctx)
	}
	return nil
}

func (p *Panel) ObjectOrder() string {
	if lw := p.listWidget(nil); lw != nil {
		return lw.ObjectOrder()
	}
	return "name"
}

// HasInteractiveCommand reports whether the active command widget is interactive.
func (p *Panel) HasInteractiveCommand() bool {
	return p.mode == PanelModeCommand && p.commandInteractive
}

// HasCommandFocus reports whether the command widget currently holds keyboard focus.
func (p *Panel) HasCommandFocus() bool {
	return p.mode == PanelModeCommand && p.commandFocused
}

// CommandWatchInterval reports the current watch interval for the command widget.
func (p *Panel) CommandWatchInterval(ctx context.Context) time.Duration {
	if cw := p.commandWidget(ctx); cw != nil {
		return cw.WatchInterval()
	}
	return 0
}

// SetCommandWatchInterval updates the command widget watch interval if present.
func (p *Panel) SetCommandWatchInterval(ctx context.Context, interval time.Duration) tea.Cmd {
	if cw := p.commandWidget(ctx); cw != nil {
		return cw.SetWatchInterval(ctx, interval)
	}
	return nil
}

// SetCommandFocusHandler installs a callback invoked when the command widget focus toggles.
func (p *Panel) SetCommandFocusHandler(h func(bool) tea.Cmd) {
	p.commandFocusHandler = h
}

func (p *Panel) setCommandFocus(focused bool) tea.Cmd {
	if p.commandFocused == focused {
		return nil
	}
	p.commandFocused = focused
	p.invalidateRenderCache()
	if h := p.commandFocusHandler; h != nil {
		return h(focused)
	}
	return nil
}

// RestartCommand reruns the active command widget if present.
func (p *Panel) RestartCommand(ctx context.Context) tea.Cmd {
	if cw := p.commandWidget(ctx); cw != nil {
		return cw.Restart(ctx)
	}
	return nil
}

// SelectByRowID moves the selection to the row with the given ID if present.
func (p *Panel) SelectByRowID(ctx context.Context, id string) {
	if lw := p.listWidget(ctx); lw != nil {
		lw.SelectByRowID(ctx, id)
	}
	p.notifySelectionListeners(ctx, panelcontent.Selection{ID: id, Path: p.currentPath})
}

// SelectRowIDs marks multiple rows as selected and focuses the first match.
func (p *Panel) SelectRowIDs(ctx context.Context, ids []string) {
	if lw := p.listWidget(ctx); lw != nil {
		lw.SelectRowIDs(ctx, ids)
	}
	if len(ids) > 0 {
		p.notifySelectionListeners(ctx, panelcontent.Selection{ID: ids[0], Path: p.currentPath})
	}
}

func (p *Panel) currentSelectionID(ctx context.Context) string {
	if lw := p.listWidget(ctx); lw != nil {
		return lw.CurrentSelectionID(ctx)
	}
	return ""
}

// SelectedNavItem resolves the currently focused navigation item, skipping the
// synthetic back entry. Returns false when no concrete item is selected.
func (p *Panel) SelectedNavItem(ctx context.Context) (models.Item, bool) {
	if lw := p.listWidget(ctx); lw != nil {
		if item, ok := lw.SelectedNavItem(ctx); ok {
			return item, true
		}
	}
	if p.lastSelection != nil {
		return p.lastSelection, true
	}
	return nil, false
}

// SetFolderNavHandler installs a callback invoked when Enter is pressed while
// folder-backed rendering is active. The callback receives whether a back
// navigation was requested and, if not back, the next Folder (may be nil).
func (p *Panel) SetFolderNavHandler(h func(back bool, selID string, next models.Folder)) {
	if lw := p.listWidget(nil); lw != nil {
		lw.SetFolderNavHandler(h)
	}
}

// RefreshFolder refreshes the BigTable rows from the current folder list.
// Used by periodic ticks to reflect informer-driven changes with a max 1s delay.
func (p *Panel) RefreshFolder(ctx context.Context) {
	p.invalidateRenderCache()
	if lw := p.listWidget(ctx); lw != nil {
		lw.RefreshFolder(ctx)
	}
	var selItem models.Item
	if item, ok := p.SelectedNavItem(ctx); ok {
		selItem = item
	}
	p.widgetSelectionChanged(ctx, panelcontent.Selection{
		ID:    p.currentSelectionID(ctx),
		Path:  p.currentPath,
		Item:  selItem,
		Force: true,
	})
}

// NotifySelection explicitly broadcasts a selection change to widgets.
func (p *Panel) NotifySelection(ctx context.Context, sel panelcontent.Selection) {
	if sel.Path == "" {
		sel.Path = p.currentPath
	}
	if sel.Item == nil {
		if item, ok := p.SelectedNavItem(ctx); ok {
			sel.Item = item
		}
	}
	if sel.ID != "" {
		p.lastSelectionID = sel.ID
	}
	if sel.Item != nil {
		p.lastSelection = sel.Item
	}
	p.invalidateRenderCache()
	ctrllog.FromContext(ctx).WithName("panel").Info("notify selection", "panel", p.title, "mode", p.mode, "selectionID", sel.ID, "force", sel.Force, "hasItem", sel.Item != nil)
	p.notifySelectionListeners(ctx, sel)
}

// SetResourceCatalog injects the namespaced resource catalog (plural -> GVK).
func (p *Panel) SetResourceCatalog(infos []kccluster.ResourceInfo) {
	_ = infos
}

// SetNamespacesDataSource wires a namespaces data source for live listings.
// Legacy live data sources removed; folders drive listings now.

// SetViewConfig injects the view configuration (global + per resource overrides).
func (p *Panel) SetViewConfig(cfg *ViewConfig) {
	p.viewConfig = cfg
	p.invalidateRenderCache()
}

// SetContextCountProvider injects a function to return the number of contexts.
func (p *Panel) SetContextCountProvider(fn func() int) {
	p.contextCountProvider = fn
	p.invalidateRenderCache()
}

// countContexts returns the number of contexts or -1 if unknown.
func (p *Panel) countContexts() int {
	if p.contextCountProvider == nil {
		return -1
	}
	return p.contextCountProvider()
}

// Init initializes the panel
func (p *Panel) Init() tea.Cmd {
	ctx, cancel := context.WithTimeout(context.Background(), panelContextTimeout)
	defer cancel()
	if widget := p.ensureActiveWidget(ctx); widget != nil {
		return widget.Init(ctx)
	}
	return nil
}

// Update handles messages and updates the panel state
func (p *Panel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithTimeout(context.Background(), panelContextTimeout)
	defer cancel()
	if widget := p.ensureActiveWidget(ctx); widget != nil {
		if cmd, handled := widget.Update(ctx, msg); handled {
			p.invalidateRenderCache()
			return p, cmd
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		var action PanelAction
		var mapped bool
		switch key.String() {
		case "f1":
			action, mapped = PanelActionHelp, true
		case "f2":
			action, mapped = PanelActionOptions, true
		case "f3":
			action, mapped = PanelActionView, true
		case "f4":
			action, mapped = PanelActionEdit, true
		case "f5":
			action, mapped = PanelActionCopy, true
		case "f7":
			action, mapped = PanelActionCreateNamespace, true
		case "f8":
			action, mapped = PanelActionDelete, true
		case "f9":
			action, mapped = PanelActionMenu, true
		}
		if mapped {
			actCtx, actCancel := context.WithTimeout(context.Background(), panelContextTimeout)
			defer actCancel()
			return p, p.invokeActionIfAllowed(actCtx, action)
		}
	}
	return p, nil
}

// View renders the panel
func (p *Panel) View() tea.View {
	ctx, cancel := context.WithTimeout(context.Background(), panelContextTimeout)
	defer cancel()

	info := p.frameInfo(ctx)
	header := p.renderHeader(info.Breadcrumb, info.HeaderStatus)
	content := viewString(p.renderContent(ctx))
	cursor := p.widgetCursor(ctx)
	footer := p.renderFooter(ctx, info.FooterStatus, info.SuppressFooter)

	if footer == "" {
		view := tea.NewView(lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			content,
		))
		view.Cursor = cursor
		return view
	}

	view := tea.NewView(lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		content,
		footer,
	))
	view.Cursor = cursor
	return view
}

// Render draws the fully framed panel, including optional footer, using the provided dimensions.
func (p *Panel) Render(ctx context.Context, width, height int, focused bool) string {
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	if p.renderCacheMatches(width, height, focused) {
		return p.renderCache
	}
	contentWidth := max(1, width-2)
	frameHeight := max(2, height)
	// Default content height assumes both top/bottom borders consume 2 rows.
	contentHeight := max(1, frameHeight-2)

	var (
		footerFrame  string
		footerHeight int
		info         panelcontent.FrameInfo
	)

	for i := 0; i < 3; i++ {
		p.SetDimensions(ctx, contentWidth, contentHeight)
		info = p.frameInfo(ctx)
		footerContent := p.renderFooter(ctx, info.FooterStatus, info.SuppressFooter)
		footerFrame, footerHeight = p.renderFramedFooter(footerContent, width)
		if info.SuppressFooter {
			footerFrame = ""
			footerHeight = 0
		}
		if footerHeight >= height {
			footerFrame = ""
			footerHeight = 0
		}

		newFrameHeight := max(2, height-footerHeight)
		newContentHeight := max(1, newFrameHeight-2)
		if newFrameHeight == frameHeight && newContentHeight == contentHeight {
			frameHeight = newFrameHeight
			contentHeight = newContentHeight
			break
		}
		frameHeight = newFrameHeight
		contentHeight = newContentHeight
	}

	// Ensure final dimensions are applied before rendering.
	p.SetDimensions(ctx, contentWidth, contentHeight)
	info = p.frameInfo(ctx)
	footerContent := p.renderFooter(ctx, info.FooterStatus, info.SuppressFooter)
	footerFrame, footerHeight = p.renderFramedFooter(footerContent, width)
	if info.SuppressFooter {
		footerFrame = ""
		footerHeight = 0
	}
	if footerHeight >= height {
		footerFrame = ""
		footerHeight = 0
	}
	frameHeight = max(2, height-footerHeight)
	// The frame always includes both a top and bottom border row; keep content height
	// consistent regardless of whether a footer bar is rendered.
	contentHeight = max(1, frameHeight-2)
	p.SetDimensions(ctx, contentWidth, contentHeight)
	info = p.frameInfo(ctx)

	title := info.Breadcrumb
	if title == "" {
		title = p.currentPath
	}
	contentView := p.renderContentFocused(ctx, focused)
	frame := p.renderFrame(viewString(contentView), info, title, width, frameHeight, focused, footerFrame != "")
	if footerFrame != "" {
		p.renderCache = lipgloss.JoinVertical(lipgloss.Top, frame, footerFrame)
	} else {
		p.renderCache = frame
	}
	p.renderCacheWidth = width
	p.renderCacheHeight = height
	p.renderCacheFocused = focused
	p.renderCacheValid = true
	return p.renderCache
}

// GetCurrentPath returns the current path for breadcrumbs
func (p *Panel) GetCurrentPath() string {
	return p.currentPath
}

// SetCurrentPath sets the breadcrumb path (absolute, leading slash) for this panel.
// The App is responsible for computing the path via the navigator.
func (p *Panel) SetCurrentPath(path string) {
	if p.currentPath == path {
		return
	}
	p.currentPath = path
	p.invalidateRenderCache()
}

// SetDimensions sets the panel dimensions
func (p *Panel) SetDimensions(ctx context.Context, width, height int) {
	if width == p.width && height == p.height {
		return
	}
	p.width = width
	p.height = height
	p.invalidateRenderCache()
	if widget := p.ensureActiveWidget(ctx); widget != nil {
		widget.Resize(ctx, panelcontent.Size{Width: width, Height: height})
	}
}

func (p *Panel) frameInfo(ctx context.Context) panelcontent.FrameInfo {
	info := panelcontent.FrameInfo{
		Breadcrumb: p.currentPath,
	}
	if widget := p.ensureActiveWidget(ctx); widget != nil {
		if provider, ok := widget.(panelcontent.FrameInfoProvider); ok {
			wi := provider.FrameInfo(ctx, panelcontent.FrameInfoRequest{Width: p.width})
			if wi.Breadcrumb != "" {
				info.Breadcrumb = wi.Breadcrumb
			}
			info.HeaderStatus = wi.HeaderStatus
			info.FooterStatus = wi.FooterStatus
			info.TopIndicator = wi.TopIndicator
			info.BottomIndicator = wi.BottomIndicator
			if wi.SuppressFooter {
				info.SuppressFooter = true
			}
		}
	}
	return info
}

// renderHeader renders the panel header
func (p *Panel) renderHeader(breadcrumb, status string) string {
	if breadcrumb == "" {
		breadcrumb = "/"
	}
	headerStyle := uistyles.PanelHeaderStyle.
		Width(p.width).
		Height(1)

	left := p.ellipsizePath(breadcrumb, p.width)
	if status == "" {
		return headerStyle.Align(lipgloss.Left).Render(left)
	}

	statusWidth := lipgloss.Width(status)
	if statusWidth >= p.width {
		status = truncateToWidth(status, p.width-1)
		statusWidth = lipgloss.Width(status)
	}
	if statusWidth < 0 {
		statusWidth = 0
	}
	available := p.width - statusWidth - 1
	if available < 0 {
		available = 0
	}
	left = p.ellipsizePath(breadcrumb, available)
	padding := p.width - lipgloss.Width(left) - statusWidth
	if padding < 1 {
		padding = 1
	}
	line := left + strings.Repeat(" ", padding) + status
	return headerStyle.Render(line)
}

// ellipsizePath shortens long breadcrumbs from the left by components, prefixing with "...".
func (p *Panel) ellipsizePath(path string, width int) string {
	if len(path) <= width {
		return path
	}
	if width <= 3 {
		return "..."
	}
	parts := strings.Split(path, "/")
	// Ensure leading slash does not create empty segments
	filtered := make([]string, 0, len(parts))
	for i, seg := range parts {
		if i == 0 {
			continue
		} // skip leading empty from split
		if seg != "" {
			filtered = append(filtered, seg)
		}
	}
	// Rebuild from right until fits
	acc := ""
	for i := len(filtered) - 1; i >= 0; i-- {
		candidate := "/" + filtered[i] + acc
		if len(candidate)+3 <= width {
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

// renderContent renders the panel content
func (p *Panel) renderContent(ctx context.Context) tea.View {
	return p.renderContentFocused(ctx, false)
}

// renderContentFocused renders the panel content with focus state
func (p *Panel) renderContentFocused(ctx context.Context, isFocused bool) tea.View {
	if widget := p.ensureActiveWidget(ctx); widget != nil {
		return widget.View(ctx, panelcontent.Frame{
			Size:    panelcontent.Size{Width: p.width, Height: p.height},
			Focused: isFocused,
		})
	}
	return tea.NewView("")
}

func (p *Panel) renderFooter(ctx context.Context, status string, suppress bool) string {
	return p.renderFooterWithWidth(ctx, status, suppress, p.width)
}

func (p *Panel) renderFooterWithWidth(ctx context.Context, status string, suppress bool, width int) string {
	if suppress {
		return ""
	}
	// The status string is rendered on the frame; avoid duplicating it in the footer line.
	status = ""
	renderedFooter := ""
	if widget := p.ensureActiveWidget(ctx); widget != nil {
		if fp, ok := widget.(panelcontent.FooterProvider); ok {
			renderedFooter = fp.Footer(ctx, p.width)
		}
	}
	lines := strings.Split(strings.TrimRight(renderedFooter, "\n"), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	styled := make([]string, len(lines))
	for i, line := range lines {
		text := line
		if lipgloss.Width(text) > width {
			text = truncateToWidth(text, width)
		}
		style := uistyles.PanelFooterStyle.Copy().
			Width(width).
			Height(1)
		styled[i] = style.Align(lipgloss.Left).Render(text)
	}
	return strings.Join(styled, "\n")
}

func (p *Panel) renderFramedFooter(content string, width int) (string, int) {
	if strings.TrimSpace(content) == "" {
		return "", 0
	}
	frame := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderTop(false).
		BorderForeground(lipgloss.White).
		BorderBackground(lipgloss.Blue).
		Background(lipgloss.Blue).
		Foreground(lipgloss.Color(uistyles.ColorWhite)).
		Width(width).
		Render(content)
	return frame, lipgloss.Height(frame)
}

// FrameContentSize computes the content rectangle and footer height for the given frame.
func (p *Panel) FrameContentSize(ctx context.Context, frameWidth, frameHeight int) (panelcontent.Size, int) {
	if frameWidth <= 0 {
		frameWidth = 1
	}
	if frameHeight <= 0 {
		frameHeight = 1
	}
	contentWidth := max(1, frameWidth-2)
	info := p.frameInfo(ctx)
	footerContent := p.renderFooterWithWidth(ctx, info.FooterStatus, info.SuppressFooter, contentWidth)
	footerFrame, footerHeight := p.renderFramedFooter(footerContent, frameWidth)
	if info.SuppressFooter || footerFrame == "" {
		footerHeight = 0
	}
	contentHeight := max(1, frameHeight-footerHeight-2)
	return panelcontent.Size{Width: contentWidth, Height: contentHeight}, footerHeight
}

// SetFrameDimensions accepts a frame size (outer border box) and applies the derived content size.
func (p *Panel) SetFrameDimensions(ctx context.Context, frameWidth, frameHeight int) {
	size, _ := p.FrameContentSize(ctx, frameWidth, frameHeight)
	p.SetDimensions(ctx, size.Width, size.Height)
}

func (p *Panel) renderFrame(content string, info panelcontent.FrameInfo, title string, width, height int, focused bool, hasFooter bool) string {
	if title == "" {
		title = "/"
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.White).
		BorderBackground(lipgloss.Blue).
		Background(lipgloss.Blue).
		Width(width).
		Height(height)
	if p.commandFocused {
		boxStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(uistyles.ColorDarkGrey)).
			BorderBackground(lipgloss.Color(uistyles.ColorModalBg)).
			Background(lipgloss.Color(uistyles.ColorModalBg)).
			Width(width).
			Height(height)
	}

	var labelStyle lipgloss.Style
	if focused {
		labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Black).
			Background(lipgloss.White).
			Padding(0, 1)
	} else {
		labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.White).
			Background(lipgloss.Blue).
			Padding(0, 1)
	}
	if p.commandFocused {
		labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(uistyles.ColorBlack)).
			Background(lipgloss.Color(uistyles.ColorModalBg)).
			Padding(0, 1).
			Bold(true)
	}

	border := boxStyle.GetBorderStyle()
	topBorderStyler := lipgloss.NewStyle().
		Foreground(boxStyle.GetBorderTopForeground()).
		Background(boxStyle.GetBorderTopBackground()).
		Render

	topLeft := topBorderStyler(border.TopLeft)
	topRight := topBorderStyler(border.TopRight)

	topIndicatorGlyph := indicatorGlyph(info.TopIndicator, border.Top)
	topIndicator := topBorderStyler(topIndicatorGlyph)
	availableSpace := width - lipgloss.Width(topLeft+topRight)
	indicatorWidth := lipgloss.Width(topIndicator)
	availableForLabel := max(0, availableSpace-indicatorWidth)
	title = p.ellipsizeBreadcrumbTitle(labelStyle, title, availableForLabel)
	renderedLabel := labelStyle.Render(title)
	labelWidth := lipgloss.Width(renderedLabel)
	gapWidth := max(0, availableForLabel-labelWidth)
	leftGapWidth := gapWidth / 2
	rightGapWidth := gapWidth - leftGapWidth
	leftGap := topBorderStyler(strings.Repeat(border.Top, leftGapWidth))
	rightGap := topBorderStyler(strings.Repeat(border.Top, rightGapWidth))
	top := topLeft + leftGap + renderedLabel + rightGap + topIndicator + topRight

	bottomView := boxStyle.Copy().
		BorderTop(false).
		Width(width).
		Height(height - 1).
		Render(content)

	lines := strings.Split(bottomView, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}

	// Adjust bottom corners only when a footer is attached.
	if hasFooter {
		border.BottomLeft = "├"
		border.BottomRight = "┤"
	}

	lines[len(lines)-1] = p.composeBottomBorder(border, boxStyle, info, width, modeLabel(p.mode))

	frame := top + "\n" + strings.Join(lines, "\n")

	return frame
}

func (p *Panel) composeBottomBorder(border lipgloss.Border, boxStyle lipgloss.Style, info panelcontent.FrameInfo, width int, mode string) string {
	borderStyle := lipgloss.NewStyle().
		Foreground(boxStyle.GetBorderBottomForeground()).
		Background(boxStyle.GetBorderBottomBackground())
	left := borderStyle.Render(border.BottomLeft)
	right := borderStyle.Render(border.BottomRight)
	connector := borderStyle.Render(border.Bottom)
	connectorWidth := lipgloss.Width(connector)
	indicator := borderStyle.Render(indicatorGlyph(info.BottomIndicator, border.Bottom))
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	indicatorWidth := lipgloss.Width(indicator)
	contentWidth := width - leftWidth - rightWidth - indicatorWidth - connectorWidth
	if contentWidth < 0 {
		contentWidth = 0
	}

	statusLabel := strings.TrimSpace(info.FooterStatus)
	modeLabel := strings.TrimSpace(mode)
	var parts []string
	if statusLabel != "" {
		parts = append(parts, statusLabel)
	}
	if modeLabel != "" {
		parts = append(parts, modeLabel)
	}
	text := strings.Join(parts, " • ")
	if text != "" {
		text = truncateStringToWidth(text, contentWidth)
	}
	renderedText := ""
	textWidth := 0
	if text != "" {
		style := uistyles.PanelFooterStyle.Copy()
		if p.commandFocused {
			style = style.
				Background(lipgloss.Color(uistyles.ColorModalBg)).
				Foreground(lipgloss.Color(uistyles.ColorBlack))
		}
		renderedText = style.Render(text)
		textWidth = lipgloss.Width(renderedText)
	}
	fillerWidth := contentWidth - textWidth
	if fillerWidth < 0 {
		fillerWidth = 0
	}
	filler := ""
	if fillerWidth > 0 {
		filler = borderStyle.Render(strings.Repeat(border.Bottom, fillerWidth))
	}

	return left + filler + renderedText + connector + indicator + right
}

func indicatorGlyph(indicator string, fallback string) string {
	switch strings.TrimSpace(indicator) {
	case "^":
		return "▲"
	case "v":
		return "▼"
	case "":
		return fallback
	default:
		return indicator
	}
}

func (p *Panel) ellipsizeBreadcrumbTitle(labelStyle lipgloss.Style, title string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(labelStyle.Render(title)) <= maxWidth {
		return title
	}
	if lipgloss.Width(labelStyle.Render("...")) > maxWidth {
		return "..."
	}
	parts := strings.Split(title, "/")
	segs := make([]string, 0, len(parts))
	for _, seg := range parts {
		if seg != "" {
			segs = append(segs, seg)
		}
	}
	acc := ""
	for i := len(segs) - 1; i >= 0; i-- {
		candidate := "/" + segs[i] + acc
		test := "..." + candidate
		if lipgloss.Width(labelStyle.Render(test)) <= maxWidth {
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

func truncateToWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	var b strings.Builder
	width := 0
	for _, r := range s {
		w := lipgloss.Width(string(r))
		if width+w > maxWidth {
			break
		}
		width += w
		b.WriteRune(r)
	}
	return b.String()
}

func truncateStringToWidth(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	var b strings.Builder
	width := 0
	for _, r := range s {
		w := lipgloss.Width(string(r))
		if width+w > maxWidth {
			break
		}
		width += w
		b.WriteRune(r)
	}
	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// renderItem renders a single item
func (p *Panel) GetStatus() string {
	if lw := p.listWidget(nil); lw != nil {
		return lw.Status()
	}
	return "Empty"
}

func (p *Panel) GetSelectedItems() []Item {
	if lw := p.listWidget(nil); lw != nil {
		ctx := context.Background()
		if selected := lw.SelectedItems(ctx); len(selected) > 0 {
			return selected
		}
	}
	return nil
}

func (p *Panel) GetCurrentItem() *Item {
	if lw := p.listWidget(nil); lw != nil {
		if item := lw.CurrentItem(context.Background()); item != nil {
			return item
		}
	}
	if p.lastSelection != nil {
		return &Item{Item: p.lastSelection}
	}
	return nil
}

// Items returns the current list items snapshot.
func (p *Panel) Items(ctx context.Context) []Item {
	if lw := p.listWidget(ctx); lw != nil {
		lw.RefreshFolder(ctx)
		return lw.Items()
	}
	return nil
}

// CurrentSelection returns the latest selection snapshot.
func (p *Panel) CurrentSelection(ctx context.Context) panelcontent.Selection {
	sel := panelcontent.Selection{Path: p.currentPath}
	if p.lastSelectionID != "" {
		sel.ID = p.lastSelectionID
	}
	if p.lastSelection != nil {
		sel.Item = p.lastSelection
	}
	return sel
}

// ColumnTitles exposes the current list column headers.
func (p *Panel) ColumnTitles(ctx context.Context) []string {
	if lw := p.listWidget(ctx); lw != nil {
		return lw.ColumnTitles()
	}
	return nil
}

func (p *Panel) notifySelectionListeners(ctx context.Context, sel panelcontent.Selection) {
	if sel.Path == "" {
		sel.Path = p.currentPath
	}
	if sel.Item == nil {
		if item, ok := p.SelectedNavItem(ctx); ok {
			sel.Item = item
		}
	}
	if sel.Item != nil {
		p.lastSelection = sel.Item
	}
	for _, widget := range p.widgets {
		if listener, ok := widget.(panelcontent.SelectionListener); ok && widget != nil {
			listener.OnSelectionChanged(ctx, sel)
		}
	}
}
