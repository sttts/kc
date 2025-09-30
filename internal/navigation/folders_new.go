package navigation

import (
    lipgloss "github.com/charmbracelet/lipgloss/v2"
    table "github.com/sttts/kc/internal/table"
    kccluster "github.com/sttts/kc/internal/cluster"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    types "k8s.io/apimachinery/pkg/types"
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
    // absolute breadcrumb path segments for this folder
    path []string
    // optional metadata for object list folders
    gvr       schema.GroupVersionResource
    namespace string
    hasMeta   bool
    watchOnce sync.Once
    mu    sync.Mutex
    dirty bool
}

func (b *BaseFolder) Columns() []table.Column { return b.cols }

// Title returns a systematic title for object-list folders using their
// metadata (namespace + resource). Non-object folders should override Title.
func (b *BaseFolder) Title() string {
    if b.hasMeta {
        if b.namespace != "" { return "namespaces/" + b.namespace + "/" + b.gvr.Resource }
        return b.gvr.Resource
    }
    return ""
}

// table.List implementation delegates to the lazily-populated list.
func (b *BaseFolder) ensure() {
    b.once.Do(func() { if b.list == nil { b.list = newEmptyList() }; if b.init != nil { b.init() } })
    if b.dirty {
        b.mu.Lock(); b.dirty = false; if b.init != nil { b.init() }; b.mu.Unlock()
    }
}

func (b *BaseFolder) markDirty() { b.mu.Lock(); b.dirty = true; b.mu.Unlock() }

// Refresh marks the folder content dirty so the next access will repopulate
// based on current dependencies (e.g., updated ViewOptions).
func (b *BaseFolder) Refresh() { b.markDirty() }

// watchGVR sets up an informer for the given GVR (resolved to a Kind internally)
// and marks the folder dirty on add/update/delete. This avoids leaking Kinds into
// the folder API: callers only pass GVRs.
func (b *BaseFolder) watchGVR(gvr schema.GroupVersionResource) {
    b.watchOnce.Do(func() {
        // Resolve to Kind for informer construction; keep GVR as the external identity.
        k, err := b.deps.Cl.RESTMapper().KindFor(gvr)
        if err != nil { return }
        u := &unstructured.Unstructured{}
        u.SetGroupVersionKind(k)
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
    f := &RootFolder{BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}, {Title: "Group"}, {Title: "Count"}}, path: []string{}}}
    f.init = func(){ f.populate() }
    return f
}
func (f *RootFolder) Title() string { return "/" }
func (f *RootFolder) Key() string   { return "root" }


// NamespacedGroupsFolder lists namespaced resource groups for a namespace.
type NamespacedGroupsFolder struct{ BaseFolder; ns string }
func NewNamespacedGroupsFolder(deps Deps, ns string, basePath []string) *NamespacedGroupsFolder {
    f := &NamespacedGroupsFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}, {Title: "Group"}, {Title: "Count"}}, path: append([]string(nil), basePath...)}, ns: ns}
    f.init = func(){ f.populate() }
    return f
}
func (f *NamespacedGroupsFolder) Title() string { return "namespaces/" + f.ns }
func (f *NamespacedGroupsFolder) Key() string   { return depsKey(f.deps, "namespaces/"+f.ns) }

// NamespacedObjectsFolder lists namespaced objects for a GVR + namespace.
type NamespacedObjectsFolder struct{ BaseFolder }
func NewNamespacedObjectsFolder(deps Deps, gvr schema.GroupVersionResource, ns string, basePath []string) *NamespacedObjectsFolder {
    f := &NamespacedObjectsFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}, gvr: gvr, namespace: ns, hasMeta: true, path: append([]string(nil), basePath...)}}
    f.init = func(){ f.populate() }
    return f
}
func (f *NamespacedObjectsFolder) Title() string { return f.gvr.Resource }
func (f *NamespacedObjectsFolder) Key() string {
    // Use full GVR to avoid collisions between same resource names in different groups/versions
    gv := f.gvr.GroupVersion().String()
    return depsKey(f.deps, "namespaces/"+f.namespace+"/"+gv+"/"+f.gvr.Resource)
}

