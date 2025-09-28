package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	viewpkg "github.com/sttts/kc/internal/ui/view"
	"github.com/sttts/kc/pkg/resources"
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
	// Navigation state
	currentPath string
	pathHistory []string
	// Position memory - maps path to position info
	positionMemory map[string]PositionInfo
	// Live data
	nsData              *NamespacesDataSource
	nsWatchCh           <-chan resources.Event
	nsWatchCancel       context.CancelFunc
	resourceData        *GenericDataSource // per-GVK data source
	resourceWatchCh     <-chan resources.Event
	resourceWatchCancel context.CancelFunc
	// Discovery-backed resource infos (precise, from RESTMapper);
	// we keep infos and derive full GVRs when populating items.
	namespacedInfos    []resources.ResourceInfo
	clusterInfos       []resources.ResourceInfo
	genericFactory     func(schema.GroupVersionKind) *GenericDataSource
	currentResourceGVK *schema.GroupVersionKind
	tableHeaders       []string
	tableRows          [][]string
	columnWidths       []int
	tableViewEnabled   bool
	viewConfig         *ViewConfig
	// Optional providers
	contextCountProvider func() int // returns number of contexts, or negative if unknown
}

// PositionInfo stores the cursor position and scroll state for a path
type PositionInfo struct {
	Selected  int
	ScrollTop int
}

// Item represents an item in the panel (file, directory, resource, etc.)
type Item struct {
	Name      string
	Type      ItemType
	Size      string
	Modified  string
	Selected  bool
	GVK       string // deprecated display
	TypedGVK  schema.GroupVersionKind
	TypedGVR  schema.GroupVersionResource
	Enterable bool                 // Whether Enter is meaningful (folder-like)
	Viewer    viewpkg.ViewProvider // Optional: F3 view provider for this item
}

// GetFooterInfo returns the display string for this item in the footer
func (i *Item) GetFooterInfo() string {
	// Prefer precise identity: Group/Version/Resource
	if i.TypedGVR.Version != "" || i.TypedGVR.Group != "" {
		if i.TypedGVR.Group == "" {
			// core group: show only version
			return fmt.Sprintf("%s (%s)", i.Name, i.TypedGVR.Version)
		}
		// show group/version without repeating resource
		return fmt.Sprintf("%s (%s/%s)", i.Name, i.TypedGVR.Group, i.TypedGVR.Version)
	}
	if i.GVK != "" {
		return fmt.Sprintf("%s (%s)", i.Name, i.GVK)
	}
	return i.Name
}

// ItemType represents the type of an item
type ItemType int

const (
	ItemTypeDirectory ItemType = iota
	ItemTypeFile
	ItemTypeResource
	ItemTypeNamespace
	ItemTypeContext
)

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
	}
}

// SetNamespacesDataSource wires a namespaces data source for live listings.
func (p *Panel) SetNamespacesDataSource(ds *NamespacesDataSource) {
	p.nsData = ds
}

// SetPodsDataSource retained for compatibility; prefer SetGenericDataSourceFactory.
func (p *Panel) SetPodsDataSource(ds *PodsDataSource) { /* no-op in generic mode */ }

// SetResourceCatalog injects the namespaced resource catalog (plural -> GVK).
func (p *Panel) SetResourceCatalog(infos []resources.ResourceInfo) {
	p.namespacedInfos = make([]resources.ResourceInfo, 0)
	p.clusterInfos = make([]resources.ResourceInfo, 0)
	for _, info := range infos {
		if info.Namespaced {
			p.namespacedInfos = append(p.namespacedInfos, info)
		} else {
			p.clusterInfos = append(p.clusterInfos, info)
		}
	}
}

// SetGenericDataSourceFactory sets a factory for creating per-GVK data sources.
func (p *Panel) SetGenericDataSourceFactory(factory func(schema.GroupVersionKind) *GenericDataSource) {
	p.genericFactory = factory
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
	// Load initial items
	return p.loadItems()
}

// Update handles messages and updates the panel state
func (p *Panel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case namespacesEventMsg:
		// Live update: reload namespaces and continue watching
		if p.currentPath == "/namespaces" {
			_ = p.loadItemsForPath("/namespaces")
		}
		return p, p.startNamespacesWatch()
	case resourceEventMsg:
		// Live update: reload pods for the namespace in message
		if strings.HasPrefix(p.currentPath, "/namespaces/") && strings.Contains(p.currentPath, "/pods") {
			_ = p.loadItemsForPath(p.currentPath)
		}
		return p, p.startResourceWatch(msg.namespace)
	case tea.KeyMsg:
		switch msg.String() {
		// Navigation keys (Midnight Commander style)
		case "up":
			p.moveUp()
		case "down":
			p.moveDown()
		case "left":
			p.moveUp()
		case "right":
			p.moveDown()
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
			return p, p.enterItem()
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
	// Create header
	header := p.renderHeader()

	// Create content area
	content := p.renderContent()

	// Create footer
	footer := p.renderFooter()

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		content,
		footer,
	)
}

// ViewWithoutHeader renders the panel content and footer only (no header)
func (p *Panel) ViewWithoutHeader() string {
	return p.ViewWithoutHeaderFocused(false)
}

