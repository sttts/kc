package models

import (
	"testing"
	"time"
)

func TestAgeCellHookApply(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	created := now.Add(-90 * time.Second)
	hook := newAgeCellHook([]int{1}, created)
	if hook == nil {
		t.Fatalf("expected hook to be created")
	}
	hook.now = func() time.Time { return now }

	cells := []string{"pod-a", "stale"}
	hook.apply(cells)
	if got := cells[1]; got != "90s" {
		t.Fatalf("expected Age cell to update to 90s, got %q", got)
	}
}

func TestAgeCellHookNextInterval(t *testing.T) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	created := base.Add(-30 * time.Minute)
	hook := newAgeCellHook([]int{0}, created)
	if hook == nil {
		t.Fatalf("expected hook to be created")
	}
	cases := []struct {
		name     string
		elapsed  time.Duration
		expected time.Duration
	}{
		{"seconds", time.Minute, time.Second},
		{"minutes", 30 * time.Minute, time.Minute},
		{"shortHours", 4 * time.Hour, time.Minute},
		{"longHours", 12 * time.Hour, time.Hour},
		{"fewDays", 3 * 24 * time.Hour, time.Hour},
		{"manyDays", 10 * 24 * time.Hour, 24 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := created.Add(tc.elapsed)
			if got := hook.nextInterval(now); got != tc.expected {
				t.Fatalf("expected interval %v, got %v", tc.expected, got)
			}
		})
	}
}
