package modeltesting

import (
	"context"
	"strings"

	models "github.com/sttts/kc/internal/models"
	table "github.com/sttts/kc/internal/table"
)

// NewStaticFolder builds a BaseFolder populated with the provided rows and columns.
// The title is split on "/" to derive the folder Path ("/" -> root, "a/b" -> ["a","b"]).
func NewStaticFolder(title string, cols []table.Column, rows []table.Row) *models.BaseFolder {
	var path []string
	if title != "" && title != "/" {
		for _, seg := range strings.Split(title, "/") {
			if seg != "" {
				path = append(path, seg)
			}
		}
	}
	if len(cols) == 0 {
		cols = []table.Column{{Title: " Name"}}
	}
	base := models.NewBaseFolder(models.Deps{}, cols, path)
	snapshot := append([]table.Row(nil), rows...)
	base.SetPopulate(func(context.Context) ([]table.Row, error) {
		if len(snapshot) == 0 {
			return nil, nil
		}
		out := make([]table.Row, len(snapshot))
		copy(out, snapshot)
		return out, nil
	})
	return base
}
