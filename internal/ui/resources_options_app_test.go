package ui

import (
	"context"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/sttts/kc/internal/models"
	navui "github.com/sttts/kc/internal/navigation"
	table "github.com/sttts/kc/internal/table"
	"github.com/sttts/kc/pkg/appconfig"
)

func TestResourcesOptionsChangedMsgAppliesSettings(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	app := NewApp()
	baseCfg := appconfig.Default()
	baseCfg.Resources.ShowNonEmptyOnly = true
	baseCfg.Resources.Order = appconfig.OrderAlpha
	app.cfg = baseCfg
	app.leftConfig = cloneConfig(baseCfg)

	folder := newTestResourcesFolder(app.leftConfig)
	app.leftNav = navui.NewNavigator(folder)
	app.leftPanel.UseFolder(true)
	app.leftPanel.SetFolder(ctx, folder, false)
	app.leftPanel.SetCurrentPath("/resources")
	app.activePanel = 0
	app.modalManager.Show("resources_options")

	initial := folder.rowIDs(ctx)
	if slices.Contains(initial, "batch/v1/jobs") {
		t.Fatalf("expected jobs hidden with showNonEmptyOnly=true, rows=%v", initial)
	}
	initialItems := panelResourceNames(ctx, app.leftPanel)
	if slices.Contains(initialItems, "jobs") {
		t.Fatalf("panel still listing jobs when showNonEmptyOnly=true, items=%v", initialItems)
	}

	msg := ResourcesOptionsChangedMsg{
		ShowNonEmptyOnly: false,
		Order:            "group",
		TableMode:        "scroll",
		HasInclude:       true,
		HasOrder:         true,
		Accept:           true,
		Close:            true,
	}

	if _, cmd := app.Update(msg); cmd != nil {
		_ = cmd()
	}

	if app.leftConfig.Resources.ShowNonEmptyOnly {
		t.Fatalf("left panel config showNonEmptyOnly not updated")
	}
	if app.leftConfig.Resources.Order != appconfig.OrderGroup {
		t.Fatalf("left panel order not updated, got %s", app.leftConfig.Resources.Order)
	}

	final := folder.rowIDs(ctx)
	if !slices.Contains(final, "batch/v1/jobs") {
		t.Fatalf("expected jobs visible after toggling include empty, rows=%v", final)
	}
	want := []string{
		"apps/v1/deployments",
		"apps/v1/replicasets",
		"batch/v1/cronjobs",
		"batch/v1/jobs",
	}
	if strings.Join(final, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected ordering after update: got=%v want=%v", final, want)
	}
	finalItems := panelResourceNames(ctx, app.leftPanel)
	if strings.Join(finalItems, ",") != strings.Join([]string{"deployments", "replicasets", "cronjobs", "jobs"}, ",") {
		t.Fatalf("panel items not refreshed, got=%v", finalItems)
	}
	if app.modalManager.IsModalVisible() {
		t.Fatalf("resources modal should close after Accept")
	}
}

