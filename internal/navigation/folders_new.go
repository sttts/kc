package navigation

import (
	"encoding/base64"
	"fmt"
	kccluster "github.com/sttts/kc/internal/cluster"
	table "github.com/sttts/kc/internal/table"
	tablecache "github.com/sttts/kc/internal/tablecache"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	utilduration "k8s.io/apimachinery/pkg/util/duration"
	kcache "k8s.io/client-go/tools/cache"
	yaml "sigs.k8s.io/yaml"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// BaseFolder provides lazy population scaffolding for concrete folders.
type BaseFolder struct {
	deps Deps
	cols []table.Column
	once sync.Once
	list *table.SliceList
	init func()
	// absolute breadcrumb path segments for this folder
	path []string
	// optional metadata for object list folders
	gvr       schema.GroupVersionResource
	namespace string
	hasMeta   bool
	watchOnce sync.Once
	mu        sync.Mutex
	dirty     bool
	items     map[string]Item
}

func (b *BaseFolder) Columns() []table.Column { return b.cols }

// Title returns a systematic title for object-list folders using their
// metadata (namespace + resource). Non-object folders should override Title.
func (b *BaseFolder) Title() string {
	if b.hasMeta {
		if b.namespace != "" {
			return "namespaces/" + b.namespace + "/" + b.gvr.Resource
		}
		return b.gvr.Resource
	}
	return ""
}

// table.List implementation delegates to the lazily-populated list.
func (b *BaseFolder) ensure() {
	b.once.Do(func() {
		if b.list == nil {
			b.list = newEmptyList()
		}
		if b.init != nil {
			b.init()
		}
	})
	if b.dirty {
		b.mu.Lock()
		b.dirty = false
		if b.init != nil {
			b.init()
		}
		b.mu.Unlock()
	}
}

func (b *BaseFolder) markDirty() { b.mu.Lock(); b.dirty = true; b.mu.Unlock() }

// Refresh marks the folder content dirty so the next access will repopulate
// based on current dependencies (e.g., updated ViewOptions).
func (b *BaseFolder) Refresh() { b.markDirty() }

// IsDirty reports whether the folder has been marked for refresh due to data
// changes. It is safe for concurrent use.
func (b *BaseFolder) IsDirty() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.dirty }

// watchGVR sets up an informer for the given GVR (resolved to a Kind internally)
// and marks the folder dirty on add/update/delete. This avoids leaking Kinds into
// the folder API: callers only pass GVRs.
func (b *BaseFolder) watchGVR(gvr schema.GroupVersionResource) {
	b.watchOnce.Do(func() {
		// Resolve to Kind for informer construction; keep GVR as the external identity.
		k, err := b.deps.Cl.RESTMapper().KindFor(gvr)
		if err != nil {
			return
		}
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(k)
		inf, err := b.deps.Cl.GetCache().GetInformer(b.deps.Ctx, u)
		if err != nil {
			return
		}
		inf.AddEventHandler(kcache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj interface{}) { b.markDirty() },
			UpdateFunc: func(oldObj, newObj interface{}) { b.markDirty() },
			DeleteFunc: func(obj interface{}) { b.markDirty() },
		})
	})
}
func (b *BaseFolder) Lines(top, num int) []table.Row        { b.ensure(); return b.list.Lines(top, num) }
func (b *BaseFolder) Above(id string, n int) []table.Row    { b.ensure(); return b.list.Above(id, n) }
func (b *BaseFolder) Below(id string, n int) []table.Row    { b.ensure(); return b.list.Below(id, n) }
func (b *BaseFolder) Len() int                              { b.ensure(); return b.list.Len() }
func (b *BaseFolder) Find(id string) (int, table.Row, bool) { b.ensure(); return b.list.Find(id) }

func (b *BaseFolder) setRows(rows []table.Row) {
	b.list = table.NewSliceList(rows)
	if b.items == nil {
		b.items = make(map[string]Item, len(rows))
	} else {
		for k := range b.items {
			delete(b.items, k)
		}
	}
	for _, row := range rows {
		if item, ok := row.(Item); ok {
			if id, _, _, okCols := row.Columns(); okCols {
				b.items[id] = item
			}
		}
	}
}

func (b *BaseFolder) ItemByID(id string) (Item, bool) {
	if id == "" {
		return nil, false
	}
	b.ensure()
	if b.items == nil {
		return nil, false
	}
	it, ok := b.items[id]
	return it, ok
}

// ObjectListMeta default impl for folders that are not object lists.
func (b *BaseFolder) ObjectListMeta() (schema.GroupVersionResource, string, bool) {
	if b.hasMeta {
		return b.gvr, b.namespace, true
	}
	return schema.GroupVersionResource{}, "", false
}

// RootFolder lists contexts, namespaces, and cluster-scoped resource groups.
type RootFolder struct{ BaseFolder }

