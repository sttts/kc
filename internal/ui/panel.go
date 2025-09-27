package ui

import (
    "fmt"
    "strings"
    "context"

    tea "github.com/charmbracelet/bubbletea/v2"
    "github.com/charmbracelet/lipgloss/v2"
    "github.com/sschimanski/kc/pkg/resources"
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
    nsData *NamespacesDataSource
    nsWatchCh <-chan resources.Event
    nsWatchCancel context.CancelFunc
    resourceData *GenericDataSource // generic per-GVK data source
    resourceWatchCh <-chan resources.Event
    resourceWatchCancel context.CancelFunc
    // Discovery-backed catalog of namespaced resources (plural -> GVK)
    namespacedCatalog map[string]schema.GroupVersionKind
    genericFactory func(schema.GroupVersionKind) *GenericDataSource
    currentResourceGVK *schema.GroupVersionKind
}

// PositionInfo stores the cursor position and scroll state for a path
type PositionInfo struct {
	Selected  int
	ScrollTop int
}

// Item represents an item in the panel (file, directory, resource, etc.)
type Item struct {
	Name     string
	Type     ItemType
	Size     string
	Modified string
	Selected bool
	GVK      string // Group-Version-Kind for Kubernetes resources
}

// GetFooterInfo returns the display string for this item in the footer
func (i *Item) GetFooterInfo() string {
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
        title:          title,
        items:          make([]Item, 0),
        selected:       0,
        scrollTop:      0,
        currentPath:    "/",
        pathHistory:    make([]string, 0),
        positionMemory: make(map[string]PositionInfo),
    }
}

// SetNamespacesDataSource wires a namespaces data source for live listings.
func (p *Panel) SetNamespacesDataSource(ds *NamespacesDataSource) {
    p.nsData = ds
}

// SetPodsDataSource wires a pods data source for live listings.
func (p *Panel) SetPodsDataSource(ds *PodsDataSource) { p.resourceData = ds }

// SetResourceCatalog injects the namespaced resource catalog (plural -> GVK).
func (p *Panel) SetResourceCatalog(infos []resources.ResourceInfo) {
    p.namespacedCatalog = make(map[string]schema.GroupVersionKind)
    for _, info := range infos {
        if info.Namespaced {
            p.namespacedCatalog[info.Resource] = info.GVK
        }
    }
}

