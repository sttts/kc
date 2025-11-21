package bubblelayout

import tea "charm.land/bubbletea/v2"

// OffsetCursor adjusts a cursor's coordinates so that it is relative to the
// layout region allocated for the provided view ID. When the cursor is nil the
// return value is also nil.
func (l BubbleLayoutMsg) OffsetCursor(id ID, cursor *tea.Cursor) (*tea.Cursor, error) {
	if cursor == nil {
		return nil, nil
	}
	size, err := l.Size(id)
	if err != nil {
		return nil, err
	}
	abs := tea.NewCursor(cursor.X+size.X, cursor.Y+size.Y)
	abs.Color = cursor.Color
	abs.Shape = cursor.Shape
	abs.Blink = cursor.Blink
	return abs, nil
}