// ClusterObjectsFolder lists cluster-scoped objects for a GVR.
type ClusterObjectsFolder struct{ BaseFolder }
func NewClusterObjectsFolder(deps Deps, gvr schema.GroupVersionResource, basePath []string) *ClusterObjectsFolder {
    f := &ClusterObjectsFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}, gvr: gvr, hasMeta: true, path: append([]string(nil), basePath...)}}
    f.init = func(){ f.populate() }
    return f
}
func (f *ClusterObjectsFolder) Title() string { return f.gvr.Resource }
func (f *ClusterObjectsFolder) Key() string {
    // Use full GVR for stable uniqueness across groups/versions
    gv := f.gvr.GroupVersion().String()
    return depsKey(f.deps, gv+"/"+f.gvr.Resource)
}

// PodContainersFolder lists containers + initContainers for a pod.
type PodContainersFolder struct{ BaseFolder; ns, pod string }
func NewPodContainersFolder(deps Deps, ns, pod string, basePath []string) *PodContainersFolder {
    f := &PodContainersFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}, path: append([]string(nil), basePath...)}, ns: ns, pod: pod}
    f.init = func(){ f.populate() }
    return f
}
func (f *PodContainersFolder) Title() string { return "containers" }
func (f *PodContainersFolder) Key() string   { return depsKey(f.deps, "namespaces/"+f.ns+"/pods/"+f.pod+"/containers") }

// ConfigMapKeysFolder lists data keys for a ConfigMap.
type ConfigMapKeysFolder struct{ BaseFolder; ns, name string }
func NewConfigMapKeysFolder(deps Deps, ns, name string, basePath []string) *ConfigMapKeysFolder {
    f := &ConfigMapKeysFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}, path: append([]string(nil), basePath...)}, ns: ns, name: name}
    f.init = func(){ f.populate() }
    return f
}
func (f *ConfigMapKeysFolder) Title() string { return "data" }
func (f *ConfigMapKeysFolder) Key() string   { return depsKey(f.deps, "namespaces/"+f.ns+"/configmaps/"+f.name+"/data") }
// Parent returns the coordinates of the owning ConfigMap
func (f *ConfigMapKeysFolder) Parent() (schema.GroupVersionResource, string, string) {
    return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, f.ns, f.name
}

// SecretKeysFolder lists data keys for a Secret.
type SecretKeysFolder struct{ BaseFolder; ns, name string }
func NewSecretKeysFolder(deps Deps, ns, name string, basePath []string) *SecretKeysFolder {
    f := &SecretKeysFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}, path: append([]string(nil), basePath...)}, ns: ns, name: name}
    f.init = func(){ f.populate() }
    return f
}
func (f *SecretKeysFolder) Title() string { return "data" }
func (f *SecretKeysFolder) Key() string   { return depsKey(f.deps, "namespaces/"+f.ns+"/secrets/"+f.name+"/data") }
// Parent returns the coordinates of the owning Secret
func (f *SecretKeysFolder) Parent() (schema.GroupVersionResource, string, string) {
    return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}, f.ns, f.name
}

// depsKey composes a stable Folder key from deps.CtxName and a relative path.
func depsKey(d Deps, rel string) string { return d.CtxName + "/" + rel }

// --------- population helpers ---------

func groupVersionString(gvk schema.GroupVersionKind) string {
    if gvk.Group == "" {
        return gvk.Version
    }
    return gvk.Group + "/" + gvk.Version
}

func verbsInclude(vs []string, want string) bool { for _, v := range vs { if strings.EqualFold(v, want) { return true } }; return false }

