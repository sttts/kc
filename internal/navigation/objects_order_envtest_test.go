package navigation

import (
	"context"
	"strings"
	"testing"
	"time"

	kccluster "github.com/sttts/kc/internal/cluster"
	"github.com/sttts/kc/internal/models"
	table "github.com/sttts/kc/internal/table"
	kctesting "github.com/sttts/kc/internal/testing"
	"github.com/sttts/kc/pkg/appconfig"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestClusterObjectsOrderAndAgeEnvtest(t *testing.T) {
	// Start envtest
	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil || cfg == nil {
		t.Fatalf("start envtest: %v", err)
	}
	defer func() { _ = testEnv.Stop() }()

	// Seed three namespaces with slight delays to vary creation times
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	cli, err := crclient.New(cfg, crclient.Options{Scheme: scheme})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ctx := context.TODO()
	for _, n := range []string{"a", "b", "c"} {
		if err := cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: n}}); err != nil {
			t.Fatalf("create ns %s: %v", n, err)
		}
		time.Sleep(1100 * time.Millisecond) // kube timestamps are second precision; ensure unique creation times
	}

	// kc cluster
	cl, err := kccluster.New(cfg)
	if err != nil {
		t.Fatalf("kccluster: %v", err)
	}
	go cl.Start(ctx)

	// Deps with objects order toggles
	makeDeps := func(order string) models.Deps {
		cfg := appconfig.Default()
		cfg.Resources.ShowNonEmptyOnly = false
		cfg.Resources.Columns = "normal"
		cfg.Resources.Order = appconfig.OrderAlpha
		cfg.Objects.Order = order
		cfg.Objects.Columns = "normal"
		return models.Deps{Cl: cl, Ctx: ctx, CtxName: "envtest", Config: cfg}
	}
	gvrNS := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}

	// Order by name ascending
	f1 := models.NewClusterObjectsFolder(makeDeps("name"), gvrNS, []string{"namespaces"})
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return f1.Len() >= 3 })
	rows := f1.Lines(0, 3)
	got := normFirstCells(rows)
	// Assert relative order of our namespaces regardless of other system entries
	idxA, idxB, idxC := indexOf(got, "a"), indexOf(got, "b"), indexOf(got, "c")
	if idxA < 0 || idxB < 0 || idxC < 0 || !(idxA < idxB && idxB < idxC) {
		t.Fatalf("name asc order unexpected (a<b<c): %+v (idx a=%d b=%d c=%d)", got, idxA, idxB, idxC)
	}
	// Verify Age column exists
	cols := f1.Columns()
	if len(cols) == 0 || cols[len(cols)-1].Title != "Age" {
		t.Fatalf("missing Age column: %+v", cols)
	}
	// Age cells non-empty
	ageIdx := len(cols) - 1
	for _, r := range rows {
		_, cells, _, _ := r.Columns()
		if len(cells) <= ageIdx || strings.TrimSpace(cells[ageIdx]) == "" {
			t.Fatalf("empty Age cell: %+v", cells)
		}
	}

	// Order by -name
	f2 := models.NewClusterObjectsFolder(makeDeps("-name"), gvrNS, []string{"namespaces"})
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return f2.Len() >= 3 })
	rows = f2.Lines(0, f2.Len())
	got = normFirstCells(rows)
	idxA, idxB, idxC = indexOf(got, "a"), indexOf(got, "b"), indexOf(got, "c")
	if idxA < 0 || idxB < 0 || idxC < 0 || !(idxC < idxB && idxB < idxA) {
		t.Fatalf("name desc order unexpected (c>b>a): %+v (idx a=%d b=%d c=%d)", got, idxA, idxB, idxC)
	}

	// Order by creation
	f3 := models.NewClusterObjectsFolder(makeDeps("creation"), gvrNS, []string{"namespaces"})
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return f3.Len() >= 3 })
	rows = f3.Lines(0, f3.Len())
	got = normFirstCells(rows)
	idxA, idxB, idxC = indexOf(got, "a"), indexOf(got, "b"), indexOf(got, "c")
	if idxA < 0 || idxB < 0 || idxC < 0 || !(idxA < idxB && idxB < idxC) {
		t.Fatalf("creation asc order unexpected (a<b<c): %+v (idx a=%d b=%d c=%d)", got, idxA, idxB, idxC)
	}

	// Order by -creation
	f4 := models.NewClusterObjectsFolder(makeDeps("-creation"), gvrNS, []string{"namespaces"})
	kctesting.Eventually(t, 5*time.Second, 50*time.Millisecond, func() bool { return f4.Len() >= 3 })
	rows = f4.Lines(0, f4.Len())
	got = normFirstCells(rows)
	idxA, idxB, idxC = indexOf(got, "a"), indexOf(got, "b"), indexOf(got, "c")
	if idxA < 0 || idxB < 0 || idxC < 0 || !(idxC < idxB && idxB < idxA) {
		t.Fatalf("creation desc order unexpected (c>b>a): %+v (idx a=%d b=%d c=%d)", got, idxA, idxB, idxC)
	}
}

func normFirstCells(rows []table.Row) []string {
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

func indexOf(list []string, want string) int {
	for i, v := range list {
		if v == want {
			return i
		}
	}
	return -1
}
