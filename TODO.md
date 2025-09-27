# kc Development Plan

## Milestone 1 — Hierarchy Model (Informer-Based)
- Path schema and router: `/cluster`, `/cluster/<res>`, `/cluster/namespaces/<ns>/<res>`, `/contexts/<ctx>/...`, `/kubeconfigs`, optional `/groups/<group>/<version>/...`.
- Object stores: controller-runtime informer caches per kubeconfig/context; keyed by GroupVersionResource + namespace.
- Table data: request server-side Table where supported; fall back to typed objects + local column adapters.
- Stability: keep cursor stable across updates; reconcile deletions by clamping index.
- Selection model: multi-select list, clears on path change; bulk ops act on selection.
- Tests: unit tests for router resolution, store lifecycles, and table adapters.

## Milestone 2 — UI Navigation on the Model
- Panel adapter reads model nodes; implements `Enter`, `Back(..)`, breadcrumbs, and `..` entries.
- Live updates: diff -> list model -> minimal reflow; preserve scroll and cursor when possible.
- Function bar: dynamic enable/disable based on node capabilities (view/edit/delete/create).
- F-keys (initial scope): implement F3 (View YAML) and F8 (Delete). Leave scaffolds/hooks for F4/F7 and menu-driven options to prioritize extensibility over completeness.
- Sorting: per-panel sort (name, created, last change; nodes/pods capacity/consumption/status) with Asc/Desc toggle.
- Tests: panel navigation logic, sorting and enablement state, selection behavior.

## Milestone 3 — Terminal Follows Navigation
- Terminal context manager for the integrated PTY session.
- Strategy (non-destructive preferred):
  - Set `KUBECONFIG` in PTY env when switching kubeconfig.
  - Maintain a `kubectl` wrapper (alias/func) with `--context` and `-n/--namespace` flags synced to UI selection.
- Alternative (opt-in): run `kubectl config use-context` and `kubectl config set-context --current --namespace=...` against a copied kubeconfig to avoid mutating the user’s file.
- Sync triggers: on path changes that imply kubeconfig/context/namespace changes.
- Tests: unit tests for context manager state transitions and command string generation.

## Backlog (Post M3)
- Menu bar (mc-style) with View options: sort keys, direction, column toggles, grouping.
- Left/Right panel modes: API, Describe, YAML, Logs (pin), Top (metrics), and Ctrl+U panel swap.
- API group hierarchy mode under `/groups/...`.
- Metrics integration for Top and consumption sorts; graceful degradation.
- Extensible actions system: per-resource action registry and external tool integration with context/env passing.

## Definition of Done
- Non-trivial logic unit-tested per AGENTS.md.
- Docs updated (README, REQUIREMENTS) as features land.
- Basic performance sanity: UI remains responsive under list updates and watches.
