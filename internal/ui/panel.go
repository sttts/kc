package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	kccluster "github.com/sttts/kc/internal/cluster"
	models "github.com/sttts/kc/internal/models"
	table "github.com/sttts/kc/internal/table"
	viewpkg "github.com/sttts/kc/internal/ui/view"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Panel represents a file/resource panel
type Panel struct {
	title     string
	items     []Item
	selected  int
	scrollTop int
	width     int
	height    int
	// BigTable for folder-backed content rendering
	bt *table.BigTable
	// Navigation state
	currentPath string
	pathHistory []string
	// Position memory - maps path to position info
	positionMemory map[string]PositionInfo
	// Discovery-backed resource infos (precise, from RESTMapper);
	// we keep infos and derive full GVRs when populating items.

	// Legacy generic data hooks (kept as no-ops; real data comes from folders)

	tableViewEnabled bool
	viewConfig       *ViewConfig
	tableMode        table.GridMode
	// Optional providers
	contextCountProvider func() int // returns number of contexts, or negative if unknown
	// Optional: folder-backed rendering (preview path using internal/navigation)
	useFolder     bool
	folder        models.Folder
	folderHasBack bool
	folderHandler func(back bool, selID string, next models.Folder)
	// Per-panel resource group view options
	resShowNonEmpty bool
	resOrder        string // "alpha", "group", "favorites"
	lastColTitles   []string
	columnsMode     string // "normal" or "wide"
	objOrder        string // "name", "-name", "creation", "-creation"
}

const panelContextTimeout = 250 * time.Millisecond

// PositionInfo stores the cursor position and scroll state for a path
type PositionInfo struct {
	Selected  int
	ScrollTop int
}

// Item represents an item in the panel (file, directory, resource, etc.)
type Item struct {
	models.Item
	Name     string
	Selected bool
	Viewer   viewpkg.ViewProvider // Optional: F3 view provider for this item
}

// NewPanel creates a new panel
func NewPanel(title string) *Panel {
	return &Panel{
		title:            title,
		items:            make([]Item, 0),
		selected:         0,
		scrollTop:        0,
		currentPath:      "/",
		pathHistory:      make([]string, 0),
		positionMemory:   make(map[string]PositionInfo),
		tableViewEnabled: true,
		resOrder:         "favorites",
		tableMode:        table.ModeScroll,
		columnsMode:      "normal",
		objOrder:         "name",
	}
}

// SetResourceViewOptions sets the per-panel view toggles for resource groups.
func (p *Panel) SetResourceViewOptions(showNonEmpty bool, order string) {
	p.resShowNonEmpty = showNonEmpty
	switch order {
	case "alpha", "group", "favorites":
		p.resOrder = order
	default:
		p.resOrder = "favorites"
	}
}

// ResourceViewOptions returns current per-panel options.
func (p *Panel) ResourceViewOptions() (bool, string) { return p.resShowNonEmpty, p.resOrder }

// ResetSelectionTop moves the cursor to the top and resets scrolling.

func (p *Panel) ResetSelectionTop(ctx context.Context) {
	p.selected = 0
	p.scrollTop = 0
	if p.useFolder && p.folder != nil && p.bt != nil {
		if p.folderHasBack {
			p.bt.Select(ctx, "__back__")
		} else {
			rows := p.folderLines(ctx, 0, 1)
			if len(rows) > 0 {
				if id, _, _, ok := rows[0].Columns(); ok {
					p.bt.Select(ctx, id)
				}
			}
		}
	}
}

