# Terminal Follow Design

aim: keep the embedded PTY's kubeconfig/context/namespace in sync with the UI without touching the user's real `$HOME/.kube/config`.

a## overlay kubeconfig per PTY

- when we spawn the terminal, create a temp directory (e.g. `$XDG_RUNTIME_DIR/kc-$PID/`) and write an overlay file (`overlay.yaml`).
- set `KUBECONFIG=/path/to/overlay.yaml:$BASE_KUBECONFIG` in the PTY process environment before `exec`-ing the user's shell.
- tools like kubectl always read from the first file in `KUBECONFIG` and write any `kubectl config set-context` changes back to that file. Keeping the overlay first isolates all mutations to our temp file.

### overlay contents

Two use cases:
1. `current-context` only: minimal overlay with just `current-context: ctx-name`. Kubectl will resolve the cluster/user from the base file.

```yaml
apiVersion: v1
kind: Config
current-context: ctx-a
# kc overlay hash: <sha256-of-rest>
```

2. shadow context with namespace override: when we need to enforce a namespace local to the PTY, define a full `contexts` entry referencing the existing cluster/user and set `current-context` to the shadow name. Kubectl will find cluster/user in the base file and use the namespace from the overlay.

```yaml
apiVersion: v1
kind: Config
contexts:
- name: ctx-a
  context:
    cluster: CLUSTER_A
    user: USER_A
    namespace: team-a
current-context: ctx-a
# kc overlay hash: <sha256-of-rest>
```

Whenever the UI switches namespaces/contexts, rewrite `overlay.yaml` with the new shadow context or current-context value. Because kubectl rereads the file on every invocation, no shell injection/export is needed.

### lifecycle/cleanup

- create the overlay inside a temp dir that's unique per PTY (`/tmp/kc-$UID/<pid>/overlay.yaml`).
- on graceful shutdown, remove the dir in a `defer`.
- on startup, scan `/tmp/kc-$UID/*` for stale directories whose owning PID no longer exists and remove them (simple PID file + `kill(0, pid)` check).

This keeps the user's filesystem clean even if kc crashes.

### optional kubeconfig-copy mode

Some users prefer not to rely on env overrides. Provide a setting that copies the base kubeconfig to the temp dir, runs `kubectl config use-context` and `kubectl config set-context --current --namespace=...` against that copy, and sets `KUBECONFIG=<copy>`. Pros: kubectl sees a "normal" config. Cons: slower, touches more disk, slightly more invasive. Leave this disabled by default.

### sync triggers

- listen for navigation events that change kubeconfig/context/namespace (e.g., entering `/contexts/foo`, `/namespaces/bar`).
- when any of those change, rewrite the overlay file. Stamp each write with a first-line comment `# kc overlay hash: <hex>` so we can detect user-driven changes (kubectx/ns) versus our own rewrites. If the overlay can't be updated (disk error), show a toast and leave the previous context in place.

### testing

Add a small package (e.g., `internal/ui/termcontext`) with a state machine: inputs are `(kubeconfig path, context name, namespace)` and outputs are the overlay contents. Unit tests cover:
- initial overlay creation (current-context only)
- namespace override writes the shadow context
- switching contexts rewrites `current-context`
- cleaning up the temp dir removes the file
- reaper deletes stale dirs

Integration tests can inject a fake PTY that records its env and file contents to confirm the app sets `KUBECONFIG` correctly and rewrites the overlay as navigation changes.

### UX summary

- At PTY launch, log a short message like `[kc] terminal following context foo/ns bar` so users know the shell is synced.
- Keep everything transparent to the user: they run `kubectl` as usual, and it operates on the selected context/namespace.
- Provide a config option to disable terminal-follow entirely for users who prefer manual control.
