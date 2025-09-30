# Mouse Input - Current Behaviour

This note captures the mouse handling that exists today so future changes can build on a shared understanding. It mirrors the Bubble Tea message flow that drives the TUI.

## Event Routing Overview

- The program opts into `tea.WithMouseCellMotion()` so Bubble Tea emits click, release, wheel, and motion events for every cell.
- `internal/ui/app.App` owns top-level routing. When a modal is not on screen it consumes all `tea.MouseMsg`s before anyone else.
- The embedded terminal (`internal/ui/terminal.Terminal`) only sees mouse messages when the UI is in fullscreen terminal mode. While panels are visible the app never forwards mouse messages to the terminal to avoid leaking escape sequences into the two-line preview.

## Panel Mode (Panels Visible, Two-Line Terminal)

- **Panel focus & selection**: A left click inside the panel region selects the panel under the cursor (left or right) and attempts to focus the row under the pointer. Folder-backed panels resolve the visible row via BigTable (`VisibleRowID`) and call `SelectByRowID`; list-based fall back to index math.
- **Scroll wheel**: Wheel up/down events call `Panel.moveUp()`/`moveDown()` on the active panel, mirroring keyboard navigation rather than pixel scrolling.
- **Right click**: Right clicks inside a panel trigger `App.showContextMenu()`, which is currently a stub (no visible UI yet).
- **Double click**: Double-click detection is confined to folder-backed panels. If the same row ID on the same panel is clicked twice within `cfg.Input.Mouse.DoubleClickTimeout` (default 300 ms) the app invokes `Panel.enterItem()` for the selected row; otherwise it only updates the stored click metadata.
- **Two-line terminal strip**: Clicks on the two-line terminal preview are ignored. Mouse interaction with the embedded shell is keyboard-only while panels are visible.
- **Function key bar**: Clicks on the footer bar are acted upon when the button is released. The bar recomputes the rendered labels and maps the x coordinate to the corresponding function key. Disabled actions (e.g. F5/F6 today) swallow the click; enabled ones dispatch the same commands as their keyboard counterparts. The trailing `Ctrl+O` button switches to fullscreen terminal mode.

## Fullscreen Terminal Mode

- The bottom line renders a "Ctrl+O - Return to panels" toggle. Left-button release on that line switches back to panel mode and re-enables panel rendering.
- All other mouse events are forwarded to Bubbleterm. The terminal component does not yet inspect escape sequences (`SetPTYWantsMouse` is unused), so forwarding is unconditional while in fullscreen.
- When returning to panel mode the app re-enables the panel layout immediately; any mouse messages that arrive afterwards are still routed through panel-mode handlers (there is currently no suppression grace period).

## Modals

- When a modal is open it receives mouse messages first. Non-key messages that the modal does not consume (including mouse events) are still passed to the terminal component so the two-line preview stays up to date, but mouse events do not reach the panels until the modal is dismissed.

## Configuration Surface

- Double-click timing is the only mouse-related setting today. Users can customise it in `~/.kc/config.yaml` under `input.mouse.doubleClickTimeout`. The value is parsed as a Go duration string; zero values fall back to the 300 ms default.

## Known Gaps

- Drag, selection boxes, hover feedback, and direct interaction with the two-line terminal strip are intentionally unimplemented.
- Context menu plumbing exists but the actual menu UI is still a TODO, so right clicks currently have no visible effect.
- There is no detection of PTY mouse-tracking escape sequences yet, so fullscreen mode may forward mouse messages even when the underlying application never requested them.
