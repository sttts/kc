package navigation

import (
	"context"
	"strings"
	"testing"
	"time"

	kccluster "github.com/sttts/kc/internal/cluster"
	"github.com/sttts/kc/internal/navigation/models"
	table "github.com/sttts/kc/internal/table"
	kctesting "github.com/sttts/kc/internal/testing"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestNamespacedObjectsOrderAndAgeEnvtest(t *testing.T) {
	// Start envtest
	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil || cfg == nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer func() { _ = testEnv.Stop() }()

	// Seed namespace and three configmaps with staggered creation times
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	cli, err := crclient.New(cfg, crclient.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ctx := context.TODO()
	if err := cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-objtest"}}); err != nil {
		t.Fatalf("create ns: %v", err)
	}
	for _, n := range []string{"a", "b", "c"} {
		if err := cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm-" + n, Namespace: "ns-objtest"}}); err != nil {
			t.Fatalf("create cm %s: %v", n, err)
		}
		time.Sleep(1100 * time.Millisecond) // kube timestamps are second precision; ensure unique creation times
	}

	// kc cluster
	cl, err := kccluster.New(cfg)
	if err != nil {
		t.Fatalf("kccluster: %v", err)
	}
	go cl.Start(ctx)

	// Deps with objects order
	makeDeps := func(order string) models.Deps {
		return models.Deps{Cl: cl, Ctx: ctx, CtxName: "envtest", ViewOptions: func() models.ViewOptions { return models.ViewOptions{Columns: "normal", ObjectsOrder: order} }}
	}
	gvrCM := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}

	// Asc by name
	f1 := NewNamespacedObjectsFolder(makeDeps("name"), gvrCM, "ns-objtest", []string{"namespaces", "ns-objtest", "configmaps"})
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return f1.Len() >= 3 })
	rows := f1.Lines(0, 3)
	got := []string{}
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) > 0 {
			v := cells[0]
			if len(v) > 0 && v[0] == '/' {
				v = v[1:]
			}
			got = append(got, v)
		}
	}
	if !(got[0] == "cm-a" && got[1] == "cm-b") {
		t.Fatalf("name asc unexpected: %+v", got)
	}
	// Age column present and non-empty
	cols := f1.Columns()
	if len(cols) == 0 || cols[len(cols)-1].Title != "Age" {
		t.Fatalf("missing Age col: %+v", cols)
	}
	ageIdx := len(cols) - 1
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) <= ageIdx || strings.TrimSpace(cells[ageIdx]) == "" {
			t.Fatalf("empty Age cell: %+v", cells)
		}
	}

	// Desc by name
	f2 := NewNamespacedObjectsFolder(makeDeps("-name"), gvrCM, "ns-objtest", []string{"namespaces", "ns-objtest", "configmaps"})
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return f2.Len() >= 3 })
	rows = f2.Lines(0, f2.Len())
	got = normFirstCellsNS(rows)
	idxA, idxB, idxC := indexOfNS(got, "cm-a"), indexOfNS(got, "cm-b"), indexOfNS(got, "cm-c")
	if idxA < 0 || idxB < 0 || idxC < 0 || !(idxC < idxB && idxB < idxA) {
		t.Fatalf("name desc unexpected (cm-c>cm-b>cm-a): %+v (idx a=%d b=%d c=%d)", got, idxA, idxB, idxC)
	}

	// Asc by creation
	f3 := NewNamespacedObjectsFolder(makeDeps("creation"), gvrCM, "ns-objtest", []string{"namespaces", "ns-objtest", "configmaps"})
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return f3.Len() >= 3 })
	rows = f3.Lines(0, f3.Len())
	got = normFirstCellsNS(rows)
	idxA, idxB, idxC = indexOfNS(got, "cm-a"), indexOfNS(got, "cm-b"), indexOfNS(got, "cm-c")
	if idxA < 0 || idxB < 0 || idxC < 0 || !(idxA < idxB && idxB < idxC) {
		t.Fatalf("creation asc unexpected (cm-a<cm-b<cm-c): %+v (idx a=%d b=%d c=%d)", got, idxA, idxB, idxC)
	}

	// Desc by creation
	f4 := NewNamespacedObjectsFolder(makeDeps("-creation"), gvrCM, "ns-objtest", []string{"namespaces", "ns-objtest", "configmaps"})
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return f4.Len() >= 3 })
	rows = f4.Lines(0, f4.Len())
	got = normFirstCellsNS(rows)
	idxA, idxB, idxC = indexOfNS(got, "cm-a"), indexOfNS(got, "cm-b"), indexOfNS(got, "cm-c")
	if idxA < 0 || idxB < 0 || idxC < 0 || !(idxC < idxB && idxB < idxA) {
		t.Fatalf("name desc unexpected (cm-c>cm-b>cm-a): %+v (idx a=%d b=%d c=%d)", got, idxA, idxB, idxC)
	}

}

func normFirstCellsNS(rows []table.Row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) > 0 {
			v := cells[0]
			if strings.HasPrefix(v, "/") {
				v = v[1:]
			}
			out = append(out, v)
		}
	}
	return out
}

func indexOfNS(list []string, want string) int {
	for i, v := range list {
		if v == want {
			return i
		}
	}
	return -1
}
