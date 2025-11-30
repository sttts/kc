package ui

import (
	"testing"
	"time"

	"github.com/sttts/kc/internal/ui/panelcontent"
	"github.com/sttts/kc/pkg/appconfig"
)

func TestCommandWidgetHeartbeatStatus(t *testing.T) {
	widget := NewCommandWidget(panelcontent.WidgetDeps{Config: appconfig.Default()}, appconfig.CommandConfig{Name: "cmd"})

	// No status when nothing running and no exit.
	if got := widget.heartbeatStatus(); got != "" {
		t.Fatalf("expected empty heartbeat, got %q", got)
	}

	widget.running = true
	widget.heartbeatOn = true
	widget.heartbeatUntil = time.Now().Add(time.Second)
	if got := widget.heartbeatStatus(); got == "" {
		t.Fatalf("expected heartbeat while running")
	}

	widget.running = false
	widget.heartbeatOn = false
	widget.exitKnown = true
	widget.exitCode = 1
	if got := widget.heartbeatStatus(); got == "" {
		t.Fatalf("expected heartbeat for non-zero exit")
	}
}
