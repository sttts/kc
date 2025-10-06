package appconfig

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	yaml "sigs.k8s.io/yaml"
)

type ViewerConfig struct {
	Theme string `json:"theme"`
}

const (
	ColumnsModeNormal = "normal"
	ColumnsModeWide   = "wide"
)

const (
	ObjectsOrderName         = "name"
	ObjectsOrderNameDesc     = "-name"
	ObjectsOrderCreation     = "creation"
	ObjectsOrderCreationDesc = "-creation"
)

type HorizontalConfig struct {
	Step int `json:"step"`
}

type ScrollingConfig struct {
	Horizontal HorizontalConfig `json:"horizontal"`
}

type PanelConfig struct {
	Scrolling ScrollingConfig `json:"scrolling"`
	Table     TableConfig     `json:"table"`
}

type MouseConfig struct {
	DoubleClickTimeout metav1.Duration `json:"doubleClickTimeout"`
}

type InputConfig struct {
	Mouse MouseConfig `json:"mouse"`
}

type ClustersConfig struct {
	TTL metav1.Duration `json:"ttl"` // duration, e.g. 2m, 30s
}

type KubernetesConfig struct {
	Clusters ClustersConfig `json:"clusters"`
}

// ResourcesViewOrder is the ordering mode for resource groups.
// Valid values: "alpha", "group", "favorites".
type ResourcesViewOrder string

const (
	OrderAlpha     ResourcesViewOrder = "alpha"
	OrderGroup     ResourcesViewOrder = "group"
	OrderFavorites ResourcesViewOrder = "favorites"
)

// ResourcesViewConfig controls how resource groups are displayed.
type ResourcesViewConfig struct {
	// ShowNonEmptyOnly toggles filtering of resource groups to those with >0 objects.
	ShowNonEmptyOnly bool `json:"showNonEmptyOnly"`
	// Order controls ordering of groups. Valid values are "alpha", "group", "favorites".
	Order ResourcesViewOrder `json:"order"`
	// Favorites lists resource plural names to prioritize when OrderFavorites is active.
	Favorites []string `json:"favorites"`
	// Columns controls which server-side table columns are shown. Valid values are "normal" and "wide".
	Columns string `json:"columns"`
	// ObjectsOrder controls ordering within object lists when drilling into resources. Valid values are "name", "-name", "creation", "-creation".
	ObjectsOrder string `json:"objectsOrder"`
	// PeekInterval throttles how often empty-resource peeks hit the API (default 30s).
	PeekInterval metav1.Duration `json:"peekInterval"`
}

type Config struct {
	Viewer     ViewerConfig        `json:"viewer"`
	Panel      PanelConfig         `json:"panel"`
	Input      InputConfig         `json:"input"`
	Kubernetes KubernetesConfig    `json:"kubernetes"`
	Resources  ResourcesViewConfig `json:"resources"`
	Objects    ObjectsConfig       `json:"objects"`
}

// ObjectsConfig controls object-list specific options.
type ObjectsConfig struct {
	// Order controls ordering within object lists. Valid values conform to ObjectsOrder* constants (Name, NameDesc, Creation, CreationDesc).
	Order string `json:"order"`
	// Columns controls which columns are shown. Valid values are ColumnsModeNormal and ColumnsModeWide.
	Columns string `json:"columns"`
}

// TableMode selects how tables render horizontally.
// "scroll": horizontal panning across all columns.
// "fit": fit all columns within the viewport width.
type TableMode string

const (
	TableModeScroll TableMode = "scroll"
	TableModeFit    TableMode = "fit"
)

type TableConfig struct {
	Mode TableMode `json:"mode"`
}

func Default() *Config {
	return &Config{
		Viewer:     ViewerConfig{Theme: "dracula"},
		Panel:      PanelConfig{Scrolling: ScrollingConfig{Horizontal: HorizontalConfig{Step: 4}}, Table: TableConfig{Mode: TableModeScroll}},
		Input:      InputConfig{Mouse: MouseConfig{DoubleClickTimeout: metav1.Duration{Duration: 300 * time.Millisecond}}},
		Kubernetes: KubernetesConfig{Clusters: ClustersConfig{TTL: metav1.Duration{Duration: 2 * time.Minute}}},
		Resources: ResourcesViewConfig{
			ShowNonEmptyOnly: true,
			Order:            OrderAlpha,
			Columns:          ColumnsModeNormal,
			PeekInterval:     metav1.Duration{Duration: 30 * time.Second},
			// Seed favorites with a sensible default set similar to `kubectl get all`.
			Favorites: []string{
				"pods", "services", "deployments", "replicasets", "statefulsets",
				"daemonsets", "jobs", "cronjobs", "configmaps", "secrets",
				"ingresses", "networkpolicies", "persistentvolumeclaims",
			},
		},
		Objects: ObjectsConfig{Order: ObjectsOrderName, Columns: ColumnsModeNormal},
	}
}

