# Panels Architecture

## Goals
- Allow each panel to switch between multiple modes (list, describe, manifest, file browser) without turning the panel into a monolith.
- Keep business logic and data fetching inside mode-specific widgets so the panel shell only handles layout, focus, and navigation state.
- Provide clean extension points for future modes and easy unit testing of both the shell and individual widgets.

## Core Concepts

### Panel Shell
- Lives in `internal/ui/panel/` and exposes the existing `Panel` API to the rest of the TUI.
- Tracks panel dimensions, focus, scroll position, and the currently active mode.
- Owns a `map[Mode]WidgetFactory` registry; activating a mode instantiates a widget via its factory.
- Forwards lifecycle events (init, resize, focus changes, selection changes) to the active widget.
- Never inspects widget internals—rendering, key handling, and business logic stay inside widgets.

### Widgets
- Implement a narrow `Widget` interface housed in `internal/ui/panelcontent/`:
  - `Init(context.Context) tea.Cmd`
  - `Update(context.Context, tea.Msg) (tea.Cmd, bool)` – returns whether the widget consumed the message.
  - `View(context.Context, Frame) string` – draws the widget within the supplied frame metadata.
  - `Resize(Size)`, `SetFocus(bool)`, `Teardown()` for lifecycle events.
  - Optional hooks such as `OnSelectionChanged(Selection)` or `OnThemeChanged(string)` exposed through separate interfaces; the panel shell type-asserts and calls them when relevant.
- Widgets receive a `WidgetDeps` struct from the factory containing everything they need (cluster/client access, theme resolver, callbacks to query the opposite panel, async runner, etc.).
- Existing behaviors migrate into dedicated widgets:
  - **ListWidget** wraps the current folder + BigTable rendering and exposes selection updates back to the shell.
  - **DescribeWidget** builds a describe view via helpers (see “Shared Services”) and renders it using a `TextViewer`.
  - **ManifestWidget** reuses the `ViewContent()` contract to display YAML.
  - **FileWidget** can later embed a navigator without touching the panel shell.

### Mode Controller
- Implemented in `ui/app.go` to coordinate both panels.
- Handles mode dialogs (`Alt+F1`, `Alt+F2`, plus `Ctrl+1/2`) and tells the target panel to activate the chosen mode.
- Maintains cross-panel relationships: when the source panel selection changes, the controller notifies dependent widgets (describe/manifest) via their optional hooks.
- Keeps Norton Commander–style shortcuts and full-width toggling logic near the app root so the panel remains UI-agnostic.

## Shared Services
- Move describe/manifest formatting into a reusable package (e.g., `pkg/render` or `pkg/describe`).
- `DescribeWidget` requests data through helpers like `describeobject.Fetch(ctx, deps.Client, selection)`; failures surface as friendly widget messages.
- `ManifestWidget` reuses the existing `models.Viewable` contract to keep behavior consistent with the F3 viewer.
- Theme updates flow from app config into widgets through `WidgetDeps`.

## Input Handling
- The app only owns truly global shortcuts (quit, fullscreen/toggle terminal, `Alt+F1/Alt+F2`, `Ctrl+1/2`). Every other key and mouse event is forwarded directly to the active panel.
- The panel shell contains a small input router: it gives the widget first shot at each `tea.Msg`; if the widget returns “handled,” the shell stops. Otherwise the shell applies its minimal built-in navigation (selection movement, folder enter/back).
- Widgets declare their own key and mouse behaviour. `ListWidget` consumes navigation keys, selection toggles, and scroll wheel; `DescribeWidget` might handle search/theme keys; future widgets can claim drag/drop or other mouse gestures.
- Mouse coordinates are normalized by the panel before invoking the widget so individual widgets never worry about frame borders.
- Cross-panel effects still surface as high-level events (`SelectionChanged`, `ModeChanged`) rather than raw keycodes, keeping both `App` and `Panel` lean.

## Lifecycle Flow
1. App bootstraps two panels, builds a `WidgetDeps` per panel, registers widget factories for each supported mode.
2. When a mode is first activated, the panel instantiates the widget and calls `Init`.
3. Resizing or focus changes trigger `Resize`/`SetFocus` on the active widget.
4. Panel selection changes (from ListWidget, mouse, keyboard) emit a `SelectionChanged` event to the app-level controller, which forwards updates to widgets exposing `OnSelectionChanged`.
5. Rendering calls `widget.View(ctx, frame)`; the panel merely wraps the result with header/footer chrome.

## Testing Strategy
- **Panel shell tests**: mock `Widget` implementations to verify registry behavior, lifecycle propagation, and focus handling without relying on concrete widgets.
- **Widget tests**: live next to each implementation (`panelcontent/list`, `panelcontent/describe`, etc.) and use fake `WidgetDeps`.
- **Integration tests**: extend `ui/app_panel_nav_test.go` to cover mode switching, full-width toggling, and describe/manifest refresh paths while asserting only high-level outcomes.

## Extension Checklist
- Create a new widget in `internal/ui/panelcontent/<mode>/`.
- Implement the `Widget` interface plus optional hooks.
- Register the widget factory in `ui/app.go` with the appropriate mode enum.
- Update the mode dialog to list the new entry (and any help overlays to document shortcuts).
- Add widget-focused tests and, if needed, mock-based panel tests to cover new hooks.
