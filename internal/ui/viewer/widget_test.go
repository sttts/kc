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

func TestWidgetFooterCursor(t *testing.T) {
	w := New("")
	w.SetContent("foo\nbar", Metadata{})
	if cur := w.FooterCursor(10); cur != nil {
		t.Fatalf("expected nil cursor when not in search mode")
	}

	if cmd := w.beginSearch(); cmd != nil {
		_ = cmd()
	}
	w.searchField.SetValue("abc")
	cur := w.FooterCursor(20)
	if cur == nil {
		t.Fatalf("expected cursor when search active")
	}
	if cur.X == 0 {
		t.Fatalf("expected cursor X > 0 after input, got %d", cur.X)
	}
	if cur.Y != 0 {
		t.Fatalf("expected cursor Y == 0, got %d", cur.Y)
	}

	// Ensure updates do not mutate the original cursor reference.
	cur2 := w.FooterCursor(20)
	if cur2 == cur {
		t.Fatalf("expected FooterCursor to return a clone")
	}
}

func TestWidgetAppendLinesExtendsMatches(t *testing.T) {
	w := New("")
	w.SetPlainMode(true)
	w.SetContent("alpha", Metadata{})
	w.searchQuery = "beta"
	w.updateSearchMatches()
	if w.HasSearchMatches() {
		t.Fatalf("did not expect initial matches")
	}
	w.AppendLines([]string{"beta"})
	if !w.HasSearchMatches() {
		t.Fatalf("expected matches after append")
	}
	if len(w.searchMatches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(w.searchMatches))
	}
	if !w.advanceMatch() {
		t.Fatalf("expected advanceMatch to succeed on appended data")
	}
}