func panelResourceNames(ctx context.Context, panel *Panel) []string {
	if panel == nil {
		return nil
	}
	panel.RefreshFolder(ctx)
	items := panel.Items(ctx)
	if len(items) == 0 {
		if f := panel.Folder(); f != nil {
			rows := f.Lines(ctx, 0, f.Len(ctx))
			for _, row := range rows {
				id, cells, _, ok := row.Columns()
				if !ok {
					continue
				}
				name := ""
				if len(cells) > 0 {
					name = cells[0]
				} else if id != "" {
					name = id
				}
				items = append(items, Item{Name: name})
			}
		}
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return names
}

type testResourcesFolder struct {
	cfg   *appconfig.Config
	path  []string
	mu    sync.Mutex
	rows  []table.Row
	dirty bool
}

func newTestResourcesFolder(cfg *appconfig.Config) *testResourcesFolder {
	return &testResourcesFolder{
		cfg:   cfg,
		path:  []string{"resources"},
		dirty: true,
	}
}

func (f *testResourcesFolder) ensureRowsLocked() {
	if !f.dirty && f.rows != nil {
		return
	}
	f.rows = buildResourceRows(f.cfg, f.path)
	f.dirty = false
}

func (f *testResourcesFolder) currentRows() []table.Row {
	f.mu.Lock()
	f.ensureRowsLocked()
	rows := make([]table.Row, len(f.rows))
	copy(rows, f.rows)
	f.mu.Unlock()
	return rows
}

func (f *testResourcesFolder) Columns() []table.Column {
	return []table.Column{{Title: " Name"}, {Title: "Group"}, {Title: "Count"}}
}

func (f *testResourcesFolder) Path() []string { return append([]string(nil), f.path...) }

func (f *testResourcesFolder) Lines(ctx context.Context, top, num int) []table.Row {
	rows := f.currentRows()
	if num <= 0 || top >= len(rows) {
		return nil
	}
	if top < 0 {
		top = 0
	}
	end := top + num
	if end > len(rows) {
		end = len(rows)
	}
	return append([]table.Row(nil), rows[top:end]...)
}

func (f *testResourcesFolder) Above(ctx context.Context, id string, n int) []table.Row {
	idx, _, ok := f.Find(ctx, id)
	if !ok {
		return nil
	}
	start := idx - n
	if start < 0 {
		start = 0
	}
	return f.Lines(ctx, start, idx-start)
}

func (f *testResourcesFolder) Below(ctx context.Context, id string, n int) []table.Row {
	idx, _, ok := f.Find(ctx, id)
	if !ok {
		return nil
	}
	return f.Lines(ctx, idx+1, n)
}

func (f *testResourcesFolder) Len(ctx context.Context) int {
	return len(f.currentRows())
}

func (f *testResourcesFolder) Find(ctx context.Context, id string) (int, table.Row, bool) {
	rows := f.currentRows()
	for i, row := range rows {
		rid, _, _, ok := row.Columns()
		if ok && rid == id {
			return i, row, true
		}
	}
	return -1, nil, false
}

func (f *testResourcesFolder) ItemByID(ctx context.Context, id string) (models.Item, bool) {
	_, row, ok := f.Find(ctx, id)
	if !ok {
		return nil, false
	}
	item, ok := row.(models.Item)
	return item, ok
}

func (f *testResourcesFolder) Refresh() {
	f.mu.Lock()
	f.dirty = true
	f.mu.Unlock()
}

func (f *testResourcesFolder) IsDirty() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dirty
}

func (f *testResourcesFolder) rowIDs(ctx context.Context) []string {
	rows := f.currentRows()
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if id, _, _, ok := row.Columns(); ok {
			out = append(out, id)
		}
	}
	return out
}

func buildResourceRows(cfg *appconfig.Config, basePath []string) []table.Row {
	resources := []struct {
		id       string
		name     string
		group    string
		nonEmpty bool
	}{
		{id: "apps/v1/deployments", name: "deployments", group: "apps", nonEmpty: true},
		{id: "apps/v1/replicasets", name: "replicasets", group: "apps", nonEmpty: true},
		{id: "batch/v1/cronjobs", name: "cronjobs", group: "batch", nonEmpty: true},
		{id: "batch/v1/jobs", name: "jobs", group: "batch", nonEmpty: false},
	}

	filtered := make([]struct {
		id    string
		name  string
		group string
	}, 0, len(resources))
	for _, r := range resources {
		if cfg.Resources.ShowNonEmptyOnly && !r.nonEmpty {
			continue
		}
		filtered = append(filtered, struct {
			id    string
			name  string
			group string
		}{id: r.id, name: r.name, group: r.group})
	}

	order := strings.ToLower(string(cfg.Resources.Order))
	switch order {
	case "group":
		sort.Slice(filtered, func(i, j int) bool {
			if filtered[i].group == filtered[j].group {
				return filtered[i].name < filtered[j].name
			}
			return filtered[i].group < filtered[j].group
		})
	case "favorites":
		favs := favoritesSet(cfg.Resources.Favorites)
		sort.Slice(filtered, func(i, j int) bool {
			fi := favs[strings.ToLower(filtered[i].name)]
			fj := favs[strings.ToLower(filtered[j].name)]
			if fi != fj {
				return fi
			}
			return filtered[i].name < filtered[j].name
		})
	default:
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].name < filtered[j].name
		})
	}

	rows := make([]table.Row, 0, len(filtered))
	for _, r := range filtered {
		path := append([]string(nil), basePath...)
		path = append(path, r.name)
		cells := []string{"/" + r.name, r.group, ""}
		item := models.NewSimpleItem(r.id, cells, path, models.WhiteStyle())
		rows = append(rows, item)
	}
	return rows
}

func favoritesSet(list []string) map[string]bool {
	if len(list) == 0 {
		return nil
	}
	set := make(map[string]bool, len(list))
	for _, item := range list {
		if item == "" {
			continue
		}
		set[strings.ToLower(item)] = true
	}
	return set
}
