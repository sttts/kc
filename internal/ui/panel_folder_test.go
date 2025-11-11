package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/sttts/kc/internal/models"
	modeltesting "github.com/sttts/kc/internal/models/testing"
	nav "github.com/sttts/kc/internal/navigation"
	table "github.com/sttts/kc/internal/table"
)

// make a simple folder with provided row names
func mkTestFolder(path []string, names ...string) models.Folder {
	rows := make([]table.Row, 0, len(names))
	base := append([]string(nil), path...)
	for _, n := range names {
		rows = append(rows, models.NewSimpleItem(n, []string{n}, append(append([]string(nil), base...), n), models.WhiteStyle()))
	}
	title := "/"
	if len(path) > 0 {
		title = strings.Join(path, "/")
	}
	return modeltesting.NewStaticFolder(title, []table.Column{{Title: " Name"}}, rows)
}

func folderPathString(f models.Folder) string {
	if f == nil {
		return ""
	}
	path := f.Path()
	if len(path) == 0 {
		return "/"
	}
	return strings.Join(path, "/")
}

func panelItemNames(ctx context.Context, p *Panel) []string {
	items := p.Items(ctx)
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return names
}

func selectByName(ctx context.Context, t *testing.T, p *Panel, name string) {
	t.Helper()
	items := p.Items(ctx)
	for _, item := range items {
		if item.Name == name {
			if id, _, _, ok := item.Columns(); ok {
				p.SelectByRowID(ctx, id)
				return
			}
		}
	}
	t.Fatalf("row %q not found in items %v", name, panelItemNames(ctx, p))
}

func currentItemName(ctx context.Context, p *Panel) string {
	if item := p.GetCurrentItem(); item != nil {
		return item.Name
	}
	return ""
}

func enterByName(ctx context.Context, t *testing.T, p *Panel, name string) {
	t.Helper()
	selectByName(ctx, t, p, name)
	_ = p.Enter(ctx)
}

func enterBack(ctx context.Context, p *Panel) {
	p.ResetSelectionTop(ctx)
	_ = p.Enter(ctx)
}

func setupPanelFolder(ctx context.Context, p *Panel, folder models.Folder, hasBack bool) {
	p.UseFolder(ctx, true)
	p.SetFolder(ctx, folder, hasBack)
	p.RefreshFolder(ctx)
}

func TestEnterBackFromNamespacesFolder(t *testing.T) {
	p := NewPanel("")
	// seed with a namespaces folder and back enabled
	f := mkTestFolder([]string{"namespaces"}, "default", "kube-system")
	ctx := t.Context()
	setupPanelFolder(ctx, p, f, true)
	backCalls := 0
	p.SetFolderNavHandler(func(back bool, _ string, next models.Folder) {
		if back {
			backCalls++
		}
	})
	items := p.Items(ctx)
	if len(items) == 0 || items[0].Name != ".." {
		t.Fatalf("expected back item at top, got %+v", items)
	}
	p.ResetSelectionTop(ctx)
	// act
	_ = p.Enter(ctx)
	if backCalls != 1 {
		t.Fatalf("expected one back call, got %d", backCalls)
	}
}

func TestEnterFromNamespacesIntoGroups(t *testing.T) {
	p := NewPanel("")
	// namespaces folder with one namespace row that enters a groups folder
	groups := mkTestFolder([]string{"groups"}, "pods", "configmaps")
	nsRow := modeltesting.NewEnterableItem("default", []string{"default"}, []string{"namespaces", "default"}, func() (models.Folder, error) { return groups, nil }, models.WhiteStyle())
	nsFolder := modeltesting.NewStaticFolder("namespaces", []table.Column{{Title: " Name"}}, []table.Row{nsRow})
	ctx := t.Context()
	setupPanelFolder(ctx, p, nsFolder, true)
	var gotNext models.Folder
	p.SetFolderNavHandler(func(back bool, _ string, next models.Folder) {
		if !back {
			gotNext = next
		}
	})
	items := p.Items(ctx)
	if len(items) < 2 {
		t.Fatalf("expected namespace row present, got %+v", items)
	}
	if id, _, _, ok := items[1].Columns(); ok {
		p.SelectByRowID(ctx, id)
	}
	_ = p.Enter(ctx)
	if gotNext == nil || strings.Join(gotNext.Path(), "/") != "groups" {
		t.Fatalf("expected to navigate to groups folder, got %v", gotNext.Path())
	}
}