func NewRootFolder(deps Deps) *RootFolder {
	f := &RootFolder{BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}, {Title: "Group"}, {Title: "Count"}}, path: []string{}}}
	f.init = func() { f.populate() }
	return f
}
func (f *RootFolder) Title() string { return "/" }
func (f *RootFolder) Key() string   { return "root" }

// NamespacedGroupsFolder lists namespaced resource groups for a namespace.
type NamespacedGroupsFolder struct {
	BaseFolder
	ns string
}

func NewNamespacedGroupsFolder(deps Deps, ns string, basePath []string) *NamespacedGroupsFolder {
	f := &NamespacedGroupsFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}, {Title: "Group"}, {Title: "Count"}}, path: append([]string(nil), basePath...)}, ns: ns}
	f.init = func() { f.populate() }
	return f
}
func (f *NamespacedGroupsFolder) Title() string { return "namespaces/" + f.ns }
func (f *NamespacedGroupsFolder) Key() string   { return depsKey(f.deps, "namespaces/"+f.ns) }

// NamespacedObjectsFolder lists namespaced objects for a GVR + namespace.
type NamespacedObjectsFolder struct{ BaseFolder }

func NewNamespacedObjectsFolder(deps Deps, gvr schema.GroupVersionResource, ns string, basePath []string) *NamespacedObjectsFolder {
	f := &NamespacedObjectsFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}, gvr: gvr, namespace: ns, hasMeta: true, path: append([]string(nil), basePath...)}}
	f.init = func() { f.populate() }
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
	f.init = func() { f.populate() }
	return f
}
func (f *ClusterObjectsFolder) Title() string { return f.gvr.Resource }
func (f *ClusterObjectsFolder) Key() string {
	// Use full GVR for stable uniqueness across groups/versions
	gv := f.gvr.GroupVersion().String()
	return depsKey(f.deps, gv+"/"+f.gvr.Resource)
}

// PodContainersFolder lists containers + initContainers for a pod.
type PodContainersFolder struct {
	BaseFolder
	ns, pod string
}

func NewPodContainersFolder(deps Deps, ns, pod string, basePath []string) *PodContainersFolder {
	f := &PodContainersFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}, path: append([]string(nil), basePath...)}, ns: ns, pod: pod}
	f.init = func() { f.populate() }
	return f
}
func (f *PodContainersFolder) Title() string { return "containers" }
func (f *PodContainersFolder) Key() string {
	return depsKey(f.deps, "namespaces/"+f.ns+"/pods/"+f.pod+"/containers")
}

// ConfigMapKeysFolder lists data keys for a ConfigMap.
type ConfigMapKeysFolder struct {
	BaseFolder
	ns, name string
}

func NewConfigMapKeysFolder(deps Deps, ns, name string, basePath []string) *ConfigMapKeysFolder {
	f := &ConfigMapKeysFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}, path: append([]string(nil), basePath...)}, ns: ns, name: name}
	f.init = func() { f.populate() }
	return f
}
func (f *ConfigMapKeysFolder) Title() string { return "data" }
func (f *ConfigMapKeysFolder) Key() string {
	return depsKey(f.deps, "namespaces/"+f.ns+"/configmaps/"+f.name+"/data")
}

// Parent returns the coordinates of the owning ConfigMap
func (f *ConfigMapKeysFolder) Parent() (schema.GroupVersionResource, string, string) {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, f.ns, f.name
}

// SecretKeysFolder lists data keys for a Secret.
type SecretKeysFolder struct {
	BaseFolder
	ns, name string
}

func NewSecretKeysFolder(deps Deps, ns, name string, basePath []string) *SecretKeysFolder {
	f := &SecretKeysFolder{BaseFolder: BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}}, path: append([]string(nil), basePath...)}, ns: ns, name: name}
	f.init = func() { f.populate() }
	return f
}
func (f *SecretKeysFolder) Title() string { return "data" }
func (f *SecretKeysFolder) Key() string {
	return depsKey(f.deps, "namespaces/"+f.ns+"/secrets/"+f.name+"/data")
}

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

