package navigation

import (
    "testing"
    table "github.com/sttts/kc/internal/table"
)

// helper to make a trivial folder with a title and one name column
func mkFolder(title, key string, names ...string) Folder {
    rows := make([]table.Row, 0, len(names))
    for _, n := range names { rows = append(rows, NewSimpleItem(n, []string{n}, WhiteStyle())) }
    return NewSliceFolder(title, key, []table.Column{{Title: " Name"}}, rows)
}

func TestNavigator_BackFromRootNamespacesGoesToRoot(t *testing.T) {
    root := mkFolder("/", "root", "contexts", "namespaces")
    ns := mkFolder("namespaces", "root/namespaces", "default", "kube-system")

    nav := NewNavigator(root)
    if nav.HasBack() { t.Fatalf("unexpected back available at root") }
    nav.Push(ns)
    if !nav.HasBack() { t.Fatalf("expected back after entering namespaces") }
    cur := nav.Back()
    if cur == nil || cur.Title() != "/" { t.Fatalf("expected to be back at root, got %v", cur) }
}

func TestNavigator_BackFromContextNamespacesGoesToContexts(t *testing.T) {
    root := mkFolder("/", "root", "contexts")
    contexts := mkFolder("contexts", "root/contexts", "ctxA", "ctxB")
    ctxNamespaces := mkFolder("namespaces", "contexts/ctxA/namespaces", "default")

    nav := NewNavigator(root)
    nav.Push(contexts)
    if !nav.HasBack() { t.Fatalf("expected back after entering contexts") }
    nav.Push(ctxNamespaces)
    if !nav.HasBack() { t.Fatalf("expected back after entering ctx namespaces") }
    if cur := nav.Back(); cur == nil || cur.Title() != "contexts" {
        t.Fatalf("expected to be back at contexts, got %v", cur)
    }
}

