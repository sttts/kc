# Pod Filesystem Browser

Design for exposing a per-container filesystem tree inside kc without depending on `kubectl` binaries or altering user images.

## Goals

- Surface a `root` folder under every container row so users can browse files and directories.
- Keep the primary data path completely in-process: use Kubernetes exec APIs via `client-go`, not `kubectl`.
- Maintain long-lived shell sessions per container to make navigation responsive and preserve working directory state.
- Detect containers that lack a shell toolchain (e.g., Chainguard/distroless) and offer a controlled fallback using debug/ephemeral containers.
- Provide a uniform viewer experience: selecting files should stream contents into the existing kc viewer modal.

## Non-goals

- Full remote terminal access (this feature only surfaces read-only browsing plus optional file downloads).
- Persisting modifications to the target filesystem; write operations stay out-of-scope for now.
- Supporting clusters that block *all* exec/debug actions (kc should surface clear errors instead).

## Architecture Overview

```
container item (/pods/foo/containers/bar)
└─ /root (PodContainerRootFolder)
   ├─ etc/
   │  ├─ passwd
   │  └─ hosts
   └─ var/
      └─ log/
         └─ messages
```

1. `PodContainerListFolder` gains a second child next to `logs`: entering `/root` instantiates a `PodContainerRootFolder`.
2. `PodContainerRootFolder` creates/borrows an `ExecSession` (see below) and lists directory entries at `/`.
3. Each directory entry is an enterable child folder that reuses the same session; files are viewable items that stream content.
4. When the folder hierarchy is dropped (navigator back or panel change), the session closes.

## ExecSession Abstraction

Create `internal/podfs` to host the exec plumbing and expose:

```go
type ExecSession interface {
    List(ctx context.Context, path string) ([]FileEntry, error)
    ReadFile(ctx context.Context, path string, limit int64) (io.ReadCloser, error)
    Close() error
}

type FileEntry struct {
    Name      string
    Path      string
    Type      EntryType // file, dir, symlink, other
    Size      int64
    Mode      fs.FileMode
    UpdatedAt time.Time
}
```

### Native Shell Path

- Build `ExecSession` on top of `remotecommand.Executor` using the kube REST config already available via `Deps`.
- Spawn a **single** `sh` process per session with `Tty` enabled and `PS1=` to avoid prompts.
- After bootstrap, send small shell functions that implement the filesystem protocol (see below) so subsequent commands stay terse.
- Serialize requests on a per-session goroutine; callers enqueue commands so we never interleave shell output.
- Enforce timeouts via `context.WithTimeout` per call; if the shell wedges, cancel the exec stream and reopen a fresh session.

#### Shell Command Protocol

Once the session starts we inject two helpers:

```sh
__kc_init() {
  command -v busybox >/dev/null 2>&1 && KC_BUSYBOX=1 || KC_BUSYBOX=0
  printf '__kc_ready__\n'
}

__kc_ls() {
  dir="$1"
  cd -- "$dir" 2>/dev/null || { printf '__kc_err__|ENOENT\n'; return; }
  for f in .* *; do
    [ "$f" = "." ] || [ "$f" = ".." ] || [ "$f" = "*" ] || [ "$f" = ".*" ] || [ -e "$f" ] || continue
    if [ "$KC_BUSYBOX" = "1" ]; then
      busybox stat -c '%f|%s|%m|%Y|%n' -- "$f"
    elif command -v stat >/dev/null 2>&1; then
      stat -c '%f|%s|%m|%Y|%n' -- "$f"
    else
      # Fallback: type + size via test/du (less metadata but always available)
      if [ -d "$f" ]; then type="dir"; else type="file"; fi
      size=$(wc -c <"$f" 2>/dev/null || echo 0)
      printf '%s|%s|0000|0|%s\n' "$type" "$size" "$f"
    fi
  done
  printf '__kc_ls_done__\n'
}

__kc_cat() {
  path="$1"; limit="$2"
  if [ "$limit" -gt 0 ]; then
    dd if="$path" bs=4096 count=$(( (limit + 4095)/4096 )) 2>/dev/null
  else
    cat -- "$path"
  fi
  printf '__kc_cat_done__\n'
}
```

