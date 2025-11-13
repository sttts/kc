package models

import (
	"context"
	"strings"

	kccluster "github.com/sttts/kc/internal/cluster"
	"github.com/sttts/kc/internal/podfs"
	"github.com/sttts/kc/pkg/appconfig"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
)

// NamespaceClusterFactory provides namespace-scoped clusters on demand.
type NamespaceClusterFactory func(ctx context.Context, key kccluster.Key) (*kccluster.Cluster, error)

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
		return d
	}
	ctx := d.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	key := d.ClusterKey
	key.Namespace = ns
	cl, err := d.NamespaceFactory(ctx, key)
	if err != nil {
		crlog.FromContext(ctx).Error(err, "namespace cluster acquisition failed", "namespace", ns)
		return d
	}
	d.Cl = cl
	d.ClusterKey = key
	return d
}
