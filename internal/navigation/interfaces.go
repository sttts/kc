package navigation

import (
	"github.com/charmbracelet/lipgloss/v2"
	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Item drives both rendering (table.Row) and actions. It must return a stable
// ID and aligned cells with per-cell styles (ASCII only).
type Item interface {
	table.Row        // Columns() (id, cells, styles, exists)
	Details() string // concise info shown in status/footer
	// Path returns the absolute breadcrumb path segments for this item,
	// excluding the leading root slash. Example: ["namespaces","ns1","pods","pod-0"].
	Path() []string
}

// ObjectItem is implemented by items that represent actual Kubernetes objects.
// Non-object entries (containers, logs, config keys) do not need to implement it.
type ObjectItem interface {
	Item
	GVR() schema.GroupVersionResource
	Namespace() string
	Name() string
	// Optional: visuals only — implement when needed:
	// GVK() schema.GroupVersionKind
}

// Viewable indicates that an item can produce focused viewer content for F3.
// Items opting out should return ErrNoViewContent from ViewContent.
type Viewable interface {
	ViewContent() (title, body, lang, mime, filename string, err error)
}

// Countable exposes aggregate information about list-like items such as resource groups.
// Count must use the shared informer (starting it if necessary) so follow-up consumers benefit
// from a warm cache, while Empty should perform a lightweight peek without booting informers.
type Countable interface {
	Count() int
	Empty() bool
}

// Enterable is a capability: selecting Enter on the item yields a Folder.
// The returned Folder knows how to populate itself (e.g., via a bound manager).
type Enterable interface {
	Item
	Enter() (Folder, error)
}

// Back marks a row as the "go up" action. It is handled by the App,
// which pops the navigation stack and restores the previous folder state.
type Back interface{ Item }

// Folder represents a navigable listing. It implements table.List for rows
// and provides the visible column descriptors and breadcrumb/title identity.
type Folder interface {
	table.List               // windowed access to table rows
	Columns() []table.Column // column titles (+ future width/priority/orientation)
	Title() string           // short label for breadcrumbs
	Key() string             // stable identity for history/restore (e.g., context/ns/GVR and child kind)
	ItemByID(string) (Item, bool)
}

// Refreshable allows external triggers (like view option changes) to request
// that a Folder rebuild its listing on next access.
type Refreshable interface {
	Refresh()
}

// DirtyAware is implemented by folders that can report if their content is out
// of date (e.g., due to informer events) and should be repopulated on next
// access.
type DirtyAware interface {
	IsDirty() bool
}

// KeyFolder is implemented by folders that list key-value entries under a
// parent Kubernetes object (e.g., ConfigMap/Secret data). It returns the
// parent object's coordinates.
type KeyFolder interface {
	Folder
	Parent() (schema.GroupVersionResource, string, string) // gvr, namespace, name
}

// GreenStyle returns a style for the default (green) row rendering.
// Selection/other modes can override at the table layer.
func GreenStyle() *lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	return &s
}

// DimStyle returns a faint gray style suitable for secondary columns (e.g., Group).
func DimStyle() *lipgloss.Style {
	s := lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("#7D7D7D"))
	return &s
}

// WhiteStyle returns a white foreground style for primary name columns.
func WhiteStyle() *lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	return &s
}
