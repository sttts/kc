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

type Config struct {
	Viewer ViewerConfig `json:"viewer"`
}

func Default() *Config { return &Config{Viewer: ViewerConfig{Theme: "dracula"}} }

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
