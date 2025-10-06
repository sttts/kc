package models

import (
	"context"

	kccluster "github.com/sttts/kc/internal/cluster"
	"github.com/sttts/kc/pkg/appconfig"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Deps contains the inputs required by navigation folders.
// Invariants:
//   - Cl is non-nil and already started.
//   - Ctx is non-nil and used for informer/list operations.
//   - CtxName is the human-facing context label (may be empty for cluster-scoped views).
//   - KubeConfig always contains the discovered contexts (never nil maps).
//   - AppConfig is non-nil and already validated by appconfig loading.
type Deps struct {
	Cl         *kccluster.Cluster
	Ctx        context.Context
	CtxName    string
	KubeConfig clientcmdapi.Config
	AppConfig  *appconfig.Config
}
