package appconfig

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	yaml "sigs.k8s.io/yaml"
)

type ViewerConfig struct {
    Theme string `json:"theme"`
}

type HorizontalConfig struct {
    Step int `json:"step"`
}

type ScrollingConfig struct {
    Horizontal HorizontalConfig `json:"horizontal"`
}

type PanelConfig struct {
    Scrolling ScrollingConfig `json:"scrolling"`
}

type Config struct {
    Viewer ViewerConfig `json:"viewer"`
    Panel  PanelConfig  `json:"panel"`
}

func Default() *Config {
    return &Config{
        Viewer: ViewerConfig{Theme: "dracula"},
        Panel:  PanelConfig{Scrolling: ScrollingConfig{Horizontal: HorizontalConfig{Step: 4}}},
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
        if strings.EqualFold(k, "panel") { panel = v; break }
    }
    if pm, ok := panel.(map[string]any); ok {
        var scrolling any
        for k, v := range pm {
            if strings.EqualFold(k, "scrolling") { scrolling = v; break }
        }
        if sm, ok := scrolling.(map[string]any); ok {
            var horizontal any
            for k, v := range sm {
                if strings.EqualFold(k, "horizontal") { horizontal = v; break }
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
    if cfg.Panel.Scrolling.Horizontal.Step <= 0 { cfg.Panel.Scrolling.Horizontal.Step = 4 }
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
    data, err := yaml.Marshal(&out)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
