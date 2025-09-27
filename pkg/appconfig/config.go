package appconfig

import (
    "errors"
    "io/fs"
    "os"
    "path/filepath"

    yaml "sigs.k8s.io/yaml"
)

type ViewerConfig struct {
    Theme string `yaml:"theme"`
}

type Config struct {
    Viewer ViewerConfig `yaml:"viewer"`
}

func Default() *Config { return &Config{Viewer: ViewerConfig{Theme: "dracula"}} }

func path() (string, error) {
    home, err := os.UserHomeDir()
    if err != nil { return "", err }
    return filepath.Join(home, ".kc", "config.yaml"), nil
}

// Load reads ~/.kc/config.yaml if present, otherwise returns defaults.
func Load() (*Config, error) {
    cfg := Default()
    p, err := path()
    if err != nil { return cfg, err }
    data, err := os.ReadFile(p)
    if err != nil {
        if errors.Is(err, fs.ErrNotExist) { return cfg, nil }
        return cfg, err
    }
    if err := yaml.Unmarshal(data, cfg); err != nil { return cfg, err }
    if cfg.Viewer.Theme == "" { cfg.Viewer.Theme = "dracula" }
    return cfg, nil
}

// Save writes the config to ~/.kc/config.yaml, creating the directory if needed.
func Save(cfg *Config) error {
    p, err := path()
    if err != nil { return err }
    if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { return err }
    data, err := yaml.Marshal(cfg)
    if err != nil { return err }
    return os.WriteFile(p, data, 0o644)
}

