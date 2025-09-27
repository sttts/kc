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

## Milestone 2 — UI Navigation on the Model
- Panel adapter reads model nodes; implements `Enter`, `Back(..)`, breadcrumbs, and `..` entries.
- Live updates: diff -> list model -> minimal reflow; preserve scroll and cursor when possible.
- Function bar: dynamic enable/disable based on node capabilities (view/edit/delete/create).
- F-keys (initial scope): implement F3 (View YAML) and F8 (Delete). Leave scaffolds/hooks for F4/F7 and menu-driven options to prioritize extensibility over completeness.
- Sorting: per-panel sort (name, created, last change; nodes/pods capacity/consumption/status) with Asc/Desc toggle.
- Tests: panel navigation logic, sorting and enablement state, selection behavior.

Current tasks
- [ ] Wire panel to Router/Store; implement `Enter`, `..`, breadcrumbs.
- [ ] Implement F3 using server YAML (kubectl or client-go) and F8 delete with confirm; add hooks for F4/F7.
- [ ] Implement per-panel sorting toggle UI and apply to list model.
 - [ ] Use Watch events to drive live updates; keep cursor stable as much as possible.
  - [ ] Ensure initial `Synced` event triggers first render to avoid empty flashes.
  - [ ] Extend live listings to namespace resources (e.g., `/namespaces/<ns>/pods`).

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
