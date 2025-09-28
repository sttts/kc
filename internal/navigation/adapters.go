package navigation

import (
    table "github.com/sttts/kc/internal/table"
)

// ColumnsToTitles extracts column titles for legacy renderers that expect []string.
func ColumnsToTitles(cols []table.Column) []string {
    out := make([]string, len(cols))
    for i := range cols { out[i] = cols[i].Title }
    return out
}

// RowsToCells converts []table.Row into [][]string of cells for legacy renderers.
// It drops styles and pads missing cells with empty strings to the max column count.
func RowsToCells(rows []table.Row) [][]string {
    // determine max columns across rows
    maxCols := 0
    tmp := make([][]string, len(rows))
    for i, r := range rows {
        _, cells, _, _ := r.Columns()
        tmp[i] = cells
        if len(cells) > maxCols { maxCols = len(cells) }
    }
    out := make([][]string, len(rows))
    for i := range rows {
        cells := tmp[i]
        if len(cells) == maxCols {
            out[i] = cells
            continue
        }
        padded := make([]string, maxCols)
        copy(padded, cells)
        out[i] = padded
    }
    return out
}

