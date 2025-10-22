package ui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	kccluster "github.com/sttts/kc/internal/cluster"
	models "github.com/sttts/kc/internal/models"
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
	listwidget "github.com/sttts/kc/internal/ui/panelcontent/list"
	manifestwidget "github.com/sttts/kc/internal/ui/panelcontent/manifest"
	uistyles "github.com/sttts/kc/internal/ui/styles"
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
	lastSelectionID      string
	lastSelection        models.Item
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
	p.RegisterMode(PanelModeFile, func(panel *Panel) panelcontent.Widget {
		return newPlaceholderWidget(panel, "File mode placeholder")
	})
	return p
}

func (p *Panel) listWidgetDeps() panelcontent.WidgetDeps {
	return panelcontent.WidgetDeps{
		InvokeAction:     p.invokeWidgetAction,
		Path:             func() string { return p.currentPath },
		SelectionChanged: p.widgetSelectionChanged,
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
		if item, ok := p.SelectedNavItem(ctx); ok {
			sel.Item = item
		}
	}
	changed := sel.ID != "" && sel.ID != p.lastSelectionID
	if sel.ID != "" {
		p.lastSelectionID = sel.ID
	}
	if sel.Item != nil {
		p.lastSelection = sel.Item
	}
	p.notifySelectionListeners(ctx, sel)
	if !changed {
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

// SetMode switches the active view mode and ensures the widget is initialized.
func (p *Panel) SetMode(ctx context.Context, mode PanelViewMode) tea.Cmd {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), panelContextTimeout)
		defer cancel()
	}
	if current := p.ensureActiveWidget(ctx); current != nil && p.mode != mode {
		current.SetFocus(ctx, false)
	}
	p.mode = mode
	w := p.ensureActiveWidget(ctx)
	if w == nil {
		return nil
	}
	w.Resize(ctx, panelcontent.Size{Width: p.width, Height: p.height})
	w.SetFocus(ctx, true)
	cmd := w.Init(ctx)
	p.widgetSelectionChanged(ctx, panelcontent.Selection{
		ID:   p.currentSelectionID(ctx),
		Path: p.currentPath,
	})
	return cmd
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
	if ctx != nil {
		widget.Resize(ctx, panelcontent.Size{Width: p.width, Height: p.height})
	}
	return widget
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
	p.widgetSelectionChanged(ctx, panelcontent.Selection{ID: p.currentSelectionID(ctx), Path: p.currentPath})
}

// UseFolder toggles folder-backed rendering.
func (p *Panel) UseFolder(on bool) {
	if lw := p.listWidget(nil); lw != nil {
		lw.UseFolder(on)
	}
	p.widgetSelectionChanged(nil, panelcontent.Selection{ID: p.lastSelectionID, Path: p.currentPath})
}

// ClearFolder disables folder-backed rendering and clears current folder.
func (p *Panel) ClearFolder() {
	if lw := p.listWidget(nil); lw != nil {
		lw.ClearFolder()
	}
	p.widgetSelectionChanged(nil, panelcontent.Selection{ID: "", Path: p.currentPath})
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

// SelectByRowID moves the selection to the row with the given ID if present.
func (p *Panel) SelectByRowID(ctx context.Context, id string) {
	if lw := p.listWidget(ctx); lw != nil {
		lw.SelectByRowID(ctx, id)
	}
	p.notifySelectionListeners(ctx, panelcontent.Selection{ID: id, Path: p.currentPath})
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
	if lw := p.listWidget(ctx); lw != nil {
		lw.RefreshFolder(ctx)
	}
	p.widgetSelectionChanged(ctx, panelcontent.Selection{ID: p.currentSelectionID(ctx), Path: p.currentPath})
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
	p.notifySelectionListeners(ctx, sel)
}

// SetResourceCatalog injects the namespaced resource catalog (plural -> GVK).
func (p *Panel) SetResourceCatalog(infos []kccluster.ResourceInfo) {
	_ = infos
}

// SetNamespacesDataSource wires a namespaces data source for live listings.
// Legacy live data sources removed; folders drive listings now.

// SetViewConfig injects the view configuration (global + per resource overrides).
func (p *Panel) SetViewConfig(cfg *ViewConfig) { p.viewConfig = cfg }

// SetContextCountProvider injects a function to return the number of contexts.
func (p *Panel) SetContextCountProvider(fn func() int) { p.contextCountProvider = fn }

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
func (p *Panel) View() string {
	ctx, cancel := context.WithTimeout(context.Background(), panelContextTimeout)
	defer cancel()

	info := p.frameInfo(ctx)
	header := p.renderHeader(info.Breadcrumb, info.HeaderStatus)
	content := p.renderContent(ctx)
	footer := p.renderFooter(ctx, info.FooterStatus, info.SuppressFooter)

	if footer == "" {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			content,
		)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		content,
		footer,
	)
}

