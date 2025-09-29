package ui

import (
    "testing"
    "context"
    nav "github.com/sttts/kc/internal/navigation"
    kccluster "github.com/sttts/kc/internal/cluster"
)

func TestFooterShowsGroupVersionForPods(t *testing.T) {
    if testCfg == nil { t.Skip("envtest not available") }
    cl, err := kccluster.New(testCfg)
    if err != nil { t.Fatalf("cluster: %v", err) }
    ctx := context.TODO()
    go cl.Start(ctx)
    deps := nav.Deps{Cl: cl, Ctx: ctx, CtxName: "env"}
    folder := nav.NewNamespacedGroupsFolder(deps, "default")
    // Build panel and attach folder
    p := NewPanel("")
    p.UseFolder(true)
    p.SetFolder(folder, false)
    _ = p.ViewContentOnlyFocused(false) // sync
    // Scan folder rows to find /pods index
    rows := folder.Lines(0, folder.Len())
    idx := -1
    for i := range rows {
        _, cells, _, _ := rows[i].Columns()
        if len(cells) > 0 && cells[0] == "/pods" { idx = i; break }
    }
    if idx < 0 { t.Skip("/pods not present in groups (env may not expose pods)") }
    if idx >= len(p.items) { t.Fatalf("panel items not synced; idx=%d items=%d", idx, len(p.items)) }
    got := p.items[idx].GetFooterInfo()
    if got != "pods (v1)" {
        t.Fatalf("footer = %q, want 'pods (v1)'", got)
    }
}