// Root: "/namespaces" + cluster-scoped resources with counts
func (f *RootFolder) populate() {
    rows := make([]table.Row, 0, 64)
    nameSty := WhiteStyle()
    // Contexts entry (if provided) with count
    if f.deps.ListContexts != nil {
        base := append(append([]string(nil), f.path...), "contexts")
        cnt := 0
        if f.deps.ListContexts != nil { cnt = len(f.deps.ListContexts()) }
        rows = append(rows, NewEnterableItemStyled("contexts", []string{"/contexts", "", fmt.Sprintf("%d", cnt)}, base, []*lipgloss.Style{GreenStyle(), nil, nil}, func() (Folder, error) { return NewContextsFolder(f.deps, base), nil }))
    }
    // Namespaces entry (core group appears as just version: v1) with count
    nsBase := append(append([]string(nil), f.path...), "namespaces")
    nsCount := 0
    gvrNS := schema.GroupVersionResource{Group:"", Version:"v1", Resource:"namespaces"}
    if lst, err := f.deps.Cl.ListByGVR(f.deps.Ctx, gvrNS, ""); err == nil { nsCount = len(lst.Items) }
    rows = append(rows, NewEnterableItemStyled("namespaces", []string{"/namespaces", "v1", fmt.Sprintf("%d", nsCount)}, nsBase, []*lipgloss.Style{nameSty, nil, nil}, func() (Folder, error) { return NewClusterObjectsFolder(f.deps, gvrNS, nsBase), nil }))
    // Cluster-scoped resources
    if infos, err := f.deps.Cl.GetResourceInfos(); err == nil {
        type row struct{ info kccluster.ResourceInfo; gvr schema.GroupVersionResource; count int }
        tmp := make([]row, 0, len(infos))
        for _, info := range infos {
            if info.Namespaced || info.Resource == "namespaces" { continue }
            if !verbsInclude(info.Verbs, "list") { continue }
            gvr := schema.GroupVersionResource{Group: info.GVK.Group, Version: info.GVK.Version, Resource: info.Resource}
            n := 0
            if lst, err := f.deps.Cl.ListByGVR(f.deps.Ctx, gvr, ""); err == nil { n = len(lst.Items) }
            tmp = append(tmp, row{info: info, gvr: gvr, count: n})
        }
        // Optional filter: only non-empty
        opts := ViewOptions{}
        if f.deps.ViewOptions != nil { opts = f.deps.ViewOptions() }
        out := make([]row, 0, len(tmp))
        for _, r := range tmp {
            if opts.ShowNonEmptyOnly && r.count == 0 { continue }
            out = append(out, r)
        }
        // Sort according to order
        switch opts.Order {
        case "group":
            sort.Slice(out, func(i, j int) bool {
                gi, gj := out[i].info.GVK.Group, out[j].info.GVK.Group
                if gi == gj { return out[i].info.Resource < out[j].info.Resource }
                return gi < gj
            })
        case "favorites":
            fav := opts.Favorites
            isFav := func(res string) bool { return fav != nil && fav[strings.ToLower(res)] }
            sort.Slice(out, func(i, j int) bool {
                fi, fj := isFav(out[i].info.Resource), isFav(out[j].info.Resource)
                if fi != fj { return fi } // favorites first
                return out[i].info.Resource < out[j].info.Resource
            })
        default: // "alpha"
            sort.Slice(out, func(i, j int) bool { return out[i].info.Resource < out[j].info.Resource })
        }
        for _, r := range out {
            id := r.gvr.Group + "/" + r.gvr.Version + "/" + r.gvr.Resource
            base := append(append([]string(nil), f.path...), r.info.Resource)
            rows = append(rows, NewEnterableItemStyled(id, []string{"/"+r.info.Resource, groupVersionString(r.info.GVK), fmt.Sprintf("%d", r.count)}, base, []*lipgloss.Style{nameSty, nil, nil}, func() (Folder, error) { return NewClusterObjectsFolder(f.deps, r.gvr, base), nil }))
        }
    }
    f.list = table.NewSliceList(rows)
}

// ContextRootFolder shows a context-scoped root, equivalent to "/" but under /contexts/<ctx>.
type ContextRootFolder struct{ BaseFolder }

func NewContextRootFolder(deps Deps, basePath []string) *ContextRootFolder {
    f := &ContextRootFolder{BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}, {Title: "Group"}, {Title: "Count"}}, path: append([]string(nil), basePath...)}}
    f.init = func(){ f.populate() }
    return f
}

func (f *ContextRootFolder) Title() string { return "contexts/" + f.deps.CtxName }
func (f *ContextRootFolder) Key() string   { return depsKey(f.deps, "contexts/"+f.deps.CtxName) }