func TestSelectionRestoredOnBack(t *testing.T) {
	p := NewPanel("")
	// Build a root folder with enterable namespaces row
	groups := mkTestFolder([]string{"groups"}, "pods")
	rows := []table.Row{
		models.NewSimpleItem("contexts", []string{"contexts"}, []string{"contexts"}, models.WhiteStyle()),
		modeltesting.NewEnterableItem("namespaces", []string{"namespaces"}, []string{"namespaces"}, func() (models.Folder, error) { return groups, nil }, models.WhiteStyle()),
	}
	root := modeltesting.NewStaticFolder("/", []table.Column{{Title: " Name"}}, rows)

	// Wire navigator-like handler
	navg := nav.NewNavigator(root)
	ctx := t.Context()
	setupPanelFolder(ctx, p, root, false)
	p.SetFolderNavHandler(func(back bool, selID string, next models.Folder) {
		if back {
			navg.Back()
		} else if next != nil {
			navg.SetSelectionID(selID)
			navg.Push(next)
		}
		cur := navg.Current()
		hasBack := navg.HasBack()
		p.SetFolder(ctx, cur, hasBack)
		p.RefreshFolder(ctx)
		if back {
			id := navg.CurrentSelectionID()
			if id != "" {
				p.SelectByRowID(ctx, id)
			} else {
				p.ResetSelectionTop(ctx)
			}
		} else {
			p.ResetSelectionTop(ctx)
		}
	})

	// Select namespaces in root and enter
	selectByName(ctx, t, p, "namespaces")
	_ = p.Enter(ctx) // into groups
	if got := folderPathString(navg.Current()); got != "groups" {
		t.Fatalf("expected to be in groups, got %s", got)
	}

	// Now back; selection should restore to namespaces in root
	p.ResetSelectionTop(ctx)
	_ = p.Enter(ctx) // back
	if got := folderPathString(navg.Current()); got != "/" {
		t.Fatalf("expected to be back at root, got %s", got)
	}
	selectByName(ctx, t, p, "namespaces")
	if id := navg.CurrentSelectionID(); id != "" && id != p.currentSelectionID(ctx) {
		t.Fatalf("expected selection restored to %s, got %s", id, p.currentSelectionID(ctx))
	}
}

