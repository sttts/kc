# kc Development Plan

### Immediate plan — Kubectl-compatible CLI intents (docs/designs/cli-intents.md)
1. ✅ Extend `cmd/kc` CLI parsing: add `get`/`logs` subcommands, accept multi-resource/name syntaxes (`TYPE`, `TYPE NAME`, `TYPE/NAME`, comma-separated lists), and capture supported flags (`-n/--namespace`, `-o yaml`, `-c`, `--follow`).
2. ✅ Add `StartupIntent` to `ui.RunConfig`, store it on `App`, and invoke `applyStartupIntent` once navigation is initialized.
3. ✅ Implement `applyStartupIntent` helpers: resolve resources via RESTMapper, compute shared navigation paths, execute `navigation.GoTo`, and multi-select panels per intent (resource groups, object lists, mixed `TYPE/NAME` requests).
4. ✅ Wire `get -o yaml` to switch the right panel into manifest mode while leaving the left panel on the requested list/object, selecting multiple objects when provided.
5. ✅ Implement `logs` intent flow: container resolution heuristics, GoTo down to the logs row, enqueue `openViewerForPanel`, and propagate follow/container flags.
6. ✅ Add envtest-style navigation/logs tests exercising multi-target selection, manifest preview, and logs intents (README/design doc updated).

## Milestone 1 — Hierarchy Model (Informer-Based)
- Path schema and router: `/cluster`, `/cluster/<res>`, `/cluster/namespaces/<ns>/<res>`, `/contexts/<ctx>/...`, `/kubeconfigs`, optional `/groups/<group>/<version>/...`.
- Object stores: controller-runtime informer caches per kubeconfig/context; keyed by GroupVersionResource + namespace.
- Table data: request server-side Table where supported; fall back to typed objects + local column adapters.
- Stability: keep cursor stable across updates; reconcile deletions by clamping index.
- Selection model: multi-select list, clears on path change; bulk ops act on selection.
- Tests: unit tests for router resolution, store lifecycles, and table adapters.

Current tasks
- [x] **Top priority — Store: implement Watch via cache informers with payload (PartialObjectMetadata fields) and emit a `Synced` event after informer sync.**
- [x] **Top priority — Discovery: add periodic discovery refresh (~30s) by invalidating cached discovery and resetting RESTMapper; ensure CRDs appear/disappear dynamically.**
- [x] Table horizontal scroll: when a server-side Table exceeds panel width, support column-wise horizontal scrolling with Left/Right keys. Only enable when the terminal has not received typed input (same gating logic used for Enter routing to terminal vs panel).
- [x] Table column separators vs selection: uninterrupted selector across columns. Today lipgloss.table uses a single global border style so the cyan selection bar is visually interrupted at the vertical divider. Explore upstream support in lipgloss.table for per-row column-border styling (inherit row background) or an extension hook. For now, accept the interruption and revisit later.

Detailed next steps (post‑compaction anchors)
- [x] Live updates via controller‑runtime cache informers (no custom Store):
  - [x] For each folder that should refresh on changes, use namespace-scoped dynamic watches to obtain per-folder streams.
  - [x] Hook Add/Update/Delete to invalidate the folder’s cached list (e.g., set `once` to zero or refresh `list` in a thread-safe manner) and emit an initial `Synced` event.
  - [x] Ensure all watchers share the app’s `ctx` and terminate after idle TTL.

## Milestone 2 — UI Navigation on the Model
- [x] Panel adapter reads model nodes; implements `Enter`, `Back(..)`, breadcrumbs, and `..` entries.
- [x] Live updates: diff -> list model -> minimal reflow; preserve scroll and cursor when possible.
- Function bar state (open): dynamically gray out unavailable actions per selection; currently the footer is static except for global env flags.
- [x] F-keys initial scope: implemented F3 (View YAML), F7 (Create Namespace), and F8 (Delete), with hooks for additional keys.
- [x] Sorting: per-panel sort order exposed via the options menu and applied to list models.
- Tests: ensure panel navigation logic, future function-bar enablement, and selection behavior remain covered once the function bar is dynamic.

Current tasks
- [ ] Function bar: compute capability-aware enablement for view/edit/delete/create actions and update footer styling accordingly.
- [x] Implement per-panel sorting toggle UI and apply to list model.
 - [x] Use Watch events to drive live updates; keep cursor stable as much as possible.
  - [x] Ensure initial `Synced` event triggers first render to avoid empty flashes.
  - [x] Extend live listings to namespace resources (e.g., `/namespaces/<ns>/pods`).

## Terminal Follow Mode & Context Sync
- [x] Overlay/copy kubeconfig material references real cluster/user from the active context instead of placeholders; copy mode and tests updated.
- [x] Every namespace transition updates the terminal overlay and emits a `termctx` log (covering startup, manual navigation, and runtime namespace switches).

## Object Views (Core UX)
- [x] Unify F-key and `Esc+digit` handling across app and modals (everywhere F-keys work, Esc+digit should too).
- [x] YAML viewer search: start with `F7`/`Ctrl+F`/`/` (documented as `F7`+`F` in function bar); `F2` to continue to next match; highlight matches.
- [x] Pods detail: entering a pod shows container list (containers + initContainers). Under each container, add a `logs` subresource. `F3` on `logs` opens a modal viewer; `Ctrl+F` follows (jump to end + watch). `Esc` closes.
- [x] ConfigMaps/Secrets: entering shows data keys as file-like entries. `F3` views value in modal. Handle binary secret data gracefully.
  - [x] Wire viewers for `ConfigMapKeysFolder` and `SecretKeysFolder` (value rendering).
  - [x] Add `LogsView` and wire under `PodContainersFolder`.
  - [x] Logs: implement a logs viewer (follow mode, search). Wire container “logs” entries to open it.
