package ui

import (
    "testing"
    nav "github.com/sttts/kc/internal/navigation"
    table "github.com/sttts/kc/internal/table"
)

// make a simple folder with provided row names
func mkTestFolder(title string, names ...string) nav.Folder {
    rows := make([]table.Row, 0, len(names))
    for _, n := range names { rows = append(rows, nav.NewSimpleItem(n, []string{n}, nav.WhiteStyle())) }
    return nav.NewSliceFolder(title, title, []table.Column{{Title: " Name"}}, rows)
}

func TestEnterBackFromNamespacesFolder(t *testing.T) {
    p := NewPanel("")
    // seed with a namespaces folder and back enabled
    f := mkTestFolder("namespaces", "default", "kube-system")
    p.SetFolder(f, true)
    p.UseFolder(true)
    backCalls := 0
    p.SetFolderNavHandler(func(back bool, next nav.Folder) {
        if back { backCalls++ }
    })
    // sync and select the back item
    p.syncFromFolder()
    if len(p.items) == 0 || p.items[0].Name != ".." {
        t.Fatalf("expected back item at top, got %+v", p.items)
    }
    p.selected = 0
    // act
    _ = p.enterItem()
    if backCalls != 1 {
        t.Fatalf("expected one back call, got %d", backCalls)
    }
}

