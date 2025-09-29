# kc Development Plan

## Milestone 1 — Hierarchy Model (Informer-Based)
- Path schema and router: `/cluster`, `/cluster/<res>`, `/cluster/namespaces/<ns>/<res>`, `/contexts/<ctx>/...`, `/kubeconfigs`, optional `/groups/<group>/<version>/...`.
- Object stores: controller-runtime informer caches per kubeconfig/context; keyed by GroupVersionResource + namespace.
- Table data: request server-side Table where supported; fall back to typed objects + local column adapters.
- Stability: keep cursor stable across updates; reconcile deletions by clamping index.
- Selection model: multi-select list, clears on path change; bulk ops act on selection.
- Tests: unit tests for router resolution, store lifecycles, and table adapters.

Current tasks
- [x] Router: add unit tests for `pkg/navigation/router.go` (Parse/Build/Parent cases incl. groups mode).
- [x] Store: scaffold `ReadOnlyStore` on controller-runtime clusters (one `cluster.Cluster` per kubeconfig+context) and wire into navigation for List.
- [ ] Store: implement Watch via cache informers with payload (PartialObjectMetadata fields) and emit a `Synced` event after informer sync.
- [ ] Store: make ClusterPool TTL configurable and add an eviction unit test.
- [x] Integration: provide a small helper to construct and inject a `CRStoreProvider` from kubeconfig+context in app startup (`resources.NewStoreProviderForContext`).
- [ ] App: call the helper during app startup and inject into navigation; manage pool lifecycle on shutdown.
- [ ] Navigation: refactor to consume Router + Store incrementally (namespaces → pods first; lazy child loading, no fragile string matching).
 - [ ] Discovery: add periodic discovery refresh (~30s) by invalidating cached discovery and resetting RESTMapper; ensure CRDs appear/disappear dynamically.
 - [ ] Generalize data sources and watchers: move from pods-specific to generic GVK/GVR-driven listings and watches; use discovery to enumerate resources under namespaces.
 - [ ] Table horizontal scroll: when a server-side Table exceeds panel width, support column-wise horizontal scrolling with Left/Right keys. Only enable when the terminal has not received typed input (same gating logic used for Enter routing to terminal vs panel).
 - [ ] Table column separators vs selection: uninterrupted selector across columns. Today lipgloss.table uses a single global border style so the cyan selection bar is visually interrupted at the vertical divider. Explore upstream support in lipgloss.table for per-row column-border styling (inherit row background) or an extension hook. For now, accept the interruption and revisit later.
- [ ] Favorites: build a favorites list of resource types (seed from discovery alias "all"); allow users to add/remove favorites to override discovery. Persist and use favorites to populate resource selectors and shortcuts.

Hierarchy refactor and tests — ordered
- [ ] Navigation: make Folders self‑sufficient. Each Folder lazily populates its rows from injected Deps and Enterable rows return the next Folder. Keep `WithBack` for a synthetic ".." row in presentation.
- [ ] Extract UI‑agnostic Folder constructors into `internal/navigation` with a small `Deps` bundle (ResMgr, Store, CtxName). Remove row‑building from the UI.
- [ ] Programmatic goto (namespaces): implement simple Enter‑driven path stepping for `/namespaces/<ns>` without builders.
- [ ] Envtest integration tests: start apiserver, seed ns/configmap/secret/node; verify walking Root → Namespaces → Groups → Objects → Keys; Back to parent; cluster‑scoped list. Tests only import navigation/internal/cluster/table (no UI).

Detailed next steps (post‑compaction anchors)
- [ ] Replace legacy folder builders in `internal/ui/app.go` (buildNamespacesFolder/buildNamespacedGroupsFolder/buildNamespacedObjectsFolder/buildClusterObjectsFolder) with the new self‑sufficient folders:
  - Root: `nav.NewRootFolder(nav.Deps{Cl:a.cl, Ctx:a.ctx, CtxName:a.currentCtx.Name})` [done]
  - Namespaces: `nav.NewNamespacesFolder(deps)`
  - Groups: `nav.NewNamespacedGroupsFolder(deps, ns)`
  - Objects (namespaced/cluster): `nav.NewNamespacedObjectsFolder(deps, gvr, ns)` / `nav.NewClusterObjectsFolder(deps, gvr)`
  - Virtual children (pods/configmaps/secrets): use the GVR→child registry; no string comparisons.
