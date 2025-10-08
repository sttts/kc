package table

import "testing"

func TestLinkedListBasicOps(t *testing.T) {
	ll := NewLinkedList([]Row{SimpleRow{ID: "a"}, SimpleRow{ID: "b"}})
	ctx := t.Context()
	if ll.Len(ctx) != 2 {
		t.Fatalf("len want 2 got %d", ll.Len(ctx))
	}
	if !ll.InsertAfterID("a", SimpleRow{ID: "a1"}) {
		t.Fatalf("insert after failed")
	}
	if !ll.InsertBeforeID("b", SimpleRow{ID: "x"}) {
		t.Fatalf("insert before failed")
	}
	if _, _, ok := ll.Find(ctx, "x"); !ok {
		t.Fatalf("expected to find x")
	}
	if n := ll.RemoveIDs("a1", "x"); n != 2 {
		t.Fatalf("expected removed 2, got %d", n)
	}
}
