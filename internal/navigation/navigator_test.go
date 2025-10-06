package navigation

import (
	"strings"
	"testing"

	"github.com/sttts/kc/internal/navigation/models"
	table "github.com/sttts/kc/internal/table"
)

// helper to make a trivial folder with a path and one name column
func mkFolder(path []string, key string, names ...string) models.Folder {
	rows := make([]table.Row, 0, len(names))
	base := append([]string(nil), path...)
	for _, n := range names {
		rows = append(rows, models.NewSimpleItem(n, []string{n}, append(append([]string(nil), base...), n), models.WhiteStyle()))
	}
	title := "/"
	if len(path) > 0 {
		title = strings.Join(path, "/")
	}
	return NewSliceFolder(title, key, []table.Column{{Title: " Name"}}, rows)
}

func TestNavigator_BackFromRootNamespacesGoesToRoot(t *testing.T) {
	root := mkFolder(nil, "root", "contexts", "namespaces")
	ns := mkFolder([]string{"namespaces"}, "root/namespaces", "default", "kube-system")

	nav := NewNavigator(root)
	if nav.HasBack() {
		t.Fatalf("unexpected back available at root")
	}
	nav.Push(ns)
	if !nav.HasBack() {
		t.Fatalf("expected back after entering namespaces")
	}
	cur := nav.Back()
	if cur == nil || len(cur.Path()) != 0 {
		t.Fatalf("expected root path empty, got %v", cur.Path())
	}
}

func TestNavigator_BackFromContextNamespacesGoesToContexts(t *testing.T) {
	root := mkFolder(nil, "root", "contexts")
	contexts := mkFolder([]string{"contexts"}, "root/contexts", "ctxA", "ctxB")
	ctxNamespaces := mkFolder([]string{"contexts", "ctxA", "namespaces"}, "contexts/ctxA/namespaces", "default")

	nav := NewNavigator(root)
	nav.Push(contexts)
	if !nav.HasBack() {
		t.Fatalf("expected back after entering contexts")
	}
	nav.Push(ctxNamespaces)
	if !nav.HasBack() {
		t.Fatalf("expected back after entering ctx namespaces")
	}
	if cur := nav.Back(); cur == nil || !equalPath(cur.Path(), []string{"contexts"}) {
		t.Fatalf("expected to be back at contexts, got %v", cur.Path())
	}
}

func equalPath(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
