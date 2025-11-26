package ui

import (
	"testing"
	"time"
)

func TestViewOptionsModelPanelWidthCommit(t *testing.T) {
	model := NewViewOptionsModel(ViewOptionsConfig{
		PanelIndex:        1,
		PanelModes:        []PanelViewMode{PanelModeList, PanelModeManifest},
		ActivePanelMode:   PanelModeList,
		PanelWidthPercent: 75,
		TableMode:         "scroll",
	})

	msg := model.commit(true, false)
	if !msg.SetPanelWidth {
		t.Fatalf("expected SetPanelWidth to be true")
	}
	if msg.PanelWidthPercent != 75 {
		t.Fatalf("expected PanelWidthPercent=75, got %d", msg.PanelWidthPercent)
	}

	// Move focus to panel width entry and cycle once
	model.moveFocus(1)
	model.adjustCurrent(1)

	msg = model.commit(true, false)
	if msg.PanelWidthPercent != 100 {
		t.Fatalf("expected PanelWidthPercent=100 after cycle, got %d", msg.PanelWidthPercent)
	}
	if !msg.SetPanelWidth {
		t.Fatalf("expected SetPanelWidth true after adjustCurrent")
	}
}

func TestViewOptionsPanelWidthClamp(t *testing.T) {
	model := NewViewOptionsModel(ViewOptionsConfig{
		PanelIndex:        0,
		PanelModes:        []PanelViewMode{PanelModeList},
		ActivePanelMode:   PanelModeList,
		PanelWidthPercent: 25,
		TableMode:         "scroll",
	})

	model.moveFocus(1)
	model.adjustCurrent(-1)
	if msg := model.commit(true, false); msg.PanelWidthPercent != 25 {
		t.Fatalf("expected lower bound 25, got %d", msg.PanelWidthPercent)
	}

	for range panelWidthPercentOptions {
		model.adjustCurrent(1)
	}
	msg := model.commit(true, false)
	if msg.PanelWidthPercent != 100 {
		t.Fatalf("expected upper bound 100, got %d", msg.PanelWidthPercent)
	}
	model.adjustCurrent(1)
	if msg := model.commit(true, false); msg.PanelWidthPercent != 100 {
		t.Fatalf("expected to stay at upper bound, got %d", msg.PanelWidthPercent)
	}
}

func TestViewOptionsCommandWatchCommit(t *testing.T) {
	t.Parallel()

	model := NewViewOptionsModel(ViewOptionsConfig{
		PanelIndex:        0,
		PanelModes:        []PanelViewMode{PanelModeCommand},
		ActivePanelMode:   PanelModeCommand,
		PanelWidthPercent: 50,
		Command: &ViewOptionsCommandConfig{
			WatchInterval: 10 * time.Second,
		},
	})

	// Focus command watch option (panel mode -> panel width -> watch)
	model.moveFocus(1)
	model.moveFocus(1)

	msg := model.commit(true, false)
	if msg.Command == nil {
		t.Fatalf("expected command payload in commit")
	}
	if msg.Command.WatchInterval != 10*time.Second {
		t.Fatalf("expected watch interval 10s, got %s", msg.Command.WatchInterval)
	}

	model.adjustCurrent(1)
	msg = model.commit(true, false)
	if msg.Command.WatchInterval != 20*time.Second {
		t.Fatalf("expected watch interval to advance to 20s, got %s", msg.Command.WatchInterval)
	}
}
