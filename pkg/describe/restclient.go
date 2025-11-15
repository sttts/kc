package describe

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type staticRESTClientGetter struct {
	config    *rest.Config
	mapper    meta.RESTMapper
	discovery discovery.CachedDiscoveryInterface
	loader    clientcmd.ClientConfig
}

var _ genericclioptions.RESTClientGetter = (*staticRESTClientGetter)(nil)

func newStaticRESTClientGetter(cfg *rest.Config, mapper meta.RESTMapper, disco discovery.CachedDiscoveryInterface, loader clientcmd.ClientConfig) *staticRESTClientGetter {
	copyCfg := rest.CopyConfig(cfg)
	if loader == nil {
		loader = newStaticClientConfig(copyCfg)
	}
	return &staticRESTClientGetter{
		config:    copyCfg,
		mapper:    mapper,
		discovery: disco,
		loader:    loader,
	}
}

func (s *staticRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	if s.config == nil {
		return nil, fmt.Errorf("rest config unavailable")
	}
	return rest.CopyConfig(s.config), nil
}

func (s *staticRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	if s.discovery != nil {
		return s.discovery, nil
	}
	cfg, err := s.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(dc), nil
}

func (s *staticRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	if s.mapper != nil {
		return s.mapper, nil
	}
	dc, err := s.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	base := restmapper.NewDeferredDiscoveryRESTMapper(dc)
	return restmapper.NewShortcutExpander(base, dc, func(string) {}), nil
}

func (s *staticRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	if s.loader != nil {
		return s.loader
	}
	return newStaticClientConfig(s.config)
}

type staticClientConfig struct {
	cfg       *rest.Config
	namespace string
	raw       clientcmdapi.Config
}

func newStaticClientConfig(cfg *rest.Config) clientcmd.ClientConfig {
	return &staticClientConfig{cfg: rest.CopyConfig(cfg)}
}

func (s *staticClientConfig) RawConfig() (clientcmdapi.Config, error) {
	return s.raw, nil
}

func (s *staticClientConfig) ClientConfig() (*rest.Config, error) {
	if s.cfg == nil {
		return nil, fmt.Errorf("rest config unavailable")
	}
	return rest.CopyConfig(s.cfg), nil
}

func (s *staticClientConfig) Namespace() (string, bool, error) {
	if s.namespace == "" {
		return "", false, nil
	}
	return s.namespace, true, nil
}

func (s *staticClientConfig) ConfigAccess() clientcmd.ConfigAccess {
	return clientcmd.NewDefaultClientConfigLoadingRules()
}
