package podfs

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// NewFactory returns a native exec-backed factory with optional helper support.
func NewFactory(cfg *rest.Config) (Factory, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil kubeconfig")
	}
	base := rest.CopyConfig(cfg)
	base = rest.AddUserAgent(base, "kc-podfs")
	if base.QPS == 0 {
		base.QPS = 50
	}
	if base.Burst == 0 {
		base.Burst = 100
	}
	clientset, err := kubernetes.NewForConfig(base)
	if err != nil {
		return nil, fmt.Errorf("podfs: kubernetes client: %w", err)
	}
	return &nativeFactory{
		cfg:    base,
		client: clientset,
		helper: newHelperManager(clientset),
	}, nil
}

type nativeFactory struct {
	cfg    *rest.Config
	client kubernetes.Interface
	helper *helperManager
}

func (f *nativeFactory) NewSession(ctx context.Context, spec SessionSpec) (ExecSession, error) {
	if spec.Namespace == "" || spec.Pod == "" || spec.Container == "" {
		return nil, fmt.Errorf("podfs: namespace, pod, and container are required")
	}
	sess, err := newNativeSession(ctx, f.cfg, f.client, spec)
	if err == nil {
		return sess, nil
	}
	if !shouldFallback(err) || f.helper == nil || !f.helper.available() {
		return nil, err
	}
	helperSpec, helperErr := f.helper.ensureHelper(ctx, spec)
	if helperErr != nil {
		return nil, helperErr
	}
	helperSession, err := newNativeSession(ctx, f.cfg, f.client, helperSpec)
	if err != nil {
		return nil, err
	}
	return &helperAwareSession{ExecSession: helperSession, helperUsed: true}, nil
}

type helperAwareSession struct {
	ExecSession
	helperUsed bool
}

func (h helperAwareSession) HelperUsed() bool { return h.helperUsed }

func shouldFallback(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrShellMissing) {
		return true
	}
	var missing MissingCommandError
	return errors.As(err, &missing)
}
