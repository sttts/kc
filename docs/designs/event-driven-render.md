# Event-Driven Rendering

## Problem
Bubble Tea re-renders every time we return from `Update`, even if state hasn’t changed. Today, kc always returns from `Update` with some state change or timer message, so the render loop runs continuously (~60 FPS). That hits `Panel.Render`, `BigTable.rebuildWindow`, and `lipgloss` on every tick even when nothing visible changes, keeping CPU usage high.

## Goals
- Only re-render when visible UI state actually changes (selection, focus, folders, toast/spinner, etc.).
- Eliminate idle redraws coming from periodic ticks (FolderTickMsg, BusyTickMsg) when there is nothing to display.
- Maintain responsiveness: user input and dirty events still trigger immediate renders.

## Proposed Approach

### Current Idle Message Sources
- ~~**FolderTickMsg** (`internal/ui/app.go`: Init + handler) – always scheduled every second, even when no folders are dirty. Every tick runs through `App.Update`, touches both panels, and returns another tick, forcing a render.~~ (Removed; dirty listeners now drive refreshes immediately after folder changes.)
- **BusyTickMsg** (`internal/ui/app.go`: BusyShow/BusyTick cases) – once the spinner starts we keep ticking at 100ms regardless of whether the busy indicator is still visible until `BusyHideMsg` resets `busyActive`.
- **toastTickMsg** (`internal/ui/app.go`: showToast/tick) – similar cadence to BusyTick; we continue to reschedule ticks after the toast expires until the handler notices `toastActive` is false.
- **Modal RedrawTickMsg** (`internal/ui/modal.go`: 249) – any windowed modal with a background function schedules a redraw tick every 100ms to keep the background fresh, even when the modal content is static.
- **Escape-sequence timers** (`internal/ui/modal.go`: 207, `App.Update` ESC handler) – these only fire temporarily after ESC, but we should ensure they are short-lived and not re-arming when no modal is open.
- **Background watchers** (`App.watchDiscovery`, namespace retry) – these rarely fire, but when they do they currently trigger a full render regardless of whether any visible state changed.

### 1. Classify Messages
- **State-altering messages**: selection changes, folder dirty, toast/busy toggles, window size changes, key/mouse events, modal interactions. These should still update state and trigger a render.
- **Idle/keepalive messages**: FolderTickMsg (only needed as a fallback), BusyTickMsg (spinner), toastTickMsg. If they do not change state (e.g., spinner hidden, folder not dirty), absorb them and return `nil` so Bubble Tea doesn’t render.

### 2. Dirty Flags & Render Cache
- Each panel already caches its last rendered frame. Expose `Panel.HasCachedFrame(width,height,focused)` so App can skip calling `Render` when nothing invalidated the cache.
- Track per-panel dirty flags set by: selection changes, folder dirty listener, SetDimensions, mode/focus changes.
- When a dirty flag is false and there is no toast/busy update, skip returning a new model from `App.Update` (`return a, nil`).

### 3. Ticks & Timers
- Folder refresh ticks have been removed entirely; folder dirty listeners trigger refreshes immediately and we perform an eager refresh when assigning a new folder so panels render once without waiting for external events.
- Busy spinner: only continue BusyTick when spinner is visible. When hidden, stop scheduling ticks.
- Toast auto-dismiss: keep `toastTickMsg`, but if the toast has already expired, don’t schedule another tick.

### 4. App.Update guard
Add a helper `stateChanged bool` that is set anytime a message mutates state. At the end of `Update`, if `stateChanged` is false and there are no queued commands, return `a, nil`. This prevents Bubble Tea from re-rendering for messages we fully absorbed.

### 5. Testing
- Add integration tests (internal/ui) that simulate idle periods (no dirty events) to confirm `App.Update` returns `nil` and `Panel.Render` isn’t invoked.
- Add logging or counters around `Panel.Render` in debug builds to confirm renders correlate with actual events.
- Manual profiling after implementation to ensure render path is near-zero when idle.

## Implementation Steps
1. Add `stateChanged bool` to App.Update; set it whenever we mutate panels, toasts, busy flags, etc.; skip returning commands when false.
2. Ensure every message handler that currently no-ops (e.g., FolderDirtyMsg when panel nil) doesn’t mark dirty.
3. Stop scheduling FolderTickMsg unless needed (already debounced dirty listener triggers actual refresh). Optionally remove the tick entirely once we trust the dirty events. ✅
4. Gate BusyTick/toastTick by checking `busyActive`/`toastActive` before scheduling.
5. Extend Panel to expose `InvalidateCache` hooks (already added) and optionally `ShouldRender(width,height,focused bool)` to help App decide whether to render.
6. Update docs to explain event-driven rendering so future features preserve the behavior.