// SetFolder enables folder-backed rendering using the new navigation package.
// This does not alter legacy behaviors beyond rendering headers/rows from the
// folder for preview purposes. Selection/enter logic remains unchanged.
func (p *Panel) SetFolder(ctx context.Context, f models.Folder, hasBack bool) {
	p.folder = f
	p.folderHasBack = hasBack
	// Initialize or refresh BigTable from folder columns and data when enabled
	if p.useFolder && p.folder != nil {
		// Force population so Columns() reflects server-provided headers
		_ = p.folderLen(ctx)
		cols := p.folder.Columns()
		p.lastColTitles = columnsToTitles(cols)
		bt := table.NewBigTable(cols, p.folder, max(1, p.width), max(1, p.height))
		bt.SetMode(ctx, p.tableMode)
		// Apply panel-aligned styles
		st := table.DefaultStyles()
		st.Header = PanelTableHeaderStyle
		st.Cell = PanelItemStyle
		st.Selector = PanelItemSelectedStyle                                   // cursor highlight
		st.Marked = lipgloss.NewStyle().Foreground(lipgloss.Yellow).Bold(true) // multi-select style
		// Match outer frame border color (white) for inner verticals
		st.Border = lipgloss.NewStyle().
			Foreground(lipgloss.White).
			Background(lipgloss.Blue).
			BorderForeground(lipgloss.White).
			BorderBackground(lipgloss.Blue)
		bt.SetStyles(st)
		// Enable custom vertical separators that adopt the row background.
		bt.BorderVertical(ctx, true)
		p.bt = &bt
	} else {
		p.bt = nil
	}
}

// UseFolder toggles folder-backed rendering.
func (p *Panel) UseFolder(on bool) { p.useFolder = on }

// ClearFolder disables folder-backed rendering and clears current folder.
func (p *Panel) ClearFolder() { p.folder = nil; p.folderHasBack = false; p.useFolder = false }

// syncFromFolder rebuilds table headers/rows and a minimal items slice from
// the current folder so existing rendering paths can display it.
func (p *Panel) syncFromFolder(ctx context.Context) {
	if !p.useFolder || p.folder == nil {
		return
	}

	rowCount := p.folderLen(ctx)
	rows := p.folderLines(ctx, 0, rowCount)

	isKeysFolder := false
	var parentNS, parentName string
	var isSecret bool
	if kf, ok := p.folder.(models.KeyFolder); ok {
		gvr, ns, name := kf.Parent()
		if gvr.Resource == "configmaps" || gvr.Resource == "secrets" {
			isKeysFolder = true
			parentNS, parentName = ns, name
			isSecret = (gvr.Resource == "secrets")
		}
	}

	items := make([]Item, 0, len(rows)+1)
	for _, row := range rows {
		if back, ok := row.(models.Back); ok && back.IsBack() {
			if itemRow, ok := row.(models.Item); ok {
				items = append(items, Item{Item: itemRow, Name: ".."})
			} else {
				items = append(items, Item{Name: ".."})
			}
			continue
		}
		itemRow, ok := row.(models.Item)
		if !ok {
			continue
		}
		_, rcells, _, _ := itemRow.Columns()
		displayName := ""
		if len(rcells) > 0 {
			displayName = rcells[0]
			if strings.HasPrefix(displayName, "/") {
				displayName = strings.TrimPrefix(displayName, "/")
			}
		}
		entry := Item{
			Item: itemRow,
			Name: displayName,
		}
		if isKeysFolder && displayName != "" {
			entry.Viewer = &viewpkg.ConfigKeyView{Namespace: parentNS, Name: parentName, Key: displayName, IsSecret: isSecret}
		}
		items = append(items, entry)
	}

	p.items = items
}

// SetTableMode updates the panel's table rendering mode ("scroll" or "fit").
// It applies immediately to an existing BigTable instance.
func (p *Panel) SetTableMode(ctx context.Context, mode string) {
	m := strings.ToLower(mode)
	switch m {
	case "fit":
		p.tableMode = table.ModeFit
	default:
		p.tableMode = table.ModeScroll
	}
	if p.bt != nil {
		p.bt.SetMode(ctx, p.tableMode)
	}
}

// TableMode returns the current mode label ("scroll" or "fit").
func (p *Panel) TableMode() string {
	if p.tableMode == table.ModeFit {
		return "fit"
	}
	return "scroll"
}

// SetColumnsMode updates which server-side table columns to show (normal or wide).
func (p *Panel) SetColumnsMode(ctx context.Context, mode string) {
	if strings.EqualFold(mode, "wide") {
		p.columnsMode = "wide"
	} else {
		p.columnsMode = "normal"
	}
	// Rebuild BigTable headers on next refresh via folder change detection.
	p.RefreshFolder(ctx)
}

