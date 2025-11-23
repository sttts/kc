# kc Development Plan

## Milestone 2 — UI Navigation on the Model
- Function bar state (open): dynamically gray out unavailable actions per selection; currently the footer is static except for global env flags.
- Tests: ensure panel navigation logic, future function-bar enablement, and selection behavior remain covered once the function bar is dynamic.

Current tasks
- [x] Function bar: compute capability-aware enablement for view/edit/delete/create actions and update footer styling accordingly.

## Pod Filesystem Browser follow-ups
- [ ] Implement per-image/per-container shell probes, cache the results, and short-circuit to the fallback path when built-in shells/tools are missing.

- [ ] Upgrade controller-runtime to a newer release to improve informer/cache behavior and reduce API churn.

## Milestone 3 — Terminal Follows Navigation
- Terminal context manager for the integrated PTY session.
- Strategy (non-destructive preferred):
  - Set `KUBECONFIG` in PTY env when switching kubeconfig.
  - Maintain a `kubectl` wrapper (alias/func) with `--context` and `-n/--namespace` flags synced to UI selection.
- Alternative (opt-in): run `kubectl config use-context` and `kubectl config set-context --current --namespace=...` against a copied kubeconfig to avoid mutating the user’s file.
- Sync triggers: on path changes that imply kubeconfig/context/namespace changes.
- Tests: unit tests for context manager state transitions and command string generation.

Current tasks
- [x] Prototype env-based sync (KUBECONFIG, --context, --namespace) for the PTY.
- [ ] Optional: implement kubeconfig-copy approach; guard with a setting.
- [ ] Add small unit tests for command construction and state.

## Backlog (Post M3)
- [ ] Menu bar (mc-style) with View options: sort keys, direction, column toggles, grouping. *(Later)*
- [ ] Favorites list of resource types (allow users to add/remove favorites, persist, and use them to populate selectors/shortcuts). *(Later)*
- [ ] Panel modes roadmap:
- [ ] Provide a logs pin mode so a panel can stay on streaming `kubectl logs` output for the selected object. *(Later)*
- [ ] Implement a Top/metrics mode to show resource usage summaries. *(Later)*
- [x] Add Ctrl+U (or similar) to swap left/right panel contents. *(Next)*
- [ ] Selection sync options: let users choose whether describe/manifest follow left→right or right→left selection, or lock a panel to a specific resource. *(Needs clarification)*
- [ ] Auto-refresh indicator in detail views (spinner/toast) so users know when describe/manifest content refreshes. *(Next)*
- [ ] API group hierarchy mode under `/groups/...`. *(Later)*
- [ ] Metrics integration for Top and consumption sorts; graceful degradation. *(Later)*
- [ ] Extensible actions system: per-resource action registry and external tool integration with context/env passing. *(Later)*
- [x] Replace direct namespace list checks with a cheap lookup (no raw list) to avoid raw client calls during navigation. *(Next)*

Tracking
- Non-trivial logic MUST be unit-tested (see AGENTS.md).
- Keep commits focused; use partial staging; reference TODO items in commit bodies when helpful.

## Definition of Done
- [ ] Non-trivial logic unit-tested per AGENTS.md.
- [ ] Docs updated (README, REQUIREMENTS) as features land.
- [ ] Basic performance sanity: UI remains responsive under list updates and watches.
- [ ] Extend GVR→child registry with more defaults as needed (e.g., deployments→replicasets) and provide a public registration hook. *(Next - lower priority)*
- [ ] Cleanup/refactors backlog:
  - [ ] Factor selection replay logic into a helper to avoid duplicated “fetch/force notify” code. *(Next)*
  - [ ] Broaden panel tests to cover manifest/list modes and selection replay permutations. *(Next)*
