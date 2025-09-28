package navigation

// Navigator is a minimal, UI-agnostic helper to manage Folder navigation.
// It maintains a simple stack of Folders and exposes push/back semantics.
// Panels can render the current Folder and show a back item when HasBack().
type Navigator struct {
    stack []Folder
}

// NewNavigator constructs a navigator with an optional root folder.
func NewNavigator(root Folder) *Navigator {
    n := &Navigator{}
    if root != nil { n.stack = []Folder{root} }
    return n
}

// SetRoot resets the stack to the provided root.
func (n *Navigator) SetRoot(root Folder) { if root == nil { n.stack = nil } else { n.stack = []Folder{root} } }

// Current returns the top-most folder (or nil if empty).
func (n *Navigator) Current() Folder {
    if len(n.stack) == 0 { return nil }
    return n.stack[len(n.stack)-1]
}

// Push appends a new folder if non-nil and returns it.
func (n *Navigator) Push(f Folder) Folder {
    if f == nil { return n.Current() }
    n.stack = append(n.stack, f)
    return f
}

// Back pops one folder if possible and returns the resulting current folder.
func (n *Navigator) Back() Folder {
    if len(n.stack) == 0 { return nil }
    if len(n.stack) == 1 { return n.stack[0] }
    n.stack = n.stack[:len(n.stack)-1]
    return n.stack[len(n.stack)-1]
}

// HasBack reports whether a back action is possible.
func (n *Navigator) HasBack() bool { return len(n.stack) > 1 }

