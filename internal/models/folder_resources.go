package models

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	kccluster "github.com/sttts/kc/internal/cluster"
	table "github.com/sttts/kc/internal/table"
	"github.com/sttts/kc/pkg/appconfig"
	"golang.org/x/exp/slices"
	"k8s.io/apimachinery/pkg/runtime/schema"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
)

// ResourcesFolder provides shared scaffolding for resource-group folders (namespace and cluster scoped).
type ResourcesFolder struct {
	*BaseFolder
	items     map[string]*ResourceGroupItem
	lastSpecs map[string]resourceGroupSignature
	overrides *resourceViewOptions
}

type resourceViewOptions struct {
	showNonEmpty *bool
	order        appconfig.ResourcesViewOrder
	hasOrder     bool
	favorites    []string
	hasFavorites bool
}

// NewResourcesFolder constructs a ResourcesFolder with default columns and caller-provided metadata.
func NewResourcesFolder(base *BaseFolder) *ResourcesFolder {
	base.SetColumns([]table.Column{{Title: " Name"}, {Title: "Group"}, {Title: "Count"}})
	return &ResourcesFolder{
		BaseFolder: base,
		items:      make(map[string]*ResourceGroupItem),
		lastSpecs:  make(map[string]resourceGroupSignature),
	}
}

func (f *ResourcesFolder) viewSettings() (bool, appconfig.ResourcesViewOrder, []string) {
	show := true
	order := appconfig.OrderAlpha
	var favorites []string
	if cfg := f.Deps.AppConfig; cfg != nil {
		show = cfg.Resources.ShowNonEmptyOnly
		order = cfg.Resources.Order
		favorites = cfg.Resources.Favorites
	}
	if f.overrides != nil {
		if f.overrides.showNonEmpty != nil {
			show = *f.overrides.showNonEmpty
		}
		if f.overrides.hasOrder {
			order = f.overrides.order
		}
		if f.overrides.hasFavorites {
			favorites = f.overrides.favorites
		}
	}
	return show, order, favorites
}

func (f *ResourcesFolder) ApplyResourceViewOptions(showNonEmpty bool, order appconfig.ResourcesViewOrder, favorites []string) {
	if f == nil {
		return
	}
	if f.overrides == nil {
		f.overrides = &resourceViewOptions{}
	}
	ov := f.overrides
	changed := false
	if ov.showNonEmpty == nil || *ov.showNonEmpty != showNonEmpty {
		val := showNonEmpty
		ov.showNonEmpty = &val
		changed = true
	}
	if !ov.hasOrder || ov.order != order {
		ov.order = order
		ov.hasOrder = true
		changed = true
	}
	if favorites != nil {
		if !ov.hasFavorites || !slices.Equal(ov.favorites, favorites) {
			ov.favorites = append([]string(nil), favorites...)
			ov.hasFavorites = true
			changed = true
		}
	} else if ov.hasFavorites {
		ov.hasFavorites = false
		ov.favorites = nil
		changed = true
	}
	if changed {
		f.Refresh()
	}
}

func (f *ResourcesFolder) finalize(ctx context.Context, specs []resourceGroupSpec) []table.Row {
	log := crlog.FromContext(ctx).WithName("resourcesFolder")

	if len(specs) == 0 {
		changed := len(f.items) > 0 || len(f.lastSpecs) > 0
		f.items = make(map[string]*ResourceGroupItem)
		f.lastSpecs = make(map[string]resourceGroupSignature)
		if changed {
			f.BaseFolder.markDirtyFromSource()
			log.Info("resources cleared")
		}
		return nil
	}

	showNonEmpty, _, _ := f.viewSettings()
	rows := make([]table.Row, 0, len(specs))
	seen := make(map[string]*ResourceGroupItem, len(specs))
	sigs := make(map[string]resourceGroupSignature, len(specs))
	changed := len(specs) != len(f.lastSpecs)
	for idx, spec := range specs {
		item, created := f.ensureResourceGroupItem(spec)
		if item == nil {
			continue
		}
		item.applySpec(spec, f.Deps, created)
		current := item
		handler := func() { f.onResourceGroupCountChanged(current) }
		item.SetOnChange(handler)
		item.ComputeCountAsync(handler)
		visible := true
		empty := item.Empty()
		if item.Forbidden() {
			visible = false
		} else if showNonEmpty && empty {
			visible = false
		}
		sig := makeResourceGroupSignature(spec, visible, idx)
		sigs[spec.id] = sig
		if !changed {
			prev, ok := f.lastSpecs[spec.id]
			if !ok || prev != sig {
				changed = true
			}
		}
		seen[spec.id] = item
		if count, ok := item.TryCount(); ok {
			item.setCountCell(fmt.Sprintf("%d", count))
		} else {
			item.setCountCell("")
		}
		if !visible {
			continue
		}
		rows = append(rows, item)
	}

	if len(rows) == 0 {
		f.items = seen
		f.lastSpecs = sigs
		if changed {
			f.BaseFolder.markDirtyFromSource()
			log.Info("resources updated", "count", 0)
		}
		return nil
	}

	if !changed {
		for id := range f.lastSpecs {
			if _, ok := sigs[id]; !ok {
				changed = true
				break
			}
		}
	}

	f.items = seen
	f.lastSpecs = sigs
	if changed {
		f.BaseFolder.markDirtyFromSource()
		log.Info("resources updated", "count", len(rows))
	}
	return rows
}

