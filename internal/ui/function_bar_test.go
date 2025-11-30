package ui

import (
	"strings"
	"testing"

	uistyles "github.com/sttts/kc/internal/ui/styles"
)

func TestFunctionBarEnterHintInteractiveCommand(t *testing.T) {
	fb := newFunctionBar()
	state := functionBarState{
		Width:            120,
		PanelMode:        PanelModeCommand,
		PanelInteractive: true,
		Capabilities:     PanelCapabilities{HasHelp: true, HasOptions: true},
	}

	view := fb.Render(state)

	want := uistyles.FunctionKeyStyle.Render("Enter") + uistyles.FunctionKeyDescriptionStyle.Render("Focus")
	if !strings.Contains(view, want) {
		t.Fatalf("expected enter focus hint, got:\n%s", view)
	}
}

func TestFunctionBarEnterHintDisabledWhenTerminalHasInput(t *testing.T) {
	fb := newFunctionBar()
	state := functionBarState{
		Width:            120,
		PanelMode:        PanelModeCommand,
		PanelInteractive: true,
		TerminalHasInput: true,
		Capabilities:     PanelCapabilities{HasHelp: true},
	}

	view := fb.Render(state)

	disabled := uistyles.FunctionKeyStyle.Render("Enter") + uistyles.FunctionKeyDisabledStyle.Render("Focus")
	if !strings.Contains(view, disabled) {
		t.Fatalf("expected disabled enter hint when terminal has input, got:\n%s", view)
	}
}

func TestFunctionBarCommandFocusedHidesEnter(t *testing.T) {
	fb := newFunctionBar()
	state := functionBarState{
		Width:                120,
		PanelMode:            PanelModeCommand,
		PanelInteractive:     true,
		PanelHasCommandFocus: true,
	}

	view := fb.Render(state)

	if strings.Contains(view, "Enter") {
		t.Fatalf("did not expect enter hint while command focused, got:\n%s", view)
	}
	drop := uistyles.FunctionKeyStyle.Render("Esc Esc") + uistyles.FunctionKeyDescriptionStyle.Render("Drop focus")
	if !strings.Contains(view, drop) {
		t.Fatalf("expected drop focus hint while focused, got:\n%s", view)
	}
}

func TestMaskCapabilitiesByMode(t *testing.T) {
	caps := PanelCapabilities{CanView: true, CanCopy: true, CanEdit: true, CanDelete: true, CanCreateNS: true, HasOptions: true, HasContextMenu: true, HasHelp: true}

	got := maskCapabilitiesForMode(PanelModeDescribe, false, caps)
	if got.CanView || got.CanCopy || got.CanEdit || got.CanDelete || got.CanCreateNS {
		t.Fatalf("describe mode should disable resource actions: %+v", got)
	}

	got = maskCapabilitiesForMode(PanelModeCommand, true, caps)
	if got.HasHelp || got.HasOptions || got.HasContextMenu {
		t.Fatalf("focused command should hide help/options/menu: %+v", got)
	}
	if got.CanView || got.CanEdit || got.CanDelete || got.CanCopy || got.CanCreateNS {
		t.Fatalf("command mode should disable resource actions: %+v", got)
	}

	got = maskCapabilitiesForMode(PanelModeList, false, caps)
	if !got.CanView || !got.CanEdit || !got.HasOptions || !got.HasContextMenu {
		t.Fatalf("list mode should keep capabilities intact: %+v", got)
	}
}