// ColumnsMode returns the current columns mode label.
func (p *Panel) ColumnsMode() string { return p.columnsMode }

// SetObjectOrder updates object list ordering mode.
func (p *Panel) SetObjectOrder(ctx context.Context, order string) {
	switch strings.ToLower(order) {
	case "name", "-name", "creation", "-creation":
		p.objOrder = strings.ToLower(order)
	default:
		p.objOrder = "name"
	}
	if p.folder != nil {
		p.RefreshFolder(ctx)
	}
}

func (p *Panel) ObjectOrder() string { return p.objOrder }

// SelectByRowID moves the selection to the row with the given ID if present.
// It matches against the folder's row IDs and adjusts for the synthetic back row.
func (p *Panel) SelectByRowID(ctx context.Context, id string) {
	if !p.useFolder || p.folder == nil || id == "" {
		p.ResetSelectionTop(ctx)
		return
	}
	// Ensure items reflect current folder
	p.syncFromFolder(ctx)
	// Find the absolute row index in the (wrapped) folder
	rowCount := p.folderLen(ctx)
	rows := p.folderLines(ctx, 0, rowCount)
	idx := -1
	for i := range rows {
		rid, _, _, _ := rows[i].Columns()
		if rid == id {
			idx = i
			break
		}
	}
	// Fallback
	if idx < 0 {
		p.ResetSelectionTop(ctx)
		return
	}
	// Wrapped folder already includes back row at index 0; selection index equals row index
	sel := idx
	if sel < 0 {
		sel = 0
	}
	if sel >= len(p.items) {
		sel = len(p.items) - 1
	}
	p.selected = sel
	p.adjustScroll()
	if p.bt != nil {
		p.bt.Select(ctx, id)
	}
}

func (p *Panel) selectedRowID(ctx context.Context) string {
	if !p.useFolder || p.folder == nil {
		return ""
	}
	if p.bt != nil {
		if id, ok := p.bt.CurrentID(ctx); ok {
			return id
		}
	}
	limit := p.folderLen(ctx)
	if p.selected < 0 || p.selected >= limit {
		return ""
	}
	rows := p.folderLines(ctx, p.selected, 1)
	if len(rows) == 0 {
		return ""
	}
	id, _, _, ok := rows[0].Columns()
	if !ok {
		return ""
	}
	return id
}

// SelectedNavItem resolves the currently focused navigation item, skipping the
// synthetic back entry. Returns false when no concrete item is selected.
func (p *Panel) SelectedNavItem(ctx context.Context) (models.Item, bool) {
	if !p.useFolder || p.folder == nil {
		return nil, false
	}
	p.syncFromFolder(ctx)
	id := p.selectedRowID(ctx)
	if id == "" {
		return nil, false
	}
	item, ok := p.folderItemByID(ctx, id)
	if !ok || item == nil {
		return nil, false
	}
	if back, ok := item.(models.Back); ok && back.IsBack() {
		return nil, false
	}
	return item, true
}

// SetFolderNavHandler installs a callback invoked when Enter is pressed while
// folder-backed rendering is active. The callback receives whether a back
// navigation was requested and, if not back, the next Folder (may be nil).
func (p *Panel) SetFolderNavHandler(h func(back bool, selID string, next models.Folder)) {
	p.folderHandler = h
}

