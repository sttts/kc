# UI Conventions

This document captures conventions for interaction, layout, input routing, and visual affordances used across the TUI. The goal is to keep behavior consistent and predictable while leaving room for evolution.

## Modal Types

- Fullscreen
  - Purpose: immersive viewers/editors (e.g., terminal fullscreen, YAML viewer).
  - Keys: function keys are allowed. ESC ESC closes (double-ESC); single ESC does not close.
  - Borders & Style: at most a single border; overall style follows the main app (panel) colors.
  - Stacking: topmost replaces main view; bottom line may contain a toggle (e.g., Ctrl+O to return).

- Windowed (overlay)
  - Purpose: small option pickers/dialogs pinned over the main view (e.g., theme selector, F2 resources).
  - Keys: function keys are not handled by the content; use arrows/space/Enter/Ctrl+S. Enter accepts. ESC cancels (no changes applied). Ctrl+S saves as defaults when supported.
  - Borders & Style: double border; light grey background with black text; title centered in the top border.
  - Stacking: allowed; topmost modal renders over a snapshot of the underlying view.

## Global Layout

- Panels: two side-by-side panels with frames and footers.
- Terminal: 2-line command box below panels for quick shell input; always remains live. Fullscreen terminal replaces the main view and shows a one-line return hint.
- Function Keys: a single bottom bar with F1–F10 and Ctrl+O labels. The bar also hosts a right-aligned title.
- Toasts: transient, one-line red bar shown above the function key bar; auto-dismiss after a TTL.
- Busy Overlay: a small, centered 2x2 ASCII spinner overlay for long operations; purely visual, does not intercept input.
- Resize: the entire layout reflows on terminal resize; windowed modals re-center and resnapshot their background.

## Input Routing

- Panels vs Terminal
  - Typing in the 2-line terminal keeps input routed to the terminal until Enter/Ctrl+C is sent; afterwards routing returns to panels.
  - Navigation keys (arrows, home/end, pgup/pgdown, Insert, Ctrl+T, etc.) route to the active panel when the terminal has no pending input.
  - Ctrl+O toggles fullscreen terminal on/off.

- ESC Behavior
  - In normal views: ESC cancels panel-local states (search, etc. – TBD).
  - Fullscreen modals: ESC ESC closes (double-ESC). Single ESC does not close.
  - Windowed modals: single ESC closes immediately. Double-ESC also closes (for consistency).
  - ESC+Number inside modals: reserved for special actions (e.g., ESC+2 opens the theme selector from viewers if supported).

- Function Keys
  - Main view: F1–F10 act as labeled on the function key bar.
  - Fullscreen modals: F-keys are allowed and handled by the fullscreen content.
  - Windowed modals: F-keys are ignored by content; use explicit bindings within the dialog (e.g., arrows, space, Enter, Ctrl+S).

## Mouse Behavior

- Panel mode
  - Wheel scroll: moves selection up/down in the active panel.
  - Single-click: selects a row; double-click enters/opens when applicable (timeout configurable).
  - Clicks on function key bar: trigger the corresponding action on mouse release.
  - Mouse events are not forwarded to the terminal while panels are visible.

- Fullscreen terminal
  - Mouse input is forwarded to the terminal except on the bottom toggle line (click there returns to panels).

- Windowed modals
  - Mouse is handled by the modal content when used; otherwise ignored (no forwarding to the terminal).

## Dialog Patterns

- Resources (F2) dialog
  - Windowed overlay with double border (light grey background, black text).
  - Toggles: Include empty (Yes/No), Order (Alphabetic/Group/Favorites).
  - Keys: Up/Down to move; Left/Right/Space to toggle; Enter applies to the active panel; Ctrl+S saves as config defaults; ESC cancels.

- Theme Selector
  - Invoked from viewers with F2 (or ESC+2 inside the modal).
  - Live-previews the theme; Enter applies and persists; ESC cancels (restores previous theme).

## Feedback & Notifications

- Busy overlay
  - Shown automatically after a small delay when a long operation runs (e.g., informer start, discovery, first list).
  - Does not block input; disappears when the operation completes.

- Toasts
  - Error toasts appear above the function key bar with red styling; auto-dismiss after a few seconds.
  - Emitted via the internal toast logger adapter with rate limiting to avoid storms.

## Data & Navigation Conventions

- Breadcrumbs: always computed from navigation state; never from view titles.
- Resource identity: use full Group/Version/Resource (GVR) consistently for keys/IDs.
- Resource groups: group listings (root or namespaces/<ns>) support ordering and hide-empty filters.
- Object lists: sorted by name; unaffected by the group ordering toggle.
- Per-panel options: left and right panels may have different resource view options.

## Visual Style

- Panel frames: white borders with blue background; focused title chip centered.
- Windowed modals: double border, centered title; light grey background with black text.
- Fullscreen modals: single border at most; no function key bar inside unless explicitly rendered by content.

## Implementation Notes

- Periodic refresh (folders): driven by data change; avoid unnecessary redraws to keep terminals quiet.
- Mouse double-click timeout: configurable via `input.mouse.doubleClickTimeout`.
- Security indicators: mouse/keyboard modes are gated to reduce terminal privacy prompts.