func verbsInclude(vs []string, want string) bool {
	for _, v := range vs {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

func finalizeResourceGroupItems(base *BaseFolder, items []*ResourceGroupItem, opts ViewOptions) []table.Row {
	if len(items) == 0 {
		return nil
	}
	rows := make([]table.Row, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if opts.ShowNonEmptyOnly && item.Empty() {
			continue
		}
		if count, ok := item.TryCount(); ok {
			item.Cells[2] = fmt.Sprintf("%d", count)
		} else {
			item.Cells[2] = ""
			item.ComputeCountAsync(func() {
				if base != nil {
					base.markDirty()
				}
			})
		}
		rows = append(rows, item)
	}
	return rows
}

func objectViewContent(deps Deps, gvr schema.GroupVersionResource, namespace, name string) ViewContentFunc {
	return func() (string, string, string, string, string, error) {
		obj, err := deps.Cl.GetByGVR(deps.Ctx, gvr, namespace, name)
		if err != nil {
			return "", "", "", "", "", err
		}
		if obj == nil {
			return "", "", "", "", "", ErrNoViewContent
		}
		unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
		data, err := yaml.Marshal(obj.Object)
		if err != nil {
			return "", "", "", "", "", err
		}
		title := name
		if title == "" {
			title = obj.GetName()
		}
		if title == "" {
			title = gvr.Resource
		}
		return title, string(data), "yaml", "application/yaml", title + ".yaml", nil
	}
}

func keyViewContent(deps Deps, gvr schema.GroupVersionResource, namespace, name, key string, secret bool) ViewContentFunc {
	return func() (string, string, string, string, string, error) {
		obj, err := deps.Cl.GetByGVR(deps.Ctx, gvr, namespace, name)
		if err != nil {
			return "", "", "", "", "", err
		}
		if obj == nil {
			return "", "", "", "", "", ErrNoViewContent
		}
		data, found, _ := unstructured.NestedMap(obj.Object, "data")
		title := fmt.Sprintf("%s:%s", name, key)
		filename := fmt.Sprintf("%s_%s", name, key)
		if !found {
			return title, "", "", "", filename, nil
		}
		val, ok := data[key]
		if !ok {
			return title, "", "", "", filename, nil
		}
		switch v := val.(type) {
		case string:
			if secret {
				decoded, err := base64.StdEncoding.DecodeString(v)
				if err != nil {
					return title, v, "", "", filename, nil
				}
				if isProbablyText(decoded) {
					return title, string(decoded), "", "", filename, nil
				}
				return title, v, "", "", filename, nil
			}
			return title, v, "", "", filename, nil
		default:
			out, err := yaml.Marshal(v)
			if err != nil {
				return "", "", "", "", "", err
			}
			return title, string(out), "yaml", "application/yaml", filename + ".yaml", nil
		}
	}
}

func containerViewContent(deps Deps, namespace, pod, container string) ViewContentFunc {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	return func() (string, string, string, string, string, error) {
		obj, err := deps.Cl.GetByGVR(deps.Ctx, gvr, namespace, pod)
		if err != nil {
			return "", "", "", "", "", err
		}
		if obj == nil {
			return "", "", "", "", "", ErrNoViewContent
		}
		m := findContainer(obj.Object, container)
		if m == nil {
			return "", "", "", "", "", fmt.Errorf("container %q not found", container)
		}
		out, err := yaml.Marshal(m)
		if err != nil {
			return "", "", "", "", "", err
		}
		title := fmt.Sprintf("%s/%s", pod, container)
		return title, string(out), "yaml", "application/yaml", container + ".yaml", nil
	}
}

func findContainer(obj map[string]interface{}, name string) map[string]interface{} {
	if arr, found, _ := unstructured.NestedSlice(obj, "spec", "containers"); found {
		for _, c := range arr {
			if m, ok := c.(map[string]interface{}); ok {
				if n, _ := m["name"].(string); n == name {
					return m
				}
			}
		}
	}
	if arr, found, _ := unstructured.NestedSlice(obj, "spec", "initContainers"); found {
		for _, c := range arr {
			if m, ok := c.(map[string]interface{}); ok {
				if n, _ := m["name"].(string); n == name {
					return m
				}
			}
		}
	}
	return nil
}

func isProbablyText(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	if !utf8.Valid(b) {
		return false
	}
	for _, r := range string(b) {
		switch r {
		case '\n', '\r', '\t':
			continue
		}
		if r < 0x20 {
			return false
		}
	}
	return true
}

// Root: "/namespaces" + cluster-scoped resources with counts
func (f *RootFolder) populate() {
	rows := make([]table.Row, 0, 64)
	nameSty := WhiteStyle()
	opts := ViewOptions{}
	if f.deps.ViewOptions != nil {
		opts = f.deps.ViewOptions()
	}

	if f.deps.ListContexts != nil {
		base := append(append([]string(nil), f.path...), "contexts")
		item := newContextListItem("contexts", []string{"/contexts", "", ""}, base, GreenStyle(), f.deps.ListContexts, func() (Folder, error) {
			return NewContextsFolder(f.deps, base), nil
		})
		if !(opts.ShowNonEmptyOnly && item.Empty()) {
			item.Cells[2] = fmt.Sprintf("%d", item.Count())
			rows = append(rows, item)
		}
	}

	gvrNS := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	nsBase := append(append([]string(nil), f.path...), "namespaces")
	groupItems := make([]*ResourceGroupItem, 0, 32)
	nsItem := newResourceGroupItem(f.deps, gvrNS, "", "namespaces", []string{"/namespaces", "v1", ""}, nsBase, nameSty, true, func() (Folder, error) {
		return NewClusterObjectsFolder(f.deps, gvrNS, nsBase), nil
	})
	groupItems = append(groupItems, nsItem)

	if infos, err := f.deps.Cl.GetResourceInfos(); err == nil {
		type entry struct {
			info kccluster.ResourceInfo
			gvr  schema.GroupVersionResource
		}
		entries := make([]entry, 0, len(infos))
		for _, info := range infos {
			if info.Namespaced || info.Resource == "namespaces" {
				continue
			}
			if !verbsInclude(info.Verbs, "list") || !verbsInclude(info.Verbs, "watch") {
				continue
			}
			gvr := schema.GroupVersionResource{Group: info.GVK.Group, Version: info.GVK.Version, Resource: info.Resource}
			entries = append(entries, entry{info: info, gvr: gvr})
		}
		switch opts.Order {
		case "group":
			sort.Slice(entries, func(i, j int) bool {
				gi, gj := entries[i].info.GVK.Group, entries[j].info.GVK.Group
				if gi == gj {
					return entries[i].info.Resource < entries[j].info.Resource
				}
				return gi < gj
			})
		case "favorites":
			fav := opts.Favorites
			isFav := func(res string) bool { return fav != nil && fav[strings.ToLower(res)] }
			sort.Slice(entries, func(i, j int) bool {
				fi, fj := isFav(entries[i].info.Resource), isFav(entries[j].info.Resource)
				if fi != fj {
					return fi
				}
				return entries[i].info.Resource < entries[j].info.Resource
			})
		default:
			sort.Slice(entries, func(i, j int) bool { return entries[i].info.Resource < entries[j].info.Resource })
		}
		for _, e := range entries {
			id := e.gvr.Group + "/" + e.gvr.Version + "/" + e.gvr.Resource
			base := append(append([]string(nil), f.path...), e.info.Resource)
			item := newResourceGroupItem(f.deps, e.gvr, "", id, []string{"/" + e.info.Resource, groupVersionString(e.info.GVK), ""}, base, nameSty, true, func() (Folder, error) {
				return NewClusterObjectsFolder(f.deps, e.gvr, base), nil
			})
			groupItems = append(groupItems, item)
		}
	}

	rows = append(rows, finalizeResourceGroupItems(&f.BaseFolder, groupItems, opts)...)

	f.setRows(rows)
}

// ContextRootFolder shows a context-scoped root, equivalent to "/" but under /contexts/<ctx>.
type ContextRootFolder struct{ BaseFolder }

func NewContextRootFolder(deps Deps, basePath []string) *ContextRootFolder {
	f := &ContextRootFolder{BaseFolder{deps: deps, cols: []table.Column{{Title: " Name"}, {Title: "Group"}, {Title: "Count"}}, path: append([]string(nil), basePath...)}}
	f.init = func() { f.populate() }
	return f
}

func (f *ContextRootFolder) Title() string { return "contexts/" + f.deps.CtxName }
func (f *ContextRootFolder) Key() string   { return depsKey(f.deps, "contexts/"+f.deps.CtxName) }

func (f *ContextRootFolder) populate() {
	rows := make([]table.Row, 0, 64)
	nameSty := WhiteStyle()
	opts := ViewOptions{}
	if f.deps.ViewOptions != nil {
		opts = f.deps.ViewOptions()
	}

	gvrNS := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	nsBase := append(append([]string(nil), f.path...), "namespaces")
	groupItems := make([]*ResourceGroupItem, 0, 32)
	nsItem := newResourceGroupItem(f.deps, gvrNS, "", "namespaces", []string{"/namespaces", "v1", ""}, nsBase, nameSty, true, func() (Folder, error) {
		return NewClusterObjectsFolder(f.deps, gvrNS, nsBase), nil
	})
	groupItems = append(groupItems, nsItem)

	if infos, err := f.deps.Cl.GetResourceInfos(); err == nil {
		type entry struct {
			info kccluster.ResourceInfo
			gvr  schema.GroupVersionResource
		}
		filtered := make([]entry, 0, len(infos))
		for _, info := range infos {
			if info.Namespaced || info.Resource == "namespaces" {
				continue
			}
			if !verbsInclude(info.Verbs, "list") || !verbsInclude(info.Verbs, "watch") {
				continue
			}
			gvr := schema.GroupVersionResource{Group: info.GVK.Group, Version: info.GVK.Version, Resource: info.Resource}
			filtered = append(filtered, entry{info: info, gvr: gvr})
		}
		switch opts.Order {
		case "group":
			sort.Slice(filtered, func(i, j int) bool {
				gi, gj := filtered[i].info.GVK.Group, filtered[j].info.GVK.Group
				if gi == gj {
					return filtered[i].info.Resource < filtered[j].info.Resource
				}
				return gi < gj
			})
		case "favorites":
			fav := opts.Favorites
			isFav := func(res string) bool { return fav != nil && fav[strings.ToLower(res)] }
			sort.Slice(filtered, func(i, j int) bool {
				fi, fj := isFav(filtered[i].info.Resource), isFav(filtered[j].info.Resource)
				if fi != fj {
					return fi
				}
				return filtered[i].info.Resource < filtered[j].info.Resource
			})
		default:
			sort.Slice(filtered, func(i, j int) bool { return filtered[i].info.Resource < filtered[j].info.Resource })
		}
		for _, e := range filtered {
			id := e.gvr.Group + "/" + e.gvr.Version + "/" + e.gvr.Resource
			base := append(append([]string(nil), f.path...), e.info.Resource)
			item := newResourceGroupItem(f.deps, e.gvr, "", id, []string{"/" + e.info.Resource, groupVersionString(e.info.GVK), ""}, base, nameSty, true, func() (Folder, error) {
				return NewClusterObjectsFolder(f.deps, e.gvr, base), nil
			})
			groupItems = append(groupItems, item)
		}
	}

	rows = append(rows, finalizeResourceGroupItems(&f.BaseFolder, groupItems, opts)...)

	f.setRows(rows)
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
			base := append(append([]string(nil), f.path...), n)
			if f.deps.EnterContext != nil {
				name := n
				item := newContextItem(n, []string{n}, base, nameSty, func() (Folder, error) { return f.deps.EnterContext(name, base) })
				rows = append(rows, item)
			} else {
				rows = append(rows, newRowItem(n, []string{n}, base, nameSty))
			}
		}
	}

	f.setRows(rows)
}

