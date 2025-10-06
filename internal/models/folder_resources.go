package models

import (
	"fmt"
	"sort"
	"strings"

	kccluster "github.com/sttts/kc/internal/cluster"
	table "github.com/sttts/kc/internal/table"
	"github.com/sttts/kc/pkg/appconfig"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResourcesFolder provides shared scaffolding for resource-group folders (namespace and cluster scoped).
type ResourcesFolder struct {
	*BaseFolder
}

// NewResourcesFolder constructs a ResourcesFolder with default columns and caller-provided metadata.
func NewResourcesFolder(base *BaseFolder) *ResourcesFolder {
	base.SetColumns([]table.Column{{Title: " Name"}, {Title: "Group"}, {Title: "Count"}})
	return &ResourcesFolder{BaseFolder: base}
}

func (f *ResourcesFolder) finalize(items []*ResourceGroupItem) []table.Row {
	if len(items) == 0 {
		return nil
	}
	cfg := f.Deps.AppConfig
	showNonEmpty := cfg.Resources.ShowNonEmptyOnly
	peekInterval := cfg.Resources.PeekInterval.Duration
	rows := make([]table.Row, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if existing := reuseResourceGroupItem(f.BaseFolder, item); existing != nil {
			item = existing
		}
		item.ComputeCountAsync(func() {
			f.BaseFolder.markDirty()
		})
		if showNonEmpty && item.EmptyWithin(peekInterval) {
			continue
		}
		if count, ok := item.TryCount(); ok {
			item.Cells[2] = fmt.Sprintf("%d", count)
		} else {
			item.Cells[2] = ""
		}
		rows = append(rows, item)
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

func reuseResourceGroupItem(base *BaseFolder, fresh *ResourceGroupItem) *ResourceGroupItem {
	if base == nil || fresh == nil || base.items == nil {
		return nil
	}
	id := fresh.ID()
	if id == "" {
		return nil
	}
	if existing, ok := base.items[id]; ok {
		if cur, ok := existing.(*ResourceGroupItem); ok {
			cur.CopyFrom(fresh)
			return cur
		}
	}
	return nil
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
