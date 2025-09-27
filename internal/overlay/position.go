package overlay

// Position represents a relative offset in the TUI. There are five possible values; Top, Right,
// Bottom, Left, and Center.
type Position int

const (
	Top Position = iota + 1
	Right
	Bottom
	Left
	Center
)
