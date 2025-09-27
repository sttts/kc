package ui

// TriState represents Yes/No/Default values.
type TriState int

const (
    Default TriState = iota
    Yes
    No
)

// ViewSettings captures view-related options.
type ViewSettings struct {
    Table TriState
}

// ViewConfig holds global defaults and per-resource overrides (by plural name).
type ViewConfig struct {
    Global      ViewSettings
    PerResource map[string]ViewSettings
}

func NewViewConfig() *ViewConfig {
    return &ViewConfig{Global: ViewSettings{Table: Default}, PerResource: make(map[string]ViewSettings)}
}

// Resolve returns effective settings for a resource plural; empty resource yields global.
func (c *ViewConfig) Resolve(resource string) ViewSettings {
    eff := c.Global
    if resource == "" { return eff }
    if ov, ok := c.PerResource[resource]; ok {
        if ov.Table != Default { eff.Table = ov.Table }
    }
    return eff
}

// Set sets a tri-state option for global or a specific resource.
func (c *ViewConfig) Set(resource string, s ViewSettings) {
    if resource == "" {
        c.Global = s
        return
    }
    c.PerResource[resource] = s
}

