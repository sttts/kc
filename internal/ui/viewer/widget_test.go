package viewer

import "testing"

func TestWidgetSearchMatches(t *testing.T) {
	w := New("")
	w.SetContent("foo\nbar\nFoo baz\nnoop", Metadata{})

	w.searchQuery = "foo"
	w.updateSearchMatches()

	if !w.HasSearchMatches() {
		t.Fatalf("expected search matches")
	}
	if len(w.searchMatches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(w.searchMatches))
	}
	if w.activeMatch != 0 {
		t.Fatalf("expected initial match index 0, got %d", w.activeMatch)
	}
	if !w.advanceMatch() || w.activeMatch != 1 {
		t.Fatalf("expected to advance to second match, got %d", w.activeMatch)
	}
	if !w.previousMatch() || w.activeMatch != 0 {
		t.Fatalf("expected to move back to first match")
	}
	w.clearSearchState()
	if w.HasSearchMatches() {
		t.Fatalf("expected matches cleared after reset")
	}
}