// Namespaces: list namespaces, each enterable
// (NamespacesFolder removed; namespaces are listed via ClusterObjectsFolder for v1/namespaces.)

// Namespaced groups: configmaps/secrets/etc with counts
func (f *NamespacedGroupsFolder) populate() {
	rows := make([]table.Row, 0, 64)
	nameSty := WhiteStyle()

	infos, err := f.deps.Cl.GetResourceInfos()
	if err != nil {
		f.setRows(nil)
		return
	}

	opts := ViewOptions{}
	if f.deps.ViewOptions != nil {
		opts = f.deps.ViewOptions()
	}

	type entry struct {
		info kccluster.ResourceInfo
		gvr  schema.GroupVersionResource
	}
	entries := make([]entry, 0, len(infos))
	for _, info := range infos {
		if !info.Namespaced || !verbsInclude(info.Verbs, "list") {
			continue
		}
		if !verbsInclude(info.Verbs, "watch") {
			continue
		}
		gvr := schema.GroupVersionResource{Group: info.GVK.Group, Version: info.GVK.Version, Resource: info.Resource}
		entries = append(entries, entry{info: info, gvr: gvr})
	}

	switch opts.Order {
	case "group":
		sort.Slice(entries, func(i, j int) bool {
			gi, gj := entries[i].info.GVK.Group, entries[j].info.GVK.Group
			if gi == gj {
				return entries[i].info.Resource < entries[j].info.Resource
			}
			return gi < gj
		})
	case "favorites":
		fav := opts.Favorites
		isFav := func(res string) bool { return fav != nil && fav[strings.ToLower(res)] }
		sort.Slice(entries, func(i, j int) bool {
			fi, fj := isFav(entries[i].info.Resource), isFav(entries[j].info.Resource)
			if fi != fj {
				return fi
			}
			return entries[i].info.Resource < entries[j].info.Resource
		})
	default:
		sort.Slice(entries, func(i, j int) bool { return entries[i].info.Resource < entries[j].info.Resource })
	}

	groupItems := make([]*ResourceGroupItem, 0, len(entries))
	for _, e := range entries {
		id := e.gvr.Group + "/" + e.gvr.Version + "/" + e.gvr.Resource
		base := append(append([]string(nil), f.path...), e.info.Resource)
		item := newResourceGroupItem(f.deps, e.gvr, f.ns, id, []string{"/" + e.info.Resource, groupVersionString(e.info.GVK), ""}, base, nameSty, true, func() (Folder, error) {
			return NewNamespacedObjectsFolder(f.deps, e.gvr, f.ns, base), nil
		})
		groupItems = append(groupItems, item)
	}

	rows = append(rows, finalizeResourceGroupItems(&f.BaseFolder, groupItems, opts)...)

	f.setRows(rows)
}

