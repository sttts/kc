# CLI Startup Intents for Kubectl-Compatible Flows

## Goals
- Treat `kc` as a drop-in replacement for common `kubectl` invocations while keeping the TUI as the primary UX.
- Reuse the existing navigation stack and viewers instead of hand-coding shortcuts per command.
- Support the first batch of CLI flows:
  - `kc get <resource>` → focus the resource list in the left panel.
  - `kc get <resource> <name>` → drill into that object (e.g. pods → containers).
  - `kc get <resource> <name> -o yaml` → mirror the object in the left panel but open the right panel’s manifest preview so users immediately see the YAML.
  - `kc get <resource>/<name>` → treat the combined syntax the same as separate arguments.
  - `kc get <resource> <name1> <name2 …>` → land on the object list and multi-select each requested object.
  - `kc get <resource1>,<resource2>` or `kc get <resource1> <resource2>` → land on the namespace resources folder and multi-select each resource group.
  - Mixed pairs like `kc get pod/foo svc/bar` → navigate to the shallowest shared folder and highlight each requested row.
  - `kc logs <pod> [-c <container>] [--follow]` → open the pod/container logs viewer.
- Honor `-n/--namespace` everywhere a kubectl command would, falling back to the context/default namespace when omitted.

## High-Level Design
### CLI Parsing
- Keep the current global flags (`--version`, `--kubeconfig`, `-n/--namespace`) and introduce subcommands implemented with kong:
  - `kc get [flags] <resource> [name]`
  - `kc logs [flags] <pod>`
- Each subcommand owns the kubectl-compatible flags it needs. Unknown flags are collected for future compatibility but ignored for now.
- Parsing yields a `StartupIntent` struct:
  ```go
  type StartupIntent struct {
      Verb       KubectlVerb // e.g. VerbGet, VerbLogs
      Resource   string      // user input (pods, po, nodes,…)
      Name       string      // optional object name
      Namespace  string
      Container  string
      Follow     bool
      Output     string      // captures -o values such as "yaml"
  }
  ```
- `RunConfig` gains an optional `StartupIntent`. `cmd/kc/main.go` fills it after parsing.

### Applying the Intent
1. `ui.Run` creates the `App`, sets `app.namespaceOverride`, and copies the `StartupIntent`.
2. After `initData` finishes and `goToNamespace` wires both panels, call `app.applyStartupIntent()` on the UI goroutine using `enqueueCmd`.
3. `applyStartupIntent` performs:
   - Resource resolution: use the cluster’s RESTMapper (`ResourceFor`) so short forms like `po` work.
   - GoTo path building: translate the intent into `[]navigation.GoToStep` and call `navigation.GoTo` on the left navigator. This keeps the history stack/breadcrumbs consistent.
   - Panel sync: reuse the existing helper that pushes navigator state into `Panel.SetFolder`, `SetCurrentPath`, `UseFolder`, and `SelectByRowID`.
   - Verb-specific hooks (below).

### `get` Verb
- **Resource list (single target)**: if no name is provided, stop after selecting the resource group row in the namespace folder; the left panel now shows the object list.
- **Object drill-in (single target)**: when a name is present, add another GoTo step (`SelectionID=name`, `Enter=true`). For pods this lands inside `PodContainersFolder`; other resources remain at the object row (still selected so F3 works).
- **Multiple names for one resource**: navigate to the object list once and multi-select all named rows. We do not auto-enter a single object because there is no unique target.
- **Multiple resources**: determine the closest shared ancestor (usually the namespace resources folder). Navigate there and multi-select the resource group rows referenced either via commas or individual arguments. Mixed `TYPE/NAME` pairs (e.g., `pod/foo svc/bar`) likewise stop at the shared level; each row is selected so function keys operate on the visible set.
- **`-o yaml`**:
  - Always drive the left panel as above so navigation stays intuitive.
  - After GoTo succeeds, set `a.activePanel = 1`, ensure the right panel is in manifest mode, and call the same helper we use today for the describe/manifest viewer (selects the object on the right and refreshes `PanelModeManifest`).
  - No modal: the manifest view auto-opens in the right panel so users can scroll immediately. When multiple objects are requested, show their selection on the left but only render the first target’s manifest on the right.

### `logs` Verb
- Validate the namespace (use default when omitted).
- Resolve the pod via RESTMapper (`pods` resource).
- Determine the target container:
  - If `-c/--container` is set, use it.
  - Otherwise fetch the Pod (`Cluster.GetByGVR`); if only one regular container exists, pick it; if multiple regular containers or only init/ephemeral containers exist, emit a toast asking for `-c`.
- Build GoTo steps:
  1. `namespaces/<ns>/pods` resource row.
  2. Pod row (`Enter=true`).
  3. Container section row (`containers` / `init` / `ephemeral`).
  4. Container row.
  5. Select the `latest` logs row (no Enter).
- After the navigation completes, enqueue `a.openViewerForPanel(leftPanel)` so the logs modal opens using the existing `LogsProvider` code. Pass `Follow` from the intent into the `LogsSpec`.

### Error Handling
- If resolution fails (unknown resource, missing pod/container, namespace not found), show a toast and keep both panels at whatever state `goToNamespace` left them in.
- Never leave the navigators in a half-updated state: run `navigation.GoTo` on a clone or rollback on error.
- When some targets resolve and others fail, still navigate/select the valid set and show a toast summarizing the failures.

### Testing
- CLI parsing tests under `cmd/kc` to verify flags map to `StartupIntent`.
- UI/envtest coverage:
  - `get` without name selects resource list.
  - `get` with name positions the navigator at the object.
  - `get -o yaml` switches the right panel to manifest mode.
  - `logs` opens the logs modal and chooses the default container heuristics.

### Follow-Ups
- Extend the intent system to future verbs (`describe`, `exec`, `port-forward`, etc.).
- Surface unsupported flags (`-o json`, label selectors) with clear guidance.
- Allow `kc logs pod/container` syntax akin to `kubectl logs pod container`.
