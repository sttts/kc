package navigation

import "github.com/sttts/kc/internal/navigation/models"

// ErrNoViewContent indicates that an item does not provide focused view content.
var ErrNoViewContent = models.ErrNoViewContent

// ViewContentFunc returns textual content plus optional syntax hints for viewers.
type ViewContentFunc = models.ViewContentFunc
