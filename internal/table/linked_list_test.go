package table

import "testing"

func TestLinkedListBasicOps(t *testing.T) {
    ll := NewLinkedList([]Row{SimpleRow{ID: "a"}, SimpleRow{ID: "b"}})
    if ll.Len() != 2 { t.Fatalf("len want 2 got %d", ll.Len()) }
    if !ll.InsertAfterID("a", SimpleRow{ID: "a1"}) { t.Fatalf("insert after failed") }
    if !ll.InsertBeforeID("b", SimpleRow{ID: "x"}) { t.Fatalf("insert before failed") }
    if _, _, ok := ll.Find("x"); !ok { t.Fatalf("expected to find x") }
    if n := ll.RemoveIDs("a1", "x"); n != 2 { t.Fatalf("expected removed 2, got %d", n) }
}
