package panelcontent

import (
	"context"

	tea "charm.land/bubbletea/v2"
	models "github.com/sttts/kc/internal/models"
	"github.com/sttts/kc/pkg/describe"
)

// Size describes the drawable rectangle granted to a widget.
type Size struct {
	Width  int
	Height int
}

// Frame augments Size with focus metadata for rendering.
type Frame struct {
	Size    Size
	Focused bool
}

// Widget is the shared contract implemented by each panel mode.
type Widget interface {
	Init(context.Context) tea.Cmd
	Update(context.Context, tea.Msg) (tea.Cmd, bool)
	View(context.Context, Frame) string
	Resize(context.Context, Size)
	SetFocus(context.Context, bool)
	Teardown(context.Context)
}

// Factory constructs a widget instance with prepared dependencies.
type Factory func(WidgetDeps) Widget

// WidgetDeps contains cross-cutting helpers supplied by the panel shell.
type WidgetDeps struct {
	InvokeAction     func(context.Context, Action) tea.Cmd
	Path             func() string
	SelectionChanged func(context.Context, Selection) tea.Cmd
	SelectedItem     func(context.Context) (models.Item, bool)
	Describe         DescribeFunc
	Post             func(tea.Msg)
}

// DescribeFunc renders describe output for widget selections.
type DescribeFunc func(context.Context, describe.Target) (describe.Result, error)

// Selection represents the widget's current selection identity.
type Selection struct {
	ID   string
	Path string
	Item models.Item
	// Force notifies listeners and other panels even when the ID remains unchanged.
	Force bool
}

// SelectionProvider widgets expose the currently highlighted item.
type SelectionProvider interface {
	CurrentSelectionID(context.Context) string
}

// ItemProvider widgets expose selected row metadata.
type ItemProvider interface {
	CurrentItem(context.Context) (interface{}, bool)
}

// FooterProvider widgets can render a footer row via the panel shell.
type FooterProvider interface {
	Footer(context.Context, int) string
}

// FrameInfoProvider allows widgets to customize panel frame rendering.
type FrameInfoProvider interface {
	FrameInfo(context.Context, FrameInfoRequest) FrameInfo
}

// FrameInfoRequest supplies frame rendering context.
type FrameInfoRequest struct {
	Width int
}

// FrameInfo describes breadcrumb overrides and status strings.
type FrameInfo struct {
	Breadcrumb      string
	HeaderStatus    string
	FooterStatus    string
	TopIndicator    string
	BottomIndicator string
	SuppressFooter  bool
}

// SelectionListener widgets opt in to selection change notifications.
type SelectionListener interface {
	OnSelectionChanged(context.Context, Selection)
}

// Action identifies panel-level shortcuts widgets may invoke.
type Action string

const (
	ActionHelp    Action = "help"
	ActionOptions Action = "options"
	ActionView    Action = "view"
	ActionEdit    Action = "edit"
	ActionCreate  Action = "create-namespace"
	ActionDelete  Action = "delete"
	ActionMenu    Action = "menu"
)

// MouseIntent categorizes routed mouse events.
type MouseIntent int

const (
	MouseIntentClick MouseIntent = iota
	MouseIntentWheel
)

// MouseMsg conveys panel-relative mouse details.
type MouseMsg struct {
	Intent MouseIntent
	Row    int
	Button tea.MouseButton
	DeltaY int
}