- [ ] Programmatic goto for namespaces without builders:
  - Parse target `/namespaces/<ns>`; compose Enter steps by scanning rows for the ID and calling `Enter()`.
  - Fallbacks: missing ns ⇒ root. On success, set navigator selection to the `/<ns>` row ID.
- [ ] Live updates via controller‑runtime cache informers (no custom Store):
  - For each folder that should refresh on changes, use `cl.GetCache().GetInformer(ctx, &unstructured.Unstructured{Object: {"apiVersion": gv, "kind": kind}})` to obtain a shared informer.
  - Hook Add/Update/Delete to invalidate the folder’s cached list (e.g., set `once` to zero or refresh `list` in a thread‑safe manner) and trigger UI refresh.
  - Ensure all informers use the app’s `ctx` (no `Background()`).
- [ ] Envtest tests (internal/navigation/hierarchy_envtest_test.go):
  - Start envtest; build `cl := kccluster.New(cfg)` and `deps := nav.Deps{Cl: cl, Ctx: ctx, CtxName: "envtest"}`
  - Seed: ns `testns`; cm `cm1` with keys `a`, `b`; secret `sec1` with keys `x`, `y`; node `n1`.
  - Walk:
    1) f := nav.NewRootFolder(deps) ⇒ assert rows include `/namespaces` and at least one cluster‑scoped resource (e.g., `nodes`).
    2) Enter `/namespaces` ⇒ assert `testns` present.
    3) Enter `testns` groups ⇒ assert `/configmaps` and `/secrets` with correct counts.
    4) Enter `/configmaps` ⇒ assert `cm1` present.
    5) Enter `cm1` ⇒ assert keys `a`, `b`.
    6) Use `Navigator` + `WithBack` to test back navigation.
    7) Cluster objects for `nodes` ⇒ assert `n1` present.
- [ ] App shutdown & ctx:
  - Ensure `app.cancel()` and `clPool.Stop()` are called on exit (done) and that all folder informers share `app.ctx`.
- [ ] Registry defaults:
  - Verify `internal/navigation/registry_defaults.go` covers pods/configmaps/secrets; add more (e.g., deployments→replicasets) if desired.
- [ ] Config:
  - Make `kubernetes.clusters.ttl` (metav1.Duration) editable and documented.
- [ ] Style: keep imports consistent (use `kccluster` alias for internal cluster, upstream packages with natural aliases).

## Milestone 2 — UI Navigation on the Model
- Panel adapter reads model nodes; implements `Enter`, `Back(..)`, breadcrumbs, and `..` entries.
- Live updates: diff -> list model -> minimal reflow; preserve scroll and cursor when possible.
- Function bar: dynamic enable/disable based on node capabilities (view/edit/delete/create).
- F-keys (initial scope): implement F3 (View YAML) and F8 (Delete). Leave scaffolds/hooks for F4/F7 and menu-driven options to prioritize extensibility over completeness.
- Sorting: per-panel sort (name, created, last change; nodes/pods capacity/consumption/status) with Asc/Desc toggle.
- Tests: panel navigation logic, sorting and enablement state, selection behavior.

Current tasks
- [x] Wire panel to Folder/Store; implement `Enter`, `..`, breadcrumbs, selection restore.
- [x] Implement F3 YAML for object lists (ObjectsFolder); add hooks for F4/F7.
- [ ] Implement per-panel sorting toggle UI and apply to list model.
 - [ ] Use Watch events to drive live updates; keep cursor stable as much as possible.
  - [ ] Ensure initial `Synced` event triggers first render to avoid empty flashes.
  - [ ] Extend live listings to namespace resources (e.g., `/namespaces/<ns>/pods`).

## Object Views (Core UX)
- [x] YAML syntax highlighting for `F3` viewer with Chroma; use predefined styles (default: Dracula) and keep panel background (no style bg). Preserve managedFields stripping.
- [x] Style selector dialog to pick a Chroma theme at runtime; persist preference in `~/.kc/config.yaml` under `viewer.theme`.
- [x] F9 opens theme selector within YAML modal; footer shows `F9 Theme`.
- [ ] Unify F-key and `Esc+digit` handling across app and modals (everywhere F-keys work, Esc+digit should too).
- [ ] YAML viewer search: start with `F7`/`Ctrl+F`/`/` (documented as `F7`+`F` in function bar); `F2` to continue to next match; highlight matches.
- [ ] Pods detail: entering a pod shows container list (containers + initContainers). Under each container, add a `logs` subresource. `F3` on `logs` opens a modal viewer; `Ctrl+F` follows (jump to end + watch). `Esc` closes.
- [ ] ConfigMaps/Secrets: entering shows data keys as file-like entries. `F3` views value in modal; `F4` edits the field in an editor modal. Handle binary secret data gracefully.
  - [x] Attach precise ViewProvider scaffolds for container spec and config key values (no breadcrumb string matching).
  - [ ] Wire viewers for `ConfigMapKeysFolder` and `SecretKeysFolder` (value rendering).
  - [ ] Add `LogsView` and wire under `PodContainersFolder`.
  - [x] Attach precise ViewProvider implementations for container spec and config key values (no breadcrumb string matching).
  - [ ] Logs: implement a logs viewer (follow mode, search). Wire container “logs” entries to open it.
