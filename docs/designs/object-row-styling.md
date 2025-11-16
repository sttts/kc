# Object Row Styling Registry

## Problem

Today row colors are hard-coded in the folder builders (e.g., deleting objects are colored light red). We want to:
1. Highlight additional states (Pods/Nodes ready, Jobs succeeded/failed, Deployments available, etc.).
2. Let feature work add stylers without touching every folder.
3. Support CRDs or downstream builds that want custom colors.

## Proposal

Introduce a reusable *row styling registry* in `internal/models` that maps a `schema.GroupVersionResource` (or `GroupVersionKind`) to a small interface:

```go
type RowStyler interface {
    Apply(info RowStyleInfo) *lipgloss.Style
}

type RowStyleInfo struct {
    Metadata   metav1.ObjectMeta
    Status     map[string]interface{} // hydrated from unstructured/TableRow
    BaseStyle  *lipgloss.Style        // typically WhiteStyle()
}
```

Folders consult the registry after building each row and before calling `NewObjectRow`. The returned style replaces the default.

Registry API:
```go
func RegisterRowStyler(gvr schema.GroupVersionResource, styler RowStyler)
func RegisterGVKRowStyler(gvk schema.GroupVersionKind, styler RowStyler)
func RowStylerFor(gvr schema.GroupVersionResource, gvk schema.GroupVersionKind) RowStyler
```

* Behavior: exact GVR match wins; fallback to exact GVK; finally group-wide stylers (optional).
* Thread safety: registry guarded by RWMutex; registration happens during init (before UI starts).

Initial stylers:

| Resource | Styler |
|----------|--------|
| Pods | `ReadyConditionStyler` (red when Ready=False; dim amber when scheduled but not ready; default white when Ready=True). |
| Nodes | same as pods using node condition `Ready`. |
| Jobs | green when `status.succeeded>0`, red when `status.failed>0 && failed >= backoffLimit`. |
| Deployments/StatefulSets/DaemonSets | amber when `readyReplicas < desiredReplicas`, red when `status.conditions` contain `Available=False` or `Progressing=False`. |
| PV/PVC | teal when `Bound`, dim when `Released`, red when `Failed`. |

Existing deleting-style logic becomes just another styler registered for `*/*/*`.

## Data extraction

* `tablecache.Row` already exposes `ObjectMeta` and `TableRow`. We can inspect the `target` GVK saved on the row list to figure out resource identity and dig into `ObjectMeta`.
* When falling back to `unstructured.UnstructuredList`, we already have the full object to pass into the styler.
* For table rows, we can pass `RowStyleInfo.TableObject interface{}` (the raw `tablecache.Row`). Stylers can check for `runtime.Unstructured` to get fields. Alternatively, expose a helper `ExtractStatus(obj interface{}, fields ...string)` to make this easier.

## Integration

1. Build the registry and helpers in `internal/models/styling`.
2. Replace `styleForRow(...)` in `ObjectsFolder` with a call to `rowStylingRegistry.StyleFor(gvr, objMeta, status, baseStyle)`.
3. Port the deletion-highlighting logic into a default styler registered for every GVR. Specific stylers can chose to extend or override the base style (e.g., apply deleting red even if ready).
4. Provide `InitDefaultStylers()` to register built-ins (Pods, Nodes, Jobs, Deployments, PV/PVC). Call from `internal/models/init.go` or during app startup.

## Testing

- Unit tests per styler to ensure it returns the expected colors given different fake status payloads.
- Integration tests in `folder_objects_test.go` verifying that the registry is consulted (e.g., register a test styler and assert row colors change).

## Future extension

- Allow user config (e.g., `Config.ObjectStyles`) to map GVR → style names, overriding registry defaults.
- Expose CLI flags to toggle coloring for accessibility.

