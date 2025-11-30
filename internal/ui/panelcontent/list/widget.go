package list

import (
	"context"
	"fmt"
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	models "github.com/sttts/kc/internal/models"
	table "github.com/sttts/kc/internal/table"
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
	uistyles "github.com/sttts/kc/internal/ui/styles"
)

// Widget implements the list mode content for panels.
type Widget struct {
	deps panelcontent.WidgetDeps

	width  int
	height int

	items    []Item
	selected int
	scroll   int

	tableViewEnabled bool
	tableMode        table.GridMode
	columnsMode      string
	objOrder         string
	resShowNonEmpty  bool
	resOrder         string

	useFolder     bool
	folder        models.Folder
	folderHasBack bool
	folderHandler func(back bool, selID string, next models.Folder)

	bt            *table.BigTable
	lastColTitles []string

	positionMemory map[string]PositionInfo
}

func displayNameFromItem(it models.Item) string {
	if it == nil {
		return ""
	}
	_, cells, _, ok := it.Columns()
	if !ok || len(cells) == 0 {
		return ""
	}
	return strings.TrimPrefix(cells[0], "/")
}

// New creates a list widget with the provided dependencies.
func New(deps panelcontent.WidgetDeps) *Widget {
	return &Widget{
		deps:             deps,
		items:            make([]Item, 0),
		tableViewEnabled: true,
		tableMode:        table.ModeScroll,
		columnsMode:      "normal",
		objOrder:         "name",
		resOrder:         "favorites",
		positionMemory:   make(map[string]PositionInfo),
	}
}

func (w *Widget) Init(context.Context) tea.Cmd { return nil }

func (w *Widget) Update(ctx context.Context, msg tea.Msg) (tea.Cmd, bool) {
	prev := w.CurrentSelectionID(ctx)
	var cmd tea.Cmd
	var handled bool
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		w.width = m.Width
		w.height = m.Height
		if w.bt != nil {
			w.bt.SetSize(ctx, max(1, m.Width), max(1, m.Height))
		}
		return nil, true
	case tea.KeyMsg:
		cmd, handled = w.handleKey(ctx, m)
	case panelcontent.MouseMsg:
		cmd, handled = w.handleMouse(ctx, m)
	}
	if !handled {
		return nil, false
	}
	return w.selectionCmd(ctx, prev, cmd), true
}

func (w *Widget) View(ctx context.Context, frame panelcontent.Frame) tea.View {
	w.width = frame.Size.Width
	w.height = frame.Size.Height
	return tea.NewView(w.render(ctx, frame.Focused))
}

func (w *Widget) SetFocus(ctx context.Context, focused bool) {
	if w.bt != nil {
		w.bt.SetFocused(ctx, focused)
	}
}

func (w *Widget) Teardown(context.Context) {}

func (w *Widget) Items() []Item {
	cp := make([]Item, len(w.items))
	copy(cp, w.items)
	return cp
}

func (w *Widget) SelectedItems(ctx context.Context) []Item {
	if w.useFolder && w.folder != nil && w.bt != nil {
		ids := w.bt.SelectedIDs()
		if len(ids) == 0 {
			return nil
		}
		selected := make([]Item, 0, len(ids))
		for _, id := range ids {
			item, ok := w.folder.ItemByID(ctx, id)
			if !ok || item == nil {
				continue
			}
			name := displayNameFromItem(item)
			selected = append(selected, Item{Item: item, Name: name, Selected: true})
		}
		return selected
	}
	items := w.Items()
	selected := make([]Item, 0, len(items))
	for _, item := range items {
		if item.Selected {
			selected = append(selected, item)
		}
	}
	return selected
}

func (w *Widget) ColumnTitles() []string {
	return append([]string(nil), w.lastColTitles...)
}

func (w *Widget) CurrentItem(context.Context) *Item {
	if w.selected < 0 || w.selected >= len(w.items) {
		return nil
	}
	item := w.items[w.selected]
	return &item
}

func (w *Widget) Status() string {
	if len(w.items) == 0 {
		return "Empty"
	}
	return fmt.Sprintf("%d items", len(w.items))
}