func path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kc", "config.yaml"), nil
}

// Load reads ~/.kc/config.yaml if present, otherwise returns defaults.
func Load() (*Config, error) {
	cfg := Default()
	p, err := path()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	// First try strict unmarshal (lower-case tags)
	if err := yaml.Unmarshal(data, cfg); err == nil {
		if cfg.Viewer.Theme == "" {
			cfg.Viewer.Theme = "dracula"
		}
		if cfg.Panel.Scrolling.Horizontal.Step <= 0 {
			cfg.Panel.Scrolling.Horizontal.Step = 4
		}
		if cfg.Panel.Table.Mode != TableModeFit && cfg.Panel.Table.Mode != TableModeScroll {
			cfg.Panel.Table.Mode = TableModeScroll
		}
		if cfg.Kubernetes.Clusters.TTL.Duration == 0 {
			cfg.Kubernetes.Clusters.TTL = metav1.Duration{Duration: 2 * time.Minute}
		}
		if cfg.Input.Mouse.DoubleClickTimeout.Duration == 0 {
			cfg.Input.Mouse.DoubleClickTimeout = metav1.Duration{Duration: 300 * time.Millisecond}
		}
		// Normalize resources settings
		switch cfg.Resources.Order {
		case OrderAlpha, OrderGroup, OrderFavorites:
		default:
			cfg.Resources.Order = OrderFavorites
		}
		if cfg.Resources.Favorites == nil {
			cfg.Resources.Favorites = Default().Resources.Favorites
		}
		if strings.EqualFold(cfg.Resources.Columns, ColumnsModeWide) {
			cfg.Resources.Columns = ColumnsModeWide
		} else {
			cfg.Resources.Columns = ColumnsModeNormal
		}
		if cfg.Resources.PeekInterval.Duration <= 0 {
			cfg.Resources.PeekInterval = metav1.Duration{Duration: 30 * time.Second}
		}
		switch {
		case strings.EqualFold(cfg.Objects.Order, ObjectsOrderName):
			cfg.Objects.Order = ObjectsOrderName
		case strings.EqualFold(cfg.Objects.Order, ObjectsOrderNameDesc):
			cfg.Objects.Order = ObjectsOrderNameDesc
		case strings.EqualFold(cfg.Objects.Order, ObjectsOrderCreation):
			cfg.Objects.Order = ObjectsOrderCreation
		case strings.EqualFold(cfg.Objects.Order, ObjectsOrderCreationDesc):
			cfg.Objects.Order = ObjectsOrderCreationDesc
		default:
			cfg.Objects.Order = ObjectsOrderName
		}
		if strings.EqualFold(cfg.Objects.Columns, ColumnsModeWide) {
			cfg.Objects.Columns = ColumnsModeWide
		} else {
			cfg.Objects.Columns = ColumnsModeNormal
		}
		return cfg, nil
	}
	// Fallback: tolerate legacy/mixed-case keys by normalizing
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return cfg, err
	}
	// Find "viewer" section case-insensitively
	var viewer any
	for k, v := range raw {
		if strings.EqualFold(k, "viewer") {
			viewer = v
			break
		}
	}
	if m, ok := viewer.(map[string]any); ok {
		// Find "theme" key case-insensitively
		for k, v := range m {
			if strings.EqualFold(k, "theme") {
				if s, ok := v.(string); ok && s != "" {
					cfg.Viewer.Theme = strings.ToLower(s)
				}
			}
		}
	}
	if cfg.Viewer.Theme == "" {
		cfg.Viewer.Theme = "dracula"
	}

	// Try to read panel.scrolling.horizontal.step in a case-insensitive way
	var panel any
	for k, v := range raw {
		if strings.EqualFold(k, "panel") {
			panel = v
			break
		}
	}
	if pm, ok := panel.(map[string]any); ok {
		var scrolling any
		for k, v := range pm {
			if strings.EqualFold(k, "scrolling") {
				scrolling = v
				break
			}
		}
		if sm, ok := scrolling.(map[string]any); ok {
			var horizontal any
			for k, v := range sm {
				if strings.EqualFold(k, "horizontal") {
					horizontal = v
					break
				}
			}
			if hm, ok := horizontal.(map[string]any); ok {
				for k, v := range hm {
					if strings.EqualFold(k, "step") {
						// Accept numbers as int/float
						switch t := v.(type) {
						case int:
							cfg.Panel.Scrolling.Horizontal.Step = t
						case int64:
							cfg.Panel.Scrolling.Horizontal.Step = int(t)
						case float64:
							cfg.Panel.Scrolling.Horizontal.Step = int(t)
						}
					}
				}
			}
		}
	}
	if cfg.Panel.Scrolling.Horizontal.Step <= 0 {
		cfg.Panel.Scrolling.Horizontal.Step = 4
	}
	if cfg.Panel.Table.Mode != TableModeFit && cfg.Panel.Table.Mode != TableModeScroll {
		cfg.Panel.Table.Mode = TableModeScroll
	}
	if cfg.Kubernetes.Clusters.TTL.Duration == 0 {
		cfg.Kubernetes.Clusters.TTL = metav1.Duration{Duration: 2 * time.Minute}
	}
	if cfg.Input.Mouse.DoubleClickTimeout.Duration == 0 {
		cfg.Input.Mouse.DoubleClickTimeout = metav1.Duration{Duration: 300 * time.Millisecond}
	}
	// Normalize resources settings
	switch cfg.Resources.Order {
	case OrderAlpha, OrderGroup, OrderFavorites:
	default:
		cfg.Resources.Order = OrderFavorites
	}
	if cfg.Resources.Favorites == nil {
		cfg.Resources.Favorites = Default().Resources.Favorites
	}
	if strings.EqualFold(cfg.Resources.Columns, ColumnsModeWide) {
		cfg.Resources.Columns = ColumnsModeWide
	} else {
		cfg.Resources.Columns = ColumnsModeNormal
	}
	if cfg.Resources.PeekInterval.Duration <= 0 {
		cfg.Resources.PeekInterval = metav1.Duration{Duration: 30 * time.Second}
	}
	switch {
	case strings.EqualFold(cfg.Objects.Order, ObjectsOrderName):
		cfg.Objects.Order = ObjectsOrderName
	case strings.EqualFold(cfg.Objects.Order, ObjectsOrderNameDesc):
		cfg.Objects.Order = ObjectsOrderNameDesc
	case strings.EqualFold(cfg.Objects.Order, ObjectsOrderCreation):
		cfg.Objects.Order = ObjectsOrderCreation
	case strings.EqualFold(cfg.Objects.Order, ObjectsOrderCreationDesc):
		cfg.Objects.Order = ObjectsOrderCreationDesc
	default:
		cfg.Objects.Order = ObjectsOrderName
	}
	if strings.EqualFold(cfg.Objects.Columns, ColumnsModeWide) {
		cfg.Objects.Columns = ColumnsModeWide
	} else {
		cfg.Objects.Columns = ColumnsModeNormal
	}
	return cfg, nil
}

