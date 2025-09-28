# Kubernetes Commander (kc) Requirements

## Purpose & Scope
- Terminal UI for Kubernetes that looks and feels like Midnight Commander (mc).
- Two-panel browser over Kubernetes APIs with live data and actionable keys.

## UX Parity with mc
- Two panels; one active at a time. Switch with `Tab`.
- Navigate with arrows; `Enter` opens item; `..` goes up.
- Function-key bar always visible; actions greyed when not applicable.
- Selected items are bold yellow; selection clears when changing location.
- `Alt+I` synchronizes the inactive panel to the active panel’s location.

## Navigation Model (Paths)
- Root shows top entries followed by cluster-scoped resources:
  - `contexts` — switch context; styled bold green; selection inverts colors.
  - `namespaces` — lists namespaces; appears first after `contexts`.
  - `<cluster-scoped-resource>` — every cluster-scoped resource appears directly at the root (no `/cluster-resources`).
- Object counts: show counts after each line when known (including `namespaces`).
- Namespaced navigation:
  - `/namespaces` — list namespaces (Table when available).
  - `/namespaces/<ns>` — non-empty resource groups within the namespace (counts shown).
  - `/namespaces/<ns>/<resource>` — objects of that resource.
  - Deeper views as applicable (e.g., pod containers and subresources like `logs`).
- Contexts and kubeconfigs:
  - `/contexts` — contexts from current kubeconfig; current is emphasized.
  - `/kubeconfigs` — planned; will sit alongside `contexts` and use bold green.
- Every level shows `..` to navigate back.

## Selection & Bulk Actions
- Toggle selection with `Ctrl+T` or `Insert`; multiple selections allowed.
- Actions operate on selection when non-empty, otherwise on the focused item.

## Actions & Keys
- `F3` View YAML; `F4` Edit; `F7` Create; `F8` Delete with confirmation.
- `Ctrl+O` toggles full-screen terminal; `Alt+I` syncs panel locations.

## Terminal Integration
- Terminal follows kubeconfig, context, and namespace changes.
- Enter-only navigation if the terminal didn’t consume typed keys.

## Data Model & Live Updates
- Live via controller-runtime informers; prefer Table responses for columns.
- Each kubeconfig+context uses its own controller-runtime `cluster` (and cache).
- UI list updates are driven by watch events; emit a `Synced` signal after initial sync.

## API Surface Coverage
- Discover all resources (including CRDs) via discovery.
- Periodically refresh discovery (~30s) and reset RESTMapper to pick up changes.

## Resource‑Agnostic Model
- Navigation data structures carry full GroupVersionResource identities obtained from the cluster RESTMapper. The UI shows only resource names; internal state remains precise.
- Watch/list by GVR and namespace; render server-provided Table columns when available.
- Compose resource-specific enhancements via handlers/actions.

## Presentation
- Path breadcrumb overlays the frame (e.g., `/namespaces/kube-system/pods`) and is ellipsized from the left to keep borders intact.
- YAML viewer uses Chroma syntax highlighting without overriding the panel background.

## Contexts & Kubeconfigs
- All browsing is for the current kubeconfig and context by default.
- `/contexts` switches context; `/kubeconfigs` lists known configs and can add new ones (planned).

## API Group Hierarchy (Optional)
- Optional mode to expose group/version layout:
  - `/groups/<api-group>/<version>/[namespaces/<ns>/]<resource>`
- Group mode aids discovery and disambiguation; both classic and group paths resolve to the same data source.

## Non-Goals (for now)
- No destructive operations without confirmation.
- No partial edits without server-side validation; changes apply on save.

## Performance & Reliability
- Avoid blocking the UI on network; async loads with placeholders.
- Cache informer data per kubeconfig/context/namespace; reuse when possible.

## Table Component
- Modes: two horizontal modes
  - Fit mode: truncate ASCII cell text to fit panel width, then apply style.
  - Left/Right mode: no pre-truncate; allow horizontal scroll with arrow keys.
- Scale: support tens of thousands of rows via windowing/virtualization; render only visible rows.
- Cells: base strings are ASCII only; every cell is rendered through a style (lipgloss) applied to the full string.
- Model interface (no SetCell): provide row/list providers with stable IDs
  - `Row`: `Columns() (id string, cells []string, styles []*lipgloss.Style, exists bool)`
  - `List`: `Lines(top, num int) []Row`, `Above(rowID string, num int) []Row`, `Below(rowID string, num int) []Row`
  - Implementations may stream/live-update; IDs keep cursor stable across changes.
- Selector line: visible when focused; moves only when previously selected row disappears, then clamps to next (or previous if no next).
- Dynamic content: optimized for frequent updates; diff minimally and preserve scroll/cursor.
- Selection: toggle with `Ctrl+T` or `Insert`; selected rows render with selection style.
- Reusability: component is self-contained under `internal/table` with a clear public API.
- Styling: highly customizable borders and header (including none and vertical column separators) using Bubble Tea v2 + lipgloss; sensible defaults fully overridable.

## Viewer Search
- Start a search with `F7`, `Ctrl+F`, or `/`; `F2` finds next.
- Highlight all matches and scroll current match into view.

## Filtering & Find in Panels
- Panel object views include filtering via menu.
- `Ctrl+F` finds in the current listing; highlight and support “find next”.

## Selection by Pattern
- `+` opens a pattern dialog when the terminal has no input:
  - Input field supports glob by default (e.g., `*.yaml`), with a checkbox to switch to regexp.
  - On confirm, all objects whose names match the pattern are added to the selection.
- `-` opens the same dialog and removes matching objects from the selection.
- Pattern scope is the current view (resource object list). Matching is against the object’s name as rendered in the first column.