func (w *Widget) Footer(ctx context.Context, width int) string {
	footerText := ""
	if current := w.CurrentItem(ctx); current != nil {
		if w.useFolder && w.folder != nil {
			rowCount := w.folderLen(ctx)
			rows := w.folderLines(ctx, 0, rowCount)
			if w.selected >= 0 && w.selected < len(rows) {
				if id, _, _, ok := rows[w.selected].Columns(); ok && id != "__back__" {
					if detailer, ok := rows[w.selected].(interface{ Details() string }); ok {
						if d := detailer.Details(); d != "" {
							footerText = d
						}
					}
				}
			}
		}
		if footerText == "" && current.Item != nil {
			if d := current.Details(); d != "" {
				footerText = d
			}
		}
		if footerText == "" {
			name := current.Name
			if name == "" && current.Item != nil {
				if _, cells, _, ok := current.Columns(); ok && len(cells) > 0 {
					name = cells[0]
				}
			}
			footerText = name
		}
	} else {
		selectedCount := 0
		for _, item := range w.items {
			if item.Selected {
				selectedCount++
			}
		}
		footerText = fmt.Sprintf("%d/%d items", selectedCount, len(w.items))
	}
	if lipgloss.Width(footerText) > width {
		if width >= 0 && width < len(footerText) {
			footerText = footerText[:width]
		}
	}
	return uistyles.PanelFooterStyle.
		Width(width).
		Height(1).
		Align(lipgloss.Left).
		Render(footerText)
}

func (w *Widget) FrameInfo(ctx context.Context, req panelcontent.FrameInfoRequest) panelcontent.FrameInfo {
	info := panelcontent.FrameInfo{
		TopIndicator:    "─",
		BottomIndicator: "─",
	}
	if len(w.items) == 0 {
		return info
	}
	ordinals := make([]int, len(w.items))
	totalReal := 0
	for i := range w.items {
		if isBackItem(w.items[i]) {
			continue
		}
		totalReal++
		ordinals[i] = totalReal
	}
	if totalReal == 0 {
		return info
	}
	current := 0
	if w.selected >= 0 && w.selected < len(ordinals) {
		current = ordinals[w.selected]
	}
	visibleStart := w.scroll
	visibleEnd := w.scroll + w.visibleHeight()
	if visibleEnd > len(ordinals) {
		visibleEnd = len(ordinals)
	}
	bottomOrdinal := current
	for i := visibleStart; i < visibleEnd; i++ {
		if ordinals[i] > bottomOrdinal {
			bottomOrdinal = ordinals[i]
		}
	}
	percent := 0
	if totalReal > 0 {
		percent = int(math.Round(float64(bottomOrdinal) * 100 / float64(totalReal)))
		if percent < 0 {
			percent = 0
		}
		if percent > 100 {
			percent = 100
		}
	}
	topMore := false
	for i := 0; i < visibleStart; i++ {
		if ordinals[i] > 0 {
			topMore = true
			break
		}
	}
	bottomMore := false
	for i := visibleEnd; i < len(ordinals); i++ {
		if ordinals[i] > 0 {
			bottomMore = true
			break
		}
	}
	if topMore {
		info.TopIndicator = "^"
	}
	if bottomMore {
		info.BottomIndicator = "v"
	}
	info.FooterStatus = fmt.Sprintf("%d/%d • %d%%", current, totalReal, percent)
	return info
}

func (w *Widget) CurrentSelectionID(ctx context.Context) string {
	if id := w.selectedRowID(ctx); id != "" {
		return id
	}
	if item := w.CurrentItem(ctx); item != nil {
		if item.Item != nil {
			if rid, _, _, ok := item.Item.Columns(); ok && rid != "" {
				return rid
			}
		}
		if item.Name != "" {
			return item.Name
		}
	}
	return ""
}

func (w *Widget) SelectedNavItem(ctx context.Context) (models.Item, bool) {
	if !w.useFolder || w.folder == nil {
		if current := w.CurrentItem(ctx); current != nil && current.Item != nil {
			if back, ok := current.Item.(models.Back); ok && back.IsBack() {
				return nil, false
			}
			if navItem, ok := current.Item.(models.Item); ok {
				return navItem, true
			}
		}
		return nil, false
	}
	w.syncFromFolder(ctx)
	id := w.selectedRowID(ctx)
	if id == "" {
		return nil, false
	}
	item, ok := w.folderItemByID(ctx, id)
	if !ok || item == nil {
		return nil, false
	}
	if back, ok := item.(models.Back); ok && back.IsBack() {
		return nil, false
	}
	return item, true
}

