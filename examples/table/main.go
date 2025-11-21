package main

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	table "github.com/sttts/kc/internal/table"
)

func main() {
	cols := []table.Column{{Title: " Name"}, {Title: "Group"}, {Title: "Count"}}
	rows := make([]table.Row, 0, len(data))
	for _, d := range data {
		r := table.SimpleRow{ID: d.ID}
		r.SetColumn(0, d.Name, nil)
		r.SetColumn(1, d.Group, nil)
		r.SetColumn(2, d.Count, nil)
		rows = append(rows, r)
	}
	list := table.NewSliceList(rows)
	ctx := context.Background()

	bt := table.NewBigTable(cols, list, 60, 10)
	bt.SetMode(ctx, table.ModeFit)
	bt.SetFocused(ctx, true)
	fmt.Println("Fit mode:\n" + viewString(bt.View()))

	bt.SetMode(ctx, table.ModeScroll)
	bt.SetFocused(ctx, true)
	fmt.Println("\nScroll mode:\n" + viewString(bt.View()))
}

type rowData struct {
	ID    string
	Name  string
	Group string
	Count string
}

var data = []rowData{
	{ID: "pods", Name: "/pods", Group: "", Count: "12"},
	{ID: "deployments", Name: "/deployments", Group: "apps", Count: "5"},
	{ID: "configmaps", Name: "/configmaps", Group: "", Count: "8"},
}

func viewString(view tea.View) string {
	if view.Content == nil {
		return ""
	}
	return fmt.Sprint(view.Content)
}
