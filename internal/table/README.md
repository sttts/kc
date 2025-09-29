# Table Component (internal/table)

An importable, high-performance table component built on Bubble Tea v2. It supports massive datasets, two render modes, per‑cell styling, stable selection, and swappable data providers.

## Design & Features
- Modes: Auto (lipgloss.table decides widths) and Fit (constrain columns to fit width; no manual ASCII slicing).
- Scale: Virtualized rendering — only visible rows are rendered; suitable for tens of thousands of rows.
- Model-driven: The table consumes a `List` of `Row` items with stable IDs; no `SetCell` API.
- Styling: Every cell renders through a `lipgloss.Style`; column headers and inner vertical separators are customizable. No outside frame or header underline.
- Selection: Multi-select via stable row IDs; selection overlay style independent from focus.
- Stability: Selector moves to next when the focused row disappears, else previous; selection prunes vanished IDs.
- Providers: Slice-backed (`SliceList`) and doubly linked (`LinkedList`), both storing `Row`. `SimpleRow` is a reusable `Row` implementation.

## Interfaces
- `Row`: `Columns() (id string, cells []string, styles []*lipgloss.Style, exists bool)`
- `List`: `Lines(top, num int) []Row`, `Above(rowID, num int) []Row`, `Below(rowID, num int) []Row`, `Len() int`, `Find(rowID string) (int, Row, bool)`

## Integrating
- Import: `table "github.com/sttts/kc/internal/table"`
- Construct a provider and the table:
  ```go
  cols := []table.Column{{Title: "ID", Width: 12}, {Title: "Status", Width: 8}}
  rows := []table.Row{table.SimpleRow{ID: "id-1"}, table.SimpleRow{ID: "id-2"}}
  list := table.NewSliceList(rows) // or table.NewLinkedList(rows)
  bt := table.NewBigTable(cols, list, width, height)
  ```
- In your Bubble Tea model: forward `tea.Msg` to `bt.Update(msg)`, call `bt.SetSize` on window changes, and render with `bt.View()`.

## Styling
- Configure styles via `Styles` and `SetStyles`:
  ```go
  st := table.DefaultStyles()
  st.Header = st.Header.Bold(true).Foreground(lipgloss.Color("#8F8"))
  st.Selector = lipgloss.NewStyle().Background(lipgloss.Color("12")).Foreground(lipgloss.Color("0"))
  st.Cell = lipgloss.NewStyle()
  bt.SetStyles(st)
  ```
- Per-cell styles: return `[]*lipgloss.Style` from your `Row.Columns()`; each cell’s style inherits from `Styles.Cell` and the selection overlay.
- Selector highlight: applied to the focused row; multi-select overlay also uses `Styles.Selector`.

## Testing
- Package tests: `go test ./internal/table -v`
- Covers: selection overlay, provider ops, selector stability, and render height for Auto/Fit.

## Notes
- Always import v2 packages: `github.com/charmbracelet/bubbletea/v2`, `github.com/charmbracelet/bubbles/v2/...`, `github.com/charmbracelet/lipgloss/v2`.
- Rows are ASCII only; ANSI is produced by styles. No custom ASCII truncation is performed by the component.