// Folder exposes the current backing folder when folder mode is active.
func (w *Widget) Folder() models.Folder {
	return w.folder
}

// Enter triggers folder navigation for the current selection when applicable.
func (w *Widget) Enter(ctx context.Context) tea.Cmd {
	return w.enterItem(ctx)
}

func (w *Widget) SetFolder(ctx context.Context, f models.Folder, hasBack bool) {
	w.folder = f
	w.folderHasBack = hasBack
	w.applyObjectOrderToFolder()
	if w.useFolder && w.folder != nil {
		w.ensureBigTable(ctx)
	} else {
		w.bt = nil
	}
	w.syncFromFolder(ctx)
}

func (w *Widget) SetFolderNavHandler(h func(back bool, selID string, next models.Folder)) {
	w.folderHandler = h
}

func (w *Widget) UseFolder(on bool) {
	w.useFolder = on
	if !on {
		w.bt = nil
		return
	}
	w.applyObjectOrderToFolder()
}

func (w *Widget) ClearFolder() {
	w.folder = nil
	w.folderHasBack = false
	w.useFolder = false
	w.bt = nil
}

func (w *Widget) RefreshFolder(ctx context.Context) {
	if !w.useFolder || w.folder == nil {
		return
	}
	if w.bt == nil {
		w.ensureBigTable(ctx)
		return
	}
	_ = w.folderLen(ctx)
	newCols := w.folder.Columns()
	titles := columnsToTitles(newCols)
	same := len(titles) == len(w.lastColTitles)
	if same {
		for i := range titles {
			if titles[i] != w.lastColTitles[i] {
				same = false
				break
			}
		}
	}
	if !same {
		bt := table.NewBigTable(newCols, w.folder, max(1, w.width), max(1, w.height))
		bt.SetMode(ctx, w.tableMode)
		w.lastColTitles = titles
		w.decorateBigTable(ctx, &bt)
		w.bt = &bt
	} else {
		w.bt.SetList(ctx, w.folder)
		w.bt.Refresh(ctx)
	}
	w.syncFromFolder(ctx)
}

func (w *Widget) SetResourceViewOptions(showNonEmpty bool, order string) {
	w.resShowNonEmpty = showNonEmpty
	switch order {
	case "alpha", "group", "favorites":
		w.resOrder = order
	default:
		w.resOrder = "favorites"
	}
}

func (w *Widget) ResourceViewOptions() (bool, string) {
	return w.resShowNonEmpty, w.resOrder
}

func (w *Widget) SetTableMode(ctx context.Context, mode string) {
	switch strings.ToLower(mode) {
	case "fit":
		w.tableMode = table.ModeFit
	default:
		w.tableMode = table.ModeScroll
	}
	if w.bt != nil {
		w.bt.SetMode(ctx, w.tableMode)
	}
}

func (w *Widget) TableMode() string {
	if w.tableMode == table.ModeFit {
		return "fit"
	}
	return "scroll"
}

func (w *Widget) SetColumnsMode(ctx context.Context, mode string) {
	if strings.EqualFold(mode, "wide") {
		w.columnsMode = "wide"
	} else {
		w.columnsMode = "normal"
	}
	w.RefreshFolder(ctx)
}

func (w *Widget) ColumnsMode() string { return w.columnsMode }

func (w *Widget) SetObjectOrder(ctx context.Context, order string) {
	w.objOrder = models.NormalizeObjectOrder(order)
	w.applyObjectOrderToFolder()
	w.RefreshFolder(ctx)
}

func (w *Widget) ObjectOrder() string { return w.objOrder }

func (w *Widget) applyObjectOrderToFolder() {
	if !w.useFolder || w.folder == nil {
		return
	}
	if configurable, ok := w.folder.(models.ObjectOrderConfigurable); ok {
		configurable.ApplyObjectOrder(w.objOrder)
	}
}

func (w *Widget) ToggleColumnsMode(ctx context.Context) tea.Cmd {
	if w.columnsMode == "wide" {
		w.SetColumnsMode(ctx, "normal")
	} else {
		w.SetColumnsMode(ctx, "wide")
	}
	return nil
}

