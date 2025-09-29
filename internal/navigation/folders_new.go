package navigation

import (
    lipgloss "github.com/charmbracelet/lipgloss/v2"
    table "github.com/sttts/kc/internal/table"
    kccluster "github.com/sttts/kc/internal/cluster"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    kcache "k8s.io/client-go/tools/cache"
    "sync"
    "fmt"
    "strings"
    "sort"
)

// BaseFolder provides lazy population scaffolding for concrete folders.
type BaseFolder struct {
    deps  Deps
    cols  []table.Column
    once  sync.Once
    list  *table.SliceList
    init  func()
    // optional metadata for object list folders
    gvr       schema.GroupVersionResource
    namespace string
    hasMeta   bool
    watchOnce sync.Once
    mu    sync.Mutex
    dirty bool
}

func (b *BaseFolder) Columns() []table.Column { return b.cols }

// table.List implementation delegates to the lazily-populated list.
func (b *BaseFolder) ensure() {
    b.once.Do(func() { if b.list == nil { b.list = newEmptyList() }; if b.init != nil { b.init() } })
    if b.dirty {
        b.mu.Lock(); b.dirty = false; if b.init != nil { b.init() }; b.mu.Unlock()
    }
}

func (b *BaseFolder) markDirty() { b.mu.Lock(); b.dirty = true; b.mu.Unlock() }

func (b *BaseFolder) watchGVK(kind schema.GroupVersionKind) {
    b.watchOnce.Do(func() {
        u := &unstructured.Unstructured{}
        u.SetGroupVersionKind(kind)
        inf, err := b.deps.Cl.GetCache().GetInformer(b.deps.Ctx, u)
        if err != nil { return }
        inf.AddEventHandler(kcache.ResourceEventHandlerFuncs{
            AddFunc: func(obj interface{}) { b.markDirty() },
            UpdateFunc: func(oldObj, newObj interface{}) { b.markDirty() },
            DeleteFunc: func(obj interface{}) { b.markDirty() },
        })
    })
}
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
    f.init = func(){ f.populate() }
    return f
}
func (f *RootFolder) Title() string { return "/" }
func (f *RootFolder) Key() string   { return "root" }

// NamespacesFolder lists namespaces.
type NamespacesFolder struct{ BaseFolder }
func NewNamespacesFolder(deps Deps) *NamespacesFolder {
    f := &NamespacesFolder{BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}}}
    f.init = func(){ f.populate() }
    return f
}
func (f *NamespacesFolder) Title() string { return "namespaces" }
func (f *NamespacesFolder) Key() string   { return depsKey(f.deps, "namespaces") }

// NamespacedGroupsFolder lists namespaced resource groups for a namespace.
type NamespacedGroupsFolder struct{ BaseFolder; ns string }
func NewNamespacedGroupsFolder(deps Deps, ns string) *NamespacedGroupsFolder {
    f := &NamespacedGroupsFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}, {Title: "Group"}, {Title: "Count"}}}, ns: ns}
    f.init = func(){ f.populate() }
    return f
}
func (f *NamespacedGroupsFolder) Title() string { return "namespaces/" + f.ns }
func (f *NamespacedGroupsFolder) Key() string   { return depsKey(f.deps, "namespaces/"+f.ns) }

// NamespacedObjectsFolder lists namespaced objects for a GVR + namespace.
type NamespacedObjectsFolder struct{ BaseFolder }
func NewNamespacedObjectsFolder(deps Deps, gvr schema.GroupVersionResource, ns string) *NamespacedObjectsFolder {
    f := &NamespacedObjectsFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}, gvr: gvr, namespace: ns, hasMeta: true}}
    f.init = func(){ f.populate() }
    return f
}
func (f *NamespacedObjectsFolder) Title() string { return f.gvr.Resource }
func (f *NamespacedObjectsFolder) Key() string   { return depsKey(f.deps, "namespaces/"+f.namespace+"/"+f.gvr.Resource) }

