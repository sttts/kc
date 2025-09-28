# Table Component Requirements

- Modes: two horizontal modes.
  - Fit mode: truncate ASCII cell text to fit the available width, then apply style.
  - Left/Right mode: don’t pre-truncate; allow horizontal panning with arrow keys.
- Scale: handle tens of thousands of rows via windowing/virtualization; render only the visible slice.
- Cells: base strings are ASCII (no ANSI in data). Each cell is rendered with a lipgloss style applied to the entire string. Use `SimpleRow` to hold `id`, `cells`, and per-cell styles; allow updates via `SetColumn`.
- Model interface (no SetCell): provide data via stable-ID rows/lists.
  - Row: `Columns() (id string, cells []string, styles []*lipgloss.Style, exists bool)`.
  - List: `Lines(top, num int) []Row`, `Above(rowID string, num int) []Row`, `Below(rowID string, num int) []Row`.
- Selector line: visible when focused. If the selected row disappears, move selection to the next row; if none, move to previous.
- Highly dynamic data: optimize for frequent updates; preserve scroll/cursor; minimal reflow.
- Selection: toggle with Ctrl+T or Insert; selected rows use a configurable selection style.
- Reusability: component should be reusable across views; avoid coupling to resource types.
- Styling: header, borders, and column separators must be customizable (including no borders) following Bubble Tea v2 + lipgloss conventions; provide good defaults and full overrides.
- Dependencies policy: always import Bubble Tea/Bubbles/Lipgloss using `/v2` paths; run `go mod tidy` after changes.