func (f *NamespacedObjectsFolder) populate() {
	nameSty := WhiteStyle()
	opts := ViewOptions{}
	if f.deps.ViewOptions != nil {
		opts = f.deps.ViewOptions()
	}

	// Try server-side Rows via tablecache first for richer columns
	if rl, err := f.deps.Cl.ListRowsByGVR(f.deps.Ctx, f.gvr, f.namespace); err == nil && rl != nil && len(rl.Items) > 0 {
		vis := make([]int, 0, len(rl.Columns))
		for i, c := range rl.Columns {
			if opts.Columns == "wide" || c.Priority == 0 {
				vis = append(vis, i)
			}
		}
		cols := make([]table.Column, len(vis)+1)
		for i := range vis {
			c := rl.Columns[vis[i]]
			cols[i] = table.Column{Title: c.Name, Width: 0}
		}
		cols[len(cols)-1] = table.Column{Title: "Age"}
		f.cols = cols

		idxs := make([]int, len(rl.Items))
		for i := range idxs {
			idxs[i] = i
		}

		nameOf := func(rr *tablecache.Row) string {
			if rr == nil {
				return ""
			}
			n := rr.Name
			if n == "" {
				if len(rr.Cells) > 0 {
					if s, ok := rr.Cells[0].(string); ok {
						if strings.HasPrefix(s, "/") {
							s = s[1:]
						}
						n = s
					}
				}
			}
			return strings.ToLower(n)
		}

		switch strings.ToLower(opts.ObjectsOrder) {
		case "-name":
			sort.Slice(idxs, func(i, j int) bool { return nameOf(&rl.Items[idxs[i]]) > nameOf(&rl.Items[idxs[j]]) })
		case "creation":
			sort.Slice(idxs, func(i, j int) bool {
				return rl.Items[idxs[i]].ObjectMeta.CreationTimestamp.Time.Before(rl.Items[idxs[j]].ObjectMeta.CreationTimestamp.Time)
			})
		case "-creation":
			sort.Slice(idxs, func(i, j int) bool {
				return rl.Items[idxs[i]].ObjectMeta.CreationTimestamp.Time.After(rl.Items[idxs[j]].ObjectMeta.CreationTimestamp.Time)
			})
		default:
			sort.Slice(idxs, func(i, j int) bool { return nameOf(&rl.Items[idxs[i]]) < nameOf(&rl.Items[idxs[j]]) })
		}

		rows := make([]table.Row, 0, len(rl.Items))
		ctor, hasChild := childFor(f.gvr)
		kind := ""
		if k, e := f.deps.Cl.RESTMapper().KindFor(f.gvr); e == nil {
			kind = k.Kind
		}
		gvStr := f.gvr.GroupVersion().String()

		for _, ii := range idxs {
			rr := &rl.Items[ii]
			nm := rr.Name
			if nm == "" && len(rr.Cells) > 0 {
				if s, ok := rr.Cells[0].(string); ok {
					nm = s
				}
			}
			if nm == "" {
				nm = fmt.Sprintf("row-%d", ii)
			}
			id := nm
			cells := make([]string, len(vis)+1)
			for j := range vis {
				idx := vis[j]
				if idx < len(rr.Cells) {
					cells[j] = fmt.Sprint(rr.Cells[idx])
				}
			}
			var age string
			if !rr.ObjectMeta.CreationTimestamp.IsZero() {
				age = utilduration.HumanDuration(time.Since(rr.ObjectMeta.CreationTimestamp.Time))
			}
			cells[len(cells)-1] = age
			if len(cells) > 0 {
				if hasChild {
					cells[0] = "/" + nm
				} else {
					cells[0] = nm
				}
			}

			base := append(append([]string(nil), f.path...), nm)
			obj := newObjectRow(id, cells, base, f.gvr, f.namespace, nm, nameSty)
			obj.WithViewContent(objectViewContent(f.deps, f.gvr, f.namespace, nm))
			if kind != "" {
				nn := types.NamespacedName{Namespace: f.namespace, Name: nm}.String()
				obj.details = fmt.Sprintf("%s (%s %s)", nn, kind, gvStr)
			}

			if hasChild {
				ns := f.namespace
				name := nm
				rows = append(rows, newObjectWithChildItem(obj, func() (Folder, error) { return ctor(f.deps, ns, name, base), nil }))
			} else {
				rows = append(rows, obj)
			}
		}

		f.setRows(rows)
		f.watchGVR(f.gvr)
		return
	}

	// Fallback: metadata via cached client
	lst, err := f.deps.Cl.ListByGVR(f.deps.Ctx, f.gvr, f.namespace)
	if err != nil {
		f.setRows(nil)
		return
	}
	f.watchGVR(f.gvr)

	names := make([]string, 0, len(lst.Items))
	for i := range lst.Items {
		names = append(names, lst.Items[i].GetName())
	}
	sort.Strings(names)

	rows := make([]table.Row, 0, len(names))
	kind := ""
	if k, err := f.deps.Cl.RESTMapper().KindFor(f.gvr); err == nil {
		kind = k.Kind
	}
	gvStr := f.gvr.GroupVersion().String()

	ctor, hasChild := childFor(f.gvr)
	for _, nm := range names {
		base := append(append([]string(nil), f.path...), nm)
		nameCell := nm
		if hasChild {
			nameCell = "/" + strings.TrimPrefix(nameCell, "/")
		}
		obj := newObjectRow(nm, []string{nameCell}, base, f.gvr, f.namespace, nm, nameSty)
		obj.WithViewContent(objectViewContent(f.deps, f.gvr, f.namespace, nm))
		if kind != "" {
			nn := types.NamespacedName{Namespace: f.namespace, Name: nm}.String()
			obj.details = fmt.Sprintf("%s (%s %s)", nn, kind, gvStr)
		}

		if hasChild {
			ns := f.namespace
			name := nm
			rows = append(rows, newObjectWithChildItem(obj, func() (Folder, error) { return ctor(f.deps, ns, name, base), nil }))
		} else {
			rows = append(rows, obj)
		}
	}

	f.setRows(rows)
}

