package navigation

import (
	"context"
	"testing"

	"github.com/charmbracelet/lipgloss/v2"
	models "github.com/sttts/kc/internal/models"
	modeltesting "github.com/sttts/kc/internal/models/testing"
	table "github.com/sttts/kc/internal/table"
)

type enterItem struct {
	*models.RowItem
	next models.Folder
}

var _ models.Enterable = (*enterItem)(nil)

func newEnterItem(id string, next models.Folder) *enterItem {
	style := lipgloss.NewStyle()
	row := models.NewRowItem(id, []string{"/" + id}, []string{id}, &style)
	return &enterItem{RowItem: row, next: next}
}

func (e *enterItem) Enter() (models.Folder, error) {
	return e.next, nil
}

func TestGoTo(t *testing.T) {
	ctx := context.Background()

	leaf := modeltesting.NewSliceFolder("resources", []table.Column{{Title: " Name"}}, nil)
	nsFolder := modeltesting.NewSliceFolder("namespaces", []table.Column{{Title: " Name"}}, []table.Row{
		newEnterItem("default", leaf),
	})
	root := modeltesting.NewSliceFolder("/", []table.Column{{Title: " Name"}}, []table.Row{
		newEnterItem("namespaces", nsFolder),
	})

	nav := NewNavigator(root)
	result, err := GoTo(ctx, nav, []GoToStep{
		{SelectionID: "namespaces", Enter: true},
		{SelectionID: "default", Enter: true},
	})
	if err != nil {
		t.Fatalf("GoTo returned error: %v", err)
	}
	if len(result.Entered) != 2 {
		t.Fatalf("entered count = %d, want 2", len(result.Entered))
	}
	if nav.Current() != leaf {
		t.Fatalf("current folder mismatch, got %v want %v", nav.Current(), leaf)
	}
	path := nav.Path(ctx)
	if path != "/namespaces/default" {
		t.Fatalf("path = %q, want /namespaces/default", path)
	}
}

func TestGoToMissingItem(t *testing.T) {
	ctx := context.Background()
	root := modeltesting.NewSliceFolder("/", nil, nil)
	nav := NewNavigator(root)
	_, err := GoTo(ctx, nav, []GoToStep{
		{SelectionID: "missing", Enter: true},
	})
	if err == nil {
		t.Fatal("expected error for missing row, got nil")
	}
}
