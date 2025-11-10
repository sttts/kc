# Contributing to kc

First things first: kc is intentionally 100 % vibe coded. Every line so far came from an AI run, and no human has performed a real code review. If you step in, you are blazing the trail for the rest of us—thank you!

## Dev Environment

- Go 1.24+ (module toolchain is pinned in `go.mod`).
- A working Kubernetes control-plane for envtests (`kubebuilder` assets in `/usr/local/kubebuilder`), or skip those suites if you cannot run envtest locally.

Handy commands:

```bash
go fmt ./...
go test ./... -v
go vet ./...
go build ./cmd/kc
```

## Workflow & Expectations

- Imports for Bubble Tea/Bubbles must use the v2 paths (e.g. `github.com/charmbracelet/bubbletea/v2`).
- Read `AGENTS.md` for logging rules, naming conventions, module boundaries, and UI guidelines. Contributions should follow those instructions unless you propose an intentional refactor.
- The UI is driven by controller-runtime caches; avoid touching live clusters in tests. Envtests cover navigation and resource hierarchy behavior.
- Always add or update tests when you change non-trivial logic. The envtest suites live under `internal/models` and `internal/navigation`.

## Opening Changes

1. Fork the repo (or create a feature branch if you have write access).
2. Make your edits, keeping commits focused.
3. Run the relevant `go test` / `go vet` / `go build` commands above.
4. Mention that kc is still “AI-authored” in your PR description so reviewers know to double-check assumptions.

Thank you for helping turn a vibe-coded TUI into something people can rely on!