func TestSelectionRestoreWithinContexts(t *testing.T) {
	p := NewPanel("")
	// Build folders: root -> contexts -> ctxA namespaces
	ctxANamespaces := mkTestFolder([]string{"namespaces"}, "default", "kube-system")
	// contexts folder: ctxA enterable to its namespaces, plus another context
	ctxsRows := []table.Row{
		modeltesting.NewEnterableItem("ctxA", []string{"ctxA"}, []string{"contexts", "ctxA"}, func() (models.Folder, error) { return ctxANamespaces, nil }, models.WhiteStyle()),
		models.NewSimpleItem("ctxB", []string{"ctxB"}, []string{"contexts", "ctxB"}, models.WhiteStyle()),
	}
	contexts := modeltesting.NewStaticFolder("contexts", []table.Column{{Title: " Name"}}, ctxsRows)

	// root folder with enterable contexts
	root := modeltesting.NewStaticFolder("/", []table.Column{{Title: " Name"}}, []table.Row{
		modeltesting.NewEnterableItem("contexts", []string{"contexts"}, []string{"contexts"}, func() (models.Folder, error) { return contexts, nil }, models.WhiteStyle()),
	})

	// Wire navigator-like handler
	navg := nav.NewNavigator(root)
	ctx := t.Context()
	p.SetFolder(ctx, root, false)
	p.UseFolder(ctx, true)
	p.SetFolderNavHandler(func(back bool, selID string, next models.Folder) {
		if back {
			navg.Back()
		} else if next != nil {
			navg.SetSelectionID(selID)
			navg.Push(next)
		}
		cur := navg.Current()
		p.SetFolder(ctx, cur, navg.HasBack())
		if back {
			id := navg.CurrentSelectionID()
			if id != "" {
				p.SelectByRowID(ctx, id)
			} else {
				p.ResetSelectionTop(ctx)
			}
		} else {
			p.ResetSelectionTop(ctx)
		}
	})

	p.RefreshFolder(ctx)
	enterByName(ctx, t, p, "contexts")
	if got := folderPathString(navg.Current()); got != "contexts" {
		t.Fatalf("expected contexts folder, got %s", got)
	}

	p.RefreshFolder(ctx)
	enterByName(ctx, t, p, "ctxA")
	if got := folderPathString(navg.Current()); got != "namespaces" {
		t.Fatalf("expected namespaces for ctxA, got %s", got)
	}

	enterBack(ctx, p)
	if got := folderPathString(navg.Current()); got != "contexts" {
		t.Fatalf("expected contexts after back, got %s", got)
	}
	if name := currentItemName(ctx, p); name != "ctxA" {
		t.Fatalf("expected ctxA selected after back, got %s", name)
	}

	enterBack(ctx, p)
	if got := folderPathString(navg.Current()); got != "/" {
		t.Fatalf("expected root after back, got %s", got)
	}
	if name := currentItemName(ctx, p); name != "contexts" {
		t.Fatalf("expected contexts selected at root, got %s", name)
	}
}

func TestSelectionRestoreNamespacesToGroupsAndBack(t *testing.T) {
	p := NewPanel("")
	// Build folders: root(namespaces) -> groups
	groups := mkTestFolder([]string{"groups"}, "pods", "configmaps")
	nsRow := modeltesting.NewEnterableItem("default", []string{"default"}, []string{"namespaces", "default"}, func() (models.Folder, error) { return groups, nil }, models.WhiteStyle())
	nsFolder := modeltesting.NewStaticFolder("namespaces", []table.Column{{Title: " Name"}}, []table.Row{nsRow})
	root := modeltesting.NewStaticFolder("/", []table.Column{{Title: " Name"}}, []table.Row{modeltesting.NewEnterableItem("namespaces", []string{"namespaces"}, []string{"namespaces"}, func() (models.Folder, error) { return nsFolder, nil }, models.WhiteStyle())})

	navg := nav.NewNavigator(root)
	ctx := t.Context()
	p.SetFolder(ctx, root, false)
	p.UseFolder(ctx, true)
	p.SetFolderNavHandler(func(back bool, selID string, next models.Folder) {
		if back {
			navg.Back()
		} else if next != nil {
			navg.SetSelectionID(selID)
			navg.Push(next)
		}
		cur := navg.Current()
		p.SetFolder(ctx, cur, navg.HasBack())
		if back {
			id := navg.CurrentSelectionID()
			if id != "" {
				p.SelectByRowID(ctx, id)
			} else {
				p.ResetSelectionTop(ctx)
			}
		} else {
			p.ResetSelectionTop(ctx)
		}
	})

	// Enter namespaces from root
	p.RefreshFolder(ctx)
	enterByName(ctx, t, p, "namespaces")
	if got := folderPathString(navg.Current()); got != "namespaces" {
		t.Fatalf("expected namespaces, got %s", got)
	}

	// Select default and enter groups
	p.RefreshFolder(ctx)
	enterByName(ctx, t, p, "default")
	if got := folderPathString(navg.Current()); got != "groups" {
		t.Fatalf("expected groups, got %s", got)
	}

	// Back to namespaces; selection should be "default"
	enterBack(ctx, p)
	if got := folderPathString(navg.Current()); got != "namespaces" {
		t.Fatalf("expected namespaces after back, got %s", got)
	}
	if name := currentItemName(ctx, p); name != "default" {
		t.Fatalf("expected selection restored to default, got %s", name)
	}
}
