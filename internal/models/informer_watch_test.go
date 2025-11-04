package models

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	kccluster "github.com/sttts/kc/internal/cluster"
	kctesting "github.com/sttts/kc/internal/testing"
	"github.com/sttts/kc/pkg/appconfig"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestStartInformerForResourceEmitsSyncedAndWatches(t *testing.T) {
	t.Parallel()

	env := &envtest.Environment{}
	cfg, err := env.Start()
	if err != nil || cfg == nil {
		if err != nil && strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("envtest unavailable: %v", err)
		}
		t.Fatalf("start envtest: %v", err)
	}
	defer func() { _ = env.Stop() }()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 to scheme: %v", err)
	}

	cl, err := kccluster.New(cfg, kccluster.WithScheme(scheme))
	if err != nil {
		t.Fatalf("cluster: %v", err)
	}

	ctx := t.Context()
	go cl.Start(ctx)

	nsName := fmt.Sprintf("watch-%d", time.Now().UnixNano())
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
	if err := cl.GetClient().Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}

	deps := Deps{
		Cl:         cl,
		Ctx:        ctx,
		AppConfig:  appconfig.Default(),
		KubeConfig: clientcmdapi.Config{},
	}

	gvrPods := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

	var seen atomic.Int32
	eventCh := make(chan struct{}, 8)
	var stopped atomic.Bool
	cancelWatch, err := startInformerForResource(deps, gvrPods, nsName, "", func() {
		if seen.Add(1) <= int32(cap(eventCh)) {
			select {
			case eventCh <- struct{}{}:
			default:
			}
		}
	}, func() {
		stopped.Store(true)
	})
	if err != nil {
		t.Fatalf("start informer: %v", err)
	}
	t.Cleanup(func() {
		cancelWatch()
	})

	waitEvent := func(msg string) {
		select {
		case <-eventCh:
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for %s event", msg)
		}
	}

	waitEvent("Synced")

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "watch-pod",
			Namespace: nsName,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "pause",
				Image: "registry.k8s.io/pause:3.9",
			}},
		},
	}
	if err := cl.GetClient().Create(ctx, pod); err != nil {
		t.Fatalf("create pod: %v", err)
	}

	waitEvent("Add")

	otherNS := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName + "-other"}}
	if err := cl.GetClient().Create(ctx, otherNS); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create other namespace: %v", err)
	}

	otherPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-pod",
			Namespace: otherNS.Name,
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "pause", Image: "registry.k8s.io/pause:3.9"}}},
	}
	if err := cl.GetClient().Create(ctx, otherPod); err != nil {
		t.Fatalf("create other pod: %v", err)
	}

	select {
	case <-eventCh:
		t.Fatalf("unexpected watch event for other namespace")
	case <-time.After(500 * time.Millisecond):
	}

	cancelWatch()
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool {
		return stopped.Load()
	})
}