// ClusterObjectsFolder lists cluster-scoped objects for a GVR.
type ClusterObjectsFolder struct{ BaseFolder }
func NewClusterObjectsFolder(deps Deps, gvr schema.GroupVersionResource) *ClusterObjectsFolder {
    f := &ClusterObjectsFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}, gvr: gvr, hasMeta: true}}
    f.init = func(){ f.populate() }
    return f
}
func (f *ClusterObjectsFolder) Title() string { return f.gvr.Resource }
func (f *ClusterObjectsFolder) Key() string   { return depsKey(f.deps, f.gvr.Resource) }

// PodContainersFolder lists containers + initContainers for a pod.
type PodContainersFolder struct{ BaseFolder; ns, pod string }
func NewPodContainersFolder(deps Deps, ns, pod string) *PodContainersFolder {
    f := &PodContainersFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}}, ns: ns, pod: pod}
    f.init = func(){ f.populate() }
    return f
}
func (f *PodContainersFolder) Title() string { return "containers" }
func (f *PodContainersFolder) Key() string   { return depsKey(f.deps, "namespaces/"+f.ns+"/pods/"+f.pod+"/containers") }

// ConfigMapKeysFolder lists data keys for a ConfigMap.
type ConfigMapKeysFolder struct{ BaseFolder; ns, name string }
func NewConfigMapKeysFolder(deps Deps, ns, name string) *ConfigMapKeysFolder {
    f := &ConfigMapKeysFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}}, ns: ns, name: name}
    f.init = func(){ f.populate() }
    return f
}
func (f *ConfigMapKeysFolder) Title() string { return "data" }
func (f *ConfigMapKeysFolder) Key() string   { return depsKey(f.deps, "namespaces/"+f.ns+"/configmaps/"+f.name+"/data") }

// SecretKeysFolder lists data keys for a Secret.
type SecretKeysFolder struct{ BaseFolder; ns, name string }
func NewSecretKeysFolder(deps Deps, ns, name string) *SecretKeysFolder {
    f := &SecretKeysFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}}, ns: ns, name: name}
    f.init = func(){ f.populate() }
    return f
}
func (f *SecretKeysFolder) Title() string { return "data" }
func (f *SecretKeysFolder) Key() string   { return depsKey(f.deps, "namespaces/"+f.ns+"/secrets/"+f.name+"/data") }

// depsKey composes a stable Folder key from deps.CtxName and a relative path.
func depsKey(d Deps, rel string) string { return d.CtxName + "/" + rel }

// --------- population helpers ---------

func groupVersionString(gvk schema.GroupVersionKind) string {
    g := gvk.Group
    if g == "" { g = "core" }
    return g + "/" + gvk.Version
}

func verbsInclude(vs []string, want string) bool { for _, v := range vs { if strings.EqualFold(v, want) { return true } }; return false }

// Root: "/namespaces" + cluster-scoped resources with counts
func (f *RootFolder) populate() {
    rows := make([]table.Row, 0, 64)
    nameSty := WhiteStyle()
    // Namespaces entry
    rows = append(rows, NewEnterableItemStyled("namespaces", []string{"/namespaces", "core/v1", ""}, []*lipgloss.Style{nameSty, DimStyle(), nil}, func() (Folder, error) { return NewNamespacesFolder(f.deps), nil }))
    // Cluster-scoped resources
    if infos, err := f.deps.Cl.GetResourceInfos(); err == nil {
        // Filter and sort by resource name (plural)
        filtered := make([]kccluster.ResourceInfo, 0, len(infos))
        for _, info := range infos {
            if info.Namespaced || info.Resource == "namespaces" { continue }
            if !verbsInclude(info.Verbs, "list") { continue }
            filtered = append(filtered, info)
        }
        sort.Slice(filtered, func(i, j int) bool { return filtered[i].Resource < filtered[j].Resource })
        for _, info := range filtered {
            gvr, err := f.deps.Cl.GVKToGVR(info.GVK); if err != nil { continue }
            n := 0
            if lst, err := f.deps.Cl.ListByGVR(f.deps.Ctx, gvr, ""); err == nil { n = len(lst.Items) }
            // Use a unique GVR-based ID (group/version/resource) to avoid collisions (e.g., events).
            id := gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
            rows = append(rows, NewEnterableItemStyled(id, []string{"/"+info.Resource, groupVersionString(info.GVK), fmt.Sprintf("%d", n)}, []*lipgloss.Style{nameSty, DimStyle(), nil}, func() (Folder, error) { return NewClusterObjectsFolder(f.deps, gvr), nil }))
        }
    }
    f.list = table.NewSliceList(rows)
}