func (w *Widget) SelectByRowID(ctx context.Context, id string) {
	if !w.useFolder || w.folder == nil || id == "" {
		w.ResetSelection(ctx)
		return
	}
	w.syncFromFolder(ctx)
	rowCount := w.folderLen(ctx)
	rows := w.folderLines(ctx, 0, rowCount)
	idx := -1
	for i := range rows {
		rid, _, _, _ := rows[i].Columns()
		if rid == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		w.ResetSelection(ctx)
		return
	}
	sel := idx
	if sel < 0 {
		sel = 0
	}
	if sel >= len(w.items) {
		sel = len(w.items) - 1
	}
	w.selected = sel
	w.adjustScroll()
	if w.bt != nil {
		w.bt.Select(ctx, id)
	}
	w.saveCurrentPosition()
}

// SelectRowIDs marks multiple row IDs as selected, focusing the first match.
func (w *Widget) SelectRowIDs(ctx context.Context, ids []string) {
	clean := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		w.ResetSelection(ctx)
		return
	}
	if !w.useFolder || w.folder == nil {
		return
	}
	w.syncFromFolder(ctx)
	rowCount := w.folderLen(ctx)
	rows := w.folderLines(ctx, 0, rowCount)
	firstIdx := -1
	firstID := ""
	matches := make(map[string]struct{}, len(clean))
	for _, id := range clean {
		matches[id] = struct{}{}
	}
	rowIDs := make([]string, len(rows))
	if w.bt != nil {
		w.bt.ClearMarks()
	}
	for idx := range rows {
		rowID, _, _, ok := rows[idx].Columns()
		if !ok || rowID == "" {
			continue
		}
		rowIDs[idx] = rowID
		if _, ok := matches[rowID]; ok {
			if idx < len(w.items) {
				w.items[idx].Selected = true
			}
			if firstIdx == -1 {
				firstIdx = idx
				firstID = rowID
			}
			w.toggleBigTableSelection(ctx, rowID, true)
		} else if idx < len(w.items) {
			w.items[idx].Selected = false
		}
	}
	if firstIdx == -1 {
		w.ResetSelection(ctx)
		return
	}
	w.selected = firstIdx
	w.adjustScroll()
	if w.bt != nil && firstID != "" {
		w.bt.Select(ctx, firstID)
	}
	w.saveCurrentPosition()
}

func (w *Widget) toggleBigTableSelection(ctx context.Context, id string, add bool) bool {
	if w.bt == nil || id == "" {
		return false
	}
	if add {
		return w.bt.Mark(ctx, id, true)
	}
	return w.bt.Mark(ctx, id, false)
}

func (w *Widget) ResetSelection(ctx context.Context) {
	w.selected = 0
	w.scroll = 0
	if w.useFolder && w.folder != nil && w.bt != nil {
		if w.folderHasBack {
			w.bt.Select(ctx, "__back__")
		} else {
			rows := w.folderLines(ctx, 0, 1)
			if len(rows) > 0 {
				if id, _, _, ok := rows[0].Columns(); ok {
					w.bt.Select(ctx, id)
				}
			}
		}
	}
	w.saveCurrentPosition()
}

func (w *Widget) RestorePosition() {
	path := w.currentPath()
	if path == "" {
		w.selected = 0
		w.scroll = 0
		return
	}
	if pos, ok := w.positionMemory[path]; ok {
		w.selected = pos.Selected
		w.scroll = pos.ScrollTop
		visible := w.visibleHeight()
		if w.selected >= len(w.items) {
			w.selected = len(w.items) - 1
		}
		if w.selected < 0 {
			w.selected = 0
		}
		if w.scroll < 0 {
			w.scroll = 0
		}
		maxScroll := 0
		if len(w.items) > visible {
			maxScroll = len(w.items) - visible
		}
		if w.scroll > maxScroll {
			w.scroll = maxScroll
		}
		if w.selected < w.scroll {
			w.scroll = w.selected
		} else if w.selected >= w.scroll+visible {
			w.scroll = w.selected - visible + 1
			if w.scroll < 0 {
				w.scroll = 0
			}
		}
	} else {
		w.selected = 0
		w.scroll = 0
	}
}

func (w *Widget) ClearPositionMemory() {
	w.positionMemory = make(map[string]PositionInfo)
}

func (w *Widget) CurrentItems() []Item { return w.Items() }

// --- input handling --------------------------------------------------------

