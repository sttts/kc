package models

import (
	"testing"
)

func TestBaseFolderDirtyListener(t *testing.T) {
	base := NewBaseFolder(Deps{}, nil, nil)
	calls := 0
	cancel := base.RegisterDirtyListener(func() { calls++ })
	if cancel == nil {
		t.Fatalf("expected cancel func")
	}

	base.Refresh()
	if calls != 1 {
		t.Fatalf("expected listener to fire once, got %d", calls)
	}

	cancel()
	base.Refresh()
	if calls != 1 {
		t.Fatalf("expected listener to stop after cancel, got %d", calls)
	}
}
