package cluster

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
)

func TestSelectorKeyForScopesDeterministic(t *testing.T) {
	pods := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	configmaps := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}

	scopeA := map[schema.GroupVersionResource]crcache.ByObject{
		pods:       {Field: fields.OneTermEqualSelector("spec.nodeName", "node-a")},
		configmaps: {Label: labels.SelectorFromSet(labels.Set{"env": "prod"})},
	}
	scopeB := map[schema.GroupVersionResource]crcache.ByObject{
		configmaps: {Label: labels.SelectorFromSet(labels.Set{"env": "prod"})},
		pods:       {Field: fields.OneTermEqualSelector("spec.nodeName", "node-a")},
	}

	keyA := SelectorKeyForScopes(scopeA)
	keyB := SelectorKeyForScopes(scopeB)

	if keyA == "" {
		t.Fatalf("expected selector key to be non-empty")
	}
	if keyA != keyB {
		t.Fatalf("selector keys differ for same scopes: %q vs %q", keyA, keyB)
	}
	if want := "spec.nodeName=node-a"; !contains(keyA, want) {
		t.Fatalf("selector key %q missing %q", keyA, want)
	}
	if want := "env=prod"; !contains(keyA, want) {
		t.Fatalf("selector key %q missing %q", keyA, want)
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
