package ui

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/sttts/kc/pkg/kubeconfig"
)

func TestInitialNamespace(t *testing.T) {
	tcs := []struct {
		name     string
		ctxNs    string
		override string
		want     string
	}{
		{name: "context namespace used", ctxNs: "dev", want: "dev"},
		{name: "override wins", ctxNs: "dev", override: "prod", want: "prod"},
		{name: "default when empty", ctxNs: "", want: corev1.NamespaceDefault},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			app := NewApp()
			app.currentCtx = &kubeconfig.Context{Name: "ctx", Namespace: tc.ctxNs}
			app.namespaceOverride = tc.override
			if got := app.initialNamespace(); got != tc.want {
				t.Fatalf("initialNamespace() = %q, want %q", got, tc.want)
			}
		})
	}
}