// RefreshFolder refreshes the BigTable rows from the current folder list.
// Used by periodic ticks to reflect informer-driven changes with a max 1s delay.
func (p *Panel) RefreshFolder(ctx context.Context) {
	if p.useFolder && p.folder != nil && p.bt != nil {
		// If folder's visible columns changed (e.g., server-side Table columns),
		// rebuild the BigTable with the new headers.
		// Ensure folder data/columns are current before comparing
		_ = p.folderLen(ctx)
		newCols := p.folder.Columns()
		// Compare titles only (width hints are advisory)
		titles := columnsToTitles(newCols)
		same := len(titles) == len(p.lastColTitles)
		if same {
			for i := range titles {
				if titles[i] != p.lastColTitles[i] {
					same = false
					break
				}
			}
		}
		if !same {
			bt := table.NewBigTable(newCols, p.folder, max(1, p.width), max(1, p.height))
			bt.SetMode(ctx, p.tableMode)
			p.lastColTitles = titles
			st := table.DefaultStyles()
			st.Header = PanelTableHeaderStyle
			st.Cell = PanelItemStyle
			st.Selector = PanelItemSelectedStyle
			st.Marked = lipgloss.NewStyle().Foreground(lipgloss.Yellow).Bold(true)
			st.Border = lipgloss.NewStyle().
				Foreground(lipgloss.White).
				Background(lipgloss.Blue).
				BorderForeground(lipgloss.White).
				BorderBackground(lipgloss.Blue)
			bt.SetStyles(st)
			bt.BorderVertical(ctx, true)
			p.bt = &bt
		} else {
			p.bt.SetList(ctx, p.folder)
			p.bt.Refresh(ctx)
		}
	}
}

// SetNamespacesDataSource wires a namespaces data source for live listings.
// Legacy live data sources removed; folders drive listings now.

// SetPodsDataSource retained for compatibility; prefer SetGenericDataSourceFactory.
// Legacy shim type and setter for backwards compatibility.
type PodsDataSource struct{}

func (p *Panel) SetPodsDataSource(ds *PodsDataSource) { /* deprecated */ }

// SetResourceCatalog injects the namespaced resource catalog (plural -> GVK).
func (p *Panel) SetResourceCatalog(infos []kccluster.ResourceInfo) {
	// no-op; folders supply resource information directly
}

// SetGenericDataSourceFactory sets a factory for creating per-GVK data sources.
// Deprecated; no-op now that folders provide data directly.
func (p *Panel) SetGenericDataSourceFactory(factory func(schema.GroupVersionKind) *GenericDataSource) {
}

// renderFooter renders the panel footer
func (p *Panel) renderFooter(ctx context.Context) string {
	var footerText string

	currentItem := p.GetCurrentItem()
	if currentItem != nil {
		if p.useFolder && p.folder != nil {
			rowCount := p.folderLen(ctx)
			rows := p.folderLines(ctx, 0, rowCount)
			if p.selected >= 0 && p.selected < len(rows) {
				if id, _, _, ok := rows[p.selected].Columns(); ok && id != "__back__" {
					if detailer, ok := rows[p.selected].(interface{ Details() string }); ok {
						if d := detailer.Details(); d != "" {
							footerText = d
						}
					}
				}
			}
		}
		if footerText == "" && currentItem.Item != nil {
			if d := currentItem.Details(); d != "" {
				footerText = d
			}
		}
		if footerText == "" {
			name := currentItem.Name
			if name == "" && currentItem.Item != nil {
				if _, cells, _, ok := currentItem.Columns(); ok && len(cells) > 0 {
					name = cells[0]
				}
			}
			footerText = name
		}
	} else {
		selectedCount := 0
		for _, item := range p.items {
			if item.Selected {
				selectedCount++
			}
		}
		footerText = fmt.Sprintf("%d/%d items", selectedCount, len(p.items))
	}

	if lipgloss.Width(footerText) > p.width {
		if p.width >= 0 && p.width < len(footerText) {
			footerText = footerText[:p.width]
		}
	}

	return PanelFooterStyle.
		Width(p.width).
		Height(1).
		Align(lipgloss.Left).
		Render(footerText)
}

// --- Legacy datasource shims (no-ops) ---------------------------------------

// nsEvent/resEvent are local event shims to avoid old resources.Event type.
type nsEvent struct{}
type resEvent struct{}

// NamespacesDataSource is a legacy interface placeholder.
type NamespacesDataSource struct{}

func (n *NamespacesDataSource) ListTable() ([]string, [][]string, []Item, error) {
	return nil, nil, nil, fmt.Errorf("not supported")
}
func (n *NamespacesDataSource) List() ([]Item, error) { return nil, fmt.Errorf("not supported") }
func (n *NamespacesDataSource) Watch(ctx context.Context) (<-chan nsEvent, func(), error) {
	ch := make(chan nsEvent)
	close(ch)
	stop := func() {}
	return ch, stop, nil
}

