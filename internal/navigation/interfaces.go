package navigation

import (
    table "github.com/sttts/kc/internal/table"
    "github.com/charmbracelet/lipgloss/v2"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

// Item drives both rendering (table.Row) and actions. It must return a stable
// ID and aligned cells with per-cell styles (ASCII only).
type Item interface {
    table.Row        // Columns() (id, cells, styles, exists)
    Details() string // concise info shown in status/footer
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

// Viewable is a capability: an item can render a focused view for F3.
// Items that don’t implement Viewable fall back to a generic object view (when ObjectItem).
type Viewable interface {
    BuildView() (title, body string, err error)
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
    table.List                // windowed access to table rows
    Columns() []table.Column  // column titles (+ future width/priority/orientation)
    Title() string            // short label for breadcrumbs
    Key() string              // stable identity for history/restore (e.g., context/ns/GVR and child kind)
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
