package navigation

import (
	"context"
	"strings"

	"github.com/sttts/kc/internal/models"
)

// Navigator is a minimal, UI-agnostic helper to manage Folder navigation.
// It maintains a simple stack of Folders and exposes push/back semantics.
// Panels can render the current Folder and show a back item when HasBack().
type Navigator struct {
	stack []frame
}

type frame struct {
	f     models.Folder
	selID string
}

// NewNavigator constructs a navigator with an optional root folder.
func NewNavigator(root models.Folder) *Navigator {
	n := &Navigator{}
	if root != nil {
		n.stack = []frame{{f: root}}
	}
	return n
}

// Current returns the top-most folder (or nil if empty).
func (n *Navigator) Current() models.Folder {
	if len(n.stack) == 0 {
		return nil
	}
	return n.stack[len(n.stack)-1].f
}

// Push appends a new folder if non-nil and returns it.
func (n *Navigator) Push(f models.Folder) models.Folder {
	if f == nil {
		return n.Current()
	}
	n.stack = append(n.stack, frame{f: f})
	return f
}

// Back pops one folder if possible and returns the resulting current folder.
func (n *Navigator) Back() models.Folder {
	if len(n.stack) == 0 {
		return nil
	}
	if len(n.stack) == 1 {
		return n.stack[0].f
	}
	n.stack = n.stack[:len(n.stack)-1]
	return n.stack[len(n.stack)-1].f
}

// HasBack reports whether a back action is possible.
func (n *Navigator) HasBack() bool { return len(n.stack) > 1 }

// SetSelectionID stores the selection ID for the current frame (previous folder).
func (n *Navigator) SetSelectionID(id string) {
	if len(n.stack) > 0 {
		n.stack[len(n.stack)-1].selID = id
	}
}

// CurrentSelectionID returns the remembered selection for the current frame.
func (n *Navigator) CurrentSelectionID() string {
	if len(n.stack) == 0 {
		return ""
	}
	return n.stack[len(n.stack)-1].selID
}

// Path computes the breadcrumb path for the current stack based on the
// selected row IDs in each parent frame and the first column text of those rows.
// It ignores synthetic back rows ("__back__"). The returned string is an
// absolute path starting with a leading "/". Root with no selections yields "/".
func (n *Navigator) Path(ctx context.Context) string {
	if len(n.stack) == 0 {
		return "/"
	}
	segments := make([]string, 0, len(n.stack))
	// For each parent frame (exclude the last/current folder), use its selID
	// to find the row and take the first cell (trim a single leading "/").
	for i := 0; i < len(n.stack)-1; i++ {
		fr := n.stack[i]
		if fr.selID == "" || fr.selID == "__back__" || fr.f == nil {
			continue
		}
		_, row, ok := fr.f.Find(ctx, fr.selID)
		if !ok || row == nil {
			continue
		}
		_, cells, _, ok2 := row.Columns()
		if !ok2 || len(cells) == 0 {
			continue
		}
		seg := cells[0]
		if len(seg) > 0 && seg[0] == '/' {
			seg = seg[1:]
		}
		if seg != "" && seg != ".." {
			segments = append(segments, seg)
		}
	}
	if len(segments) == 0 {
		cur := n.Current()
		if cur == nil {
			return "/"
		}
		path := cur.Path()
		if len(path) == 0 {
			return "/"
		}
		return "/" + strings.Join(path, "/")
	}
	return "/" + strings.Join(segments, "/")
}