// ViewWithoutHeaderFocused renders the panel content and footer with focus state
func (p *Panel) ViewWithoutHeaderFocused(isFocused bool) string {
	// Create content area
	content := p.renderContentFocused(isFocused)

	// Create footer
	footer := p.renderFooter()

	return lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		footer,
	)
}

// ViewContentOnlyFocused renders just the panel content without header or footer
func (p *Panel) ViewContentOnlyFocused(isFocused bool) string {
	return p.renderContentFocused(isFocused)
}

// GetCurrentPath returns the current path for breadcrumbs
func (p *Panel) GetCurrentPath() string {
	return p.currentPath
}

// GetFooter returns the rendered footer for external use
func (p *Panel) GetFooter() string {
	return p.renderFooter()
}

// SetDimensions sets the panel dimensions
func (p *Panel) SetDimensions(width, height int) {
	p.width = width
	p.height = height
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
func (p *Panel) renderContent() string {
	return p.renderContentFocused(false)
}

// renderContentFocused renders the panel content with focus state
func (p *Panel) renderContentFocused(isFocused bool) string {
	if len(p.items) == 0 {
		return PanelContentStyle.
			Width(p.width).
			Height(p.height).
			Align(lipgloss.Center).
			Render("No items")
	}

	// Calculate visible items
	visibleHeight := p.height - 1
	start := p.scrollTop
	end := start + visibleHeight
	if end > len(p.items) {
		end = len(p.items)
	}

	// Render visible items
	var lines []string
	// Render table header first when applicable (object lists or resource-group lists)
	if p.tableRows != nil && (p.shouldRenderTable() || p.isGroupListView()) && (((strings.HasPrefix(p.currentPath, "/namespaces/") && len(strings.Split(p.currentPath, "/")) >= 4) || p.currentPath == "/namespaces") || p.isGroupListView()) {
		p.columnWidths = p.computeColumnWidths(p.tableHeaders, p.tableRows, p.width-2)
		header := p.formatRow(p.tableHeaders, p.columnWidths)
		// Add one-char prefix to align with type column in rows
		prefixed := " " + header
		if len(prefixed) > p.width {
			prefixed = prefixed[:p.width]
		}
		lines = append(lines, PanelTableHeaderStyle.Width(p.width).Render(prefixed))
	}
	for i := start; i < end; i++ {
		item := p.items[i]
		line := p.renderItem(item, i == p.selected && isFocused)
		lines = append(lines, line)
		if len(lines) >= visibleHeight {
			break
		}
	}

	// Fill remaining space if needed
	for len(lines) < visibleHeight {
		emptyLine := PanelContentStyle.Width(p.width).Render("")
		lines = append(lines, emptyLine)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// isObjectsPath reports whether the current path points at an object listing
// of a resource (cluster-scoped or namespaced) and returns the resource name.
func (p *Panel) isObjectsPath() (string, bool) {
	parts := strings.Split(p.currentPath, "/")
	if len(parts) == 2 && parts[0] == "" && parts[1] != "" { // "/<resource>"
		return parts[1], true
	}
	if len(parts) >= 4 && parts[1] == "namespaces" && parts[3] != "" { // "/namespaces/<ns>/<resource>"
		return parts[3], true
	}
	return "", false
}

// isGroupListView reports whether we're listing resource groups (root or namespace level).
func (p *Panel) isGroupListView() bool {
	if p.currentPath == "/" {
		return true
	}
	parts := strings.Split(p.currentPath, "/")
	return len(parts) == 3 && parts[1] == "namespaces"
}

// buildResourceGroupItems returns resource group items with counts, hiding empty ones.
// If namespace is empty, cluster-scoped resources are listed; otherwise namespaced.
func (p *Panel) buildResourceGroupItems(infos []resources.ResourceInfo, namespace string, skipNamespaceResource bool) []Item {
	if len(infos) == 0 || p.genericFactory == nil {
		return nil
	}
	// Sort deterministically by resource plural
	names := make([]string, 0, len(infos))
	for _, info := range infos {
		names = append(names, info.Resource)
	}
	sort.Strings(names)
	// Map for lookup
	byRes := make(map[string]resources.ResourceInfo, len(infos))
	for _, info := range infos {
		byRes[info.Resource] = info
	}
	var out []Item
	for _, res := range names {
		if skipNamespaceResource && res == "namespaces" {
			continue
		}
		info := byRes[res]
		gvk := info.GVK
		// Filter out resources without list support
		hasList := false
		for _, v := range info.Verbs {
			if v == "list" {
				hasList = true
				break
			}
		}
		if !hasList {
			continue
		}
		ds := p.genericFactory(gvk)
		if ds == nil {
			continue
		}
		// Count via Table when available, fallback to List. On error, include entry
		// with unknown count so the user can still enter.
		count := -1
		if _, rows, _, err := ds.ListTable(namespace); err == nil && rows != nil {
			count = len(rows)
		} else if lst, err := ds.List(namespace); err == nil {
			count = len(lst)
		}
		if count == 0 {
			continue // verified empty -> hide
		}
		size := ""
		if count > 0 {
			size = fmt.Sprintf("%d", count)
		}
		it := Item{Name: res, Type: ItemTypeResource, Size: size, Enterable: true, TypedGVK: gvk}
		it.TypedGVR = schema.GroupVersionResource{Group: gvk.Group, Version: gvk.Version, Resource: res}
		out = append(out, it)
	}
	return out
}

// renderItem renders a single item
func (p *Panel) renderItem(item Item, selected bool) string {
	// Create item line
	var line strings.Builder

	// Item type indicator: show '/' for directories, namespaces, contexts, and explicitly enterable items
	enterable := (item.Type == ItemTypeDirectory) || (item.Type == ItemTypeNamespace) || (item.Type == ItemTypeContext) || item.Enterable
	if enterable {
		line.WriteString("/")
	} else {
		line.WriteString(" ")
	}

	// Item name or table row (object lists or resource-group lists)
	if p.tableRows != nil && item.Name != ".." && (p.isGroupListView() || ((strings.HasPrefix(p.currentPath, "/namespaces/") && len(strings.Split(p.currentPath, "/")) >= 4) || p.currentPath == "/namespaces")) {
		// Determine row index, accounting for optional ".." at top
		idx := p.indexOf(item)
		if idx >= 0 {
			if len(p.items) > 0 && p.items[0].Name == ".." {
				idx--
			}
			if idx >= 0 && idx < len(p.tableRows) {
				// format cells with column widths
				rowStr := p.formatRow(p.tableRows[idx], p.columnWidths)
				line.WriteString(rowStr)
			} else {
				line.WriteString(item.Name)
			}
		} else {
			line.WriteString(item.Name)
		}
	} else {
		line.WriteString(item.Name)
	}

	// Right-align counts for resource-group listings (root and /namespaces/<ns>).
	parts := strings.Split(p.currentPath, "/")
	isGroupListing := (p.currentPath == "/") || (len(parts) == 3 && parts[1] == "namespaces")
	if isGroupListing && p.tableRows == nil {
		prefix := ""
		if (item.Type == ItemTypeDirectory) || (item.Type == ItemTypeNamespace) || (item.Type == ItemTypeContext) || item.Enterable {
			prefix += "/"
		} else {
			prefix += " "
		}
		name := item.Name
		count := item.Size
		rightCol := len(count)
		if rightCol > p.width {
			rightCol = p.width
		}
		leftW := max(0, p.width-rightCol)
		// group/version string in faint white (light gray)
		group := item.TypedGVK.Group
		if group == "" {
			group = "core"
		}
		gv := group + "/" + item.TypedGVK.Version
		gvEsc := "\033[2m\033[37m" + gv + "\033[39m\033[22m"
		prefixW := lipgloss.Width(prefix)
		innerW := max(0, leftW-prefixW)
		showGV := (len(name)+2+len(gv) <= innerW)
		var body strings.Builder
		body.WriteString(name)
		if showGV {
			body.WriteString("  ")
			body.WriteString(gvEsc)
		}
		visibleInner := len(name)
		if showGV {
			visibleInner += 2 + len(gv)
		}
		if visibleInner < innerW {
			body.WriteString(strings.Repeat(" ", innerW-visibleInner))
		}
		// Compose
		line.Reset()
		line.WriteString(prefix)
		line.WriteString(body.String())
		line.WriteString(count)
	} else if (item.Size != "" || item.Modified != "") && !(isGroupListing && p.tableRows != nil) {
		// Generic trailing info: keep simple spacing, trimming to width.
		current := line.String()
		info := strings.TrimSpace(strings.TrimSpace(item.Size + " " + item.Modified))
		// If there is space, insert at the right edge; otherwise, append after a single space.
		if info != "" {
			maxBase := max(0, p.width-len(info))
			if len(current) > maxBase {
				current = current[:maxBase]
			}
			if len(current) < maxBase {
				current += strings.Repeat(" ", maxBase-len(current))
			}
			current += info
			line.Reset()
			line.WriteString(current)
		}
	}

	// Insert a dimmed middle column with group/version for object listings
	if p.tableRows == nil {
		if _, ok := p.isObjectsPath(); ok && item.Type == ItemTypeResource && item.Name != ".." {
			base := line.String()
			group := item.TypedGVK.Group
			if group == "" {
				group = "core"
			}
			gv := group + "/" + item.TypedGVK.Version
			gvEsc := "\033[90m" + gv + "\033[39m"
			mid := p.width / 2
			if mid < len(base)+2 {
				mid = len(base) + 2
			}
			if mid+len(gv) <= p.width {
				if len(base) < mid {
					base += strings.Repeat(" ", mid-len(base))
				}
				base += gvEsc
				if len(base) > p.width {
					base = base[:p.width]
				}
				line.Reset()
				line.WriteString(base)
			}
		}
	}

	// Ensure line doesn't exceed panel width
	lineStr := line.String()
	if len(lineStr) > p.width {
		lineStr = lineStr[:p.width]
	}

	// Apply styling
	style := PanelItemStyle.Width(p.width)
	if selected {
		style = PanelItemSelectedStyle.Width(p.width)
	} else if p.currentPath == "/" && (item.Name == "contexts" || item.Name == "kubeconfigs") {
		// Special bold green label for top entries when not selected
		style = style.Foreground(lipgloss.Green).Bold(true)
	}
	// Highlight multi-selected items (Ctrl+T/Insert) in bold yellow
	if item.Selected {
		style = style.Foreground(lipgloss.Yellow).Bold(true)
	}

	return style.Render(lineStr)
}

// shouldRenderTable returns whether table view is effective considering overrides.
func (p *Panel) shouldRenderTable() bool {
	if p.tableRows == nil {
		return false
	}
	parts := strings.Split(p.currentPath, "/")
	var res string
	if len(parts) >= 4 {
		res = parts[3]
	}
	if p.viewConfig != nil {
		eff := p.viewConfig.Resolve(res)
		switch eff.Table {
		case Yes:
			return true
		case No:
			return false
		}
	}
	return p.tableViewEnabled
}

// indexOf returns the index of the given item by name match.
func (p *Panel) indexOf(target Item) int {
	for i := range p.items {
		if p.items[i].Name == target.Name && p.items[i].Type == target.Type {
			return i
		}
	}
	return -1
}

// isObjectEnterable returns whether objects of a given resource type (plural) are enterable.
func (p *Panel) isObjectEnterable(resource string) bool {
	switch resource {
	case "pods":
		return true // containers/logs subresources
	case "secrets", "configmaps":
		return true // keys-as-files view planned
	default:
		return false
	}
}

// computeColumnWidths determines column widths that fit into the panel width.
func (p *Panel) computeColumnWidths(headers []string, rows [][]string, width int) []int {
	n := len(headers)
	if n == 0 {
		return nil
	}
	widths := make([]int, n)
	for i := 0; i < n; i++ {
		widths[i] = lipgloss.Width(headers[i])
	}
	for _, r := range rows {
		for i := 0; i < n && i < len(r); i++ {
			if l := lipgloss.Width(fmt.Sprint(r[i])); l > widths[i] {
				widths[i] = l
			}
		}
	}
	spaces := n - 1
	budget := width - spaces
	if budget <= n { // minimal 1 char per col
		for i := 0; i < n; i++ {
			widths[i] = 1
		}
		return widths
	}
	sum := 0
	for _, w := range widths {
		sum += w
	}
	if sum <= budget {
		return widths
	}
	// Cap each column to maxPerCol and then reduce widest until fits
	maxPerCol := budget / n
	for i := 0; i < n; i++ {
		if widths[i] > maxPerCol {
			widths[i] = maxPerCol
		}
	}
	sum = 0
	for _, w := range widths {
		sum += w
	}
	// Reduce from widest columns until sum fits
	for sum > budget {
		// find widest index
		idx := 0
		for i := 1; i < n; i++ {
			if widths[i] > widths[idx] {
				idx = i
			}
		}
		if widths[idx] <= 1 {
			break
		}
		widths[idx]--
		sum--
	}
	return widths
}

// formatRow pads/trims cells to widths and joins with a single space.
func (p *Panel) formatRow(cells []string, widths []int) string {
	if widths == nil {
		return strings.Join(cells, "  ")
	}
	n := len(widths)
	out := make([]string, n)
	for i := 0; i < n; i++ {
		var s string
		if i < len(cells) {
			s = fmt.Sprint(cells[i])
		} else {
			s = ""
		}
		w := widths[i]
		vis := lipgloss.Width(s)
		if vis > w {
			for len(s) > 0 && lipgloss.Width(s) > w {
				s = s[:len(s)-1]
			}
			// ensure foreground/bold reset after trimmed ANSI content
			s += "\033[39m\033[22m"
		} else if vis < w {
			s = s + strings.Repeat(" ", w-vis)
		}
		out[i] = s
	}
	row := strings.Join(out, " ")
	if lipgloss.Width(row) > p.width {
		row = row[:p.width]
	}
	return row
}

// renderFooter renders the panel footer
func (p *Panel) renderFooter() string {
	var footerText string

	// Get current item info
	currentItem := p.GetCurrentItem()
	if currentItem != nil {
		footerText = currentItem.GetFooterInfo()
	} else {
		// Fallback to item count
		selectedCount := 0
		for _, item := range p.items {
			if item.Selected {
				selectedCount++
			}
		}
		footerText = fmt.Sprintf("%d/%d items", selectedCount, len(p.items))
	}

	// Do not wrap: hard‑cut to available width
	if lipgloss.Width(footerText) > p.width {
		// naive cut; acceptable for ASCII content
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

// Navigation methods
func (p *Panel) moveUp() {
	if p.selected > 0 {
		p.selected--
		p.adjustScroll()
		// Save position after moving
		p.saveCurrentPosition()
	}
}

func (p *Panel) moveDown() {
	if p.selected < len(p.items)-1 {
		p.selected++
		p.adjustScroll()
		// Save position after moving
		p.saveCurrentPosition()
	}
}

func (p *Panel) adjustScroll() {
	visibleHeight := p.height - 2

	if p.selected < p.scrollTop {
		p.scrollTop = p.selected
	} else if p.selected >= p.scrollTop+visibleHeight {
		p.scrollTop = p.selected - visibleHeight + 1
	}
}

// Selection methods
func (p *Panel) toggleSelection() {
	if p.selected < len(p.items) {
		p.items[p.selected].Selected = !p.items[p.selected].Selected
		// Move cursor down after toggling, staying in bounds and keeping visible
		if p.selected < len(p.items)-1 {
			p.selected++
			p.adjustScroll()
		}
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

// Item interaction
func (p *Panel) enterItem() tea.Cmd {
	if p.selected < len(p.items) {
		item := p.items[p.selected]
		return p.handleItemEnter(item)
	}
	return nil
}

func (p *Panel) handleItemEnter(item Item) tea.Cmd {
	switch item.Type {
	case ItemTypeDirectory:
		return p.enterDirectory(item)
	case ItemTypeResource:
		return p.enterResource(item)
	case ItemTypeNamespace:
		return p.enterNamespace(item)
	case ItemTypeContext:
		return p.enterContext(item)
	default:
		return nil
	}
}

// Navigation methods for item handling
func (p *Panel) enterDirectory(item Item) tea.Cmd {
	// Handle ".." (parent directory)
	if item.Name == ".." {
		return p.goToParent()
	}

	// Navigate to subdirectory
	newPath := p.currentPath
	if newPath == "/" {
		newPath = "/" + item.Name
	} else {
		newPath = newPath + "/" + item.Name
	}

	// When entering a folder, move cursor to ".." (top) in the new view.
	cmd := p.navigateTo(newPath, true) // Add to history when going forward
	p.selected = 0
	p.scrollTop = 0
	return cmd
}

func (p *Panel) enterResource(item Item) tea.Cmd {
	// Navigate into resource listings depending on location.
	switch {
	case strings.HasPrefix(p.currentPath, "/namespaces/"):
		// Namespaced resource listing
		newPath := p.currentPath + "/" + item.Name
		cmd := p.navigateTo(newPath, true)
		p.selected, p.scrollTop = 0, 0
		return cmd
	case p.currentPath == "/":
		// Cluster-scoped resource listing at root
		newPath := "/" + item.Name
		cmd := p.navigateTo(newPath, true)
		p.selected, p.scrollTop = 0, 0
		return cmd
	case strings.HasPrefix(p.currentPath, "/contexts/"):
		// Context-qualified cluster resources (same shape as root, under context)
		newPath := p.currentPath + "/" + item.Name
		cmd := p.navigateTo(newPath, true)
		p.selected, p.scrollTop = 0, 0
		return cmd
	default:
		return nil
	}
}

func (p *Panel) enterNamespace(item Item) tea.Cmd {
	// Navigate into namespace
	newPath := "/namespaces/" + item.Name
	cmd := p.navigateTo(newPath, true) // Add to history when going forward
	p.selected = 0
	p.scrollTop = 0
	return cmd
}

func (p *Panel) enterContext(item Item) tea.Cmd {
	// Switch context
	newPath := "/contexts/" + item.Name
	cmd := p.navigateTo(newPath, true) // Add to history when going forward
	p.selected = 0
	p.scrollTop = 0
	return cmd
}

// goToParent navigates to the parent directory
func (p *Panel) goToParent() tea.Cmd {
	// Go up one level in path
	if p.currentPath == "/" {
		return nil // Already at root
	}

	// Find parent path
	lastSlash := -1
	for i := len(p.currentPath) - 1; i >= 0; i-- {
		if p.currentPath[i] == '/' {
			lastSlash = i
			break
		}
	}

	// Child name we are returning from, to reselect in parent
	childName := p.currentPath
	if lastSlash >= 0 && lastSlash+1 < len(p.currentPath) {
		childName = p.currentPath[lastSlash+1:]
	}

	var newPath string
	if lastSlash <= 0 {
		newPath = "/"
	} else {
		newPath = p.currentPath[:lastSlash]
	}

	cmd := p.navigateTo(newPath, false) // Don't add to history when going back

	// Reselect the child we came from in the parent view
	for i, it := range p.items {
		if it.Name == childName {
			p.selected = i
			// ensure selection is visible
			visibleHeight := max(1, p.height-2)
			if p.selected < p.scrollTop {
				p.scrollTop = p.selected
			} else if p.selected >= p.scrollTop+visibleHeight {
				p.scrollTop = p.selected - visibleHeight + 1
				if p.scrollTop < 0 {
					p.scrollTop = 0
				}
			}
			break
		}
	}

	return cmd
}

// navigateTo navigates to a specific path
func (p *Panel) navigateTo(path string, addToHistory bool) tea.Cmd {
	// Save current position before navigating away
	p.saveCurrentPosition()

	// Stop any active watches when leaving a path
	if p.currentPath == "/namespaces" && path != "/namespaces" {
		p.stopNamespacesWatch()
	}
	if strings.Contains(p.currentPath, "/pods") && !strings.Contains(path, "/pods") {
		p.stopResourceWatch()
	}

	// Add current path to history only if requested
	if addToHistory && p.currentPath != path {
		p.pathHistory = append(p.pathHistory, p.currentPath)
	}
	p.currentPath = path

	// Reload items for the new path
	return p.loadItemsForPath(path)
}

// saveCurrentPosition saves the current cursor position and scroll state
func (p *Panel) saveCurrentPosition() {
	if p.currentPath != "" {
		p.positionMemory[p.currentPath] = PositionInfo{
			Selected:  p.selected,
			ScrollTop: p.scrollTop,
		}
	}
}

// restorePosition restores the cursor position and scroll state for a path
func (p *Panel) restorePosition(path string) {
	if pos, exists := p.positionMemory[path]; exists {
		p.selected = pos.Selected
		p.scrollTop = pos.ScrollTop

		// Ensure position is within bounds
		if p.selected >= len(p.items) {
			p.selected = len(p.items) - 1
		}
		if p.selected < 0 {
			p.selected = 0
		}
		// Ensure scroll position keeps selection visible and within bounds
		visibleHeight := max(1, p.height-2)
		maxScroll := 0
		if len(p.items) > visibleHeight {
			maxScroll = len(p.items) - visibleHeight
		}
		if p.scrollTop < 0 {
			p.scrollTop = 0
		}
		if p.scrollTop > maxScroll {
			p.scrollTop = maxScroll
		}
		// Bring selection into view if needed
		if p.selected < p.scrollTop {
			p.scrollTop = p.selected
		} else if p.selected >= p.scrollTop+visibleHeight {
			p.scrollTop = p.selected - visibleHeight + 1
			if p.scrollTop < 0 {
				p.scrollTop = 0
			}
		}
	} else {
		// No saved position, reset to top
		p.selected = 0
		p.scrollTop = 0
	}
}

// clearPositionMemory clears all saved positions (useful for refresh)
func (p *Panel) clearPositionMemory() {
	p.positionMemory = make(map[string]PositionInfo)
}

// clearPositionForPath clears the saved position for a specific path
func (p *Panel) clearPositionForPath(path string) {
	delete(p.positionMemory, path)
}

// Data loading
func (p *Panel) loadItems() tea.Cmd {
	return p.loadItemsForPath(p.currentPath)
}

// loadItemsForPath loads items for a specific path
func (p *Panel) loadItemsForPath(path string) tea.Cmd {
	p.items = make([]Item, 0)

	// Add parent directory if not at root
	if path != "/" {
		p.items = append(p.items, Item{
			Name:     "..",
			Type:     ItemTypeDirectory,
			Size:     "",
			Modified: "",
		})
	}

	// Load items based on path
	switch path {
	case "/":
		// Root: contexts first (no group/count), then namespaces (real resource row),
		// then other cluster-scoped resources.
		p.items = append(p.items, Item{Name: "contexts", Type: ItemTypeDirectory})
		groups := p.buildResourceGroupItems(p.clusterInfos, "", false)
		// Move namespaces to the top after contexts if present
		nsIdx := -1
		for i, it := range groups {
			if it.Name == "namespaces" {
				nsIdx = i
				break
			}
		}
		if nsIdx >= 0 {
			p.items = append(p.items, groups[nsIdx])
			groups = append(groups[:nsIdx], groups[nsIdx+1:]...)
		}
		p.items = append(p.items, groups...)

		// Prepare table headers/rows for group list view
		p.tableHeaders = []string{"Name", "Group", "Count"}
		startRow := 0
		if len(p.items) > 0 && p.items[0].Name == ".." {
			startRow = 1
		}
		p.tableRows = make([][]string, len(p.items)-startRow)
		for i := startRow; i < len(p.items); i++ {
			it := p.items[i]
			gv := ""
			if it.Name != "contexts" && it.Name != ".." {
				g := it.TypedGVK.Group
				v := it.TypedGVK.Version
				if g == "" {
					gv = v
				} else if v != "" {
					gv = g + "/" + v
				} else {
					gv = g
				}
			}
			gvDim := ""
			if gv != "" {
				gvDim = "\033[2m" + gv + "\033[22m"
			}
			cnt := it.Size
			if it.Name == "contexts" || it.Name == ".." {
				cnt = ""
			}
			p.tableRows[i-startRow] = []string{it.Name, gvDim, cnt}
		}

	case "/contexts":
		// Contexts level - show available contexts
		p.items = append(p.items, []Item{
			{Name: "minikube", Type: ItemTypeContext, Size: "", Modified: "2h"},
			{Name: "docker-desktop", Type: ItemTypeContext, Size: "", Modified: "1h"},
			{Name: "kind-kind", Type: ItemTypeContext, Size: "", Modified: "30m"},
		}...)

	case "/namespaces":
		// Namespaces level - server-side table when available
		if p.nsData != nil {
			if headers, rows, items, err := p.nsData.ListTable(); err == nil {
				p.tableHeaders = headers
				p.tableRows = rows
				for i := range items {
					items[i].TypedGVK = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
					items[i].TypedGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
				}
				p.items = append(p.items, items...)
			} else if items, err := p.nsData.List(); err == nil {
				p.tableHeaders, p.tableRows = nil, nil
				for i := range items {
					items[i].TypedGVK = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
					items[i].TypedGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
				}
				p.items = append(p.items, items...)
			} else {
				p.items = append(p.items, Item{Name: fmt.Sprintf("error: %v", err), Type: ItemTypeDirectory})
			}
			return p.startNamespacesWatch()
		} else {
			p.items = append(p.items, []Item{
				{Name: "default", Type: ItemTypeNamespace, GVK: "v1 Namespace"},
				{Name: "kube-system", Type: ItemTypeNamespace, GVK: "v1 Namespace"},
				{Name: "kube-public", Type: ItemTypeNamespace, GVK: "v1 Namespace"},
			}...)
		}

	case "/cluster-resources":
		// Deprecated: mirror root listing (skip 'namespaces')
		p.items = append(p.items, p.buildResourceGroupItems(p.clusterInfos, "", true)...)

	default:
		// Check if it's a context path
		if len(path) > 10 && path[:10] == "/contexts/" {
			// contextName := path[10:] // TODO: Use context name for actual resource loading
			// Show namespaces and non-empty cluster resources for this context
			p.items = append(p.items, Item{Name: "namespaces", Type: ItemTypeDirectory})
			p.items = append(p.items, p.buildResourceGroupItems(p.clusterInfos, "", true)...)
		} else if len(path) > 12 && path[:12] == "/namespaces/" {
			// /namespaces/<ns>[/<resource>]
			parts := strings.Split(path, "/")
			if len(parts) == 3 {
				// namespace level: list resource groups from discovery, hide empties and show counts
				ns := parts[2]
				p.items = append(p.items, p.buildResourceGroupItems(p.namespacedInfos, ns, false)...)
				// Group list table for namespace
				p.tableHeaders = []string{"Name", "Group", "Count"}
				startRow := 0
				if len(p.items) > 0 && p.items[0].Name == ".." {
					startRow = 1
				}
				p.tableRows = make([][]string, len(p.items)-startRow)
				for i := startRow; i < len(p.items); i++ {
					it := p.items[i]
					g := it.TypedGVK.Group
					v := it.TypedGVK.Version
					gv := ""
					if g == "" {
						gv = v
					} else if v != "" {
						gv = g + "/" + v
					} else {
						gv = g
					}
					gvDim := ""
					if gv != "" {
						gvDim = "\033[2m" + gv + "\033[22m"
					}
					cnt := it.Size
					if it.Name == ".." {
						cnt = ""
					}
					p.tableRows[i-startRow] = []string{it.Name, gvDim, cnt}
				}
				if len(p.items) == 1 && p.items[0].Name == ".." {
					// no resources, leave empty indicator
					p.items = append(p.items, Item{Name: "(no resources)", Type: ItemTypeDirectory})
				}
			} else if len(parts) == 2 {
				// Cluster-scoped resource objects: "/<resource>"
				res := parts[1]
				var gvk schema.GroupVersionKind
				found := false
				for _, info := range p.clusterInfos {
					if info.Resource == res {
						gvk = info.GVK
						found = true
						break
					}
				}
				if found && p.genericFactory != nil {
					if p.currentResourceGVK == nil || *p.currentResourceGVK != gvk {
						p.resourceData = p.genericFactory(gvk)
						p.currentResourceGVK = &gvk
					}
					if p.resourceData != nil {
						if headers, rows, items, err := p.resourceData.ListTable(""); err == nil {
							p.tableHeaders, p.tableRows = headers, rows
							for i := range items {
								items[i].Enterable = p.isObjectEnterable(res)
								items[i].TypedGVK = *p.currentResourceGVK
								items[i].TypedGVR = schema.GroupVersionResource{Group: p.currentResourceGVK.Group, Version: p.currentResourceGVK.Version, Resource: res}
							}
							p.items = append(p.items, items...)
						} else if items, err := p.resourceData.List(""); err == nil {
							p.tableHeaders, p.tableRows = nil, nil
							for i := range items {
								items[i].Enterable = p.isObjectEnterable(res)
								items[i].TypedGVK = *p.currentResourceGVK
								items[i].TypedGVR = schema.GroupVersionResource{Group: p.currentResourceGVK.Group, Version: p.currentResourceGVK.Version, Resource: res}
							}
							p.items = append(p.items, items...)
						}
						return nil
					}
				}
			} else if len(parts) >= 4 {
				ns := parts[2]
				res := parts[3]
				// Object-level navigation
				if len(parts) == 5 {
					name := parts[4]
					if res == "pods" && p.genericFactory != nil {
						// Show containers for the pod
						var gvk schema.GroupVersionKind
						found := false
						for _, info := range p.namespacedInfos {
							if info.Resource == res {
								gvk = info.GVK
								found = true
								break
							}
						}
						if found {
							ds := p.genericFactory(gvk)
							if ds != nil {
								if obj, err := ds.Get(ns, name); err == nil && obj != nil {
									// containers
									if arr, found, _ := unstructured.NestedSlice(obj.Object, "spec", "containers"); found {
										for _, c := range arr {
											if m, ok := c.(map[string]interface{}); ok {
												if n, ok := m["name"].(string); ok {
													p.items = append(p.items, Item{Name: n, Type: ItemTypeDirectory, Enterable: true, Viewer: &viewpkg.PodContainerView{Namespace: ns, Pod: name, Container: n}})
												}
											}
										}
									}
									// initContainers
									if arr, found, _ := unstructured.NestedSlice(obj.Object, "spec", "initContainers"); found {
										for _, c := range arr {
											if m, ok := c.(map[string]interface{}); ok {
												if n, ok := m["name"].(string); ok {
													p.items = append(p.items, Item{Name: n, Type: ItemTypeDirectory, Enterable: true, Viewer: &viewpkg.PodContainerView{Namespace: ns, Pod: name, Container: n}})
												}
											}
										}
									}
								}
							}
						}
					} else if (res == "configmaps" || res == "secrets") && p.genericFactory != nil {
						var gvk schema.GroupVersionKind
						found := false
						for _, info := range p.namespacedInfos {
							if info.Resource == res {
								gvk = info.GVK
								found = true
								break
							}
						}
						if found {
							ds := p.genericFactory(gvk)
							if ds != nil {
								if obj, err := ds.Get(ns, name); err == nil && obj != nil {
									// list data keys
									if data, found, _ := unstructured.NestedMap(obj.Object, "data"); found {
										keys := make([]string, 0, len(data))
										for k := range data {
											keys = append(keys, k)
										}
										sort.Strings(keys)
										for _, k := range keys {
											p.items = append(p.items, Item{Name: k, Type: ItemTypeFile, Viewer: &viewpkg.ConfigKeyView{Namespace: ns, Name: name, Key: k, IsSecret: res == "secrets"}})
										}
									}
								}
							}
						}
					}
					// Additional container sub-view: /namespaces/<ns>/pods/<pod>/<container>
					if len(parts) == 6 && res == "pods" {
						// Inside a container folder: show a logs entry
						p.items = append(p.items, Item{Name: "logs", Type: ItemTypeFile})
						return nil
					}
					// Done with object view
					return nil
				}
				var gvk schema.GroupVersionKind
				found := false
				for _, info := range p.namespacedInfos {
					if info.Resource == res {
						gvk = info.GVK
						found = true
						break
					}
				}
				if found && p.genericFactory != nil {
					if p.currentResourceGVK == nil || *p.currentResourceGVK != gvk {
						p.resourceData = p.genericFactory(gvk)
						p.currentResourceGVK = &gvk
					}
				}
				if p.resourceData != nil {
					// Prefer server-side table if available
					if headers, rows, items, err := p.resourceData.ListTable(ns); err == nil {
						p.tableHeaders = headers
						p.tableRows = rows
						// Mark enterable per-resource policy and set typed GVK
						for i := range items {
							items[i].Enterable = p.isObjectEnterable(res)
							if p.currentResourceGVK != nil {
								items[i].TypedGVK = *p.currentResourceGVK
								items[i].TypedGVR = schema.GroupVersionResource{Group: p.currentResourceGVK.Group, Version: p.currentResourceGVK.Version, Resource: res}
							}
						}
						p.items = append(p.items, items...)
					} else if items, err := p.resourceData.List(ns); err == nil {
						p.tableHeaders, p.tableRows = nil, nil
						for i := range items {
							items[i].Enterable = p.isObjectEnterable(res)
							if p.currentResourceGVK != nil {
								items[i].TypedGVK = *p.currentResourceGVK
								items[i].TypedGVR = schema.GroupVersionResource{Group: p.currentResourceGVK.Group, Version: p.currentResourceGVK.Version, Resource: res}
							}
						}
						p.items = append(p.items, items...)
					} else {
						p.items = append(p.items, Item{Name: fmt.Sprintf("error: %v", err), Type: ItemTypeDirectory})
					}
					return p.startResourceWatch(ns)
				}
			}
		} else {
			// Unknown path - show empty
			p.items = append(p.items, Item{
				Name:     "Empty",
				Type:     ItemTypeDirectory,
				Size:     "",
				Modified: "",
			})
		}
	}

	// Restore position for this path
	p.restorePosition(path)

	return nil
}

// namespacesEventMsg signals that namespaces changed; payload not needed for a reload.
type namespacesEventMsg struct{}
type resourceEventMsg struct{ namespace string }

// startNamespacesWatch sets up a watch loop and returns a Cmd to await the first event.
func (p *Panel) startNamespacesWatch() tea.Cmd {
	// If an existing watch is present, keep it.
	if p.nsWatchCh == nil && p.nsData != nil {
		ctx, cancel := context.WithCancel(context.Background())
		ch, stop, err := p.nsData.Watch(ctx)
		if err != nil {
			cancel()
		} else {
			p.nsWatchCh = ch
			p.nsWatchCancel = func() { stop(); cancel() }
		}
	}
	if p.nsWatchCh == nil {
		return nil
	}
	return func() tea.Msg {
		// Block until next event or channel close; then signal UI to reload.
		if _, ok := <-p.nsWatchCh; ok {
			return namespacesEventMsg{}
		}
		return namespacesEventMsg{}
	}
}

// stopNamespacesWatch cancels the namespaces watch if running.
func (p *Panel) stopNamespacesWatch() {
	if p.nsWatchCancel != nil {
		p.nsWatchCancel()
		p.nsWatchCancel = nil
		p.nsWatchCh = nil
	}
}

// startResourceWatch watches a namespaced resource list (currently pods) in a namespace.
func (p *Panel) startResourceWatch(ns string) tea.Cmd {
	if p.resourceData == nil {
		return nil
	}
	if p.resourceWatchCh == nil {
		ctx, cancel := context.WithCancel(context.Background())
		ch, stop, err := p.resourceData.Watch(ctx, ns)
		if err == nil {
			p.resourceWatchCh = ch
			p.resourceWatchCancel = func() { stop(); cancel() }
		} else {
			cancel()
		}
	}
	if p.resourceWatchCh == nil {
		return nil
	}
	return func() tea.Msg {
		if _, ok := <-p.resourceWatchCh; ok {
			return resourceEventMsg{namespace: ns}
		}
		return resourceEventMsg{namespace: ns}
	}
}

func (p *Panel) stopResourceWatch() {
	if p.resourceWatchCancel != nil {
		p.resourceWatchCancel()
		p.resourceWatchCancel = nil
		p.resourceWatchCh = nil
	}
}

// Getters
func (p *Panel) GetTitle() string {
	return p.title
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
	return p.loadItems()
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

// Helper functions
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
