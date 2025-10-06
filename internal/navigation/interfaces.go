package navigation

import (
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sttts/kc/internal/navigation/models"
	table "github.com/sttts/kc/internal/table"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Re-export core interfaces from the models package so existing callers can
// depend on navigation without importing models directly.
type (
	Item       = models.Item
	ObjectItem = models.ObjectItem
	Folder     = models.Folder
	BackItem   = models.BackItem
)

// Additional capabilities -----------------------------------------------------

type Viewable interface {
	ViewContent() (title, body, lang, mime, filename string, err error)
}

type Countable interface {
	Count() int
	Empty() bool
}

type Enterable interface {
	Item
	Enter() (Folder, error)
}

type Back interface {
	Item
}

// KeyFolder identifies key/value listings such as ConfigMap or Secret data.
type KeyFolder interface {
	Folder
	Parent() (schema.GroupVersionResource, string, string)
}

// Helpers --------------------------------------------------------------------

func GreenStyle() *lipgloss.Style { return models.GreenStyle() }
func DimStyle() *lipgloss.Style   { return models.DimStyle() }
func WhiteStyle() *lipgloss.Style { return models.WhiteStyle() }

// Convenience alias for table.List so call sites remain unchanged.
type TableList = table.List
