package ui

// ModalSizer allows modal content to provide a preferred content size so the
// surrounding frame can shrink to fit.
type ModalSizer interface {
	// PreferredSize returns the desired content width/height given the maximum
	// allowed content dimensions (exclusive of the modal frame). Returned
	// values should already be clamped to the provided maxima.
	PreferredSize(maxContentWidth, maxContentHeight int) (width, height int)
}
