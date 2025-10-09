package navigation

import (
	"context"
	"fmt"

	"github.com/sttts/kc/internal/models"
)

// GoToStep describes a selection to apply on the current folder and whether to
// enter the selected row to push a child folder on the navigator stack.
type GoToStep struct {
	SelectionID string
	Enter       bool
}

// GoToResult reports the folders entered while executing GoTo. The list is in
// the same order as the steps that requested Enter=true.
type GoToResult struct {
	Entered []models.Folder
}

// GoTo walks the navigator through the provided steps by selecting rows and, if
// requested, entering the selected row to push a child folder. It returns an
// error if any step cannot be satisfied (missing row, non-enterable row, or
// nil navigator/folder).
func GoTo(ctx context.Context, nav *Navigator, steps []GoToStep) (GoToResult, error) {
	var res GoToResult
	if nav == nil {
		return res, fmt.Errorf("navigation: nil navigator")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cur := nav.Current()
	if cur == nil && len(steps) > 0 {
		return res, fmt.Errorf("navigation: empty stack")
	}
	for idx, step := range steps {
		if cur == nil {
			return res, fmt.Errorf("navigation: nil folder at step %d", idx)
		}
		if step.SelectionID == "" {
			return res, fmt.Errorf("navigation: empty selection id at step %d", idx)
		}
		// Ensure rows are populated before lookup; Len triggers lazy evaluation.
		cur.Len(ctx)
		nav.SetSelectionID(step.SelectionID)
		if !step.Enter {
			continue
		}
		item, ok := cur.ItemByID(ctx, step.SelectionID)
		if !ok {
			return res, fmt.Errorf("navigation: item %q not found", step.SelectionID)
		}
		enterable, ok := item.(models.Enterable)
		if !ok {
			return res, fmt.Errorf("navigation: item %q cannot be entered", step.SelectionID)
		}
		next, err := enterable.Enter()
		if err != nil {
			return res, fmt.Errorf("navigation: entering %q: %w", step.SelectionID, err)
		}
		if next == nil {
			return res, fmt.Errorf("navigation: item %q returned nil folder", step.SelectionID)
		}
		nav.Push(next)
		res.Entered = append(res.Entered, next)
		cur = next
	}
	return res, nil
}
