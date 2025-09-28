# Table Demo (internal/table/demo)

A minimal Bubble Tea app showcasing the `table` package.

## Run
- Slice provider (default): `go run ./internal/table/demo slice`
- Linked list provider: `go run ./internal/table/demo linked`

## Keys
- `m`: toggle mode (Scroll ↔ Fit)
- `↑/↓/PgUp/PgDn/Home/End`: navigate
- `ctrl+t` or `Insert`: toggle selection for current row
- `i`: insert a new row after the current row
- `d` or `Delete`: delete the current row
- `t`: toggle provider (slice ↔ linked)
- `q`: quit

## Notes
- The demo generates ASCII-only cell content and applies per-cell `lipgloss` styles.
- The underlying component lives at `github.com/sttts/kc/internal/table`.

## Styling the Demo
- You can customize the look by calling `SetStyles` on the `BigTable` instance. Example:
  ```go
  st := table.DefaultStyles()
  st.Selector = st.Selector.Background(lipgloss.Color("#4444FF")).Foreground(lipgloss.Color("#000000"))
  st.Header = st.Header.Foreground(lipgloss.Color("#A0FFA0"))
  bt.SetStyles(st)
  ```