func (f *ContextRootFolder) populate() {
    rows := make([]table.Row, 0, 64)
    nameSty := WhiteStyle()
    // Namespaces entry within the context (with count)
    nsBase := append(append([]string(nil), f.path...), "namespaces")
    nsCount := 0
    gvrNS := schema.GroupVersionResource{Group:"", Version:"v1", Resource:"namespaces"}
    if lst, err := f.deps.Cl.ListByGVR(f.deps.Ctx, gvrNS, ""); err == nil { nsCount = len(lst.Items) }
    rows = append(rows, NewEnterableItemStyled("namespaces", []string{"/namespaces", "v1", fmt.Sprintf("%d", nsCount)}, nsBase, []*lipgloss.Style{nameSty, nil, nil}, func() (Folder, error) { return NewClusterObjectsFolder(f.deps, gvrNS, nsBase), nil }))
    // Cluster-scoped resources for this context
    if infos, err := f.deps.Cl.GetResourceInfos(); err == nil {
        filtered := make([]kccluster.ResourceInfo, 0, len(infos))
        for _, info := range infos {
            if info.Namespaced || info.Resource == "namespaces" { continue }
            if !verbsInclude(info.Verbs, "list") { continue }
            filtered = append(filtered, info)
        }
        sort.Slice(filtered, func(i, j int) bool { return filtered[i].Resource < filtered[j].Resource })
        for _, info := range filtered {
            gvr := schema.GroupVersionResource{Group: info.GVK.Group, Version: info.GVK.Version, Resource: info.Resource}
            n := 0
            if lst, err := f.deps.Cl.ListByGVR(f.deps.Ctx, gvr, ""); err == nil { n = len(lst.Items) }
            id := gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
            base := append(append([]string(nil), f.path...), info.Resource)
            rows = append(rows, NewEnterableItemStyled(id, []string{"/"+info.Resource, groupVersionString(info.GVK), fmt.Sprintf("%d", n)}, base, []*lipgloss.Style{nameSty, nil, nil}, func() (Folder, error) { return NewClusterObjectsFolder(f.deps, gvr, base), nil }))
        }
    }
    f.list = table.NewSliceList(rows)
}

// ContextsFolder lists available contexts (if provided in Deps).
type ContextsFolder struct{ BaseFolder }
func NewContextsFolder(deps Deps, basePath []string) *ContextsFolder {
    f := &ContextsFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}, path: append([]string(nil), basePath...)}}
    f.init = func() { f.populate() }
    return f
}
func (f *ContextsFolder) Title() string { return "contexts" }
func (f *ContextsFolder) Key() string   { return depsKey(f.deps, "contexts") }
func (f *ContextsFolder) populate() {
    nameSty := WhiteStyle()
    rows := make([]table.Row, 0, 16)
    if f.deps.ListContexts != nil {
        names := f.deps.ListContexts()
        sort.Strings(names)
        for _, n := range names {
            if f.deps.EnterContext != nil {
                name := n
                base := append(append([]string(nil), f.path...), name)
                // Default to green for enterable contexts
                rows = append(rows, NewEnterableItem(name, []string{name}, base, func() (Folder, error) { return f.deps.EnterContext(name, base) }, nil))
            } else {
                rows = append(rows, NewSimpleItem(n, []string{n}, append(append([]string(nil), f.path...), n), nameSty))
            }
        }
    }
    f.list = table.NewSliceList(rows)
}

// Namespaces: list namespaces, each enterable
// (NamespacesFolder removed; namespaces are listed via ClusterObjectsFolder for v1/namespaces.)

