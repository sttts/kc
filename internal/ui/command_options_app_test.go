package ui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sttts/kc/pkg/appconfig"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestCommandOptionsChangedMsgAppliesWatchInterval(t *testing.T) {
	t.Parallel()

	app := NewApp()
	app.activePanel = 0
	app.modalManager.Show("view_options")

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	if model, _ := app.leftPanel.Update(tea.WindowSizeMsg{Width: 80, Height: 24}); model != nil {
		app.leftPanel = model.(*Panel)
	}

	cmdCfg := appconfig.CommandConfig{
		Name:    "Top Nodes",
		Command: "echo ok",
	}
	// Frame size is irrelevant for this test; use zeroes.
	_ = app.leftPanel.StartCommand(ctx, cmdCfg, nil, schema.GroupVersionResource{}, 0, 0)

	msg := CommandOptionsChangedMsg{
		WatchInterval: 2 * time.Second,
		Accept:        true,
		Close:         true,
	}

	if _, cmd := app.Update(msg); cmd != nil {
		// We don't need to execute the returned command (tea.Tick) for state assertions.
	}

	if got := app.leftPanel.CommandWatchInterval(ctx); got != 2*time.Second {
		t.Fatalf("expected command watch interval 2s, got %s", got)
	}
	if app.modalManager.IsModalVisible() {
		t.Fatalf("command options modal should close after commit")
	}
}
