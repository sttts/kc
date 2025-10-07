package models

import "testing"

func TestResourceGroupItemNotifyIfChanged(t *testing.T) {
	r := &ResourceGroupItem{
		RowItem:   NewRowItem("group", nil, nil, nil),
		watchable: true,
	}

	var fired int
	r.SetOnChange(func() { fired++ })

	// No known values yet, nothing should fire.
	r.notifyIfChanged(nil)
	if fired != 0 {
		t.Fatalf("expected no change, got %d", fired)
	}

	// Publish first count value – should trigger.
	r.mu.Lock()
	r.count = 5
	r.countKnown = true
	r.mu.Unlock()

	r.notifyIfChanged(nil)
	if fired != 1 {
		t.Fatalf("expected count change to trigger once, got %d", fired)
	}

	// Same count again should not trigger.
	r.notifyIfChanged(nil)
	if fired != 1 {
		t.Fatalf("expected no additional trigger, got %d", fired)
	}

	// Publish empty flag change – should trigger again.
	r.mu.Lock()
	r.empty = true
	r.emptyKnown = true
	r.mu.Unlock()

	r.notifyIfChanged(nil)
	if fired != 2 {
		t.Fatalf("expected empty change to trigger, got %d", fired)
	}
}

func TestResourceGroupItemNotifyOnUpdateCallback(t *testing.T) {
	r := &ResourceGroupItem{RowItem: NewRowItem("group", nil, nil, nil), watchable: true}

	var changeCount, updateCount int
	r.SetOnChange(func() { changeCount++ })

	r.mu.Lock()
	r.count = 1
	r.countKnown = true
	r.mu.Unlock()

	r.notifyIfChanged(func() { updateCount++ })
	if changeCount != 1 || updateCount != 1 {
		t.Fatalf("expected both change and update callbacks, got change=%d update=%d", changeCount, updateCount)
	}

	// No further updates when values unchanged.
	r.notifyIfChanged(func() { updateCount++ })
	if changeCount != 1 || updateCount != 1 {
		t.Fatalf("expected no additional callbacks, got change=%d update=%d", changeCount, updateCount)
	}
}
