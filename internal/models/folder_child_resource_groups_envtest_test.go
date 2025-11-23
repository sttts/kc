package models

import (
	"context"
	"reflect"
	"testing"
	"time"

	kccluster "github.com/sttts/kc/internal/cluster"
	"github.com/sttts/kc/pkg/appconfig"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestChildResourceGroupsFolderCountsOwnedChildren(t *testing.T) {
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

	ns := "child-resources"
	createNamespace(t, cl, ns)
	dep := createDeployment(t, cl, ns, "demo-deploy")
	ownedRS := createReplicaSet(t, cl, ns, "owned-rs", dep)
	createReplicaSet(t, cl, ns, "unowned-rs", nil)
	createPod(t, cl, ns, "owned-pod", ownedRS)

	deps := Deps{
		Cl:        cl,
		Ctx:       ctx,
		AppConfig: appconfig.Default(),
	}
	path := []string{"namespaces", ns, "deployments", dep.Name}
	parentGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	childGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	folder := NewChildResourceGroupsFolder(deps, parentGVR, ns, dep.Name, []schema.GroupVersionResource{childGVR}, path)

	rows, err := folder.populate(ctx)
	if err != nil {
		t.Fatalf("populate: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	item, ok := rows[0].(*ResourceGroupItem)
	if !ok {
		t.Fatalf("row type = %T, want *ResourceGroupItem", rows[0])
	}
	if got := item.Path(); !reflect.DeepEqual(got, append(path, childGVR.Resource)) {
		t.Fatalf("Path() = %v, want %v", got, append(path, childGVR.Resource))
	}
	waitForResourceCount(t, item, 1)

	childFolder, err := item.Enter()
	if err != nil {
		t.Fatalf("enter: %v", err)
	}
	objFolder, ok := childFolder.(*ObjectsFolder)
	if !ok {
		t.Fatalf("child folder type = %T, want *ObjectsFolder", childFolder)
	}
	if got := objFolder.Path(); !reflect.DeepEqual(got, append(path, childGVR.Resource)) {
		t.Fatalf("child folder Path() = %v, want %v", got, append(path, childGVR.Resource))
	}
	// Ensure filtering only returns owned replicasets.
	objectRows, err := objFolder.rows.populate(ctx)
	if err != nil {
		t.Fatalf("populate objects: %v", err)
	}
	if len(objectRows) != 1 {
		t.Fatalf("object rows = %d, want 1", len(objectRows))
	}
}

func waitForResourceCount(t *testing.T, item *ResourceGroupItem, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if count, ok := item.TryCount(); ok && count == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	if count, ok := item.TryCount(); ok {
		t.Fatalf("TryCount() = (%d, %v), want (%d, true)", count, ok, want)
	}
	t.Fatalf("TryCount() not ready, want %d", want)
}

func createDeployment(t *testing.T, cl *kccluster.Cluster, namespace, name string) *appsv1.Deployment {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "c", Image: "busybox", Command: []string{"sleep", "3600"}},
					},
				},
			},
		},
	}
	if err := cl.GetClient().Create(ctx, dep); err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	out := &appsv1.Deployment{}
	if err := waitForObject(ctx, cl.GetClient(), types.NamespacedName{Namespace: namespace, Name: name}, out); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	t.Cleanup(func() {
		delCtx, cancelDel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelDel()
		_ = cl.GetClient().Delete(delCtx, out)
	})
	return out
}

func createReplicaSet(t *testing.T, cl *kccluster.Cluster, namespace, name string, owner *appsv1.Deployment) *appsv1.ReplicaSet {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.ReplicaSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "c", Image: "busybox", Command: []string{"sleep", "3600"}},
					},
				},
			},
		},
	}
	if owner != nil {
		controller := true
		block := true
		rs.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         "apps/v1",
				Kind:               "Deployment",
				Name:               owner.Name,
				UID:                owner.UID,
				Controller:         &controller,
				BlockOwnerDeletion: &block,
			},
		}
	}
	if err := cl.GetClient().Create(ctx, rs); err != nil {
		t.Fatalf("create replicaset: %v", err)
	}
	out := &appsv1.ReplicaSet{}
	if err := waitForObject(ctx, cl.GetClient(), types.NamespacedName{Namespace: namespace, Name: name}, out); err != nil {
		t.Fatalf("get replicaset: %v", err)
	}
	t.Cleanup(func() {
		delCtx, cancelDel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelDel()
		_ = cl.GetClient().Delete(delCtx, out)
	})
	return out
}

func createPod(t *testing.T, cl *kccluster.Cluster, namespace, name string, owner *appsv1.ReplicaSet) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "c", Image: "busybox", Command: []string{"sleep", "3600"}},
			},
		},
	}
	if owner != nil {
		controller := true
		block := true
		pod.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         "apps/v1",
				Kind:               "ReplicaSet",
				Name:               owner.Name,
				UID:                owner.UID,
				Controller:         &controller,
				BlockOwnerDeletion: &block,
			},
		}
	}
	if err := cl.GetClient().Create(ctx, pod); err != nil {
		t.Fatalf("create pod: %v", err)
	}
	t.Cleanup(func() {
		delCtx, cancelDel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelDel()
		_ = cl.GetClient().Delete(delCtx, pod)
	})
}

func waitForObject(ctx context.Context, cl client.Client, key types.NamespacedName, obj client.Object) error {
	return wait.PollImmediate(50*time.Millisecond, 5*time.Second, func() (bool, error) {
		err := cl.Get(ctx, key, obj)
		switch {
		case err == nil:
			return true, nil
		case apierrors.IsNotFound(err):
			return false, nil
		default:
			return false, err
		}
	})
}
