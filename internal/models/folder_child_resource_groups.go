package models

import (
	"context"
	"fmt"
	"sync"

	table "github.com/sttts/kc/internal/table"
	"github.com/sttts/kc/pkg/appconfig"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	toolscache "k8s.io/client-go/tools/cache"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
)

// ChildResourceGroupsFolder renders child resources of a parent object as resource
// group entries (name, group, count) before drilling into the filtered object list.
type ChildResourceGroupsFolder struct {
	*ResourcesFolder
	parentGVR       schema.GroupVersionResource
	parentNamespace string
	parentName      string
	childGVRs       []schema.GroupVersionResource
	childGVKs       map[schema.GroupVersionResource]schema.GroupVersionKind

	mu             sync.Mutex
	parentUID      types.UID
	watchOnce      map[schema.GroupVersionResource]*sync.Once
	informerGetter func(context.Context, *unstructured.Unstructured) (toolscache.SharedIndexInformer, bool)
}

// NewChildResourceGroupsFolder builds a folder with one entry per childGVR under
// the given parent object path. Each entry opens the filtered child object list.
func NewChildResourceGroupsFolder(deps Deps, parentGVR schema.GroupVersionResource, parentNamespace, parentName string, childGVRs []schema.GroupVersionResource, path []string) *ChildResourceGroupsFolder {
	base := NewBaseFolder(deps, nil, path)
	folder := &ChildResourceGroupsFolder{
		ResourcesFolder: NewResourcesFolder(base),
		parentGVR:       parentGVR,
		parentNamespace: parentNamespace,
		parentName:      parentName,
		childGVRs:       append([]schema.GroupVersionResource(nil), childGVRs...),
		childGVKs:       make(map[schema.GroupVersionResource]schema.GroupVersionKind, len(childGVRs)),
		watchOnce:       make(map[schema.GroupVersionResource]*sync.Once, len(childGVRs)),
	}
	order := appconfig.OrderAlpha
	if deps.AppConfig != nil {
		order = deps.AppConfig.Resources.Order
	}
	folder.ApplyResourceViewOptions(false, order, nil)
	base.SetPopulate(folder.populate)
	folder.informerGetter = func(ctx context.Context, obj *unstructured.Unstructured) (toolscache.SharedIndexInformer, bool) {
		if deps.Cl == nil || obj == nil {
			return nil, false
		}
		inf, err := deps.Cl.GetCache().GetInformer(ctx, obj, crcache.BlockUntilSynced(false))
		if err != nil {
			return nil, false
		}
		indexer, ok := inf.(toolscache.SharedIndexInformer)
		if !ok {
			return nil, false
		}
		return indexer, indexer.HasSynced()
	}
	return folder
}

