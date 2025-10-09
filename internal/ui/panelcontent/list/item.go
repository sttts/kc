package list

import (
	models "github.com/sttts/kc/internal/models"
)

// Item mirrors panel row metadata stored in list widgets.
type Item struct {
	models.Item
	Name     string
	Selected bool
}

// PositionInfo tracks scroll/selection for restoring per path.
type PositionInfo struct {
	Selected  int
	ScrollTop int
}