// Namespaces: list namespaces, each enterable
func (f *NamespacesFolder) populate() {
    nameSty := WhiteStyle()
    gvk := schema.GroupVersionKind{Group:"", Version:"v1", Kind:"Namespace"}
    gvr, err := f.deps.Cl.GVKToGVR(gvk); if err != nil { f.list = newEmptyList(); return }
    lst, err := f.deps.Cl.ListByGVR(f.deps.Ctx, gvr, ""); if err != nil { f.list = newEmptyList(); return }
    // Ensure we watch namespace changes (debounced refresh in UI)
    f.watchGVK(gvk)
    rows := make([]table.Row, 0, len(lst.Items))
    for i := range lst.Items { ns := lst.Items[i].GetName(); rows = append(rows, NewEnterableItem(ns, []string{"/"+ns}, func() (Folder, error) { return NewNamespacedGroupsFolder(f.deps, ns), nil }, nameSty)) }
    f.list = table.NewSliceList(rows)
}

// Namespaced groups: configmaps/secrets/etc with counts
func (f *NamespacedGroupsFolder) populate() {
    rows := make([]table.Row, 0, 64)
    nameSty := WhiteStyle()
    infos, err := f.deps.Cl.GetResourceInfos(); if err != nil { f.list = newEmptyList(); return }
    filtered := make([]kccluster.ResourceInfo, 0, len(infos))
    for _, info := range infos { if info.Namespaced && verbsInclude(info.Verbs, "list") { filtered = append(filtered, info) } }
    sort.Slice(filtered, func(i, j int) bool { return filtered[i].Resource < filtered[j].Resource })
    for _, info := range filtered {
        gvr, err := f.deps.Cl.GVKToGVR(info.GVK); if err != nil { continue }
        n := 0
        if lst, err := f.deps.Cl.ListByGVR(f.deps.Ctx, gvr, f.ns); err == nil { n = len(lst.Items) }
        // Use GVR-based unique ID; do not dim Group column in namespaced groups.
        id := gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
        rows = append(rows, NewEnterableItemStyled(id, []string{"/"+info.Resource, groupVersionString(info.GVK), fmt.Sprintf("%d", n)}, []*lipgloss.Style{nameSty, nil, nil}, func() (Folder, error) { return NewNamespacedObjectsFolder(f.deps, gvr, f.ns), nil }))
    }
    f.list = table.NewSliceList(rows)
}

func (f *NamespacedObjectsFolder) populate() {
    nameSty := WhiteStyle()
    lst, err := f.deps.Cl.ListByGVR(f.deps.Ctx, f.gvr, f.namespace); if err != nil { f.list = newEmptyList(); return }
    if k, err := f.deps.Cl.RESTMapper().KindFor(f.gvr); err == nil { f.watchGVK(k) }
    // Sort names
    names := make([]string, 0, len(lst.Items))
    for i := range lst.Items { names = append(names, lst.Items[i].GetName()) }
    sort.Strings(names)
    rows := make([]table.Row, 0, len(names))
    for _, nm := range names {
        if ctor, ok := childFor(f.gvr); ok {
            ns := f.namespace; name := nm
            rows = append(rows, NewEnterableItem(nm, []string{nm}, func() (Folder, error) { return ctor(f.deps, ns, name), nil }, nameSty))
        } else {
            rows = append(rows, NewSimpleItem(nm, []string{nm}, nameSty))
        }
    }
    f.list = table.NewSliceList(rows)
}

