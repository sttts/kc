package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
)

func TestWaitFolderDirtyEventsDeliversMessage(t *testing.T) {
	t.Parallel()
	app := NewApp()
	defer app.cancel()

	cmd := app.waitFolderDirtyEvents()
	if cmd == nil {
		t.Fatal("expected folder dirty wait command")
	}

	result := make(chan tea.Msg, 1)
	go func() {
		result <- cmd()
	}()

	// Allow goroutine to block on the channel before sending.
	select {
	case <-result:
		t.Fatal("command returned unexpectedly before event")
	case <-time.After(10 * time.Millisecond):
	}

	app.signalFolderDirty(1)

	select {
	case msg := <-result:
		fd, ok := msg.(FolderDirtyMsg)
		if !ok {
			t.Fatalf("unexpected message type %T", msg)
		}
		if fd.PanelIdx != 1 {
			t.Fatalf("unexpected panel index %d", fd.PanelIdx)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for folder dirty message")
	}
}