// Save writes the config to ~/.kc/config.yaml, creating the directory if needed.
func Save(cfg *Config) error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	// Enforce lower-case style names for consistency
	out := *cfg
	out.Viewer.Theme = strings.ToLower(out.Viewer.Theme)
	// Normalize order value
	switch out.Resources.Order {
	case OrderAlpha, OrderGroup, OrderFavorites:
	default:
		out.Resources.Order = OrderFavorites
	}
	if strings.EqualFold(out.Resources.Columns, ColumnsModeWide) {
		out.Resources.Columns = ColumnsModeWide
	} else {
		out.Resources.Columns = ColumnsModeNormal
	}
	if out.Resources.PeekInterval.Duration <= 0 {
		out.Resources.PeekInterval = metav1.Duration{Duration: 30 * time.Second}
	}
	switch {
	case strings.EqualFold(out.Objects.Order, ObjectsOrderName):
		out.Objects.Order = ObjectsOrderName
	case strings.EqualFold(out.Objects.Order, ObjectsOrderNameDesc):
		out.Objects.Order = ObjectsOrderNameDesc
	case strings.EqualFold(out.Objects.Order, ObjectsOrderCreation):
		out.Objects.Order = ObjectsOrderCreation
	case strings.EqualFold(out.Objects.Order, ObjectsOrderCreationDesc):
		out.Objects.Order = ObjectsOrderCreationDesc
	default:
		out.Objects.Order = ObjectsOrderName
	}
	if strings.EqualFold(out.Objects.Columns, ColumnsModeWide) {
		out.Objects.Columns = ColumnsModeWide
	} else {
		out.Objects.Columns = ColumnsModeNormal
	}
	data, err := yaml.Marshal(&out)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
