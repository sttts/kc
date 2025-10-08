package models

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss/v2"
	kccluster "github.com/sttts/kc/internal/cluster"
	table "github.com/sttts/kc/internal/table"
	"github.com/sttts/kc/pkg/appconfig"
	"k8s.io/apimachinery/pkg/runtime/schema"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
)

// ResourcesFolder provides shared scaffolding for resource-group folders (namespace and cluster scoped).
type ResourcesFolder struct {
	*BaseFolder
	items     map[string]*ResourceGroupItem
	lastSpecs map[string]resourceGroupSignature
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

	cfg := f.Deps.AppConfig
	showNonEmpty := cfg.Resources.ShowNonEmptyOnly
	rows := make([]table.Row, 0, len(specs))
	seen := make(map[string]*ResourceGroupItem, len(specs))
	sigs := make(map[string]resourceGroupSignature, len(specs))
	changed := len(specs) != len(f.lastSpecs)
	for _, spec := range specs {
		item, created := f.ensureResourceGroupItem(spec)
		if item == nil {
			continue
		}
		item.applySpec(spec, f.Deps, created)
		if created {
			item.SetOnChange(func() { f.BaseFolder.markDirty() })
		}
		item.ComputeCountAsync(nil)
		visible := true
		if showNonEmpty && item.Empty() {
			visible = false
		}
		sig := makeResourceGroupSignature(spec, visible)
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
		sort.Slice(entries, func(i, j int) bool {
			gi, gj := entries[i].info.GVK.Group, entries[j].info.GVK.Group
			if gi == gj {
				return entries[i].info.Resource < entries[j].info.Resource
			}
			return gi < gj
		})
	case appconfig.OrderFavorites:
		isFav := func(res string) bool {
			if fav == nil {
				return false
			}
			return fav[strings.ToLower(res)]
		}
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
	gvr       schema.GroupVersionResource
	namespace string
	watchable bool
	enter     func() (Folder, error)
}

type resourceGroupSignature struct {
	gvr       schema.GroupVersionResource
	namespace string
	watchable bool
	cellsHash string
	pathHash  string
	visible   bool
}

func makeResourceGroupSignature(spec resourceGroupSpec, visible bool) resourceGroupSignature {
	return resourceGroupSignature{
		gvr:       spec.gvr,
		namespace: spec.namespace,
		watchable: spec.watchable,
		cellsHash: joinStrings(spec.cells),
		pathHash:  joinStrings(spec.path),
		visible:   visible,
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
	item := NewResourceGroupItem(f.Deps, spec.gvr, spec.namespace, spec.id, spec.cells, spec.path, spec.style, spec.watchable, spec.enter)
	return item, true
}