func (f *ClusterObjectsFolder) populate() {
	nameSty := WhiteStyle()
	opts := ViewOptions{}
	if f.deps.ViewOptions != nil {
		opts = f.deps.ViewOptions()
	}

	if rl, err := f.deps.Cl.ListRowsByGVR(f.deps.Ctx, f.gvr, ""); err == nil && rl != nil && len(rl.Items) > 0 {
		vis := make([]int, 0, len(rl.Columns))
		for i, c := range rl.Columns {
			if opts.Columns == "wide" || c.Priority == 0 {
				vis = append(vis, i)
			}
		}
		cols := make([]table.Column, len(vis)+1)
		for i := range vis {
			c := rl.Columns[vis[i]]
			cols[i] = table.Column{Title: c.Name, Width: 0}
		}
		cols[len(cols)-1] = table.Column{Title: "Age"}
		f.cols = cols

		idxs := make([]int, len(rl.Items))
		for i := range idxs {
			idxs[i] = i
		}

		nameOf := func(rr *tablecache.Row) string {
			if rr == nil {
				return ""
			}
			n := rr.Name
			if n == "" {
				if len(rr.Cells) > 0 {
					if s, ok := rr.Cells[0].(string); ok {
						if strings.HasPrefix(s, "/") {
							s = s[1:]
						}
						n = s
					}
				}
			}
			return strings.ToLower(n)
		}

		switch strings.ToLower(opts.ObjectsOrder) {
		case "-name":
			sort.Slice(idxs, func(i, j int) bool { return nameOf(&rl.Items[idxs[i]]) > nameOf(&rl.Items[idxs[j]]) })
		case "creation":
			sort.Slice(idxs, func(i, j int) bool {
				return rl.Items[idxs[i]].ObjectMeta.CreationTimestamp.Time.Before(rl.Items[idxs[j]].ObjectMeta.CreationTimestamp.Time)
			})
		case "-creation":
			sort.Slice(idxs, func(i, j int) bool {
				return rl.Items[idxs[i]].ObjectMeta.CreationTimestamp.Time.After(rl.Items[idxs[j]].ObjectMeta.CreationTimestamp.Time)
			})
		default:
			sort.Slice(idxs, func(i, j int) bool { return nameOf(&rl.Items[idxs[i]]) < nameOf(&rl.Items[idxs[j]]) })
		}

		rows := make([]table.Row, 0, len(rl.Items))
		ctor, hasChild := childFor(f.gvr)
		kind := ""
		if k, e := f.deps.Cl.RESTMapper().KindFor(f.gvr); e == nil {
			kind = k.Kind
		}
		gvStr := f.gvr.GroupVersion().String()

		for _, ii := range idxs {
			rr := &rl.Items[ii]
			nm := rr.Name
			if nm == "" && len(rr.Cells) > 0 {
				if s, ok := rr.Cells[0].(string); ok {
					nm = s
				}
			}
			if nm == "" {
				nm = fmt.Sprintf("row-%d", ii)
			}
			id := nm
			cells := make([]string, len(vis)+1)
			for j := range vis {
				idx := vis[j]
				if idx < len(rr.Cells) {
					cells[j] = fmt.Sprint(rr.Cells[idx])
				}
			}
			var age string
			if !rr.ObjectMeta.CreationTimestamp.IsZero() {
				age = utilduration.HumanDuration(time.Since(rr.ObjectMeta.CreationTimestamp.Time))
			}
			cells[len(cells)-1] = age
			if len(cells) > 0 {
				if hasChild {
					cells[0] = "/" + nm
				} else {
					cells[0] = nm
				}
			}

			base := append(append([]string(nil), f.path...), nm)
			obj := newObjectRow(id, cells, base, f.gvr, "", nm, nameSty)
			obj.WithViewContent(objectViewContent(f.deps, f.gvr, "", nm))
			if kind != "" {
				obj.details = fmt.Sprintf("%s (%s %s)", nm, kind, gvStr)
			}

			if hasChild {
				name := nm
				rows = append(rows, newObjectWithChildItem(obj, func() (Folder, error) { return ctor(f.deps, "", name, base), nil }))
			} else {
				rows = append(rows, obj)
			}
		}

		f.setRows(rows)
		f.watchGVR(f.gvr)
		return
	}

	lst, err := f.deps.Cl.ListByGVR(f.deps.Ctx, f.gvr, "")
	if err != nil {
		f.setRows(nil)
		return
	}
	f.watchGVR(f.gvr)

	names := make([]string, 0, len(lst.Items))
	for i := range lst.Items {
		names = append(names, lst.Items[i].GetName())
	}
	sort.Strings(names)

	rows := make([]table.Row, 0, len(names))
	kind := ""
	if k, err := f.deps.Cl.RESTMapper().KindFor(f.gvr); err == nil {
		kind = k.Kind
	}
	gvStr := f.gvr.GroupVersion().String()
	ctor, hasChild := childFor(f.gvr)

	for _, nm := range names {
		base := append(append([]string(nil), f.path...), nm)
		nameCell := nm
		if hasChild {
			nameCell = "/" + strings.TrimPrefix(nameCell, "/")
		}
		obj := newObjectRow(nm, []string{nameCell}, base, f.gvr, "", nm, nameSty)
		obj.WithViewContent(objectViewContent(f.deps, f.gvr, "", nm))
		if kind != "" {
			obj.details = fmt.Sprintf("%s (%s %s)", nm, kind, gvStr)
		}

		if hasChild {
			name := nm
			rows = append(rows, newObjectWithChildItem(obj, func() (Folder, error) { return ctor(f.deps, "", name, base), nil }))
		} else {
			rows = append(rows, obj)
		}
	}

	f.setRows(rows)
}

