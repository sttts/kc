# Repository Guidelines

This folder is self-contained for the table component. See `./REQUIREMENTS.md` for specs and `./TODO.md` for tasks.

## Structure
- `main.go`: demo app showcasing the table.
- `truncate_test.go`: width/truncation tests (extend as features land).
- Future: component source under this folder with tests next to code.

## Interfaces
- `Row`: `Columns() (id string, cells []string, styles []*lipgloss.Style, exists bool)` — ASCII cells + per-cell style, stable ID.
- `List`: `Lines(top, num int) []Row`, `Above(rowID string, num int) []Row`, `Below(rowID string, num int) []Row`, `Len() int` — windowed access for large datasets.

## Default Data Sources
- `SimpleRow`: reusable row with `ID`, `Cells []string`, `Styles []*lipgloss.Style` and `SetColumn(col, text, style)`.
- `SliceList` (slice-backed, stores `[]Row`): fast indexing; supports `InsertBefore/After`, `Append/Prepend`, `RemoveIDs`, `RemoveAt` by copying surrounding slices.
- `LinkedList` (doubly linked, stores `Row` nodes): efficient inserts/removes by pointer; linear indexing. Exposes `InsertBeforeID/InsertAfterID`, `Append/Prepend`, `RemoveIDs`.

Example:
```go
rows := []SliceRow{{ID:"a"}, {ID:"b"}}
list := NewSliceList(rows)
list.InsertAfter("a", SliceRow{ID:"a1"})
// Swap data in the table:
// bt.SetList(list)
```

## Build & Test
- Run demo: `go run ./internal/table`
- Build: `go build ./internal/table`
- Tests: `go test ./internal/table -v` (coverage: `-cover`)
- Format/lint: `go fmt ./internal/table/...`, `go vet ./internal/table/...`

## Style & Dependencies
- Go fmt–clean code; early returns; small, focused types.
- Naming: packages lower-case; exported `CamelCase`, unexported `camelCase`.
- Errors: wrap with `%w` (e.g., `fmt.Errorf("reading config: %w", err)`).
- Bubble Tea/Bubbles/Lipgloss v2 only: import paths must include `/v2`.

## Testing
- Standard `testing` with table-driven tests and `t.Run`.
- Keep tests deterministic; no live clusters or network.

## Commits & PRs
- Short, imperative subjects; focused diffs with relevant tests/docs.
- Before pushing: `go build ./...` and `go test ./...`.
- Include the AI co-author line in commits: `Co-Authored-By: Codex CLI Agent <noreply@openai.com>`.
