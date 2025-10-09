package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea/v2"
	models "github.com/sttts/kc/internal/models"
)

// PanelViewMode identifies the active widget/view embedded inside a panel.
type PanelViewMode int

const (
	PanelModeList PanelViewMode = iota
	PanelModeDescribe
	PanelModeManifest
	PanelModeFile
)

// PanelWidget renders a panel mode and receives routed input.
type PanelWidget interface {
	Init(ctx context.Context) tea.Cmd
	Update(ctx context.Context, msg tea.Msg) (tea.Cmd, bool)
	View(ctx context.Context, focused bool) string
	Resize(ctx context.Context, width, height int)
	SetFocus(ctx context.Context, focused bool)
}

// PanelWidgetFactory constructs a widget bound to the provided panel.
type PanelWidgetFactory func(*Panel) PanelWidget

var panelModeOrder = []PanelViewMode{PanelModeList, PanelModeDescribe, PanelModeManifest, PanelModeFile}

// listWidget implements the legacy folder/table view.
type listWidget struct {
	panel *Panel
}

func newListWidget(panel *Panel) PanelWidget {
	return &listWidget{panel: panel}
}

func (w *listWidget) Init(context.Context) tea.Cmd { return nil }

func (w *listWidget) Update(ctx context.Context, msg tea.Msg) (tea.Cmd, bool) {
	p := w.panel
	if p == nil {
		return nil, false
	}
	switch m := msg.(type) {
	case tea.KeyMsg:
		key := m.String()
		if p.useFolder && p.folder != nil && p.bt != nil {
			switch key {
			case "up", "down", "left", "right", "home", "end", "pgup", "pgdown", "ctrl+t", "insert":
				_, _ = p.bt.UpdateWithContext(ctx, m)
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
				return nil, true
			}
		}
		switch key {
		case "up":
			p.moveUp(ctx)
			return nil, true
		case "down":
			p.moveDown(ctx)
			return nil, true
		case "left":
			p.moveUp(ctx)
			return nil, true
		case "right":
			p.moveDown(ctx)
			return nil, true
		case "home":
			p.moveToTop()
			return nil, true
		case "end":
			p.moveToBottom()
			return nil, true
		case "pgup":
			p.pageUp()
			return nil, true
		case "pgdown":
			p.pageDown()
			return nil, true
		case "enter":
			return p.enterItem(ctx), true
		case "ctrl+t", "insert":
			p.toggleSelection()
			return nil, true
		case "ctrl+a":
			p.selectAll()
			return nil, true
		case "ctrl+r":
			return p.refresh(), true
		case "ctrl+v":
			p.tableViewEnabled = !p.tableViewEnabled
			return p.refresh(), true
		case "*":
			p.invertSelection()
			return nil, true
		case "+", "-":
			return p.showGlobPatternDialog(key), true
		case "f1":
			return p.invokeActionIfAllowed(ctx, PanelActionHelp), true
		case "f2":
			return p.invokeActionIfAllowed(ctx, PanelActionOptions), true
		case "f3":
			return p.invokeActionIfAllowed(ctx, PanelActionView), true
		case "f4":
			return p.invokeActionIfAllowed(ctx, PanelActionEdit), true
		case "f7":
			return p.invokeActionIfAllowed(ctx, PanelActionCreateNamespace), true
		case "f8":
			return p.invokeActionIfAllowed(ctx, PanelActionDelete), true
		case "f9":
			return p.invokeActionIfAllowed(ctx, PanelActionMenu), true
		}
	}
	return nil, false
}

func (w *listWidget) View(ctx context.Context, focused bool) string {
	if w.panel == nil {
		return ""
	}
	return w.panel.renderListContentFocused(ctx, focused)
}

func (w *listWidget) Resize(ctx context.Context, width, height int) {
	if w.panel == nil {
		return
	}
	w.panel.resizeListWidget(ctx, width, height)
}

func (w *listWidget) SetFocus(context.Context, bool) {}

// NextPanelMode returns the next mode in the default ordering.
func NextPanelMode(current PanelViewMode) PanelViewMode {
	for i, mode := range panelModeOrder {
		if mode == current {
			next := panelModeOrder[(i+1)%len(panelModeOrder)]
			return next
		}
	}
	return panelModeOrder[0]
}

// PanelModeOrder returns the canonical ordering of panel modes.
func PanelModeOrder() []PanelViewMode {
	return append([]PanelViewMode(nil), panelModeOrder...)
}
