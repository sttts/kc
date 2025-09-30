package tablecache

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// Options extends controller-runtime's cache.Options with helpers for table-aware caching.
type Options struct {
	cache.Options
}

// New constructs a cache that lists/watches Kubernetes resources using Table responses when requested.
func New(cfg *rest.Config, opts Options) (cache.Cache, error) {
	base, err := cache.New(cfg, opts.Options)
	if err != nil {
		return nil, err
	}

	sch := opts.Options.Scheme
	if sch == nil {
		sch = runtime.NewScheme()
	}

	httpClient := opts.Options.HTTPClient
	if httpClient == nil {
		httpClient, err = rest.HTTPClientFor(cfg)
		if err != nil {
			return nil, err
		}
	}

	mapper := opts.Options.Mapper
	if mapper == nil {
		mapper, err = apiutil.NewDynamicRESTMapper(cfg, httpClient)
		if err != nil {
			return nil, fmt.Errorf("tablecache: build rest mapper: %w", err)
		}
	}

	fetcher, err := newRESTTableFetcher(cfg, sch)
	if err != nil {
		return nil, err
	}

	return newRowCache(base, mapper, fetcher), nil
}

// NewFromOptions is a convenience wrapper that accepts a plain cache.Options value.
func NewFromOptions(cfg *rest.Config, cacheOpts cache.Options) (cache.Cache, error) {
	return New(cfg, Options{Options: cacheOpts})
}

// NewCacheFunc produces a cache.NewCacheFunc suitable for manager.Options.NewCache.
func NewCacheFunc() cache.NewCacheFunc {
	return func(cfg *rest.Config, opts cache.Options) (cache.Cache, error) {
		return New(cfg, Options{Options: opts})
	}
}
