# Repository Guidelines

## Dependencies Policy (Important)
- Only use Bubble Tea v2: import `github.com/charmbracelet/bubbletea/v2` everywhere (including tests). Do NOT import `github.com/charmbracelet/bubbletea` without `/v2`.
- Keep module paths consistent with Go’s major version semantics. If a module ships a v2+, the import path must include the `/vN` suffix.
- If you see mixed v0/v1 vs v2 imports, fix them immediately and run `go mod tidy`.
- Example:
  - Correct: `tea "github.com/charmbracelet/bubbletea/v2"`
  - Incorrect: `tea "github.com/charmbracelet/bubbletea"` (will break types between v1/v2)

## Project Structure & Module Organization
- `cmd/kc/`: Application entrypoint (main package).
- `internal/ui/`: TUI components (App, Panel, Terminal).
- `pkg/handlers/`: Resource handlers and registry.
- `pkg/kubeconfig/`: Kubeconfig discovery and client creation.
- `pkg/navigation/`, `pkg/resources/`: Navigation and resource helpers.
- `examples/`: Minimal runnable samples (e.g., `examples/handler`, `examples/kubeconfig`).
- Tests live next to code as `*_test.go` within each package.

## Build, Test, and Development Commands
- Build binary: `go build -o kc ./cmd/kc`
- Run binary: `./kc`
- Run without building: `go run ./cmd/kc`
- Run examples: `go run examples/handler/main.go`
- All tests (verbose): `go test ./... -v`
- With coverage: `go test ./... -cover`
- Static checks: `go vet ./...`
- Tidy modules (after dep changes): `go mod tidy`

## Coding Style & Naming Conventions
- Format: `go fmt ./...` (CI expects gofmt-clean code).
- Lint mindset: prefer small packages, clear interfaces, early returns.
- Naming: package names lower-case, no underscores; exported identifiers `CamelCase`, unexported `camelCase`.
- Errors: wrap with `%w` (e.g., `fmt.Errorf("reading config: %w", err)`).
- Files: group closely related types; avoid large god files.

## Abstraction Guidelines
- Prefer composition over wrapping: do not re-invent controller-runtime/client-go abstractions. Embed or compose original types instead of creating near-duplicates.
- Use Kubernetes/client-go types and `controller-runtime` primitives directly (clients, caches, informers, RESTMapper). Add small adapters only where strictly needed.

## Testing Guidelines
- Framework: standard `testing` with table-driven tests and `t.Run` subtests.
- Location: `*_test.go` alongside sources (e.g., `pkg/handlers/handler_test.go`).
- Policy: non-trivial logic MUST be covered by unit tests.
- Scope: prioritize `pkg/handlers`, `pkg/kubeconfig`, and `internal/ui` model logic; keep tests deterministic (no live clusters).
- Commands: `go test ./pkg/handlers/... -v`, `go test ./internal/ui/... -v`.

## Commit & Pull Request Guidelines
- Commit style (observed): short, imperative subjects (e.g., "Add cmd/kc", "Update README").
- Commit in logical pieces; do not mix unrelated changes. Use partial staging (e.g., `git add -p`) to craft focused commits.
- Recommended: optional scope prefixes (e.g., `handlers: add pod logs`) and meaningful bodies when needed.
- PRs must include: concise summary, rationale, test plan/commands, linked issues, and screenshots/GIFs for TUI changes.
- Keep changes focused; include or update tests and docs relevant to your change.

### Build & Test Before Commit
- Always compile and run tests before committing. At minimum: `go build ./...` and `go test ./...` (or the impacted packages when the full suite is costly). Do not commit if build or tests fail.
- Before committing, run `git status` to ensure there are no unintended or forgotten changes; stage only what belongs in the focused commit.

See `TODO.md` for the active development plan and current tasks.

### Git Hygiene
- If `index.lock` exists or a commit fails due to a lock, do not delete the lock; simply retry the operation later. Avoid forceful lock removal.

### AI Disclosure
- Add an explicit co-author trailer to every commit as the last line, using the AI’s name and the maker’s noreply domain, e.g.:
  `Co-Authored-By: Codex CLI Agent <noreply@openai.com>`
- Use the exact format `Co-Authored-By: <name> <noreply@maker-domain>`.

## Security & Configuration Tips
- Never commit kubeconfigs, cluster credentials, or secrets.
- Use `KUBECONFIG` or default `~/.kube/config`; prefer non-production contexts when developing.
- Avoid tests that require cluster access; abstract clients to allow fakes/mocks.
