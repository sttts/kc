package models

import "errors"

// ErrNoViewContent indicates that an item does not provide focused view content.
var ErrNoViewContent = errors.New("navigation: view content unavailable")

// ViewContentFunc returns textual content plus optional syntax hints for viewers.
type ViewContentFunc func() (title, body, lang, mime, filename string, err error)
