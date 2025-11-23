package models

import (
	"context"
	"strings"

	kccluster "github.com/sttts/kc/internal/cluster"
	"github.com/sttts/kc/internal/podfs"
	"github.com/sttts/kc/pkg/appconfig"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	crcache "sigs.k8s.io/controller-runtime/pkg/cache"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
)

// NamespaceClusterFactory provides namespace-scoped clusters on demand.
type NamespaceClusterFactory func(ctx context.Context, key kccluster.Key, selectors map[schema.GroupVersionResource]crcache.ByObject) (*kccluster.Cluster, error)

// Deps contains the inputs required by navigation folders.
// Invariants:
//   - Cl is non-nil and already started.
//   - Ctx is non-nil and used for informer/list operations.
//   - CtxName is the human-facing context label (may be empty for cluster-scoped views).
//   - KubeConfig always contains the discovered contexts (never nil maps).
//   - AppConfig is non-nil and already validated by appconfig loading.
type Deps struct {
	Cl               *kccluster.Cluster
	Ctx              context.Context
	CtxName          string
	KubeConfig       clientcmdapi.Config
	AppConfig        *appconfig.Config
	ClusterKey       kccluster.Key
	NamespaceFactory NamespaceClusterFactory
	PodFSFactory     podfs.Factory
}

// ForNamespace derives a copy of Deps using a namespace-scoped cluster when available.
func (d Deps) ForNamespace(namespace string) Deps {
	ns := strings.TrimSpace(namespace)
	if ns == "" || d.NamespaceFactory == nil {
		d.ClusterKey.Namespace = ns
		return d
	}
	ctx := d.Ctx
	key := d.ClusterKey
	key.Namespace = ns
	cl, err := d.NamespaceFactory(ctx, key, nil)
	if err != nil {
		crlog.FromContext(ctx).Error(err, "namespace cluster acquisition failed", "namespace", ns)
		return d
	}
	d.Cl = cl
	d.ClusterKey = key
	return d
}

// ForSelectorScope derives a copy of Deps using a selector-scoped cluster when available.
func (d Deps) ForSelectorScope(selectors map[schema.GroupVersionResource]crcache.ByObject) Deps {
	if len(selectors) == 0 {
		return d
	}
	key := d.ClusterKey
	key.SelectorKey = kccluster.SelectorKeyForScopes(selectors)
	if d.NamespaceFactory == nil {
		d.ClusterKey = key
		return d
	}
	cl, err := d.NamespaceFactory(d.Ctx, key, selectors)
	if err != nil {
		crlog.FromContext(d.Ctx).Error(err, "selector-scoped cluster acquisition failed", "selectorKey", key.SelectorKey)
		d.ClusterKey = key
		return d
	}
	d.Cl = cl
	d.ClusterKey = key
	return d
}
