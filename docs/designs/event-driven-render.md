# Event-Driven Rendering

## Problem
Bubble Tea re-renders every time we return from `Update`, even if state hasn’t changed. Today, kc always returns from `Update` with some state change or timer message, so the render loop runs continuously (~60 FPS). That hits `Panel.Render`, `BigTable.rebuildWindow`, and `lipgloss` on every tick even when nothing visible changes, keeping CPU usage high.

## Goals
- Only re-render when visible UI state actually changes (selection, focus, folders, toast/spinner, etc.).
- Eliminate idle redraws coming from periodic ticks (FolderTickMsg, BusyTickMsg) when there is nothing to display.
- Maintain responsiveness: user input and dirty events still trigger immediate renders.

## Proposed Approach

### 1. Classify Messages
- **State-altering messages**: selection changes, folder dirty, toast/busy toggles, window size changes, key/mouse events, modal interactions. These should still update state and trigger a render.
- **Idle/keepalive messages**: FolderTickMsg (only needed as a fallback), BusyTickMsg (spinner), toastTickMsg. If they do not change state (e.g., spinner hidden, folder not dirty), absorb them and return `nil` so Bubble Tea doesn’t render.

### 2. Dirty Flags & Render Cache
- Each panel already caches its last rendered frame. Expose `Panel.HasCachedFrame(width,height,focused)` so App can skip calling `Render` when nothing invalidated the cache.
- Track per-panel dirty flags set by: selection changes, folder dirty listener, SetDimensions, mode/focus changes.
- When a dirty flag is false and there is no toast/busy update, skip returning a new model from `App.Update` (`return a, nil`).

### 3. Ticks & Timers
- Replace the unconditional 1s `FolderTickMsg` with `WatchFolderDirty`: only arm the tick if the current folder reports `IsDirty()==true`. Once a dirty refresh happens, disable the tick until the folder reports dirty again. (Already partially done with dirty listener + debounce.)
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
3. Stop scheduling FolderTickMsg unless needed (already debounced dirty listener triggers actual refresh). Optionally remove the tick entirely once we trust the dirty events.
4. Gate BusyTick/toastTick by checking `busyActive`/`toastActive` before scheduling.
5. Extend Panel to expose `InvalidateCache` hooks (already added) and optionally `ShouldRender(width,height,focused bool)` to help App decide whether to render.
6. Update docs to explain event-driven rendering so future features preserve the behavior.
