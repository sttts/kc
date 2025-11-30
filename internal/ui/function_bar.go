package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	uistyles "github.com/sttts/kc/internal/ui/styles"
)

type functionBarState struct {
	Width                int
	ToastActive          bool
	ToastText            string
	ShowTerminal         bool
	ActivePanel          int
	PanelHasCommandFocus bool
	Capabilities         PanelCapabilities
	PanelMode            PanelViewMode
}

type functionBar struct {
	cache cachedView
	key   string
}

func newFunctionBar() *functionBar { return &functionBar{} }

func (f *functionBar) Render(state functionBarState) string {
	sig := f.signature(state)
	if view, _, ok := f.cache.get(); ok && f.key == sig {
		return view
	}

	if state.ToastActive {
		msg := state.ToastText
		maxw := state.Width
		if lipgloss.Width(msg) > maxw {
			if maxw > 1 {
				msg = sliceANSIColsRaw(msg, 0, maxw-1) + "…"
			} else {
				msg = sliceANSIColsRaw(msg, 0, maxw)
			}
		}
		toastStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("196")).
			Foreground(lipgloss.White).
			Bold(true)
		f.key = sig
		f.cache.set(toastStyle.Width(state.Width).Render(msg), nil)
		view, _, _ := f.cache.get()
		return view
	}

	var keys []string
	if state.ShowTerminal {
		keys = []string{uistyles.FunctionKeyStyle.Render("Ctrl+O") + uistyles.FunctionKeyDescriptionStyle.Render("Return to panels")}
	} else if state.PanelHasCommandFocus {
		keys = []string{uistyles.FunctionKeyStyle.Render("Esc Esc") + uistyles.FunctionKeyDescriptionStyle.Render("Drop focus")}
	} else {
		caps := state.Capabilities
		renderKey := func(key, label string, enabled bool) string {
			desc := uistyles.FunctionKeyDescriptionStyle
			if !enabled {
				desc = uistyles.FunctionKeyDisabledStyle
			}
			trimmed := strings.TrimSpace(label)
			if trimmed == "" {
				placeholder := desc.Copy().Padding(0, 0, 0, 0)
				return uistyles.FunctionKeyStyle.Render(key) + placeholder.Render(" - ")
			}
			return uistyles.FunctionKeyStyle.Render(key) + desc.Render(label)
		}

		keys = []string{
			renderKey("F1", "Help", caps.HasHelp),
			renderKey("F2", "Options", caps.HasOptions),
			renderKey("F3", "View", caps.CanView),
			renderKey("F4", "Edit", caps.CanEdit),
			renderKey("F5", "Copy", caps.CanCopy),
			renderKey("F6", "", false),
			renderKey("F7", "Namespace", caps.CanCreateNS),
			renderKey("F8", "Delete", caps.CanDelete),
			renderKey("F9", "Commands", caps.HasContextMenu),
			uistyles.FunctionKeyStyle.Render("F10") + uistyles.FunctionKeyDescriptionStyle.Render("Quit"),
			uistyles.FunctionKeyStyle.Render("Ctrl+O") + uistyles.FunctionKeyDescriptionStyle.Render("Fullscreen"),
		}
	}

	joined := lipgloss.JoinHorizontal(lipgloss.Left, keys...)
	title := " Kubernetes Commander "
	fullWidthStyle := uistyles.FunctionKeyBarStyle.Width(state.Width).Align(lipgloss.Left)
	available := state.Width - lipgloss.Width(joined) - 1
	if available < lipgloss.Width(title) {
		title = " kc "
	}
	titleStyle := uistyles.FunctionKeyTitleStyle.Align(lipgloss.Center).Width(max(0, available))
	titleRendered := titleStyle.Render(title)

	f.key = sig
	f.cache.set(fullWidthStyle.Render(joined+" "+titleRendered), nil)
	view, _, _ := f.cache.get()
	return view
}

func (f *functionBar) signature(state functionBarState) string {
	c := state.Capabilities
	return fmt.Sprintf("w=%d toast=%t tt=%s term=%t panel=%d cmd=%t mode=%d caps=%t-%t-%t-%t-%t-%t-%t-%t",
		state.Width,
		state.ToastActive,
		state.ToastText,
		state.ShowTerminal,
		state.ActivePanel,
		state.PanelHasCommandFocus,
		state.PanelMode,
		c.CanView, c.CanCopy, c.CanEdit, c.CanDelete, c.CanCreateNS, c.HasOptions, c.HasContextMenu, c.HasHelp)
}

func (f *functionBar) Invalidate() {
	f.key = ""
	f.cache.invalidate()
}