// Namespaced groups: configmaps/secrets/etc with counts
func (f *NamespacedGroupsFolder) populate() {
    rows := make([]table.Row, 0, 64)
    nameSty := WhiteStyle()
    infos, err := f.deps.Cl.GetResourceInfos(); if err != nil { f.list = newEmptyList(); return }
    type row struct{ info kccluster.ResourceInfo; gvr schema.GroupVersionResource; count int }
    tmp := make([]row, 0, len(infos))
    for _, info := range infos {
        if !info.Namespaced || !verbsInclude(info.Verbs, "list") { continue }
        gvr := schema.GroupVersionResource{Group: info.GVK.Group, Version: info.GVK.Version, Resource: info.Resource}
        n := 0
        if lst, err := f.deps.Cl.ListByGVR(f.deps.Ctx, gvr, f.ns); err == nil { n = len(lst.Items) }
        tmp = append(tmp, row{info: info, gvr: gvr, count: n})
    }
    // Optional filter: only non-empty
    opts := ViewOptions{}
    if f.deps.ViewOptions != nil { opts = f.deps.ViewOptions() }
    out := make([]row, 0, len(tmp))
    for _, r := range tmp {
        if opts.ShowNonEmptyOnly && r.count == 0 { continue }
        out = append(out, r)
    }
    // Sort according to order
    switch opts.Order {
    case "group":
        sort.Slice(out, func(i, j int) bool {
            gi, gj := out[i].info.GVK.Group, out[j].info.GVK.Group
            if gi == gj { return out[i].info.Resource < out[j].info.Resource }
            return gi < gj
        })
    case "favorites":
        fav := opts.Favorites
        isFav := func(res string) bool { return fav != nil && fav[strings.ToLower(res)] }
        sort.Slice(out, func(i, j int) bool {
            fi, fj := isFav(out[i].info.Resource), isFav(out[j].info.Resource)
            if fi != fj { return fi }
            return out[i].info.Resource < out[j].info.Resource
        })
    default:
        sort.Slice(out, func(i, j int) bool { return out[i].info.Resource < out[j].info.Resource })
    }
    for _, r := range out {
        id := r.gvr.Group + "/" + r.gvr.Version + "/" + r.gvr.Resource
        base := append(append([]string(nil), f.path...), r.info.Resource)
        rows = append(rows, NewEnterableItemStyled(id, []string{"/"+r.info.Resource, groupVersionString(r.info.GVK), fmt.Sprintf("%d", r.count)}, base, []*lipgloss.Style{nameSty, nil, nil}, func() (Folder, error) { return NewNamespacedObjectsFolder(f.deps, r.gvr, f.ns, base), nil }))
    }
    f.list = table.NewSliceList(rows)
}

func (f *NamespacedObjectsFolder) populate() {
    nameSty := WhiteStyle()
    lst, err := f.deps.Cl.ListByGVR(f.deps.Ctx, f.gvr, f.namespace); if err != nil { f.list = newEmptyList(); return }
    f.watchGVR(f.gvr)
    // Sort names
    names := make([]string, 0, len(lst.Items))
    for i := range lst.Items { names = append(names, lst.Items[i].GetName()) }
    sort.Strings(names)
    rows := make([]table.Row, 0, len(names))
    // Resolve Kind once for details
    kind := ""
    if k, err := f.deps.Cl.RESTMapper().KindFor(f.gvr); err == nil { kind = k.Kind }
    gvStr := f.gvr.GroupVersion().String()
    for _, nm := range names {
        if ctor, ok := childFor(f.gvr); ok {
            ns := f.namespace; name := nm
            base := append(append([]string(nil), f.path...), nm)
            it := NewEnterableItem(nm, []string{nm}, base, func() (Folder, error) { return ctor(f.deps, ns, name, base), nil }, nameSty)
            if kind != "" {
                nn := types.NamespacedName{Namespace: f.namespace, Name: nm}.String()
                it = it.WithDetails(fmt.Sprintf("%s (%s %s)", nn, kind, gvStr))
            }
            rows = append(rows, it)
        } else {
            base := append(append([]string(nil), f.path...), nm)
            it := NewSimpleItem(nm, []string{nm}, base, nameSty)
            if kind != "" {
                nn := types.NamespacedName{Namespace: f.namespace, Name: nm}.String()
                it = it.WithDetails(fmt.Sprintf("%s (%s %s)", nn, kind, gvStr))
            }
            rows = append(rows, it)
        }
    }
    f.list = table.NewSliceList(rows)
}