func groupVersionString(group, version string) string {
	if group == "" {
		return version
	}
	return group + "/" + version
}

func verbsInclude(verbs []string, want string) bool {
	for _, v := range verbs {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

func sortResourceEntries(entries []resourceEntry, order appconfig.ResourcesViewOrder, fav map[string]bool) {
	switch order {
	case appconfig.OrderGroup:
		sort.SliceStable(entries, func(i, j int) bool {
			groupI := entries[i].info.GVK.Group
			groupJ := entries[j].info.GVK.Group
			if groupI == groupJ {
				return entries[i].info.Resource < entries[j].info.Resource
			}
			if groupI == "" {
				return true
			}
			if groupJ == "" {
				return false
			}
			gi := groupVersionString(groupI, entries[i].info.GVK.Version)
			gj := groupVersionString(groupJ, entries[j].info.GVK.Version)
			return gi < gj
		})
	case appconfig.OrderFavorites:
		isFav := func(res string) bool {
			if fav == nil {
				return false
			}
			return fav[strings.ToLower(res)]
		}
		sort.SliceStable(entries, func(i, j int) bool {
			fi, fj := isFav(entries[i].info.Resource), isFav(entries[j].info.Resource)
			if fi != fj {
				return fi
			}
			return entries[i].info.Resource < entries[j].info.Resource
		})
	default:
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].info.Resource < entries[j].info.Resource })
	}
}

func (f *ResourcesFolder) onResourceGroupCountChanged(item *ResourceGroupItem) {
	if f == nil || item == nil {
		return
	}
	if count, ok := item.TryCount(); ok {
		item.setCountCell(fmt.Sprintf("%d", count))
	}
	f.BaseFolder.markDirtyFromSource()
}

func favoritesMap(list []string) map[string]bool {
	if len(list) == 0 {
		return nil
	}
	set := make(map[string]bool, len(list))
	for _, item := range list {
		if item == "" {
			continue
		}
		set[strings.ToLower(item)] = true
	}
	return set
}

func favoritesFromCategories(entries []resourceEntry) []string {
	if len(entries) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	favorites := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !hasAllCategory(entry.info.Categories) {
			continue
		}
		key := strings.ToLower(entry.info.Resource)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		favorites = append(favorites, entry.info.Resource)
	}
	if len(favorites) == 0 {
		return nil
	}
	return favorites
}

func hasAllCategory(categories []string) bool {
	for _, cat := range categories {
		if strings.EqualFold(cat, "all") {
			return true
		}
	}
	return false
}

type resourceEntry struct {
	info ResourceInfo
	gvr  schema.GroupVersionResource
}

type ResourceInfo = kccluster.ResourceInfo

type resourceGroupSpec struct {
	id        string
	cells     []string
	path      []string
	style     *lipgloss.Style
	detail    string
	gvr       schema.GroupVersionResource
	gvk       schema.GroupVersionKind
	namespace string
	watchable bool
	enter     func() (Folder, error)
	verbs     []string
}

type resourceGroupSignature struct {
	gvr       schema.GroupVersionResource
	namespace string
	watchable bool
	cellsHash string
	pathHash  string
	visible   bool
	detail    string
	index     int
}

func makeResourceGroupSignature(spec resourceGroupSpec, visible bool, index int) resourceGroupSignature {
	return resourceGroupSignature{
		gvr:       spec.gvr,
		namespace: spec.namespace,
		watchable: spec.watchable,
		cellsHash: joinStrings(spec.cells),
		pathHash:  joinStrings(spec.path),
		visible:   visible,
		detail:    spec.detail,
		index:     index,
	}
}

func joinStrings(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	const sep = "\x00"
	joined := parts[0]
	for i := 1; i < len(parts); i++ {
		joined += sep + parts[i]
	}
	return joined
}

func (f *ResourcesFolder) ensureResourceGroupItem(spec resourceGroupSpec) (*ResourceGroupItem, bool) {
	if existing, ok := f.items[spec.id]; ok {
		return existing, false
	}
	item := NewResourceGroupItem(f.Deps, spec.gvr, spec.gvk, spec.namespace, spec.id, spec.cells, spec.path, spec.detail, spec.style, spec.watchable, spec.enter)
	return item, true
}
