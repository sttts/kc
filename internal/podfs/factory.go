package podfs

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// NewFactory returns a native exec-backed factory.
func NewFactory(cfg *rest.Config) (Factory, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil kubeconfig")
	}
	base := rest.CopyConfig(cfg)
	base = rest.AddUserAgent(base, "kc-podfs")
	clientset, err := kubernetes.NewForConfig(base)
	if err != nil {
		return nil, fmt.Errorf("podfs: kubernetes client: %w", err)
	}
	return &nativeFactory{cfg: base, client: clientset}, nil
}

type nativeFactory struct {
	cfg    *rest.Config
	client kubernetes.Interface
}

func (f *nativeFactory) NewSession(ctx context.Context, spec SessionSpec) (ExecSession, error) {
	if spec.Namespace == "" || spec.Pod == "" || spec.Container == "" {
		return nil, fmt.Errorf("podfs: namespace, pod, and container are required")
	}
	return newNativeSession(ctx, f.cfg, f.client, spec)
}