func (f *ClusterObjectsFolder) populate() {
    nameSty := WhiteStyle()
    lst, err := f.deps.Cl.ListByGVR(f.deps.Ctx, f.gvr, ""); if err != nil { f.list = newEmptyList(); return }
    f.watchGVR(f.gvr)
    // Sort names
    names := make([]string, 0, len(lst.Items))
    for i := range lst.Items { names = append(names, lst.Items[i].GetName()) }
    sort.Strings(names)
    rows := make([]table.Row, 0, len(names))
    // Resolve Kind once for details
    kind := ""
    if k, err := f.deps.Cl.RESTMapper().KindFor(f.gvr); err == nil { kind = k.Kind }
    gvStr := f.gvr.GroupVersion().String()
    for _, nm := range names {
        if ctor, ok := childFor(f.gvr); ok {
            name := nm
            base := append(append([]string(nil), f.path...), nm)
            it := NewEnterableItem(nm, []string{nm}, base, func() (Folder, error) { return ctor(f.deps, "", name, base), nil }, nameSty)
            if kind != "" { it = it.WithDetails(fmt.Sprintf("%s (%s %s)", nm, kind, gvStr)) }
            rows = append(rows, it)
        } else {
            base := append(append([]string(nil), f.path...), nm)
            it := NewSimpleItem(nm, []string{nm}, base, nameSty)
            if kind != "" { it = it.WithDetails(fmt.Sprintf("%s (%s %s)", nm, kind, gvStr)) }
            rows = append(rows, it)
        }
    }
    f.list = table.NewSliceList(rows)
}

func (f *PodContainersFolder) populate() {
    nameSty := WhiteStyle()
    gvr := schema.GroupVersionResource{Group:"", Version:"v1", Resource:"pods"}
    f.watchGVR(gvr)
    obj, err := f.deps.Cl.GetByGVR(f.deps.Ctx, gvr, f.ns, f.pod); if err != nil || obj == nil { f.list = newEmptyList(); return }
    var pod corev1.Pod
    if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pod); err != nil { f.list = newEmptyList(); return }
    rows := make([]table.Row, 0, 8)
    for _, c := range pod.Spec.Containers { if c.Name != "" { rows = append(rows, NewSimpleItem(c.Name, []string{c.Name}, append(append([]string(nil), f.path...), c.Name), nameSty)) } }
    for _, c := range pod.Spec.InitContainers { if c.Name != "" { rows = append(rows, NewSimpleItem(c.Name, []string{c.Name}, append(append([]string(nil), f.path...), c.Name), nameSty)) } }
    f.list = table.NewSliceList(rows)
}

func (f *ConfigMapKeysFolder) populate() {
    nameSty := WhiteStyle()
    gvr := schema.GroupVersionResource{Group:"", Version:"v1", Resource:"configmaps"}
    f.watchGVR(gvr)
    obj, err := f.deps.Cl.GetByGVR(f.deps.Ctx, gvr, f.ns, f.name); if err != nil || obj == nil { f.list = newEmptyList(); return }
    var cm corev1.ConfigMap
    if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &cm); err != nil { f.list = newEmptyList(); return }
    rows := make([]table.Row, 0, 16)
    keys := make([]string, 0, len(cm.Data))
    for k := range cm.Data { keys = append(keys, k) }
    sort.Strings(keys)
    for _, k := range keys { rows = append(rows, NewSimpleItem(k, []string{k}, append(append([]string(nil), f.path...), k), nameSty)) }
    f.list = table.NewSliceList(rows)
}

func (f *SecretKeysFolder) populate() {
    nameSty := WhiteStyle()
    gvr := schema.GroupVersionResource{Group:"", Version:"v1", Resource:"secrets"}
    f.watchGVR(gvr)
    obj, err := f.deps.Cl.GetByGVR(f.deps.Ctx, gvr, f.ns, f.name); if err != nil || obj == nil { f.list = newEmptyList(); return }
    var sec corev1.Secret
    if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &sec); err != nil { f.list = newEmptyList(); return }
    rows := make([]table.Row, 0, 16)
    keys := make([]string, 0, len(sec.Data))
    for k := range sec.Data { keys = append(keys, k) }
    sort.Strings(keys)
    for _, k := range keys { rows = append(rows, NewSimpleItem(k, []string{k}, append(append([]string(nil), f.path...), k), nameSty)) }
    f.list = table.NewSliceList(rows)
}