// ViewWithoutHeader renders the panel content and footer only (no header)
func (p *Panel) ViewWithoutHeader() string {
	ctx, cancel := context.WithTimeout(context.Background(), panelContextTimeout)
	defer cancel()
	info := p.frameInfo(ctx)
	content := p.renderContent(ctx)
	footer := p.renderFooter(ctx, info.FooterStatus, info.SuppressFooter)
	if footer == "" {
		return content
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		footer,
	)
}

// ViewWithoutHeaderFocused renders the panel content and footer with focus state
func (p *Panel) ViewWithoutHeaderFocused(isFocused bool) string {
	ctx, cancel := context.WithTimeout(context.Background(), panelContextTimeout)
	defer cancel()
	info := p.frameInfo(ctx)
	content := p.renderContentFocused(ctx, isFocused)
	footer := p.renderFooter(ctx, info.FooterStatus, info.SuppressFooter)
	if footer == "" {
		return content
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		footer,
	)
}

// ViewContentOnlyFocused renders just the panel content without header or footer
func (p *Panel) ViewContentOnlyFocused(ctx context.Context, isFocused bool) string {
	return p.renderContentFocused(ctx, isFocused)
}

// GetCurrentPath returns the current path for breadcrumbs
func (p *Panel) GetCurrentPath() string {
	return p.currentPath
}

// SetCurrentPath sets the breadcrumb path (absolute, leading slash) for this panel.
// The App is responsible for computing the path via the navigator.
func (p *Panel) SetCurrentPath(path string) { p.currentPath = path }

// GetFooter returns the rendered footer for external use
func (p *Panel) GetFooter(ctx context.Context) string {
	info := p.frameInfo(ctx)
	return p.renderFooter(ctx, info.FooterStatus, info.SuppressFooter)
}

// SetDimensions sets the panel dimensions
func (p *Panel) SetDimensions(ctx context.Context, width, height int) {
	p.width = width
	p.height = height
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
			if wi.SuppressFooter {
				info.SuppressFooter = true
			}
		}
	}
	return info
}

// FrameInfo exposes frame metadata for external rendering.
func (p *Panel) FrameInfo(ctx context.Context) panelcontent.FrameInfo {
	return p.frameInfo(ctx)
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
func (p *Panel) renderContent(ctx context.Context) string {
	return p.renderContentFocused(ctx, false)
}

// renderContentFocused renders the panel content with focus state
func (p *Panel) renderContentFocused(ctx context.Context, isFocused bool) string {
	if widget := p.ensureActiveWidget(ctx); widget != nil {
		return widget.View(ctx, panelcontent.Frame{
			Size:    panelcontent.Size{Width: p.width, Height: p.height},
			Focused: isFocused,
		})
	}
	return ""
}

func (p *Panel) renderFooter(ctx context.Context, status string, suppress bool) string {
	if suppress {
		return ""
	}
	status = strings.TrimSpace(status)
	renderedFooter := ""
	if widget := p.ensureActiveWidget(ctx); widget != nil {
		if fp, ok := widget.(panelcontent.FooterProvider); ok {
			renderedFooter = fp.Footer(ctx, p.width)
		}
	}
	lines := strings.Split(strings.TrimRight(renderedFooter, "\n"), "\n")
	if len(lines) == 0 {
		if status == "" {
			return ""
		}
		lines = []string{""}
	}
	styled := make([]string, len(lines))
	for i, line := range lines {
		style := uistyles.PanelFooterStyle.Copy().
			Width(p.width).
			Height(1)
		if i == 0 && status != "" {
			statusText := truncateToWidth(status, p.width-1)
			left := truncateToWidth(line, p.width-lipgloss.Width(statusText)-1)
			padding := p.width - lipgloss.Width(left) - lipgloss.Width(statusText)
			if padding < 1 {
				padding = 1
			}
			styled[i] = style.Render(left + strings.Repeat(" ", padding) + statusText)
		} else {
			styled[i] = style.Align(lipgloss.Left).Render(line)
		}
	}
	return strings.Join(styled, "\n")
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

// renderItem renders a single item
func (p *Panel) GetStatus() string {
	if lw := p.listWidget(nil); lw != nil {
		return lw.Status()
	}
	return "Empty"
}

func (p *Panel) GetSelectedItems() []Item {
	if lw := p.listWidget(nil); lw != nil {
		items := lw.Items()
		selected := make([]Item, 0, len(items))
		for _, item := range items {
			if item.Selected {
				selected = append(selected, item)
			}
		}
		return selected
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
		if ctx != nil {
			lw.RefreshFolder(ctx)
		}
		return lw.Items()
	}
	return nil
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
