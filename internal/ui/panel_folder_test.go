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
    p.SetFolderNavHandler(func(back bool, _ string, next nav.Folder) {
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

func TestEnterFromNamespacesIntoGroups(t *testing.T) {
    p := NewPanel("")
    // namespaces folder with one namespace row that enters a groups folder
    groups := mkTestFolder("groups", "pods", "configmaps")
    nsRow := nav.NewEnterableItem("default", []string{"default"}, func() (nav.Folder, error) { return groups, nil }, nav.WhiteStyle())
    nsFolder := nav.NewSliceFolder("namespaces", "namespaces", []table.Column{{Title: " Name"}}, []table.Row{nsRow})
    p.SetFolder(nsFolder, true)
    p.UseFolder(true)
    var gotNext nav.Folder
    p.SetFolderNavHandler(func(back bool, _ string, next nav.Folder) { if !back { gotNext = next } })
    // Select the first real row (index 1 due to back row at 0)
    p.syncFromFolder()
    p.selected = 1
    _ = p.enterItem()
    if gotNext == nil || gotNext.Title() != "groups" {
        t.Fatalf("expected to navigate to groups folder, got %v", gotNext)
    }
}

func TestSelectionRestoredOnBack(t *testing.T) {
    p := NewPanel("")
    // Build a root folder with enterable namespaces row
    groups := mkTestFolder("groups", "pods")
    rows := []table.Row{
        nav.NewSimpleItem("contexts", []string{"contexts"}, nav.WhiteStyle()),
        nav.NewEnterableItem("namespaces", []string{"namespaces"}, func() (nav.Folder, error) { return groups, nil }, nav.WhiteStyle()),
    }
    root := nav.NewSliceFolder("/", "root", []table.Column{{Title: " Name"}}, rows)

    // Wire navigator-like handler
    navg := nav.NewNavigator(root)
    p.SetFolder(root, false)
    p.UseFolder(true)
    p.SetFolderNavHandler(func(back bool, selID string, next nav.Folder) {
        if back {
            navg.Back()
        } else if next != nil {
            navg.SetSelectionID(selID)
            navg.Push(next)
        }
        cur := navg.Current()
        hasBack := navg.HasBack()
        p.SetFolder(cur, hasBack)
        if back {
            id := navg.CurrentSelectionID()
            if id != "" { p.SelectByRowID(id) } else { p.ResetSelectionTop() }
        } else {
            p.ResetSelectionTop()
        }
    })

    // Select namespaces in root and enter
    p.syncFromFolder()
    idxNamespaces := -1
    for i := range p.items { if p.items[i].Name == "namespaces" { idxNamespaces = i; break } }
    if idxNamespaces < 0 { t.Fatalf("namespaces row not found in root items: %+v", p.items) }
    p.selected = idxNamespaces
    _ = p.enterItem() // into groups
    if navg.Current().Title() != "groups" { t.Fatalf("expected to be in groups, got %s", navg.Current().Title()) }

    // Now back; selection should restore to namespaces in root
    p.selected = 0 // ensure on back row
    _ = p.enterItem() // back
    if navg.Current().Title() != "/" { t.Fatalf("expected to be back at root, got %s", navg.Current().Title()) }
    // find namespaces index again
    p.syncFromFolder()
    want := -1
    for i := range p.items { if p.items[i].Name == "namespaces" { want = i; break } }
    if want < 0 { t.Fatalf("namespaces row not found after back: %+v", p.items) }
    if p.selected != want { t.Fatalf("expected selection restored to %d, got %d", want, p.selected) }
}

func TestSelectionRestoreWithinContexts(t *testing.T) {
    p := NewPanel("")
    // Build folders: root -> contexts -> ctxA namespaces
    ctxANamespaces := mkTestFolder("namespaces", "default", "kube-system")
    // contexts folder: ctxA enterable to its namespaces, plus another context
    ctxsRows := []table.Row{
        nav.NewEnterableItem("ctxA", []string{"ctxA"}, func() (nav.Folder, error) { return ctxANamespaces, nil }, nav.WhiteStyle()),
        nav.NewSimpleItem("ctxB", []string{"ctxB"}, nav.WhiteStyle()),
    }
    contexts := nav.NewSliceFolder("contexts", "contexts", []table.Column{{Title: " Name"}}, ctxsRows)

    // root folder with enterable contexts
    root := nav.NewSliceFolder("/", "root", []table.Column{{Title: " Name"}}, []table.Row{
        nav.NewEnterableItem("contexts", []string{"contexts"}, func() (nav.Folder, error) { return contexts, nil }, nav.WhiteStyle()),
    })

    // Wire navigator-like handler
    navg := nav.NewNavigator(root)
    p.SetFolder(root, false)
    p.UseFolder(true)
    p.SetFolderNavHandler(func(back bool, selID string, next nav.Folder) {
        if back {
            navg.Back()
        } else if next != nil {
            navg.SetSelectionID(selID)
            navg.Push(next)
        }
        cur := navg.Current()
        p.SetFolder(cur, navg.HasBack())
        if back {
            id := navg.CurrentSelectionID()
            if id != "" { p.SelectByRowID(id) } else { p.ResetSelectionTop() }
        } else {
            p.ResetSelectionTop()
        }
    })

    // Enter contexts from root
    p.syncFromFolder()
    // Select contexts row in root
    idxCtx := -1
    for i := range p.items { if p.items[i].Name == "contexts" { idxCtx = i; break } }
    if idxCtx < 0 { t.Fatalf("contexts row not found in root items") }
    p.selected = idxCtx
    _ = p.enterItem()
    if navg.Current().Title() != "contexts" { t.Fatalf("expected contexts folder, got %s", navg.Current().Title()) }

    // Select ctxA and enter
    p.syncFromFolder()
    idxCtxA := -1
    for i := range p.items { if p.items[i].Name == "ctxA" { idxCtxA = i; break } }
    if idxCtxA < 0 { t.Fatalf("ctxA row not found in contexts items: %+v", p.items) }
    p.selected = idxCtxA
    _ = p.enterItem()
    if navg.Current().Title() != "namespaces" { t.Fatalf("expected namespaces for ctxA, got %s", navg.Current().Title()) }

    // Go back to contexts; selection should restore to ctxA
    p.selected = 0 // back row
    _ = p.enterItem()
    if navg.Current().Title() != "contexts" { t.Fatalf("expected contexts after back, got %s", navg.Current().Title()) }
    p.syncFromFolder()
    idx := -1
    for i := range p.items { if p.items[i].Name == "ctxA" { idx = i; break } }
    if idx < 0 || p.selected != idx { t.Fatalf("expected selection restored to ctxA at %d, got %d", idx, p.selected) }

    // Back to root; selection should restore to contexts
    p.selected = 0
    _ = p.enterItem()
    if navg.Current().Title() != "/" { t.Fatalf("expected root after back, got %s", navg.Current().Title()) }
    p.syncFromFolder()
    want := -1
    for i := range p.items { if p.items[i].Name == "contexts" { want = i; break } }
    if want < 0 || p.selected != want { t.Fatalf("expected selection restored to contexts at %d, got %d", want, p.selected) }
}

func TestSelectionRestoreNamespacesToGroupsAndBack(t *testing.T) {
    p := NewPanel("")
    // Build folders: root(namespaces) -> groups
    groups := mkTestFolder("groups", "pods", "configmaps")
    nsRow := nav.NewEnterableItem("default", []string{"default"}, func() (nav.Folder, error) { return groups, nil }, nav.WhiteStyle())
    nsFolder := nav.NewSliceFolder("namespaces", "namespaces", []table.Column{{Title: " Name"}}, []table.Row{nsRow})
    root := nav.NewSliceFolder("/", "root", []table.Column{{Title: " Name"}}, []table.Row{nav.NewEnterableItem("namespaces", []string{"namespaces"}, func() (nav.Folder, error) { return nsFolder, nil }, nav.WhiteStyle())})

    navg := nav.NewNavigator(root)
    p.SetFolder(root, false)
    p.UseFolder(true)
    p.SetFolderNavHandler(func(back bool, selID string, next nav.Folder) {
        if back { navg.Back() } else if next != nil { navg.SetSelectionID(selID); navg.Push(next) }
        cur := navg.Current()
        p.SetFolder(cur, navg.HasBack())
        if back { id := navg.CurrentSelectionID(); if id != "" { p.SelectByRowID(id) } else { p.ResetSelectionTop() } } else { p.ResetSelectionTop() }
    })

    // Enter namespaces from root
    p.syncFromFolder()
    idxNS := -1
    for i := range p.items { if p.items[i].Name == "namespaces" { idxNS = i; break } }
    p.selected = idxNS
    _ = p.enterItem()
    if navg.Current().Title() != "namespaces" { t.Fatalf("expected namespaces, got %s", navg.Current().Title()) }

    // Select default and enter groups
    p.syncFromFolder()
    idxDef := -1
    for i := range p.items { if p.items[i].Name == "default" { idxDef = i; break } }
    p.selected = idxDef
    _ = p.enterItem()
    if navg.Current().Title() != "groups" { t.Fatalf("expected groups, got %s", navg.Current().Title()) }

    // Back to namespaces; selection should be "default"
    p.selected = 0
    _ = p.enterItem()
    if navg.Current().Title() != "namespaces" { t.Fatalf("expected namespaces after back, got %s", navg.Current().Title()) }
    p.syncFromFolder()
    idxDef2 := -1
    for i := range p.items { if p.items[i].Name == "default" { idxDef2 = i; break } }
    if idxDef2 < 0 || p.selected != idxDef2 { t.Fatalf("expected selection restored to default at %d, got %d", idxDef2, p.selected) }
}