func (f *ChildResourceGroupsFolder) populate(ctx context.Context) ([]table.Row, error) {
	parentUID, err := getParentUID(ctx, f.Deps, f.parentGVR, f.parentNamespace, f.parentName)
	if err != nil {
		return nil, err
	}
	f.setParentUID(parentUID)

	specs := make([]resourceGroupSpec, 0, len(f.childGVRs))
	for _, child := range f.childGVRs {
		gvk, err := f.childGVK(child)
		if err != nil {
			return nil, err
		}
		spec, err := f.childSpec(child, gvk)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	rows := f.ResourcesFolder.finalize(ctx, specs)
	for _, child := range f.childGVRs {
		gvk, _ := f.childGVK(child)
		f.ensureWatcher(ctx, child, gvk)
		f.applyCount(ctx, child, gvk, parentUID)
	}
	return rows, nil
}

func (f *ChildResourceGroupsFolder) childSpec(childGVR schema.GroupVersionResource, gvk schema.GroupVersionKind) (resourceGroupSpec, error) {
	gvLabel := groupVersionString(childGVR.Group, childGVR.Version)
	resName := childGVR.Resource
	cells := []string{"/" + resName, gvLabel, ""}
	pathCopy := append(f.Path(), resName)
	id := fmt.Sprintf("%s/%s/%s/%s/%s", f.parentNamespace, f.parentName, childGVR.Group, childGVR.Version, resName)
	child := childGVR
	ns := f.parentNamespace
	return resourceGroupSpec{
		id:        id,
		cells:     cells,
		path:      pathCopy,
		style:     WhiteStyle(),
		detail:    fmt.Sprintf("%s (%s)", resName, gvLabel),
		gvr:       child,
		gvk:       gvk,
		namespace: ns,
		watchable: false,
		enter: func() (Folder, error) {
			return NewChildResourcesFolder(f.Deps, f.parentGVR, f.parentNamespace, f.parentName, child, pathCopy), nil
		},
	}, nil
}

func (f *ChildResourceGroupsFolder) applyCount(ctx context.Context, childGVR schema.GroupVersionResource, childGVK schema.GroupVersionKind, parentUID types.UID) {
	if f == nil || f.items == nil {
		return
	}
	informer, synced := f.informerForChild(ctx, childGVK)
	if informer == nil || !synced {
		return
	}
	id := fmt.Sprintf("%s/%s/%s/%s/%s", f.parentNamespace, f.parentName, childGVR.Group, childGVR.Version, childGVR.Resource)
	item, ok := f.items[id]
	if !ok || item == nil {
		return
	}
	if count, ok := f.countChildren(informer, parentUID); ok {
		item.SetCount(count)
	}
}

func (f *ChildResourceGroupsFolder) countChildren(informer toolscache.SharedIndexInformer, parentUID types.UID) (int, bool) {
	if parentUID == "" || informer == nil {
		return 0, false
	}
	list := informer.GetIndexer().List()
	if list == nil {
		return 0, false
	}
	count := 0
	for _, raw := range list {
		switch o := raw.(type) {
		case metav1.Object:
			if f.matchesMeta(o, parentUID) {
				count++
			}
		case *unstructured.Unstructured:
			if f.matchesUnstructured(o, parentUID) {
				count++
			}
		}
	}
	return count, true
}

func (f *ChildResourceGroupsFolder) matchesMeta(obj metav1.Object, parentUID types.UID) bool {
	if obj == nil {
		return false
	}
	if obj.GetNamespace() != f.parentNamespace {
		return false
	}
	for _, owner := range obj.GetOwnerReferences() {
		if owner.UID == parentUID {
			return true
		}
	}
	return false
}

func (f *ChildResourceGroupsFolder) matchesUnstructured(u *unstructured.Unstructured, parentUID types.UID) bool {
	if u == nil {
		return false
	}
	if u.GetNamespace() != f.parentNamespace {
		return false
	}
	for _, owner := range u.GetOwnerReferences() {
		if owner.UID == parentUID {
			return true
		}
	}
	return false
}

func (f *ChildResourceGroupsFolder) childGVK(childGVR schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	f.mu.Lock()
	if gvk, ok := f.childGVKs[childGVR]; ok && !gvk.Empty() {
		f.mu.Unlock()
		return gvk, nil
	}
	f.mu.Unlock()
	gvk, err := f.Deps.Cl.RESTMapper().KindFor(childGVR)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	f.mu.Lock()
	f.childGVKs[childGVR] = gvk
	f.mu.Unlock()
	return gvk, nil
}

func (f *ChildResourceGroupsFolder) ensureWatcher(ctx context.Context, childGVR schema.GroupVersionResource, childGVK schema.GroupVersionKind) {
	if childGVK.Empty() {
		return
	}
	f.mu.Lock()
	once := f.watchOnce[childGVR]
	if once == nil {
		once = &sync.Once{}
		f.watchOnce[childGVR] = once
	}
	f.mu.Unlock()
	once.Do(func() {
		informer, _ := f.informerForChild(ctx, childGVK)
		if informer == nil {
			return
		}
		log := crlog.FromContext(ctx).WithName("childResourceGroups").WithValues("gvr", childGVR.String())
		_, err := informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
			AddFunc:    func(interface{}) { f.recount(ctx, childGVR, childGVK) },
			UpdateFunc: func(interface{}, interface{}) { f.recount(ctx, childGVR, childGVK) },
			DeleteFunc: func(interface{}) { f.recount(ctx, childGVR, childGVK) },
		})
		if err != nil {
			log.Error(err, "failed to add child informer handler")
		}
		// Counts will refresh via event handlers when informer is already running.
	})
}

func (f *ChildResourceGroupsFolder) recount(ctx context.Context, childGVR schema.GroupVersionResource, childGVK schema.GroupVersionKind) {
	parentUID := f.currentParentUID()
	if parentUID == "" {
		return
	}
	f.applyCount(ctx, childGVR, childGVK, parentUID)
}

func (f *ChildResourceGroupsFolder) setParentUID(uid types.UID) {
	f.mu.Lock()
	f.parentUID = uid
	f.mu.Unlock()
}

func (f *ChildResourceGroupsFolder) currentParentUID() types.UID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.parentUID
}

func (f *ChildResourceGroupsFolder) informerForChild(ctx context.Context, childGVK schema.GroupVersionKind) (toolscache.SharedIndexInformer, bool) {
	if childGVK.Empty() {
		return nil, false
	}
	if f.informerGetter == nil {
		return nil, false
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(childGVK)
	return f.informerGetter(ctx, obj)
}
