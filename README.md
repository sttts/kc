# Kubernetes Commander (kc)

![Kubernetes Commander TUI](docs/screenshot.png)

Experimental two-panel Kubernetes TUI inspired by Midnight Commander. Built with Go 1.24, Bubble Tea v2, and controller-runtime informers.

> **Status:** actively developed; expect breaking changes and rough edges. See `TODO.md` for the live roadmap.

## Highlights

- **Kubectl-compatible startup intents:** `kc get ...` and `kc logs ...` parse familiar CLI syntax and launch directly into the matching folders, multi-selecting objects and opening manifest/log viewers when requested (`docs/designs/cli-intents.md`).
- **Hierarchy-first navigation:** contexts → namespaces → resource groups → object folders → virtual subfolders (pod containers, ConfigMap/Secret keys, etc.). Every row is keyed by full `group/version/resource`.
- **Live data via informers:** controller-runtime caches back all folders; add/update/delete events keep selections and scroll offsets stable.
- **Two synchronized panels:** each panel can render lists or manifest viewers, supports multi-selection, per-panel options (columns/order/modes), and an integrated function-key bar. A 2-line terminal lives underneath for quick kubectl work.
- **Rich viewers:** YAML/text viewer with syntax highlighting, wrap toggle, search (`F7`), and next-match (`F3`). Logs viewer streams `kubectl logs` with follow mode, search, and End-to-follow shortcuts.
- **Config + persistence:** `~/.kc/config.yaml` controls viewer theme, panel widths, table modes, discovery TTL, mouse preferences, and more. Runtime changes (theme, table mode, etc.) persist back to disk.

## Getting Started

### Build and Run

```bash
go build -o kc ./cmd/kc   # build binary
./kc                      # run built binary

# or run directly
go run ./cmd/kc
```

The headless wrapper (`cmd/bubbleheadless`) can drive kc non-interactively: `go run ./cmd/bubbleheadless -- go run ./cmd/kc`.

### Kubectl-style shortcuts

Kubernetes Commander accepts a subset of kubectl syntax so you can jump straight to the desired view:

- `kc get pods` – focuses the Pods list for the current/flagged namespace.
- `kc get deploy myapp -o yaml` – highlights `myapp`, opens the manifest viewer in the opposite panel.
- `kc get pods/nginx pods/api svc/frontend` – multi-selects rows across resources.
- `kc logs payments-0 -c worker --follow` – drills down to the pod → container → logs row and opens the streaming viewer.

All commands honor `-n/--namespace`; when omitted, kc uses the kubeconfig’s default.

### Examples

```bash
go run examples/handler/main.go     # minimal handler wiring
go run examples/kubeconfig/main.go  # kubeconfig discovery demo
```

## Key Bindings

| Key        | Action                                                                          |
|------------|---------------------------------------------------------------------------------|
| `F1`       | Help – opens the README.md markdown viewer (windowed modal).                    |
| `F2`       | Options – context-aware panel options or viewer theme dialog.                   |
| `F3`       | View – YAML viewer, ConfigMap/Secret key viewer, pod logs viewer, etc.          |
| `F4`       | Edit – launches `kubectl edit` for the selected Kubernetes object.              |
| `F7`       | Create namespace (panels) / Search (viewers/logs).                              |
| `F8`       | Delete – opens the confirmation modal and issues a DELETE via controller client.|
| `Tab`      | Switch panels.                                                                  |
| `Ctrl+O`   | Toggle the integrated terminal.                                                 |
| `Ctrl+C`   | Quit.                                                                           |

Function keys are also reachable via `Esc+<digit>` (e.g., `Esc+3` for `F3`). Unimplemented slots (`F5/F6/F9`) are hidden/disabled until their workflows land.

## Configuration

`~/.kc/config.yaml` stores all user preferences. Key sections:

- `viewer.theme`, `viewer.mode` – chroma theme and wrap mode for YAML/text/log viewers.
- `panel.table.mode`, `panel.scrolling.horizontal.step` – object-table rendering and panning behavior.
- `resources.showNonEmptyOnly`, `resources.order`, `resources.favorites` – resource-group listing controls.
- `objects.order`, `objects.columns` – server-side Table ordering and wide/normal column display.
- `kubernetes.discovery.refresh`, `kubernetes.clusters.ttl` – discovery invalidation and cluster pool TTLs.
- `input.mouse.doubleClickTimeout` – double-click threshold for Enter events.

Run `kc` once to materialize defaults or inspect `config-default.yaml`.

## Development

Useful commands:

```bash
go fmt ./...
go test ./... -v
go vet ./...
go build ./cmd/kc
```

Keep Bubble Tea imports on v2 modules (`github.com/charmbracelet/bubbletea/v2`). See `AGENTS.md` for contributor guidelines, logging conventions, and naming rules.
