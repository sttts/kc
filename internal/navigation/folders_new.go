package navigation

import (
    table "github.com/sttts/kc/internal/table"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "sync"
)

// BaseFolder provides lazy population scaffolding for concrete folders.
type BaseFolder struct {
    deps  Deps
    cols  []table.Column
    once  sync.Once
    list  *table.SliceList
    // optional metadata for object list folders
    gvr       schema.GroupVersionResource
    namespace string
    hasMeta   bool
}

func (b *BaseFolder) Columns() []table.Column { return b.cols }

// table.List implementation delegates to the lazily-populated list.
func (b *BaseFolder) ensure() { b.once.Do(func() { if b.list == nil { b.list = newEmptyList() } }) }
func (b *BaseFolder) Lines(top, num int) []table.Row { b.ensure(); return b.list.Lines(top, num) }
func (b *BaseFolder) Above(id string, n int) []table.Row { b.ensure(); return b.list.Above(id, n) }
func (b *BaseFolder) Below(id string, n int) []table.Row { b.ensure(); return b.list.Below(id, n) }
func (b *BaseFolder) Len() int { b.ensure(); return b.list.Len() }
func (b *BaseFolder) Find(id string) (int, table.Row, bool) { b.ensure(); return b.list.Find(id) }

// ObjectListMeta default impl for folders that are not object lists.
func (b *BaseFolder) ObjectListMeta() (schema.GroupVersionResource, string, bool) {
    if b.hasMeta { return b.gvr, b.namespace, true }
    return schema.GroupVersionResource{}, "", false
}

// RootFolder lists contexts, namespaces, and cluster-scoped resource groups.
type RootFolder struct{ BaseFolder }
func NewRootFolder(deps Deps) *RootFolder {
    f := &RootFolder{BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}, {Title: "Group"}, {Title: "Count"}}}}
    return f
}
func (f *RootFolder) Title() string { return "/" }
func (f *RootFolder) Key() string   { return "root" }

// NamespacesFolder lists namespaces.
type NamespacesFolder struct{ BaseFolder }
func NewNamespacesFolder(deps Deps) *NamespacesFolder {
    f := &NamespacesFolder{BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}}}
    return f
}
func (f *NamespacesFolder) Title() string { return "namespaces" }
func (f *NamespacesFolder) Key() string   { return depsKey(f.deps, "namespaces") }

// NamespacedGroupsFolder lists namespaced resource groups for a namespace.
type NamespacedGroupsFolder struct{ BaseFolder; ns string }
func NewNamespacedGroupsFolder(deps Deps, ns string) *NamespacedGroupsFolder {
    f := &NamespacedGroupsFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}, {Title: "Group"}, {Title: "Count"}}}, ns: ns}
    return f
}
func (f *NamespacedGroupsFolder) Title() string { return "namespaces/" + f.ns }
func (f *NamespacedGroupsFolder) Key() string   { return depsKey(f.deps, "namespaces/"+f.ns) }

// NamespacedObjectsFolder lists namespaced objects for a GVR + namespace.
type NamespacedObjectsFolder struct{ BaseFolder }
func NewNamespacedObjectsFolder(deps Deps, gvr schema.GroupVersionResource, ns string) *NamespacedObjectsFolder {
    f := &NamespacedObjectsFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}, gvr: gvr, namespace: ns, hasMeta: true}}
    return f
}
func (f *NamespacedObjectsFolder) Title() string { return f.gvr.Resource }
func (f *NamespacedObjectsFolder) Key() string   { return depsKey(f.deps, "namespaces/"+f.namespace+"/"+f.gvr.Resource) }

// ClusterObjectsFolder lists cluster-scoped objects for a GVR.
type ClusterObjectsFolder struct{ BaseFolder }
func NewClusterObjectsFolder(deps Deps, gvr schema.GroupVersionResource) *ClusterObjectsFolder {
    f := &ClusterObjectsFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}, gvr: gvr, hasMeta: true}}
    return f
}
func (f *ClusterObjectsFolder) Title() string { return f.gvr.Resource }
func (f *ClusterObjectsFolder) Key() string   { return depsKey(f.deps, f.gvr.Resource) }

// PodContainersFolder lists containers + initContainers for a pod.
type PodContainersFolder struct{ BaseFolder; ns, pod string }
func NewPodContainersFolder(deps Deps, ns, pod string) *PodContainersFolder {
    f := &PodContainersFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}}, ns: ns, pod: pod}
    return f
}
func (f *PodContainersFolder) Title() string { return "containers" }
func (f *PodContainersFolder) Key() string   { return depsKey(f.deps, "namespaces/"+f.ns+"/pods/"+f.pod+"/containers") }

// ConfigMapKeysFolder lists data keys for a ConfigMap.
type ConfigMapKeysFolder struct{ BaseFolder; ns, name string }
func NewConfigMapKeysFolder(deps Deps, ns, name string) *ConfigMapKeysFolder {
    f := &ConfigMapKeysFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}}, ns: ns, name: name}
    return f
}
func (f *ConfigMapKeysFolder) Title() string { return "data" }
func (f *ConfigMapKeysFolder) Key() string   { return depsKey(f.deps, "namespaces/"+f.ns+"/configmaps/"+f.name+"/data") }

// SecretKeysFolder lists data keys for a Secret.
type SecretKeysFolder struct{ BaseFolder; ns, name string }
func NewSecretKeysFolder(deps Deps, ns, name string) *SecretKeysFolder {
    f := &SecretKeysFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}}, ns: ns, name: name}
    return f
}
func (f *SecretKeysFolder) Title() string { return "data" }
func (f *SecretKeysFolder) Key() string   { return depsKey(f.deps, "namespaces/"+f.ns+"/secrets/"+f.name+"/data") }

// depsKey composes a stable Folder key from deps.CtxName and a relative path.
func depsKey(d Deps, rel string) string { return d.CtxName + "/" + rel }