// SetGenericDataSourceFactory sets a factory for creating per-GVK data sources.
func (p *Panel) SetGenericDataSourceFactory(factory func(schema.GroupVersionKind) *GenericDataSource) {
    p.genericFactory = factory
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
	headerText := p.currentPath

	headerStyle := PanelHeaderStyle.
		Width(p.width).
		Height(1).
		Align(lipgloss.Left)

	return headerStyle.Render(headerText)
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
	for i := start; i < end; i++ {
		item := p.items[i]
		line := p.renderItem(item, i == p.selected && isFocused)
		lines = append(lines, line)
	}

	// Fill remaining space if needed
	for len(lines) < visibleHeight {
		emptyLine := PanelContentStyle.Width(p.width).Render("")
		lines = append(lines, emptyLine)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderItem renders a single item
func (p *Panel) renderItem(item Item, selected bool) string {
	// Create item line
	var line strings.Builder

	// Selection indicator
	if item.Selected {
		line.WriteString("*")
	} else {
		line.WriteString(" ")
	}

    // Item type indicator: anything enterable should look like a folder ("/")
    enterable := item.Type != ItemTypeFile
    if enterable {
        line.WriteString("/")
    } else {
        line.WriteString(" ")
    }

	// Item name
	line.WriteString(item.Name)

	// Size and modified time (if available)
	if item.Size != "" || item.Modified != "" {
		// Calculate available space for size/modified info
		currentLength := len(line.String())
		// Reserve 20 characters for size and modified time
		targetWidth := p.width - 20
		if targetWidth > currentLength {
			line.WriteString(strings.Repeat(" ", targetWidth-currentLength))
		}
		if item.Size != "" {
			line.WriteString(item.Size)
		}
		if item.Modified != "" {
			line.WriteString(" ")
			line.WriteString(item.Modified)
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
	}

	return style.Render(lineStr)
}

// renderFooter renders the panel footer
func (p *Panel) renderFooter() string {
	var footerText string

	// Get current item info
	currentItem := p.GetCurrentItem()
	if currentItem != nil {
		footerText = currentItem.GetFooterInfo()

		// Add path context if not at root
		if p.currentPath != "/" {
			footerText += fmt.Sprintf(" | %s", p.currentPath)
		}
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

	return p.navigateTo(newPath, true) // Add to history when going forward
}

func (p *Panel) enterResource(item Item) tea.Cmd {
    // Navigate into a resource within the current namespace path.
    if strings.HasPrefix(p.currentPath, "/namespaces/") {
        newPath := p.currentPath + "/" + item.Name
        return p.navigateTo(newPath, true)
    }
    return nil
}

func (p *Panel) enterNamespace(item Item) tea.Cmd {
	// Navigate into namespace
	newPath := "/namespaces/" + item.Name
	return p.navigateTo(newPath, true) // Add to history when going forward
}

func (p *Panel) enterContext(item Item) tea.Cmd {
	// Switch context
	newPath := "/contexts/" + item.Name
	return p.navigateTo(newPath, true) // Add to history when going forward
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

	var newPath string
	if lastSlash <= 0 {
		newPath = "/"
	} else {
		newPath = p.currentPath[:lastSlash]
	}

	return p.navigateTo(newPath, false) // Don't add to history when going back
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

		// Ensure scroll position is valid
		if p.scrollTop < 0 {
			p.scrollTop = 0
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
		// Root level - show contexts and cluster resources
		p.items = append(p.items, []Item{
			{Name: "contexts", Type: ItemTypeDirectory, Size: "", Modified: ""},
			{Name: "namespaces", Type: ItemTypeDirectory, Size: "", Modified: ""},
			{Name: "cluster-resources", Type: ItemTypeDirectory, Size: "", Modified: ""},
		}...)

	case "/contexts":
		// Contexts level - show available contexts
		p.items = append(p.items, []Item{
			{Name: "minikube", Type: ItemTypeContext, Size: "", Modified: "2h"},
			{Name: "docker-desktop", Type: ItemTypeContext, Size: "", Modified: "1h"},
			{Name: "kind-kind", Type: ItemTypeContext, Size: "", Modified: "30m"},
		}...)

    case "/namespaces":
        // Namespaces level - show available namespaces (live via data source if available)
        if p.nsData != nil {
            if items, err := p.nsData.List(); err == nil {
                p.items = append(p.items, items...)
            } else {
                // Fallback to placeholder on error
                p.items = append(p.items, Item{Name: fmt.Sprintf("error: %v", err), Type: ItemTypeDirectory})
            }
            // Start watching for live updates
            return p.startNamespacesWatch()
        } else {
            p.items = append(p.items, []Item{
                {Name: "default", Type: ItemTypeNamespace, Size: "", Modified: "", GVK: "v1 Namespace"},
                {Name: "kube-system", Type: ItemTypeNamespace, Size: "", Modified: "", GVK: "v1 Namespace"},
                {Name: "kube-public", Type: ItemTypeNamespace, Size: "", Modified: "", GVK: "v1 Namespace"},
            }...)
        }

	case "/cluster-resources":
		// Cluster resources level - show cluster-wide resources
		p.items = append(p.items, []Item{
			{Name: "nodes", Type: ItemTypeResource, Size: "3", Modified: "5m", GVK: "v1 Node"},
			{Name: "persistentvolumes", Type: ItemTypeResource, Size: "1", Modified: "10m", GVK: "v1 PersistentVolume"},
			{Name: "storageclasses", Type: ItemTypeResource, Size: "2", Modified: "15m", GVK: "storage.k8s.io/v1 StorageClass"},
		}...)

	default:
		// Check if it's a context path
		if len(path) > 10 && path[:10] == "/contexts/" {
			// contextName := path[10:] // TODO: Use context name for actual resource loading
			// Show namespaces and cluster resources for this context
			p.items = append(p.items, []Item{
				{Name: "namespaces", Type: ItemTypeDirectory, Size: "", Modified: ""},
				{Name: "cluster-resources", Type: ItemTypeDirectory, Size: "", Modified: ""},
			}...)
        } else if len(path) > 12 && path[:12] == "/namespaces/" {
            // /namespaces/<ns>[/<resource>]
            parts := strings.Split(path, "/")
            if len(parts) == 3 {
                // namespace level: list resource groups
                if len(p.namespacedCatalog) > 0 {
                    // Render resources from discovery
                    for res := range p.namespacedCatalog {
                        p.items = append(p.items, Item{Name: res, Type: ItemTypeResource})
                    }
                } else {
                    // Fallback for now
                    p.items = append(p.items, []Item{{Name: "pods", Type: ItemTypeResource}}...)
                }
            } else if len(parts) >= 4 {
                ns := parts[2]
                res := parts[3]
                // Resolve GVK from catalog and (re)create generic data source when needed
                if gvk, ok := p.namespacedCatalog[res]; ok && p.genericFactory != nil {
                    if p.currentResourceGVK == nil || *p.currentResourceGVK != gvk {
                        p.resourceData = p.genericFactory(gvk)
                        p.currentResourceGVK = &gvk
                    }
                }
                if p.resourceData != nil {
                    if items, err := p.resourceData.List(ns); err == nil {
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
            p.nsWatchCancel = func(){ stop(); cancel() }
        }
    }
    if p.nsWatchCh == nil { return nil }
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
    if p.resourceData == nil { return nil }
    if p.resourceWatchCh == nil {
        ctx, cancel := context.WithCancel(context.Background())
        ch, stop, err := p.resourceData.Watch(ctx, ns)
        if err == nil {
            p.resourceWatchCh = ch
            p.resourceWatchCancel = func(){ stop(); cancel() }
        } else {
            cancel()
        }
    }
    if p.resourceWatchCh == nil { return nil }
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