- The Go side waits for markers like `__kc_ls_done__` / `__kc_cat_done__` to frame responses.
- Metadata rows are pipe-delimited; the first field is a hex mode (from `stat %f`) when available, so we can infer type/permissions reliably.
- For binary file reads we stream raw bytes to the viewer and stop at the requested limit; callers can request zero/negative limit to stream entire files (bounded by viewer policy).

### Shell Probe + Image Cache

- First interaction per container consults `shellSupport[imageDigest]`:
  - `unknown`: run `command -v sh >/dev/null 2>&1 || exit 127`.
  - Cache `true/false` keyed by the resolved `status.containerStatuses[].imageID` (fall back to `spec.image` hash if empty).
  - Also store a per-container override so we can remember “this container failed even though the image passed” (security contexts can differ).
- Successful probes immediately upgrade to a long-lived session.

## Debug/Ephemeral Fallback

When the probe fails:

1. Notify the user that the container lacks shell tooling and offer to launch a debug helper (toast + modal).
2. Use `kubectl debug`-equivalent logic in-process:
   - Construct an ephemeral container spec referencing a kc-controlled helper image (e.g., `gcr.io/kc/debug-shell:latest`) that bundles BusyBox/static helper.
   - Set `targetContainer` so the new container joins the same namespaces and can see the filesystem (requires Kubernetes ≥1.22).
3. Track helper lifecycle: once the user exits the root folder or closes kc, delete the ephemeral container/pod copy.
4. After the helper becomes ready, open a native `ExecSession` against it (the helper image is known-good for shells) and proceed as normal.
5. If RBAC forbids debug pods, render an actionable error (“Exec is unavailable; cluster disallows ephemeral containers”).

## Folder & Item Types

- `PodContainerRootFolder` (new): row source backed solely by filesystem entries (no informer). Relies on `ExecSession.List` and caches results until invalidated.
- `PodFSDirItem`: enterable row representing a directory. Its `Enter` function returns another `PodContainerRootFolder` scoped to `path`.
- `PodFSFileItem`: viewable row; selecting triggers `ExecSession.ReadFile` and streams contents into kc’s viewer. Large files should stream progressively and enforce a size limit with a warning toast.
- Extend `ContainerItem` creation to append a green `root` entry next to the existing `logs` folder.

## Cancellation & Cleanup

- Sessions must observe `context.Context` passed down from the folder row sources (derived from `Deps.Ctx`). On cancel, kill the remote shell via `exec.Cmd.Process.Kill()` equivalent (`Close` on `remotecommand` streams).
- Maintain a reference count per session; when the last folder using it is dropped, call `Close`. Include a `time.Timer` so sessions idle for >N seconds auto-close to free API connections.

## UI & UX

- Navigation tree: container → `logs` + `root`. `root` rows inherit the same shortcuts (`Enter` to descend, `F3` to view file contents).
- Errors surface through toasts and row details (e.g., “permission denied”, “debug helper disabled by policy”).
- When falling back to the debug helper, show a modal summarizing the action (“kc will inject an ephemeral container into pod foo/bar”). Require confirmation unless a config flag says otherwise.
- Indicate helper usage in row details (`/root (debug helper)`).

## Security Considerations

- Honor RBAC: exec & ephemeral creation use the user’s credentials embedded in the current kubeconfig. All failures should bubble up verbatim.
- Never write files into the target container; the helper image should be read-only aside from temp directories it owns.
- Make the helper image and tag configurable so users can host it in private registries.
- Record every helper creation/deletion via `ctrllog` to aid auditing.

## Testing Strategy

- **Unit tests**: fake `ExecSession` implementations backing `PodContainerRootFolder` to cover navigation, caching, and error propagation.
- **Integration**: extend envtest or add kind-based tests that launch a pod with BusyBox plus a Chainguard-style pod to exercise both code paths (native shell success vs. debug fallback). For envtest (which lacks exec), plug a mock `ExecFactory`.
- **Manual**: script using `cmd/bubbleheadless` to walk into sample pods and verify file listings.

## Open Questions

1. Helper image distribution: do we vendor a tiny static binary instead of a full BusyBox layer?
2. Cleanup guarantees: if kc crashes mid-session, should we leave helper containers running or rely on TTL controllers?
3. Write support: should we eventually allow downloading files to local disk? (Would need explicit UX and permissions.)

These can be resolved in follow-up design iterations once the base browsing experience lands.