- [x] `F1` Help: render `README.md` in a markdown viewer (Glow widget) inside a modal; available from any panel/context.
- [x] Function key bar: dynamic and context-aware (grey out unavailable actions per location/object).
- [x] Delete modal: Enter should activate the focused button (Yes/No) in addition to Left/Right selection.

## Table View Enhancements
- [x] Horizontal scroll: when columns exceed panel width, support column‑wise scrolling with Left/Right keys; gate on “no typed input” (same Enter routing gating).
 - [x] Dim Group column and align counts per spec; refine selection style (bold yellow) in table mode.

## Table Component (internal/table)
- [x] Define public model interfaces (no SetCell):
  - [x] `type Row interface { Columns() (id string, cells []string, styles []*lipgloss.Style, exists bool) }`
  - [x] `type List interface { Lines(top, num int) []Row; Above(rowID string, num int) []Row; Below(rowID string, num int) []Row }`
- [x] Implement virtualization/windowing to support 10s of thousands of rows (render only visible rows).

## Current gaps (Hierarchy & Folders)
- [x] Replace any remaining `NewSliceFolder` usage in tests with explicit constructors (`NewResourcesFolder` / `NewObjectsFolder`).
- [ ] Implement two modes:
  - [x] Fit mode: pre-truncate ASCII to target widths, then style; no horizontal scroll.
  - [x] Left/Right mode: no pre-truncate; support horizontal panning with arrow keys.
- [x] Width management: measure plain ASCII widths; compute target widths; avoid slicing ANSI sequences.
- [x] Selector line behavior: visible only when focused; if the selected row disappears, move selection to the next row (or previous if no next).
- [x] Selection toggling: handle `Ctrl+T` and `Insert` with toggle semantics; render selected rows with selection style.
- [x] High reusability: expose configuration options (columns, borders, header style, selection style, vertical separators; allow “no border”).
- [x] Dynamic content: efficiently update from `List` provider; keep stable IDs for cursor stability; minimal diff/reflow.
- [x] Header styling: good defaults with full override capability (lipgloss styles).
- [x] Tests: unit tests for width calc, truncation, selector clamping, selection toggles, and mode switching.
- [x] Example: add a small runnable example under `examples/table` demonstrating both modes and selection.

## Milestone 3 — Terminal Follows Navigation
- Terminal context manager for the integrated PTY session.
- Strategy (non-destructive preferred):
  - Set `KUBECONFIG` in PTY env when switching kubeconfig.
  - Maintain a `kubectl` wrapper (alias/func) with `--context` and `-n/--namespace` flags synced to UI selection.
- Alternative (opt-in): run `kubectl config use-context` and `kubectl config set-context --current --namespace=...` against a copied kubeconfig to avoid mutating the user’s file.
- Sync triggers: on path changes that imply kubeconfig/context/namespace changes.
- Tests: unit tests for context manager state transitions and command string generation.

Current tasks
- [ ] Prototype env-based sync (KUBECONFIG, --context, --namespace) for the PTY.
- [ ] Optional: implement kubeconfig-copy approach; guard with a setting.
- [ ] Add small unit tests for command construction and state.

## Backlog (Post M3)
- Menu bar (mc-style) with View options: sort keys, direction, column toggles, grouping.
- Favorites list of resource types (seed from discovery alias "all"); allow users to add/remove favorites, persist, and use them to populate selectors/shortcuts.
- Expand resource-specific hierarchies beyond pods/configmaps/secrets (e.g., workloads that expose child folders or logs) once higher-priority milestones land.
- Left/Right panel modes: API, Describe, YAML, Logs (pin), Top (metrics), and Ctrl+U panel swap.
- API group hierarchy mode under `/groups/...`.
- Metrics integration for Top and consumption sorts; graceful degradation.
- Extensible actions system: per-resource action registry and external tool integration with context/env passing.

Tracking
- Non-trivial logic MUST be unit-tested (see AGENTS.md).
- Keep commits focused; use partial staging; reference TODO items in commit bodies when helpful.

## Definition of Done
- Non-trivial logic unit-tested per AGENTS.md.
- Docs updated (README, REQUIREMENTS) as features land.
- Basic performance sanity: UI remains responsive under list updates and watches.
- Panel filtering & find
  - [ ] Add object-list filtering (menu item) in panels; apply to current listing.
  - [ ] Implement `Ctrl+F` find in panels with highlighted match and `F2` next.
  - [ ] Add horizontal scrolling in panel object viewers similar to YAML (Left/Right, Ctrl-A/E), no wrapping.
- [ ] Wire watchers for group-level counts, or document that counts update on next access; consider caching counts with debounce.
- [ ] Extend GVR→child registry with more defaults as needed (e.g., deployments→replicasets) and provide a public registration hook.
- [ ] ConfigMap/Secret key editing (`F4`): launch an editor modal or external tool to mutate individual keys, then refresh the parent object.
