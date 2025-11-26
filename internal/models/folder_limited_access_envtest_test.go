package models

import (
	"context"
	"testing"
	"time"

	kccluster "github.com/sttts/kc/internal/cluster"
	table "github.com/sttts/kc/internal/table"
	kctesting "github.com/sttts/kc/internal/testing"
	"github.com/sttts/kc/pkg/appconfig"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func TestLimitedUserSeesAllowedResourcesOnly(t *testing.T) {
	if testCfg == nil {
		t.Skip("envtest not available")
	}

	ctx := t.Context()
	adminCl, err := kccluster.New(testCfg)
	if err != nil {
		t.Fatalf("admin cluster: %v", err)
	}
	go adminCl.Start(ctx)
	t.Cleanup(adminCl.Stop)

	const (
		ns   = "limited-access"
		user = "limited-user"
		pod  = "pod-visible"
		sec  = "secret-hidden"
	)

	createNamespace(t, adminCl, ns)
	createPod(t, adminCl, ns, pod)
	createSecret(t, adminCl, ns, sec)
	grantLimitedUser(t, adminCl, user)

	limitedCfg := rest.CopyConfig(testCfg)
	limitedCfg.Impersonate.UserName = user

	limitedCl, err := kccluster.New(limitedCfg)
	if err != nil {
		t.Fatalf("limited cluster: %v", err)
	}
	go limitedCl.Start(ctx)
	t.Cleanup(limitedCl.Stop)

	deps := Deps{
		Cl:        limitedCl,
		Ctx:       ctx,
		CtxName:   "envtest",
		AppConfig: appconfig.Default(),
	}

	root := NewRootFolder(deps, nil)
	waitFolder(t, root)
	assertRowPresent(t, root, "namespaces")

	nsResources := NewNamespacedResourcesFolder(deps, ns, []string{"namespaces", ns})
	waitFolder(t, nsResources)

	kctesting.Eventually(t, 5*time.Second, 100*time.Millisecond, func() bool {
		rows := rowsByID(t, nsResources, ctx)
		_, podsOK := rows[ns+"//v1/pods"]
		_, secretsOK := rows[ns+"//v1/secrets"]
		return podsOK && !secretsOK
	})
}

func rowsByID(t *testing.T, folder interface {
	Len(context.Context) int
	Lines(context.Context, int, int) []table.Row
}, ctx context.Context) map[string][]string {
	t.Helper()
	count := folder.Len(ctx)
	rows := folder.Lines(ctx, 0, count)
	out := make(map[string][]string, len(rows))
	for _, r := range rows {
		id, cells, _, ok := r.Columns()
		if !ok {
			continue
		}
		out[id] = cells
	}
	return out
}

func assertRowPresent(t *testing.T, folder interface {
	Len(context.Context) int
	Lines(context.Context, int, int) []table.Row
}, id string) {
	t.Helper()
	ctx := t.Context()
	kctesting.Eventually(t, 5*time.Second, 100*time.Millisecond, func() bool {
		rows := rowsByID(t, folder, ctx)
		_, ok := rows[id]
		return ok
	})
}

func createPod(t *testing.T, cl *kccluster.Cluster, namespace, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "pause",
				Image: "registry.k8s.io/pause:3.9",
			}},
		},
	}
	if err := cl.GetClient().Create(ctx, p); err != nil {
		t.Fatalf("create pod %s/%s: %v", namespace, name, err)
	}
	t.Cleanup(func() {
		delCtx, cancelDel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelDel()
		_ = cl.GetClient().Delete(delCtx, p)
	})
}

func createSecret(t *testing.T, cl *kccluster.Cluster, namespace, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		StringData: map[string]string{"k": "v"},
	}
	if err := cl.GetClient().Create(ctx, s); err != nil {
		t.Fatalf("create secret %s/%s: %v", namespace, name, err)
	}
	t.Cleanup(func() {
		delCtx, cancelDel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelDel()
		_ = cl.GetClient().Delete(delCtx, s)
	})
}

func grantLimitedUser(t *testing.T, cl *kccluster.Cluster, user string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "limited-reader",
		},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"namespaces"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}},
		},
	}
	if err := cl.GetClient().Create(ctx, role); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create clusterrole: %v", err)
	}
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "limited-reader",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     role.Name,
		},
		Subjects: []rbacv1.Subject{
			{Kind: rbacv1.UserKind, Name: user},
		},
	}
	if err := cl.GetClient().Create(ctx, binding); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create clusterrolebinding: %v", err)
	}
}
