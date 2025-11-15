package models

import (
	"context"

	"github.com/charmbracelet/lipgloss/v2"
	table "github.com/sttts/kc/internal/table"
	"github.com/sttts/kc/pkg/appconfig"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Item matches the navigation item contract. Navigation will alias to this type.
type Item interface {
	table.Row
	Details() string
	Path() []string
}

// ObjectItem identifies items backed by Kubernetes objects.
type ObjectItem interface {
	Item
	GVR() schema.GroupVersionResource
	Namespace() string
	Name() string
	SupportsVerb(string) bool
}

// Folder describes a navigable collection of rows.
type Folder interface {
	table.List
	Columns() []table.Column
	Path() []string
	ItemByID(context.Context, string) (Item, bool)
}

// Enterable identifies rows that can return a child folder when Enter is pressed.
type Enterable interface {
	Item
	Enter() (Folder, error)
}

// Viewable exposes focused content for viewer panes (F3).
type Viewable interface {
	ViewContent() (title, body, lang, mime, filename string, err error)
}

// LogsSpec describes a container log stream.
type LogsSpec struct {
	Namespace string
	Pod       string
	Container string
	Follow    bool
	TailLines int64
}

// DefaultLogsTailLines controls how many lines to fetch before following.
const DefaultLogsTailLines int64 = 200

// LogsProvider identifies rows that can open a streaming logs viewer.
type LogsProvider interface {
	LogsSpec() LogsSpec
}

// Countable reports aggregate information for list-style rows (resource groups, context lists).
type Countable interface {
	Count() int
	Empty() bool
}

// KeyFolder identifies key/value listings such as ConfigMap or Secret data folders.
type KeyFolder interface {
	Folder
	Parent() (schema.GroupVersionResource, string, string)
}

// Back identifies the synthetic ".." entry.
type Back interface {
	Item
	IsBack() bool
}

// ResourceViewConfigurable allows folders to customize resource view toggles at runtime.
type ResourceViewConfigurable interface {
	ApplyResourceViewOptions(showNonEmpty bool, order appconfig.ResourcesViewOrder, favorites []string)
}

// ObjectOrderConfigurable allows folders to override object ordering at runtime.
type ObjectOrderConfigurable interface {
	ApplyObjectOrder(order string)
}

// DirtyObservable folders notify listeners when their data changes.
type DirtyObservable interface {
	RegisterDirtyListener(func()) func()
}

// BackItem renders the synthetic ".." row.
type BackItem struct{}

var _ Item = BackItem{}
var _ Back = BackItem{}

func (BackItem) Columns() (string, []string, []*lipgloss.Style, bool) {
	style := GreenStyle()
	return "__back__", []string{".."}, []*lipgloss.Style{style}, true
}

func (BackItem) Details() string { return "Back" }
func (BackItem) Path() []string  { return nil }
func (BackItem) IsBack() bool    { return true }

// GreenStyle mirrors the navigation helper.
func GreenStyle() *lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	return &s
}

func WhiteStyle() *lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	return &s
}

// FavoriteResourceStyle highlights favorite resource rows.
func FavoriteResourceStyle() *lipgloss.Style {
	s := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	return &s
}

func DimStyle() *lipgloss.Style {
	s := lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#7D7D7D"))
	return &s
}
