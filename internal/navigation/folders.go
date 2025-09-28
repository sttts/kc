package navigation

import (
    table "github.com/sttts/kc/internal/table"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

// SliceFolder is a basic Folder backed by a table.SliceList and static columns.
type SliceFolder struct {
    title string
    key   string
    cols  []table.Column
    list  *table.SliceList
    // optional object-list metadata for YAML/F3
    gvr       schema.GroupVersionResource
    namespace string
    hasMeta   bool
}

var _ Folder = (*SliceFolder)(nil)

// Transitional aliases to align with design terminology while keeping
// implementation unchanged for now.
type ObjectFolder = SliceFolder
type ResourceFolder = SliceFolder
type ContextsFolder = SliceFolder

// Specialized folders for non-GVR child listings (containers, data keys).
// These are not ObjectFolders (no direct API objects), but they implement
// Folder and carry rows that are enterable/viewable as appropriate.
type PodContainersFolder = SliceFolder
type ConfigMapKeysFolder = SliceFolder
type SecretKeysFolder = SliceFolder

// NewSliceFolder builds a Folder from rows and columns with title/key metadata.
func NewSliceFolder(title, key string, cols []table.Column, rows []table.Row) *SliceFolder {
    return &SliceFolder{title: title, key: key, cols: cols, list: table.NewSliceList(rows)}
}

// Folder interface implementation -------------------------------------------------

func (f *SliceFolder) Columns() []table.Column { return f.cols }
func (f *SliceFolder) Title() string          { return f.title }
func (f *SliceFolder) Key() string            { return f.key }

// ObjectListMeta returns GVR/namespace when this folder represents a concrete
// object listing. ok=false if not applicable.
func (f *SliceFolder) ObjectListMeta() (schema.GroupVersionResource, string, bool) {
    if f.hasMeta { return f.gvr, f.namespace, true }
    return schema.GroupVersionResource{}, "", false
}

func (f *SliceFolder) Lines(top, num int) []table.Row { return f.list.Lines(top, num) }
func (f *SliceFolder) Above(rowID string, num int) []table.Row { return f.list.Above(rowID, num) }
func (f *SliceFolder) Below(rowID string, num int) []table.Row { return f.list.Below(rowID, num) }
func (f *SliceFolder) Len() int { return f.list.Len() }
func (f *SliceFolder) Find(rowID string) (int, table.Row, bool) { return f.list.Find(rowID) }

// Constructors for common folders -------------------------------------------------

// NewContextsFolder lists available contexts. Key is simply "contexts".
func NewContextsFolder(rows []table.Row) *ContextsFolder {
    return NewSliceFolder("contexts", "contexts", []table.Column{{Title: " Name"}}, rows)
}

// NewNamespacesFolder lists namespaces for a given context.
func NewNamespacesFolder(contextName string, rows []table.Row) *ObjectFolder {
    key := contextName + "/namespaces"
    sf := NewSliceFolder("namespaces", key, []table.Column{{Title: " Name"}}, rows)
    // Namespaces are cluster-scoped core/v1
    sf.gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
    sf.namespace = ""
    sf.hasMeta = true
    return sf
}

// NewGroupFolder lists objects for a GVR (namespaced or cluster-scoped).
func NewGroupFolder(contextName string, gvr schema.GroupVersionResource, namespace, title string, rows []table.Row) *ResourceFolder {
    key := contextName + "/" + gvr.String()
    if namespace != "" { key = contextName + "/namespaces/" + namespace + "/" + gvr.Resource }
    // Typical columns: Name, Group (dim), Count (right-aligned) – callers choose.
    return NewSliceFolder(title, key, []table.Column{{Title: " Name"}}, rows)
}

// NewObjectsFolder lists concrete objects of the given GVR in a namespace.
func NewObjectsFolder(contextName string, gvr schema.GroupVersionResource, namespace string, rows []table.Row) *ObjectFolder {
    title := gvr.Resource
    key := contextName + "/" + gvr.String()
    if namespace != "" { key = contextName + "/namespaces/" + namespace + "/" + gvr.Resource }
    sf := NewSliceFolder(title, key, []table.Column{{Title: " Name"}}, rows)
    sf.gvr = gvr
    sf.namespace = namespace
    sf.hasMeta = true
    return sf
}

// NewPodContainersFolder lists containers + initContainers for a pod.
func NewPodContainersFolder(contextName, namespace, pod string, rows []table.Row) *PodContainersFolder {
    title := "containers"
    key := contextName + "/namespaces/" + namespace + "/pods/" + pod + "/containers"
    return NewSliceFolder(title, key, []table.Column{{Title: " Name"}}, rows)
}

// NewConfigMapKeysFolder lists data keys for a ConfigMap.
func NewConfigMapKeysFolder(contextName, namespace, name string, rows []table.Row) *ConfigMapKeysFolder {
    title := "data"
    key := contextName + "/namespaces/" + namespace + "/configmaps/" + name + "/data"
    return NewSliceFolder(title, key, []table.Column{{Title: " Name"}}, rows)
}

// NewSecretKeysFolder lists data keys for a Secret.
func NewSecretKeysFolder(contextName, namespace, name string, rows []table.Row) *SecretKeysFolder {
    title := "data"
    key := contextName + "/namespaces/" + namespace + "/secrets/" + name + "/data"
    return NewSliceFolder(title, key, []table.Column{{Title: " Name"}}, rows)
}