func (f *PodContainersFolder) populate() {
	nameSty := WhiteStyle()
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	f.watchGVR(gvr)

	obj, err := f.deps.Cl.GetByGVR(f.deps.Ctx, gvr, f.ns, f.pod)
	if err != nil || obj == nil {
		f.setRows(nil)
		return
	}

	var pod corev1.Pod
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pod); err != nil {
		f.setRows(nil)
		return
	}

	rows := make([]table.Row, 0, len(pod.Spec.Containers)+len(pod.Spec.InitContainers))

	for _, c := range pod.Spec.Containers {
		if c.Name == "" {
			continue
		}
		base := append(append([]string(nil), f.path...), c.Name)
		item := newContainerItem(c.Name, []string{c.Name}, base, nameSty, containerViewContent(f.deps, f.ns, f.pod, c.Name))
		rows = append(rows, item)
	}
	for _, c := range pod.Spec.InitContainers {
		if c.Name == "" {
			continue
		}
		base := append(append([]string(nil), f.path...), c.Name)
		item := newContainerItem(c.Name, []string{c.Name}, base, nameSty, containerViewContent(f.deps, f.ns, f.pod, c.Name))
		item.details = "initContainer"
		rows = append(rows, item)
	}

	f.setRows(rows)
}

func (f *ConfigMapKeysFolder) populate() {
	nameSty := WhiteStyle()
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	f.watchGVR(gvr)

	obj, err := f.deps.Cl.GetByGVR(f.deps.Ctx, gvr, f.ns, f.name)
	if err != nil || obj == nil {
		f.setRows(nil)
		return
	}

	var cm corev1.ConfigMap
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &cm); err != nil {
		f.setRows(nil)
		return
	}

	keys := make([]string, 0, len(cm.Data))
	for k := range cm.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([]table.Row, 0, len(keys))
	for _, k := range keys {
		base := append(append([]string(nil), f.path...), k)
		item := newConfigKeyItem(k, []string{k}, base, nameSty, keyViewContent(f.deps, gvr, f.ns, f.name, k, false))
		rows = append(rows, item)
	}

	f.setRows(rows)
}

func (f *SecretKeysFolder) populate() {
	nameSty := WhiteStyle()
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	f.watchGVR(gvr)

	obj, err := f.deps.Cl.GetByGVR(f.deps.Ctx, gvr, f.ns, f.name)
	if err != nil || obj == nil {
		f.setRows(nil)
		return
	}

	var sec corev1.Secret
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &sec); err != nil {
		f.setRows(nil)
		return
	}

	keys := make([]string, 0, len(sec.Data))
	for k := range sec.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([]table.Row, 0, len(keys))
	for _, k := range keys {
		base := append(append([]string(nil), f.path...), k)
		item := newConfigKeyItem(k, []string{k}, base, nameSty, keyViewContent(f.deps, gvr, f.ns, f.name, k, true))
		rows = append(rows, item)
	}

	f.setRows(rows)
}
