package models

import (
	"context"
	"time"

	kccluster "github.com/sttts/kc/internal/cluster"
)

// Deps mirrors navigation.Deps but lives in the folders package to avoid cycles.
type Deps struct {
	Cl           *kccluster.Cluster
	Ctx          context.Context
	CtxName      string
	ListContexts func() []string
	EnterContext func(name string, basePath []string) (Folder, error)
	ViewOptions  func() ViewOptions
}

// ViewOptions mirrors navigation.ViewOptions for folder population behaviour.
type ViewOptions struct {
	ShowNonEmptyOnly bool
	Order            string
	Favorites        map[string]bool
	Columns          string
	ObjectsOrder     string
	PeekInterval     time.Duration
}

const defaultPeekInterval = 30 * time.Second

func resolveViewOptions(deps Deps) ViewOptions {
	if deps.ViewOptions == nil {
		return ViewOptions{PeekInterval: defaultPeekInterval}
	}
	opts := deps.ViewOptions()
	if opts.PeekInterval <= 0 {
		opts.PeekInterval = defaultPeekInterval
	}
	return opts
}