// GenericDataSource is a legacy data source placeholder.
type GenericDataSource struct{}

func (g *GenericDataSource) ListTable(ns string) ([]string, [][]string, []Item, error) {
	return nil, nil, nil, fmt.Errorf("not supported")
}
func (g *GenericDataSource) List(ns string) ([]Item, error) { return nil, fmt.Errorf("not supported") }
func (g *GenericDataSource) Get(ns, name string) (*unstructured.Unstructured, error) {
	return nil, fmt.Errorf("not supported")
}
func (g *GenericDataSource) Watch(ctx context.Context, ns string) (<-chan resEvent, func(), error) {
	ch := make(chan resEvent)
	close(ch)
	stop := func() {}
	return ch, stop, nil
}

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
	return nil
}

// Update handles messages and updates the panel state
func (p *Panel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithTimeout(context.Background(), panelContextTimeout)
	defer cancel()
	switch msg := msg.(type) {
	// Legacy watch events removed; folders handle refresh separately.
	case tea.KeyMsg:
		// When using folder-backed rendering with BigTable, route navigation/selection keys to it
		if p.useFolder && p.folder != nil && p.bt != nil {
			key := msg.String()
			switch key {
			case "up", "down", "left", "right", "home", "end", "pgup", "pgdown", "ctrl+t", "insert":
				_, _ = p.bt.UpdateWithContext(ctx, msg)
				if id, ok := p.bt.CurrentID(ctx); ok {
					if item, ok := p.folderItemByID(ctx, id); ok {
						if back, ok := item.(models.Back); ok && back.IsBack() {
							p.selected = 0
							p.scrollTop = 0
						} else {
							p.SelectByRowID(ctx, id)
						}
					}
				}
				return p, nil
			}
		}
		switch msg.String() {
		// Navigation keys (Midnight Commander style)
		case "up":
			p.moveUp(ctx)
		case "down":
			p.moveDown(ctx)
		case "left":
			p.moveUp(ctx)
		case "right":
			p.moveDown(ctx)
		case "home":
			p.moveToTop()
		case "end":
			p.moveToBottom()
		case "pgup":
			p.pageUp()
		case "pgdown":
			p.pageDown()

		// Selection keys
		case "enter":
			return p, p.enterItem(ctx)
		case "ctrl+t", "insert":
			p.toggleSelection()
		case "ctrl+a":
			p.selectAll()
		case "ctrl+r":
			return p, p.refresh()
		case "ctrl+v":
			// Toggle table view rendering on resource lists
			p.tableViewEnabled = !p.tableViewEnabled
			return p, p.refresh()
		case "*":
			p.invertSelection()
		case "+", "-":
			return p, p.showGlobPatternDialog(msg.String())

		// Function keys (handled by app)
		case "f2":
			return p, p.showResourceSelector()
		case "f3":
			return p, p.viewItem()
		case "f4":
			return p, p.editItem()
		case "f7":
			return p, p.createNamespace()
		case "f8":
			return p, p.deleteItem()
		case "f9":
			return p, p.showContextMenu()
		}
	}

	return p, nil
}

// View renders the panel
func (p *Panel) View() string {
	ctx, cancel := context.WithTimeout(context.Background(), panelContextTimeout)
	defer cancel()

	header := p.renderHeader()
	content := p.renderContent(ctx)
	footer := p.renderFooter(ctx)

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
	return p.viewWithoutHeaderWithContext(ctx, false)
}

// ViewWithoutHeaderFocused renders the panel content and footer with focus state
func (p *Panel) ViewWithoutHeaderFocused(isFocused bool) string {
	ctx, cancel := context.WithTimeout(context.Background(), panelContextTimeout)
	defer cancel()
	return p.viewWithoutHeaderWithContext(ctx, isFocused)
}

func (p *Panel) viewWithoutHeaderWithContext(ctx context.Context, isFocused bool) string {
	content := p.renderContentFocused(ctx, isFocused)
	footer := p.renderFooter(ctx)
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
	return p.renderFooter(ctx)
}