func (w *Widget) handleKey(ctx context.Context, m tea.KeyMsg) (tea.Cmd, bool) {
	key := m.String()
	if w.useFolder && w.folder != nil && w.bt != nil {
		switch key {
		case "up", "down", "left", "right", "home", "end", "pgup", "pgdown", "ctrl+t", "insert":
			_, _ = w.bt.UpdateWithContext(ctx, m)
			if id, ok := w.bt.CurrentID(ctx); ok {
				if item, ok := w.folderItemByID(ctx, id); ok {
					if back, ok := item.(models.Back); ok && back.IsBack() {
						w.selected = 0
						w.scroll = 0
					} else {
						w.SelectByRowID(ctx, id)
					}
				}
			}
			return nil, true
		}
	}
	switch key {
	case "up":
		w.moveUp()
	case "down":
		w.moveDown()
	case "left":
		w.moveUp()
	case "right":
		w.moveDown()
	case "home":
		w.moveToTop()
	case "end":
		w.moveToBottom()
	case "pgup":
		w.pageUp()
	case "pgdown":
		w.pageDown()
	case "enter":
		return w.enterItem(ctx), true
	case "ctrl+t", "insert":
		w.toggleSelection()
	case "ctrl+a":
		w.selectAll()
	case "ctrl+r":
		w.refresh()
	case "ctrl+v":
		w.tableViewEnabled = !w.tableViewEnabled
		w.refresh()
	case "ctrl+w":
		return w.ToggleColumnsMode(ctx), true
	case "*":
		w.invertSelection()
	case "+", "-":
		return w.showGlobPatternDialog(key), true
	case "f1":
		return w.invoke(ctx, panelcontent.ActionHelp), true
	case "f2":
		return w.invoke(ctx, panelcontent.ActionOptions), true
	case "f3":
		return w.invoke(ctx, panelcontent.ActionView), true
	case "f4":
		return w.invoke(ctx, panelcontent.ActionEdit), true
	case "f7":
		return w.invoke(ctx, panelcontent.ActionCreate), true
	case "f8":
		return w.invoke(ctx, panelcontent.ActionDelete), true
	case "f9":
		return w.invoke(ctx, panelcontent.ActionMenu), true
	default:
		return nil, false
	}
	w.saveCurrentPosition()
	return nil, true
}

func (w *Widget) handleMouse(ctx context.Context, msg panelcontent.MouseMsg) (tea.Cmd, bool) {
	switch msg.Intent {
	case panelcontent.MouseIntentWheel:
		if msg.DeltaY < 0 {
			w.moveUp()
		} else if msg.DeltaY > 0 {
			w.moveDown()
		}
		w.saveCurrentPosition()
		return nil, true
	case panelcontent.MouseIntentClick:
		return w.selectByVisibleRow(ctx, msg.Row, msg.Button), true
	default:
		return nil, false
	}
}

// --- rendering -------------------------------------------------------------

