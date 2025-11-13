package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	kccluster "github.com/sttts/kc/internal/cluster"
	"github.com/sttts/kc/pkg/kubeconfig"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func setupAppWithCluster(t *testing.T) (*App, *kccluster.Cluster) {
	if testCfg == nil {
		t.Skip("envtest not available")
	}
	cl, err := kccluster.New(testCfg)
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}
	ctx := t.Context()
	go cl.Start(ctx)
	t.Cleanup(cl.Stop)

	app := NewApp()
	app.cancel()
	app.ctx, app.cancel = context.WithCancel(ctx)
	t.Cleanup(app.cancel)
	app.cl = cl
	app.currentCtx = &kubeconfig.Context{Name: "env"}
	return app, cl
}

func createNamespace(t *testing.T, cl *kccluster.Cluster, name string) func() {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := cl.GetClient().Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}
	return func() {
		delCtx, cancelDel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancelDel()
		_ = cl.GetClient().Delete(delCtx, ns)
	}
}

func createPod(t *testing.T, cl *kccluster.Cluster, namespace, name string, containers []corev1.Container) func() {
	t.Helper()
	if len(containers) == 0 {
		containers = []corev1.Container{{Name: "app", Image: "busybox"}}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: containers,
		},
	}
	if err := cl.GetClient().Create(ctx, pod); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create pod %s/%s: %v", namespace, name, err)
	}
	return func() {
		delCtx, cancelDel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancelDel()
		_ = cl.GetClient().Delete(delCtx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}})
	}
}

func waitForNamespaceReady(t *testing.T, app *App, ns string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		if app.namespaceExists(ns) {
			return
		}
		select {
		case <-time.After(50 * time.Millisecond):
		case <-ctx.Done():
			t.Fatalf("namespace %q not observed: %v", ns, ctx.Err())
		}
	}
}

func TestStartupIntentGetMultiSelectEnvtest(t *testing.T) {
	app, cl := setupAppWithCluster(t)
	ns := "intent-multi"
	t.Cleanup(createNamespace(t, cl, ns))
	t.Cleanup(createPod(t, cl, ns, "pod-a", nil))
	t.Cleanup(createPod(t, cl, ns, "pod-b", nil))

	waitForNamespaceReady(t, app, ns)
	app.goToNamespace(ns)

	app.startupIntent = StartupIntent{
		Verb:      KubectlVerbGet,
		Namespace: ns,
		Get: &GetIntent{
			Tokens: []GetToken{
				{Value: "pods", ExplicitResource: true},
				{Value: "pod-a"},
				{Value: "pod-b"},
			},
		},
	}
	app.startupIntentApplied = false
	if cmd := app.applyStartupIntent(); cmd != nil {
		cmd()
	}

	wantPath := fmt.Sprintf("/namespaces/%s/pods", ns)
	if got := app.leftPanel.GetCurrentPath(); got != wantPath {
		t.Fatalf("left panel path = %q, want %q", got, wantPath)
	}
	items := app.leftPanel.GetSelectedItems()
	names := selectedItemNames(items)
	if !equalStringSets(names, []string{"pod-a", "pod-b"}) {
		t.Fatalf("selected pods = %v, want [pod-a pod-b]", names)
	}
	if app.leftPanel.Mode() != PanelModeList {
		t.Fatalf("left panel mode = %v, want list", app.leftPanel.Mode())
	}
}

func TestStartupIntentGetOutputYAMLEnvtest(t *testing.T) {
	app, cl := setupAppWithCluster(t)
	ns := "intent-yaml"
	podName := "pod-yaml"
	t.Cleanup(createNamespace(t, cl, ns))
	t.Cleanup(createPod(t, cl, ns, podName, nil))

	waitForNamespaceReady(t, app, ns)
	app.goToNamespace(ns)

	app.startupIntent = StartupIntent{
		Verb:      KubectlVerbGet,
		Namespace: ns,
		Get: &GetIntent{
			OutputFormat: "yaml",
			Tokens: []GetToken{
				{Value: "pods", ExplicitResource: true},
				{Value: podName},
			},
		},
	}
	app.startupIntentApplied = false
	if cmd := app.applyStartupIntent(); cmd != nil {
		cmd()
	}

	wantPath := fmt.Sprintf("/namespaces/%s/pods", ns)
	if got := app.leftPanel.GetCurrentPath(); got != wantPath {
		t.Fatalf("left panel path = %q, want %q", got, wantPath)
	}
	if app.rightPanel.Mode() != PanelModeManifest {
		t.Fatalf("right panel mode = %v, want manifest", app.rightPanel.Mode())
	}
	if app.activePanel != 0 {
		t.Fatalf("expected active panel to remain left (0), got %d", app.activePanel)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if item, ok := app.rightPanel.SelectedNavItem(ctx); !ok || item == nil {
		t.Fatalf("expected right panel selection")
	} else if path := "/" + strings.Join(item.Path(), "/"); path != fmt.Sprintf("/namespaces/%s/pods/%s", ns, podName) {
		t.Fatalf("right panel selection path = %q, want namespace pod path", path)
	}
}

func TestStartupIntentLogsEnvtest(t *testing.T) {
	app, cl := setupAppWithCluster(t)
	ns := "intent-logs"
	podName := "pod-logs"
	container := "main"
	t.Cleanup(createNamespace(t, cl, ns))
	t.Cleanup(createPod(t, cl, ns, podName, []corev1.Container{
		{Name: container, Image: "busybox"},
	}))

	waitForNamespaceReady(t, app, ns)
	app.goToNamespace(ns)

	app.startupIntent = StartupIntent{
		Verb:      KubectlVerbLogs,
		Namespace: ns,
		Logs: &LogsIntent{
			Pod:    podName,
			Follow: true,
		},
	}
	app.startupIntentApplied = false
	if cmd := app.applyStartupIntent(); cmd != nil {
		cmd()
	}

	wantPath := fmt.Sprintf("/namespaces/%s/pods/%s/containers/%s", ns, podName, container)
	if got := app.leftPanel.GetCurrentPath(); got != wantPath {
		t.Fatalf("left panel path = %q, want %q", got, wantPath)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if item, ok := app.leftPanel.SelectedNavItem(ctx); !ok || item == nil {
		t.Fatalf("expected left panel selection on logs entry")
	} else if path := "/" + strings.Join(item.Path(), "/"); !strings.HasSuffix(path, "/logs/latest") {
		t.Fatalf("selected item path = %q, want logs/latest entry", path)
	}
}

func selectedItemNames(items []Item) []string {
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, strings.TrimPrefix(it.Name, "/"))
	}
	sort.Strings(names)
	return names
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aCopy := append([]string(nil), a...)
	bCopy := append([]string(nil), b...)
	sort.Strings(aCopy)
	sort.Strings(bCopy)
	for i := range aCopy {
		if aCopy[i] != bCopy[i] {
			return false
		}
	}
	return true
}
