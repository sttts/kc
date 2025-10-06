package models

import (
	"github.com/charmbracelet/lipgloss/v2"
	table "github.com/sttts/kc/internal/table"
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
}

// Folder describes a navigable collection of rows.
type Folder interface {
	table.List
	Columns() []table.Column
	Path() []string
	ItemByID(string) (Item, bool)
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

func DimStyle() *lipgloss.Style {
	s := lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#7D7D7D"))
	return &s
}
