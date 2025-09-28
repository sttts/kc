# Table Component TODOs

- Define public model interfaces (no SetCell):
  - `type Row interface { Columns() (id string, cells []string, styles []*lipgloss.Style, exists bool) }`
  - `type List interface { Lines(top, num int) []Row; Above(rowID string, num int) []Row; Below(rowID string, num int) []Row }`
- Data sources:
  - [x] Slice-backed `SliceList` with reusable `SimpleRow` and methods: `InsertBefore/After`, `Append/Prepend`, `RemoveIDs`, `RemoveAt`.
  - [x] Doubly linked `LinkedList` with pointer-efficient inserts/removes and `Find/Above/Below/Lines` (uses `SimpleRow`).
- Implement virtualization/windowing to support 10s of thousands of rows (render only visible rows).
- Add two modes:
  - Fit mode: pre-truncate ASCII to target widths, then style; no horizontal scroll.
  - Left/Right mode: no pre-truncate; support horizontal panning with arrow keys.
- Width management: measure plain ASCII, compute target widths, avoid slicing ANSI (truncate before styling).
- Selector line: show when focused; on row removal, clamp selection to next/previous.
- Selection toggling: handle Ctrl+T and Insert; render selected rows with selection style.
- Styling options: header style defaults + overrides; borders on/off; vertical separators; Bubble Tea v2 idioms.
- Dynamic updates: efficiently diff and reflow; keep cursor stable via row IDs.
- v2 imports: ensure `/v2` for bubbletea, bubbles, and lipgloss; `go mod tidy`.
- Tests: width calc, truncation, selector clamping, selection toggles, and mode switching.
- Example: add `examples/table` demonstrating both modes, selection, and configurable styles.
