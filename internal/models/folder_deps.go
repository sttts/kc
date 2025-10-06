package models

import (
	"context"

	kccluster "github.com/sttts/kc/internal/cluster"
	"github.com/sttts/kc/pkg/appconfig"
)

// Deps mirrors navigation.Deps but lives in the folders package to avoid cycles.
type Deps struct {
	Cl           *kccluster.Cluster
	Ctx          context.Context
	CtxName      string
	ListContexts func() []string
	EnterContext func(name string, basePath []string) (Folder, error)
	Config       *appconfig.Config
}

// Config returns the bound application configuration, falling back to defaults when unset.
func (d Deps) Config() *appconfig.Config {
	if d.Config != nil {
		return d.Config
	}
	return appconfig.Default()
}
