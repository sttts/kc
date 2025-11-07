package list

import (
	"context"
	"fmt"
	"testing"

	models "github.com/sttts/kc/internal/models"
	table "github.com/sttts/kc/internal/table"
	panelcontent "github.com/sttts/kc/internal/ui/panelcontent"
)

func TestFrameInfoEmptyList(t *testing.T) {
	w := New(panelcontent.WidgetDeps{})

	info := w.FrameInfo(context.Background(), panelcontent.FrameInfoRequest{Width: 20})
	if info.TopIndicator != "─" || info.BottomIndicator != "─" {
		t.Fatalf("expected default indicators, got top=%q bottom=%q", info.TopIndicator, info.BottomIndicator)
	}
	if info.FooterStatus != "" {
		t.Fatalf("expected empty footer status, got %q", info.FooterStatus)
	}
	if info.SuppressFooter {
		t.Fatalf("expected footer not suppressed")
	}
}

func TestFrameInfoIndicatorsAndStatus(t *testing.T) {
	ctx := context.Background()
	w := New(panelcontent.WidgetDeps{})
	w.Resize(ctx, panelcontent.Size{Width: 16, Height: 6})

	items := make([]Item, 10)
	for i := range items {
		items[i] = Item{Name: fmt.Sprintf("item-%d", i+1)}
	}
	w.items = items

	w.selected = 0
	w.scroll = 0
	info := w.FrameInfo(ctx, panelcontent.FrameInfoRequest{Width: 16})
	if info.TopIndicator != "─" {
		t.Fatalf("expected top indicator '─', got %q", info.TopIndicator)
	}
	if info.BottomIndicator != "v" {
		t.Fatalf("expected bottom indicator 'v', got %q", info.BottomIndicator)
	}
	if got := info.FooterStatus; got != "1/10 • 40%" {
		t.Fatalf("expected footer status '1/10 • 40%%', got %q", got)
	}

	w.selected = 6
	w.scroll = 5
	info = w.FrameInfo(ctx, panelcontent.FrameInfoRequest{Width: 16})
	if info.TopIndicator != "^" {
		t.Fatalf("expected top indicator '^', got %q", info.TopIndicator)
	}
	if info.BottomIndicator != "v" {
		t.Fatalf("expected bottom indicator 'v', got %q", info.BottomIndicator)
	}
	if got := info.FooterStatus; got != "7/10 • 90%" {
		t.Fatalf("expected footer status '7/10 • 90%%', got %q", got)
	}
}

func TestWidgetAppliesObjectOrderToFolder(t *testing.T) {
	ctx := context.Background()
	w := New(panelcontent.WidgetDeps{})
	w.UseFolder(true)

	folder := &orderCaptureFolder{}
	w.SetFolder(ctx, folder, false)
	if len(folder.orders) != 1 || folder.orders[0] != models.NormalizeObjectOrder("name") {
		t.Fatalf("expected default object order to be applied, got %+v", folder.orders)
	}

	w.SetObjectOrder(ctx, "Creation")
	if len(folder.orders) != 2 {
		t.Fatalf("expected second object order application, got %+v", folder.orders)
	}
	if got := folder.orders[1]; got != models.NormalizeObjectOrder("creation") {
		t.Fatalf("expected creation order to be applied, got %q", got)
	}
}

type orderCaptureFolder struct {
	orders []string
}

func (f *orderCaptureFolder) ApplyObjectOrder(order string) { f.orders = append(f.orders, order) }

func (f *orderCaptureFolder) Columns() []table.Column { return nil }
func (f *orderCaptureFolder) Path() []string          { return nil }
func (f *orderCaptureFolder) ItemByID(context.Context, string) (models.Item, bool) {
	return nil, false
}
func (f *orderCaptureFolder) Lines(context.Context, int, int) []table.Row    { return nil }
func (f *orderCaptureFolder) Above(context.Context, string, int) []table.Row { return nil }
func (f *orderCaptureFolder) Below(context.Context, string, int) []table.Row { return nil }
func (f *orderCaptureFolder) Len(context.Context) int                        { return 0 }
func (f *orderCaptureFolder) Find(context.Context, string) (int, table.Row, bool) {
	return -1, nil, false
}
