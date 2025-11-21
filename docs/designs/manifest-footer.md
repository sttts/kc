# Manifest Footer API Path

## Goal
- Show the Kubernetes API URL path of the currently displayed manifest in the manifest panel footer so users can quickly see the precise server endpoint for the object they are viewing.

## Scope and positioning
- Only applies to the manifest view. Other modes remain unchanged.
- The footer already exists in the panel frame; manifest mode currently suppresses it. The design enables the footer when an API path is available.
- Keep the viewer’s own footer (title + position) on the left; render the API path as the footer status on the right.

## Data source
- Use the current selection when it implements `models.ObjectItem` to access `GVR()`, `Namespace()`, and `Name()`.
- Fall back to suppression (no footer) when there is no selection, the selection is not an `ObjectItem`, or required fields are missing.
- Do not invent paths from breadcrumbs alone; only compute when GVR + name are known.

## API path computation
- Add a small helper (package-local utility) to build the canonical REST path:
  - Core group: `/api/<version>/[namespaces/<ns>/]<resource>/<name>`
  - Other groups: `/apis/<group>/<version>/[namespaces/<ns>/]<resource>/<name>`
  - Omit the trailing `<name>` when absent (future reuse).
- Keep it string-only and deterministic; no network calls or discovery are required.
- Consider reuse for future describe/logs/requests displays.

## Panel/footer wiring
- Manifest widget:
  - Drop `SuppressFooter` when an API path is available.
  - Set `FrameInfo.FooterStatus` to the rendered API path.
  - Preserve existing scroll indicators, breadcrumb behavior, and viewer footer content.
- Panel shell:
  - Pass the widget-provided footer status into `renderFooter` (now called with empty status).
  - `renderFooter` already aligns status on the right of the first footer line; it can host the API path without layout changes.
  - Keep footer hidden when `SuppressFooter` is true.

## UX details
- Truncate the path via existing footer width handling; no wrapping.
- Keep the existing viewer footer text on the left; API path stays on the right of the same line.
- When search mode is active in the viewer, its search status should remain on the left; the API path still lives on the right.

## Tests
- Unit-test the API path helper (core vs grouped, namespaced vs cluster-scoped, with/without name).
- Update manifest widget frame info tests:
  - Footer suppression off when an `ObjectItem` is selected.
  - `FooterStatus` matches the computed API path.
  - Existing scroll indicators and breadcrumb expectations remain intact.
- (Optional) Add a panel shell test ensuring footer status text is rendered when provided.

## Risks and mitigations
- Incomplete selection info: mitigate by suppressing the footer unless GVR + name exist.
- Path width overflow: truncation is already handled by footer rendering; no extra logic needed.