func (w *Widget) render(ctx context.Context, focused bool) string {
	if w.useFolder && w.folder != nil {
		w.syncFromFolder(ctx)
		if w.bt == nil {
			w.ensureBigTable(ctx)
		}
		if w.bt == nil {
			return uistyles.PanelContentStyle.
				Width(w.width).
				Height(w.height).
				Align(lipgloss.Center).
				Render("Loading...")
		}
		w.bt.SetSize(ctx, max(1, w.width), max(1, w.height))
		w.bt.SetFocused(ctx, focused)
		tableView := w.bt.View()
		return lipgloss.NewStyle().
			Background(lipgloss.Blue).
			Width(w.width).
			Height(w.height).
			Render(viewString(tableView))
	}

	if len(w.items) == 0 {
		return uistyles.PanelContentStyle.
			Width(w.width).
			Height(w.height).
			Align(lipgloss.Center).
			Render("No items")
	}

	visibleHeight := max(0, w.height-1)
	start := w.scroll
	end := start + visibleHeight
	if end > len(w.items) {
		end = len(w.items)
	}
	lines := make([]string, 0, visibleHeight)
	for i := start; i < end; i++ {
		lines = append(lines, w.renderItem(w.items[i], i == w.selected && focused))
	}
	for len(lines) < w.height {
		lines = append(lines, uistyles.PanelContentStyle.Width(w.width).Render(""))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (w *Widget) renderItem(item Item, selected bool) string {
	name := item.Name
	if name == "" && item.Item != nil {
		if _, cells, _, ok := item.Columns(); ok && len(cells) > 0 {
			name = cells[0]
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
	if len(text) > w.width {
		text = text[:w.width]
	}
	style := uistyles.PanelItemStyle.Width(w.width)
	if selected {
		style = uistyles.PanelItemSelectedStyle.Width(w.width)
	}
	if item.Selected {
		style = style.Foreground(lipgloss.Yellow).Bold(true)
	}
	return style.Render(text)
}

// --- selection helpers -----------------------------------------------------

func (w *Widget) selectByVisibleRow(ctx context.Context, row int, button tea.MouseButton) tea.Cmd {
	if row < 0 {
		return nil
	}
	if w.useFolder && w.folder != nil && w.bt != nil {
		if id, ok := w.bt.VisibleRowID(row); ok {
			w.SelectByRowID(ctx, id)
			if button == tea.MouseRight {
				return w.invoke(ctx, panelcontent.ActionMenu)
			}
		}
		return nil
	}
	idx := w.scroll + row
	if idx >= 0 && idx < len(w.items) {
		w.selected = idx
		w.adjustScroll()
		w.saveCurrentPosition()
		if button == tea.MouseRight {
			return w.invoke(ctx, panelcontent.ActionMenu)
		}
	}
	return nil
}

func (w *Widget) toggleSelection() {
	if len(w.items) == 0 || w.selected < 0 || w.selected >= len(w.items) {
		return
	}
	w.items[w.selected].Selected = !w.items[w.selected].Selected
	if w.items[w.selected].Selected && w.selected < len(w.items)-1 {
		w.selected++
		w.adjustScroll()
	}
}

func (w *Widget) selectAll() {
	for i := range w.items {
		w.items[i].Selected = true
	}
}

func (w *Widget) invertSelection() {
	for i := range w.items {
		w.items[i].Selected = !w.items[i].Selected
	}
}

func (w *Widget) moveUp() {
	if w.selected > 0 {
		w.selected--
		w.adjustScroll()
	}
}

func (w *Widget) moveDown() {
	if w.selected < len(w.items)-1 {
		w.selected++
		w.adjustScroll()
	}
}

func (w *Widget) moveToTop() {
	w.selected = 0
	w.scroll = 0
}

func (w *Widget) moveToBottom() {
	if len(w.items) > 0 {
		w.selected = len(w.items) - 1
		w.scroll = max(0, len(w.items)-w.visibleHeight())
	}
}

func (w *Widget) pageUp() {
	pageSize := w.visibleHeight()
	w.selected = max(0, w.selected-pageSize)
	w.scroll = max(0, w.scroll-pageSize)
}

func (w *Widget) pageDown() {
	pageSize := w.visibleHeight()
	w.selected = min(len(w.items)-1, w.selected+pageSize)
	w.scroll = min(max(0, len(w.items)-w.visibleHeight()), w.scroll+pageSize)
}

func (w *Widget) enterItem(ctx context.Context) tea.Cmd {
	if !w.useFolder || w.folder == nil || w.folderHandler == nil {
		return nil
	}
	w.syncFromFolder(ctx)
	if len(w.items) == 0 {
		return nil
	}
	if w.selected < 0 {
		w.selected = 0
	}
	if w.selected >= len(w.items) {
		w.selected = len(w.items) - 1
	}
	rows := w.folderLines(ctx, 0, w.folderLen(ctx))
	if w.selected >= len(rows) {
		return nil
	}
	row := rows[w.selected]
	id, _, _, _ := row.Columns()
	if back, ok := row.(models.Back); ok && back.IsBack() {
		w.folderHandler(true, id, nil)
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
	w.folderHandler(false, id, next)
	return nil
}

func (w *Widget) refresh() {
	w.ClearPositionMemory()
}

func (w *Widget) showGlobPatternDialog(string) tea.Cmd {
	return nil
}

func (w *Widget) adjustScroll() {
	visible := w.visibleHeight()
	if w.selected < w.scroll {
		w.scroll = w.selected
	} else if w.selected >= w.scroll+visible {
		w.scroll = w.selected - visible + 1
		if w.scroll < 0 {
			w.scroll = 0
		}
	}
}

func (w *Widget) visibleHeight() int {
	return max(1, w.height-2)
}

func (w *Widget) saveCurrentPosition() {
	path := w.currentPath()
	if path == "" {
		return
	}
	w.positionMemory[path] = PositionInfo{
		Selected:  w.selected,
		ScrollTop: w.scroll,
	}
}

func (w *Widget) currentPath() string {
	if w.deps.Path == nil {
		return ""
	}
	return w.deps.Path()
}

func isBackItem(it Item) bool {
	if it.Name == ".." {
		return true
	}
	if it.Item != nil {
		if back, ok := it.Item.(models.Back); ok {
			return back.IsBack()
		}
	}
	return false
}

// --- folder helpers --------------------------------------------------------

func (w *Widget) syncFromFolder(ctx context.Context) {
	if !w.useFolder || w.folder == nil {
		return
	}
	rowCount := w.folderLen(ctx)
	rows := w.folderLines(ctx, 0, rowCount)
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
		items = append(items, Item{Item: itemRow, Name: displayName})
	}
	w.items = items
}

func (w *Widget) folderLen(ctx context.Context) int {
	if !w.useFolder || w.folder == nil {
		return 0
	}
	return w.folder.Len(ctx)
}

func (w *Widget) folderLines(ctx context.Context, top, num int) []table.Row {
	if !w.useFolder || w.folder == nil {
		return nil
	}
	return w.folder.Lines(ctx, top, num)
}

func (w *Widget) folderItemByID(ctx context.Context, id string) (models.Item, bool) {
	if !w.useFolder || w.folder == nil {
		return nil, false
	}
	return w.folder.ItemByID(ctx, id)
}

func (w *Widget) selectedRowID(ctx context.Context) string {
	if !w.useFolder || w.folder == nil {
		return ""
	}
	if w.bt != nil {
		if id, ok := w.bt.CurrentID(ctx); ok {
			return id
		}
	}
	limit := w.folderLen(ctx)
	if w.selected < 0 || w.selected >= limit {
		return ""
	}
	rows := w.folderLines(ctx, w.selected, 1)
	if len(rows) == 0 {
		return ""
	}
	id, _, _, ok := rows[0].Columns()
	if !ok {
		return ""
	}
	return id
}

func (w *Widget) ensureBigTable(ctx context.Context) {
	if w.folder == nil {
		return
	}
	_ = w.folderLen(ctx)
	cols := w.folder.Columns()
	w.lastColTitles = columnsToTitles(cols)
	bt := table.NewBigTable(cols, w.folder, max(1, w.width), max(1, w.height))
	bt.SetMode(ctx, w.tableMode)
	w.decorateBigTable(ctx, &bt)
	w.bt = &bt
}

func (w *Widget) decorateBigTable(ctx context.Context, bt *table.BigTable) {
	st := table.DefaultStyles()
	st.Header = uistyles.PanelTableHeaderStyle
	st.Cell = uistyles.PanelItemStyle
	st.Selector = uistyles.PanelItemSelectedStyle
	st.Marked = lipgloss.NewStyle().Foreground(lipgloss.Yellow).Bold(true)
	st.Border = lipgloss.NewStyle().
		Foreground(lipgloss.White).
		Background(lipgloss.Blue).
		BorderForeground(lipgloss.White).
		BorderBackground(lipgloss.Blue)
	bt.SetStyles(st)
	bt.BorderVertical(ctx, true)
}

// --- helpers ----------------------------------------------------------------

func (w *Widget) invoke(ctx context.Context, action panelcontent.Action) tea.Cmd {
	if w.deps.InvokeAction == nil {
		return nil
	}
	return w.deps.InvokeAction(ctx, action)
}

func (w *Widget) selectionCmd(ctx context.Context, prev string, cmds ...tea.Cmd) tea.Cmd {
	if w.deps.SelectionChanged != nil {
		cmds = append(cmds, w.deps.SelectionChanged(ctx, w.selectionSnapshot(ctx)))
	}
	return tea.Batch(cmds...)
}

func (w *Widget) selectionSnapshot(ctx context.Context) panelcontent.Selection {
	selection := panelcontent.Selection{}
	if w.deps.Path != nil {
		selection.Path = w.deps.Path()
	}
	selection.ID = w.CurrentSelectionID(ctx)
	if current := w.CurrentItem(ctx); current != nil && current.Item != nil {
		selection.Item = current.Item
	}
	return selection
}

func columnsToTitles(cols []table.Column) []string {
	out := make([]string, len(cols))
	for i := range cols {
		out[i] = cols[i].Title
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

func viewString(view tea.View) string {
	if view.Content == nil {
		return ""
	}
	return fmt.Sprint(view.Content)
}