// SetDimensions sets the panel dimensions
func (p *Panel) SetDimensions(ctx context.Context, width, height int) {
	p.width = width
	p.height = height
	if p.bt != nil {
		p.bt.SetSize(ctx, max(1, width), max(1, height))
	}
}

// renderHeader renders the panel header
func (p *Panel) renderHeader() string {
	// Show current path as breadcrumbs
	headerText := p.ellipsizePath(p.currentPath, p.width)

	headerStyle := PanelHeaderStyle.
		Width(p.width).
		Height(1).
		Align(lipgloss.Left)

	return headerStyle.Render(headerText)
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
	// If a folder is set for preview, use the BigTable view directly.
	if p.useFolder && p.folder != nil {
		p.syncFromFolder(ctx)
		if p.bt == nil {
			cols := p.folder.Columns()
			p.lastColTitles = columnsToTitles(cols)
			bt := table.NewBigTable(cols, p.folder, max(1, p.width), max(1, p.height))
			bt.SetMode(ctx, p.tableMode)
			st := table.DefaultStyles()
			st.Header = PanelTableHeaderStyle
			st.Cell = PanelItemStyle
			st.Selector = PanelItemSelectedStyle
			st.Marked = lipgloss.NewStyle().Foreground(lipgloss.Yellow).Bold(true)
			st.Border = lipgloss.NewStyle().
				Foreground(lipgloss.White).
				Background(lipgloss.Blue).
				BorderForeground(lipgloss.White).
				BorderBackground(lipgloss.Blue)
			bt.SetStyles(st)
			bt.BorderVertical(ctx, true)
			p.bt = &bt
		} else {
			p.bt.SetList(ctx, p.folder)
			p.bt.SetSize(ctx, max(1, p.width), max(1, p.height))
		}
		p.bt.SetFocused(ctx, isFocused)
		return lipgloss.NewStyle().
			Background(lipgloss.Blue).
			Width(p.width).
			Height(p.height).
			Render(p.bt.View())
	}

	if len(p.items) == 0 {
		return PanelContentStyle.
			Width(p.width).
			Height(p.height).
			Align(lipgloss.Center).
			Render("No items")
	}

	visibleHeight := max(0, p.height-1)
	start := p.scrollTop
	end := start + visibleHeight
	if end > len(p.items) {
		end = len(p.items)
	}
	var lines []string
	for i := start; i < end; i++ {
		lines = append(lines, p.renderItem(p.items[i], i == p.selected && isFocused))
	}
	for len(lines) < visibleHeight {
		lines = append(lines, PanelContentStyle.Width(p.width).Render(""))
	}
	if len(lines) == 0 {
		lines = append(lines, PanelContentStyle.Width(p.width).Render(""))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderItem renders a single item
func (p *Panel) renderItem(item Item, selected bool) string {
	name := item.Name
	if name == "" && item.Item != nil {
		if _, cells, _, ok := item.Columns(); ok {
			if len(cells) > 0 {
				name = cells[0]
			}
		}
	}
	prefix := " "
	if item.Item != nil {
		if back, ok := item.Item.(models.Back); ok && back.IsBack() {
			prefix = "/"
			name = ".."
		} else if _, ok := item.Item.(models.Enterable); ok {
			prefix = "/"
		}
	}
	text := prefix + name
	if len(text) > p.width {
		text = text[:p.width]
	}
	style := PanelItemStyle.Width(p.width)
	if selected {
		style = PanelItemSelectedStyle.Width(p.width)
	}
	if item.Selected {
		style = style.Foreground(lipgloss.Yellow).Bold(true)
	}
	return style.Render(text)
}

func (p *Panel) GetStatus() string {
	if len(p.items) == 0 {
		return "Empty"
	}
	return fmt.Sprintf("%d items", len(p.items))
}

func (p *Panel) GetSelectedItems() []Item {
	var selected []Item
	for _, item := range p.items {
		if item.Selected {
			selected = append(selected, item)
		}
	}
	return selected
}

func (p *Panel) GetCurrentItem() *Item {
	if p.selected < len(p.items) {
		return &p.items[p.selected]
	}
	return nil
}
func (p *Panel) toggleSelection() {
	if len(p.items) == 0 || p.selected < 0 || p.selected >= len(p.items) {
		return
	}
	p.items[p.selected].Selected = !p.items[p.selected].Selected
	if p.items[p.selected].Selected && p.selected < len(p.items)-1 {
		p.selected++
		p.adjustScroll()
	}
}

func (p *Panel) selectAll() {
	for i := range p.items {
		p.items[i].Selected = true
	}
}

func (p *Panel) unselectAll() {
	for i := range p.items {
		p.items[i].Selected = false
	}
}

func (p *Panel) invertSelection() {
	for i := range p.items {
		p.items[i].Selected = !p.items[i].Selected
	}
}

func (p *Panel) moveUp(ctx context.Context) {
	if p.selected > 0 {
		p.selected--
		p.adjustScroll()
	}
}

func (p *Panel) moveDown(ctx context.Context) {
	if p.selected < len(p.items)-1 {
		p.selected++
		p.adjustScroll()
	}
}

func (p *Panel) adjustScroll() {
	visible := max(1, p.height-2)
	if p.selected < p.scrollTop {
		p.scrollTop = p.selected
	} else if p.selected >= p.scrollTop+visible {
		p.scrollTop = p.selected - visible + 1
		if p.scrollTop < 0 {
			p.scrollTop = 0
		}
	}
	p.saveCurrentPosition()
}

func (p *Panel) enterItem(ctx context.Context) tea.Cmd {
	if !p.useFolder || p.folder == nil || p.folderHandler == nil {
		return nil
	}

	p.syncFromFolder(ctx)
	if len(p.items) == 0 {
		return nil
	}
	if p.selected < 0 {
		p.selected = 0
	}
	if p.selected >= len(p.items) {
		p.selected = len(p.items) - 1
	}

	rows := p.folderLines(ctx, 0, p.folderLen(ctx))
	if p.selected >= len(rows) {
		return nil
	}
	row := rows[p.selected]
	id, _, _, _ := row.Columns()
	if back, ok := row.(models.Back); ok && back.IsBack() {
		p.folderHandler(true, id, nil)
		return nil
	}
	enterable, ok := row.(models.Enterable)
	if !ok {
		return nil
	}
	next, err := enterable.Enter()
	if err != nil || next == nil {
		return nil
	}
	p.folderHandler(false, id, next)
	return nil
}

func (p *Panel) saveCurrentPosition() {
	if p.currentPath == "" {
		return
	}
	p.positionMemory[p.currentPath] = PositionInfo{
		Selected:  p.selected,
		ScrollTop: p.scrollTop,
	}
}

func (p *Panel) restorePosition(path string) {
	if pos, exists := p.positionMemory[path]; exists {
		p.selected = pos.Selected
		p.scrollTop = pos.ScrollTop
		visibleHeight := max(1, p.height-2)
		if p.selected >= len(p.items) {
			p.selected = len(p.items) - 1
		}
		if p.selected < 0 {
			p.selected = 0
		}
		if p.scrollTop < 0 {
			p.scrollTop = 0
		}
		maxScroll := 0
		if len(p.items) > visibleHeight {
			maxScroll = len(p.items) - visibleHeight
		}
		if p.scrollTop > maxScroll {
			p.scrollTop = maxScroll
		}
		if p.selected < p.scrollTop {
			p.scrollTop = p.selected
		} else if p.selected >= p.scrollTop+visibleHeight {
			p.scrollTop = p.selected - visibleHeight + 1
			if p.scrollTop < 0 {
				p.scrollTop = 0
			}
		}
	} else {
		p.selected = 0
		p.scrollTop = 0
	}
}

func (p *Panel) clearPositionMemory() {
	p.positionMemory = make(map[string]PositionInfo)
}

func (p *Panel) clearPositionForPath(path string) {
	delete(p.positionMemory, path)
}

// Navigation methods
func (p *Panel) moveToTop() {
	p.selected = 0
	p.scrollTop = 0
	p.saveCurrentPosition()
}

func (p *Panel) moveToBottom() {
	if len(p.items) > 0 {
		p.selected = len(p.items) - 1
		p.scrollTop = max(0, len(p.items)-p.height+3) // +3 for header and footer
	}
	p.saveCurrentPosition()
}

func (p *Panel) pageUp() {
	pageSize := p.height - 3 // -3 for header and footer
	p.selected = max(0, p.selected-pageSize)
	p.scrollTop = max(0, p.scrollTop-pageSize)
	p.saveCurrentPosition()
}

func (p *Panel) pageDown() {
	pageSize := p.height - 3 // -3 for header and footer
	p.selected = min(len(p.items)-1, p.selected+pageSize)
	p.scrollTop = min(max(0, len(p.items)-p.height+3), p.scrollTop+pageSize)
	p.saveCurrentPosition()
}

// Action methods
func (p *Panel) refresh() tea.Cmd {
	// Clear position memory when refreshing to ensure fresh state
	p.clearPositionMemory()
	return nil
}

func (p *Panel) showResourceSelector() tea.Cmd {
	// TODO: Implement resource selector
	return nil
}

func (p *Panel) viewItem() tea.Cmd {
	// TODO: Implement view functionality
	return nil
}

func (p *Panel) editItem() tea.Cmd {
	// TODO: Implement edit functionality
	return nil
}

func (p *Panel) createNamespace() tea.Cmd {
	// TODO: Implement namespace creation
	return nil
}

func (p *Panel) deleteItem() tea.Cmd {
	// TODO: Implement delete functionality
	return nil
}

func (p *Panel) showContextMenu() tea.Cmd {
	// TODO: Implement context menu
	return nil
}

func (p *Panel) showGlobPatternDialog(key string) tea.Cmd {
	// TODO: Implement glob pattern dialog
	// + for include pattern, - for exclude pattern
	return nil
}

// ColumnsToTitles extracts column titles for legacy renderers that expect []string.
func columnsToTitles(cols []table.Column) []string {
	out := make([]string, len(cols))
	for i := range cols {
		out[i] = cols[i].Title
	}
	return out
}

// rowsToCells converts []table.Row into [][]string of cells for legacy renderers.
// It drops styles and pads missing cells with empty strings to the max column count.
func (p *Panel) folderLen(ctx context.Context) int {
	if !p.useFolder || p.folder == nil {
		return 0
	}
	return p.folder.Len(ctx)
}

func (p *Panel) folderLines(ctx context.Context, top, num int) []table.Row {
	if !p.useFolder || p.folder == nil {
		return nil
	}
	return p.folder.Lines(ctx, top, num)
}

func (p *Panel) folderItemByID(ctx context.Context, id string) (models.Item, bool) {
	if !p.useFolder || p.folder == nil {
		return nil, false
	}
	return p.folder.ItemByID(ctx, id)
}

func (p *Panel) folderFind(ctx context.Context, id string) (int, table.Row, bool) {
	if !p.useFolder || p.folder == nil {
		return -1, nil, false
	}
	return p.folder.Find(ctx, id)
}

func (p *Panel) folderAbove(ctx context.Context, id string, n int) []table.Row {
	if !p.useFolder || p.folder == nil {
		return nil
	}
	return p.folder.Above(ctx, id, n)
}

func (p *Panel) folderBelow(ctx context.Context, id string, n int) []table.Row {
	if !p.useFolder || p.folder == nil {
		return nil
	}
	return p.folder.Below(ctx, id, n)
}

func rowsToCells(rows []table.Row) [][]string {
	// determine max columns across rows
	maxCols := 0
	tmp := make([][]string, len(rows))
	for i, r := range rows {
		_, cells, _, _ := r.Columns()
		tmp[i] = cells
		if len(cells) > maxCols {
			maxCols = len(cells)
		}
	}
	out := make([][]string, len(rows))
	for i := range rows {
		cells := tmp[i]
		if len(cells) == maxCols {
			out[i] = cells
			continue
		}
		padded := make([]string, maxCols)
		copy(padded, cells)
		out[i] = padded
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
