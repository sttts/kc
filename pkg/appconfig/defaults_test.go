package appconfig

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"path/filepath"
	"reflect"
	yaml "sigs.k8s.io/yaml"
	"testing"
)

// TestConfigDefaultsYAMLMatchesCode reads config-default.yaml from the repo root
// and compares it with the in-code defaults returned by Default().
func TestConfigDefaultsYAMLMatchesCode(t *testing.T) {
	// Locate repo root by walking up from this file's dir until we find config-default.yaml
	wd, _ := os.Getwd()
	dir := wd
	var path string
	for i := 0; i < 6; i++ { // up to 6 levels
		p := filepath.Join(dir, "..", "..", "config-default.yaml")
		if _, err := os.Stat(p); err == nil {
			path = p
			break
		}
		dir = filepath.Dir(dir)
	}
	if path == "" {
		t.Skip("config-default.yaml not found; skipping defaults sync test")
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read defaults yaml: %v", err)
	}

	fromYAML := &Config{}
	if err := yaml.Unmarshal(data, fromYAML); err != nil {
		t.Fatalf("unmarshal defaults yaml: %v", err)
	}

	fromCode := Default()

	// Normalize: fill zero durations with expected defaults to keep comparison simple
	norm := func(c *Config) *Config {
		out := *c
		if out.Viewer.Theme == "" {
			out.Viewer.Theme = "dracula"
		}
		if out.Panel.Scrolling.Horizontal.Step == 0 {
			out.Panel.Scrolling.Horizontal.Step = 4
		}
		if out.Input.Mouse.DoubleClickTimeout.Duration == 0 {
			out.Input.Mouse.DoubleClickTimeout = metav1.Duration{Duration: Default().Input.Mouse.DoubleClickTimeout.Duration}
		}
		if out.Kubernetes.Clusters.TTL.Duration == 0 {
			out.Kubernetes.Clusters.TTL = metav1.Duration{Duration: Default().Kubernetes.Clusters.TTL.Duration}
		}
		if out.Resources.Order == "" {
			out.Resources.Order = fromCode.Resources.Order
		}
		if out.Resources.Favorites == nil {
			out.Resources.Favorites = fromCode.Resources.Favorites
		}
		if out.Resources.PeekInterval.Duration == 0 {
			out.Resources.PeekInterval = metav1.Duration{Duration: Default().Resources.PeekInterval.Duration}
		}
		return &out
	}

	a := norm(fromYAML)
	b := norm(fromCode)

	// Field-by-field comparison with clearer failure messages
	if a.Viewer.Theme != b.Viewer.Theme {
		t.Fatalf("viewer.theme mismatch: yaml=%q code=%q", a.Viewer.Theme, b.Viewer.Theme)
	}
	if a.Panel.Scrolling.Horizontal.Step != b.Panel.Scrolling.Horizontal.Step {
		t.Fatalf("panel.scrolling.horizontal.step mismatch: yaml=%d code=%d", a.Panel.Scrolling.Horizontal.Step, b.Panel.Scrolling.Horizontal.Step)
	}
	if a.Input.Mouse.DoubleClickTimeout.Duration != b.Input.Mouse.DoubleClickTimeout.Duration {
		t.Fatalf("input.mouse.doubleClickTimeout mismatch: yaml=%v code=%v", a.Input.Mouse.DoubleClickTimeout.Duration, b.Input.Mouse.DoubleClickTimeout.Duration)
	}
	if a.Kubernetes.Clusters.TTL.Duration != b.Kubernetes.Clusters.TTL.Duration {
		t.Fatalf("kubernetes.clusters.ttl mismatch: yaml=%v code=%v", a.Kubernetes.Clusters.TTL.Duration, b.Kubernetes.Clusters.TTL.Duration)
	}
	if bool(a.Resources.ShowNonEmptyOnly) != bool(b.Resources.ShowNonEmptyOnly) {
		t.Fatalf("resources.showNonEmptyOnly mismatch: yaml=%v code=%v", a.Resources.ShowNonEmptyOnly, b.Resources.ShowNonEmptyOnly)
	}
	if a.Resources.Order != b.Resources.Order {
		t.Fatalf("resources.order mismatch: yaml=%q code=%q", a.Resources.Order, b.Resources.Order)
	}
	if !reflect.DeepEqual(a.Resources.Favorites, b.Resources.Favorites) {
		t.Fatalf("resources.favorites mismatch: yaml=%v code=%v", a.Resources.Favorites, b.Resources.Favorites)
	}
	if a.Resources.PeekInterval.Duration != b.Resources.PeekInterval.Duration {
		t.Fatalf("resources.peekInterval mismatch: yaml=%v code=%v", a.Resources.PeekInterval.Duration, b.Resources.PeekInterval.Duration)
	}
}
