# Table Component (internal/table)

A self-contained, high-performance table component built on Bubble Tea v2. It supports massive datasets, two horizontal modes, per‑cell styling, stable selection, and swappable data providers.

## Design & Features
- Modes: Scroll (no pre-truncation, horizontal pan) and Fit (ASCII truncate to width, then style).
- Scale: Virtualized rendering — only visible rows are rendered; suitable for tens of thousands of rows.
- Model-driven: The table consumes a `List` of `Row` items with stable IDs; no `SetCell` API.
- Styling: Every cell renders through a `lipgloss.Style`; header and borders are customizable.
- Selection: Multi-select via stable row IDs; selection overlay style independent from focus.
- Stability: Selector moves to next when the focused row disappears, else previous; selection prunes vanished IDs.
- Providers: Slice-backed (`SliceList`) and doubly linked (`LinkedList`), both storing the `Row` interface. `SimpleRow` is a reusable `Row` implementation.

## Interfaces
- `Row`: `Columns() (id string, cells []string, styles []*lipgloss.Style, exists bool)`
- `List`: `Lines(top, num int) []Row`, `Above(rowID, num int) []Row`, `Below(rowID, num int) []Row`, `Len() int`, `Find(rowID string) (int, Row, bool)`

## Usage
- Run demo: `go run ./internal/table`
- Keys: `m` toggle mode, `↑/↓/PgUp/PgDn/Home/End` navigate, `i` insert after, `d`/`Del` delete, `t` toggle provider, `ctrl+t`/`Insert` toggle selection, `q` quit.
- Construct a list:
  - Slice: `list := NewSliceList([]Row{ SimpleRow{ID: "id-1"}, SimpleRow{ID: "id-2"} })`
  - Linked: `list := NewLinkedList([]Row{ SimpleRow{ID: "id-1"} })`
- Build a table: `bt := NewBigTable(columns, list, width, height)`

## Testing
- Package tests: `go test ./internal/table -v`
- Covers: truncation, selection overlay, provider ops, selector stability, and horizontal pan.

## Notes
- Always import v2 packages: `github.com/charmbracelet/bubbletea/v2`, `github.com/charmbracelet/bubbles/v2/...`, `github.com/charmbracelet/lipgloss/v2`.
- Rows are ASCII only; ANSI is produced by styles. Truncation happens on ASCII first, then styles are applied.
