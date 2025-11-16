package models

import (
	"time"

	"k8s.io/apimachinery/pkg/util/duration"
)

// ageCellHook rewrites one or more Age columns on the fly using the object's creation timestamp.
type ageCellHook struct {
	cols    []int
	created time.Time
	now     func() time.Time
}

func newAgeCellHook(cols []int, created time.Time) *ageCellHook {
	if len(cols) == 0 || created.IsZero() {
		return nil
	}
	clean := make([]int, len(cols))
	copy(clean, cols)
	return &ageCellHook{
		cols:    clean,
		created: created,
		now:     time.Now,
	}
}

func (h *ageCellHook) apply(cells []string) []string {
	if h == nil || len(h.cols) == 0 || h.created.IsZero() {
		return cells
	}
	if cells == nil {
		return nil
	}
	nowFn := h.now
	if nowFn == nil {
		nowFn = time.Now
	}
	age := duration.HumanDuration(nowFn().Sub(h.created))
	for _, idx := range h.cols {
		if idx >= 0 && idx < len(cells) {
			cells[idx] = age
		}
	}
	return cells
}

func (h *ageCellHook) nextInterval(now time.Time) time.Duration {
	if h == nil || len(h.cols) == 0 || h.created.IsZero() {
		return 0
	}
	if now.IsZero() {
		now = time.Now()
	}
	elapsed := now.Sub(h.created)
	if elapsed < 0 {
		elapsed = 0
	}
	switch {
	case elapsed < 10*time.Minute:
		return time.Second
	case elapsed < 8*time.Hour:
		return time.Minute
	case elapsed < 8*24*time.Hour:
		return time.Hour
	default:
		return 24 * time.Hour
	}
}