- [ ] `F4` Edit: launch `kubectl edit` for the current object; refresh on successful apply.
- [ ] Function key bar: dynamic and context-aware (grey out unavailable actions per location/object).

## Terminal 2‑Line Mode
- [x] Enter and Ctrl‑C in 2‑line terminal (with prior typed input) return focus to the panel instead of sending the key to the terminal.

## Table View Enhancements
- [x] Namespaces: prefer server‑side Table with header + aligned columns.
- [ ] Horizontal scroll: when columns exceed panel width, support column‑wise scrolling with Left/Right keys; gate on “no typed input” (same Enter routing gating).
 - [ ] Dim Group column and align counts per spec; refine selection style (bold yellow) in table mode.

## Table Component (internal/table)
- [ ] Define public model interfaces (no SetCell):
  - [ ] `type Row interface { Columns() (id string, cells []string, styles []*lipgloss.Style, exists bool) }`
  - [ ] `type List interface { Lines(top, num int) []Row; Above(rowID string, num int) []Row; Below(rowID string, num int) []Row }`
- [ ] Implement virtualization/windowing to support 10s of thousands of rows (render only visible rows).
 - [x] Provide `BigTable` skeleton and tests; integrate with panels once stable.

## Current gaps (Hierarchy & Folders)
- [x] ContextsFolder: counts + default context styling.
- [x] Namespaces as ObjectsFolder: F3 YAML; enter to resource groups.
- [x] ResourcesFolder (root/ns groups): counts; Group column plain (dimming later).
- [x] Specialized child folders present (containers, keys); F3 viewers pending.
- [ ] Replace any remaining `NewSliceFolder` usage in tests with explicit constructors (`NewResourcesFolder` / `NewObjectsFolder`).
- [ ] Implement two modes:
  - [ ] Fit mode: pre-truncate ASCII to target widths, then style; no horizontal scroll.
  - [ ] Left/Right mode: no pre-truncate; support horizontal panning with arrow keys.
- [ ] Width management: measure plain ASCII widths; compute target widths; avoid slicing ANSI sequences.
- [ ] Selector line behavior: visible only when focused; if the selected row disappears, move selection to the next row (or previous if no next).
- [ ] Selection toggling: handle `Ctrl+T` and `Insert` with toggle semantics; render selected rows with selection style.
- [ ] High reusability: expose configuration options (columns, borders, header style, selection style, vertical separators; allow “no border”).
- [ ] Dynamic content: efficiently update from `List` provider; keep stable IDs for cursor stability; minimal diff/reflow.
- [ ] Bubble Tea v2 compliance: ensure imports use `/v2` for bubbletea, bubbles, and lipgloss; run `go mod tidy`.
- [ ] Header styling: good defaults with full override capability (lipgloss styles).
- [ ] Tests: unit tests for width calc, truncation, selector clamping, selection toggles, and mode switching.
- [ ] Example: add a small runnable example under `examples/table` demonstrating both modes and selection.

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

## Demos & Examples
- [x] StoreProvider demo using `ClusterPool` (2m TTL) and navigation injection (`examples/storeprovider`).

## Backlog (Post M3)
- Menu bar (mc-style) with View options: sort keys, direction, column toggles, grouping.
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
- [ ] Remove deprecated legacy builders from `internal/ui/app.go` (buildNamespacesFolder, buildNamespacedGroupsFolder, buildNamespacedObjectsFolder, buildClusterObjectsFolder). Confirm no references remain and delete code.
- [ ] Wire watchers for group-level counts, or document that counts update on next access; consider caching counts with debounce.
- [ ] Extend GVR→child registry with more defaults as needed (e.g., deployments→replicasets) and provide a public registration hook.
