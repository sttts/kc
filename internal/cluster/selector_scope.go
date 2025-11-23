package cluster

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// SelectorKeyForScopes produces a deterministic signature for pooling selector-scoped clusters.
func SelectorKeyForScopes(scopes map[schema.GroupVersionResource]cache.ByObject) string {
	if len(scopes) == 0 {
		return ""
	}
	keys := make([]schema.GroupVersionResource, 0, len(scopes))
	for gvr := range scopes {
		keys = append(keys, gvr)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Group != keys[j].Group {
			return keys[i].Group < keys[j].Group
		}
		if keys[i].Version != keys[j].Version {
			return keys[i].Version < keys[j].Version
		}
		return keys[i].Resource < keys[j].Resource
	})
	parts := make([]string, 0, len(keys))
	for _, gvr := range keys {
		sel := scopes[gvr]
		lbl := labelSelectorString(sel.Label)
		fld := fieldSelectorString(sel.Field)
		parts = append(parts, fmt.Sprintf("%s|label:%s|field:%s", gvr.String(), lbl, fld))
	}
	return strings.Join(parts, ";")
}

func labelSelectorString(sel labels.Selector) string {
	if sel == nil || sel.Empty() {
		return ""
	}
	return sel.String()
}

func fieldSelectorString(sel fields.Selector) string {
	if sel == nil || sel.Empty() {
		return ""
	}
	return sel.String()
}

func copySelectorScope(scopes map[schema.GroupVersionResource]cache.ByObject) map[schema.GroupVersionResource]cache.ByObject {
	if len(scopes) == 0 {
		return nil
	}
	out := make(map[schema.GroupVersionResource]cache.ByObject, len(scopes))
	for gvr, sel := range scopes {
		out[gvr] = sel
	}
	return out
}

func selectorListOptions(sel cache.ByObject) []crclient.ListOption {
	var opts []crclient.ListOption
	if sel.Label != nil && !sel.Label.Empty() {
		opts = append(opts, crclient.MatchingLabelsSelector{Selector: sel.Label})
	}
	if sel.Field != nil && !sel.Field.Empty() {
		opts = append(opts, crclient.MatchingFieldsSelector{Selector: sel.Field})
	}
	return opts
}