func (f *ClusterObjectsFolder) populate() {
    nameSty := WhiteStyle()
    lst, err := f.deps.Cl.ListByGVR(f.deps.Ctx, f.gvr, ""); if err != nil { f.list = newEmptyList(); return }
    if k, err := f.deps.Cl.RESTMapper().KindFor(f.gvr); err == nil { f.watchGVK(k) }
    // Sort names
    names := make([]string, 0, len(lst.Items))
    for i := range lst.Items { names = append(names, lst.Items[i].GetName()) }
    sort.Strings(names)
    rows := make([]table.Row, 0, len(names))
    for _, nm := range names {
        if ctor, ok := childFor(f.gvr); ok {
            name := nm
            rows = append(rows, NewEnterableItem(nm, []string{nm}, func() (Folder, error) { return ctor(f.deps, "", name), nil }, nameSty))
        } else {
            rows = append(rows, NewSimpleItem(nm, []string{nm}, nameSty))
        }
    }
    f.list = table.NewSliceList(rows)
}

func (f *PodContainersFolder) populate() {
    nameSty := WhiteStyle()
    gvk := schema.GroupVersionKind{Group:"", Version:"v1", Kind:"Pod"}
    gvr, err := f.deps.Cl.GVKToGVR(gvk); if err != nil { f.list = newEmptyList(); return }
    f.watchGVK(gvk)
    obj, err := f.deps.Cl.GetByGVR(f.deps.Ctx, gvr, f.ns, f.pod); if err != nil || obj == nil { f.list = newEmptyList(); return }
    var pod corev1.Pod
    if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pod); err != nil { f.list = newEmptyList(); return }
    rows := make([]table.Row, 0, 8)
    for _, c := range pod.Spec.Containers { if c.Name != "" { rows = append(rows, NewSimpleItem(c.Name, []string{c.Name}, nameSty)) } }
    for _, c := range pod.Spec.InitContainers { if c.Name != "" { rows = append(rows, NewSimpleItem(c.Name, []string{c.Name}, nameSty)) } }
    f.list = table.NewSliceList(rows)
}

func (f *ConfigMapKeysFolder) populate() {
    nameSty := WhiteStyle()
    gvk := schema.GroupVersionKind{Group:"", Version:"v1", Kind:"ConfigMap"}
    gvr, err := f.deps.Cl.GVKToGVR(gvk); if err != nil { f.list = newEmptyList(); return }
    f.watchGVK(gvk)
    obj, err := f.deps.Cl.GetByGVR(f.deps.Ctx, gvr, f.ns, f.name); if err != nil || obj == nil { f.list = newEmptyList(); return }
    var cm corev1.ConfigMap
    if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &cm); err != nil { f.list = newEmptyList(); return }
    rows := make([]table.Row, 0, 16)
    keys := make([]string, 0, len(cm.Data))
    for k := range cm.Data { keys = append(keys, k) }
    sort.Strings(keys)
    for _, k := range keys { rows = append(rows, NewSimpleItem(k, []string{k}, nameSty)) }
    f.list = table.NewSliceList(rows)
}

func (f *SecretKeysFolder) populate() {
    nameSty := WhiteStyle()
    gvk := schema.GroupVersionKind{Group:"", Version:"v1", Kind:"Secret"}
    gvr, err := f.deps.Cl.GVKToGVR(gvk); if err != nil { f.list = newEmptyList(); return }
    f.watchGVK(gvk)
    obj, err := f.deps.Cl.GetByGVR(f.deps.Ctx, gvr, f.ns, f.name); if err != nil || obj == nil { f.list = newEmptyList(); return }
    var sec corev1.Secret
    if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &sec); err != nil { f.list = newEmptyList(); return }
    rows := make([]table.Row, 0, 16)
    keys := make([]string, 0, len(sec.Data))
    for k := range sec.Data { keys = append(keys, k) }
    sort.Strings(keys)
    for _, k := range keys { rows = append(rows, NewSimpleItem(k, []string{k}, nameSty)) }
    f.list = table.NewSliceList(rows)
}
