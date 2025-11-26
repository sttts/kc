package ui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sttts/kc/internal/overlay"
	uistyles "github.com/sttts/kc/internal/ui/styles"
)

// View renders the application
func (a *App) View() tea.View {
	if view, cursor, ok := a.mainViewCache.get(); ok {
		v := viewWithCursor(view, cursor)
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		return v
	}
	// In fullscreen terminal mode, only show terminal
	if a.showTerminal {
		terminalView, terminalCursor := a.renderTerminalView()
		a.mainViewCache.set(terminalView, terminalCursor)
		v := viewWithCursor(terminalView, terminalCursor)
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		return v
	}

	// In normal mode, show main view
	mainView, mainCursor := a.renderMainView()

	// Overlay modal if visible
	if a.modalManager.IsModalVisible() {
		modalView := a.modalManager.View()
		modalCursor := modalView.Cursor
		modalContent := viewString(modalView)
		a.mainViewCache.set(modalContent, modalCursor)
		v := viewWithCursor(modalContent, modalCursor)
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		return v
	}

	a.mainViewCache.set(mainView, mainCursor)
	v := viewWithCursor(mainView, mainCursor)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// renderMainView renders the main two-panel view
func (a *App) renderMainView() (string, *tea.Cursor) {
	// Calculate dimensions
	terminalHeight := a.inlineTerminalHeight()
	// Reserve space for the inline terminal (if visible) and function keys (1)
	reserved := terminalHeight + 1
	panelHeight := a.height - reserved
	if panelHeight < 3 {
		panelHeight = 3
	}
	leftPanelWidth, rightPanelWidth := a.panelWidthsFor(a.width)

	renderPanel := func(panel *Panel, width int, focused bool) string {
		if width <= 0 {
			return ""
		}
		if cached, ok := panel.CachedFrame(width, panelHeight, focused); ok {
			return cached
		}
		ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
		defer cancel()
		return panel.Render(ctx, width, panelHeight, focused)
	}

	leftPanel := ""
	if leftPanelWidth > 0 {
		leftPanel = renderPanel(a.leftPanel, leftPanelWidth, a.activePanel == 0)
	}
	rightPanel := ""
	if rightPanelWidth > 0 {
		rightPanel = renderPanel(a.rightPanel, rightPanelWidth, a.activePanel == 1)
	}

	var panels string
	switch {
	case leftPanelWidth <= 0:
		panels = rightPanel
	case rightPanelWidth <= 0:
		panels = leftPanel
	default:
		panels = lipgloss.JoinHorizontal(
			lipgloss.Top,
			leftPanel,
			rightPanel,
		)
	}

	// Add terminal (2 lines)
	terminalView := ""
	var terminalCursor *tea.Cursor
	if terminalHeight > 0 {
		terminalView, terminalCursor = a.renderTerminalArea()
	}

	// Add function key bar
	functionKeys := a.renderFunctionKeys()
	combinedView := lipgloss.JoinVertical(lipgloss.Left, panels, functionKeys)
	if terminalHeight > 0 {
		combinedView = lipgloss.JoinVertical(
			lipgloss.Left,
			panels,
			terminalView,
			functionKeys,
		)
	}

	// Busy overlay: show a small 2x2 ASCII animation centered over the main view
	if a.busyActive {
		ov := a.renderBusyOverlay()
		if ov != "" {
			combinedView = overlay.Composite(ov, combinedView, overlay.Center, overlay.Center, 0, 0)
		}
	}

	// Adjust cursor position for the combined view
	// The cursor needs to be offset by the height of panels
	if terminalCursor != nil {
		// Calculate the offset: panels height
		offsetY := panelHeight
		adjustedCursor := tea.NewCursor(terminalCursor.X, terminalCursor.Y+offsetY)
		adjustedCursor.Blink = terminalCursor.Blink
		adjustedCursor.Color = terminalCursor.Color
		adjustedCursor.Shape = terminalCursor.Shape
		return combinedView, adjustedCursor
	}

	return combinedView, nil
}

// renderBusyOverlay returns a small 2x2 ASCII animation based on busyFrame.
func (a *App) renderBusyOverlay() string {
	// 2x2 ASCII frames: cross and bar alternation
	frames := []string{
		"\\/\n/\\", // star
		"||\n||",
		"/\\\n\\/",
		"--\n--",
	}
	f := frames[a.busyFrame%len(frames)]
	// Add a faint box/spacing around for visibility (optional)
	st := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.White).Background(lipgloss.Color("238")).Padding(0, 1)
	return st.Render(f)
}

// renderTerminalArea renders the 2-line terminal area in main view
func (a *App) renderTerminalArea() (string, *tea.Cursor) {
	if view, cursor, ok := a.terminalAreaCache.get(); ok {
		return view, cursor
	}
	terminalView := a.terminal.View()
	terminalContent := viewString(terminalView)
	a.terminalAreaCache.set(terminalContent, terminalView.Cursor)
	view, cursor, _ := a.terminalAreaCache.get()
	return view, cursor
}

// renderTerminalView renders the full-screen terminal view
func (a *App) renderTerminalView() (string, *tea.Cursor) {
	// Get terminal view
	tv := a.terminal.View()
	terminalView := viewString(tv)
	terminalCursor := tv.Cursor

	// Compose with a one-line toggle message at the bottom. To ensure it's visible,
	// clamp the terminal content to a.height-1 lines.
	toggleMsg := a.renderToggleMessage()
	lines := strings.Split(terminalView, "\n")
	maxTerm := a.height - 1
	if maxTerm < 1 {
		maxTerm = 1
	}
	if len(lines) > maxTerm {
		lines = lines[:maxTerm]
	} else if len(lines) < maxTerm {
		// pad with empty lines to keep layout stable
		pad := make([]string, maxTerm-len(lines))
		lines = append(lines, pad...)
	}
	clamped := strings.Join(lines, "\n")
	combinedView := lipgloss.JoinVertical(lipgloss.Left, clamped, toggleMsg)

	// Adjust cursor position so it never overlaps the toggle message
	if terminalCursor != nil {
		cy := terminalCursor.Y
		if cy >= maxTerm {
			cy = maxTerm - 1
		}
		if cy < 0 {
			cy = 0
		}
		adjusted := tea.NewCursor(terminalCursor.X, cy)
		adjusted.Blink = terminalCursor.Blink
		adjusted.Color = terminalCursor.Color
		adjusted.Shape = terminalCursor.Shape
		return combinedView, adjusted
	}
	return combinedView, nil
}

// refreshFoldersAfterViewChange reapplies the current folders to panels so that
// folder population re-reads the latest panel config.
func (a *App) refreshFoldersAfterViewChange() {
	if a.leftNav != nil {
		cur := a.leftNav.Current()
		ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
		a.setPanelFolder(ctx, 0, cur, a.leftNav.HasBack())
		a.leftPanel.SetCurrentPath(a.navigatorPath(a.leftNav))
		a.leftPanel.RefreshFolder(ctx)
		cancel()
	}
	if a.rightNav != nil {
		cur := a.rightNav.Current()
		ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
		a.setPanelFolder(ctx, 1, cur, a.rightNav.HasBack())
		a.rightPanel.SetCurrentPath(a.navigatorPath(a.rightNav))
		a.rightPanel.RefreshFolder(ctx)
		cancel()
	}
}

// renderFunctionKeys renders the function key bar
func (a *App) renderFunctionKeys() string {
	if view, _, ok := a.functionBarCache.get(); ok {
		return view
	}
	if a.toastActive {
		msg := a.toastText
		maxw := a.width
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
		return toastStyle.Width(a.width).Render(msg)
	}

	var keys []string

	if a.showTerminal {
		keys = []string{uistyles.FunctionKeyStyle.Render("Ctrl+O") + uistyles.FunctionKeyDescriptionStyle.Render("Return to panels")}
	} else {
		panel := a.activePanelRef()
		if panel != nil && panel.HasCommandFocus() {
			keys = []string{uistyles.FunctionKeyStyle.Render("Esc Esc") + uistyles.FunctionKeyDescriptionStyle.Render("Drop focus")}
		} else {
			caps := a.capabilitiesForPanel(panel)
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
	}

	joined := lipgloss.JoinHorizontal(lipgloss.Left, keys...)
	title := " Kubernetes Commander "
	fullWidthStyle := uistyles.FunctionKeyBarStyle.Width(a.width).Align(lipgloss.Left)
	titleStyle := uistyles.FunctionKeyTitleStyle.Align(lipgloss.Center).Width(a.width - lipgloss.Width(joined) - 1)
	titleRendered := titleStyle.Render(title)
	a.functionBarCache.set(fullWidthStyle.Render(joined+" "+titleRendered), nil)
	view, _, _ := a.functionBarCache.get()
	return view
}

// handleFunctionKeyClick maps an x coordinate on the function key bar to a key action.
func (a *App) handleFunctionKeyClick(x int) tea.Cmd {
	if a.toastActive {
		return nil
	}
	if !a.showTerminal {
		if p := a.activePanelRef(); p != nil && p.HasCommandFocus() {
			return nil
		}
	}
	var keys []struct {
		label   string
		enabled bool
		action  func() tea.Cmd
	}
	if a.showTerminal {
		keys = []struct {
			label   string
			enabled bool
			action  func() tea.Cmd
		}{
			{label: uistyles.FunctionKeyStyle.Render("Ctrl+O") + uistyles.FunctionKeyDescriptionStyle.Render("Return to panels"), enabled: true, action: func() tea.Cmd {
				a.showTerminal = false
				a.terminal.SetShowPanels(true)
				a.invalidateView("function bar return to panels")
				a.invalidateFunctionBar("terminal exit")
				a.invalidateTerminalArea("terminal exit")
				return nil
			}},
		}
	} else {
		panel := a.activePanelRef()
		caps := a.capabilitiesForPanel(panel)
		makeLbl := func(key, label string, enabled bool) string {
			desc := uistyles.FunctionKeyDescriptionStyle
			if !enabled {
				desc = uistyles.FunctionKeyDisabledStyle
			}
			return uistyles.FunctionKeyStyle.Render(key) + desc.Render(label)
		}
		invoke := func(action PanelAction) func() tea.Cmd {
			return func() tea.Cmd {
				if panel == nil {
					return nil
				}
				ctx, cancel := context.WithTimeout(a.ctx, panelContextTimeout)
				cmd := panel.invokeActionIfAllowed(ctx, action)
				cancel()
				return cmd
			}
		}
		keys = []struct {
			label   string
			enabled bool
			action  func() tea.Cmd
		}{
			{makeLbl("F1", "Help", caps.HasHelp), caps.HasHelp, invoke(PanelActionHelp)},
			{makeLbl("F2", "Options", caps.HasOptions), caps.HasOptions, invoke(PanelActionOptions)},
			{makeLbl("F3", "View", caps.CanView), caps.CanView, invoke(PanelActionView)},
			{makeLbl("F4", "Edit", caps.CanEdit), caps.CanEdit, invoke(PanelActionEdit)},
			{makeLbl("F5", "Copy", caps.CanCopy), caps.CanCopy, invoke(PanelActionCopy)},
			{makeLbl("F6", "Rename/Move", false), false, a.renameMoveItem},
			{makeLbl("F7", "Namespace", caps.CanCreateNS), caps.CanCreateNS, invoke(PanelActionCreateNamespace)},
			{makeLbl("F8", "Delete", caps.CanDelete), caps.CanDelete, invoke(PanelActionDelete)},
			{uistyles.FunctionKeyStyle.Render("F9") + uistyles.FunctionKeyDescriptionStyle.Render("Commands"), caps.HasContextMenu, invoke(PanelActionMenu)},
			{uistyles.FunctionKeyStyle.Render("F10") + uistyles.FunctionKeyDescriptionStyle.Render("Quit"), true, func() tea.Cmd { return tea.Quit }},
			{uistyles.FunctionKeyStyle.Render("Ctrl+O") + uistyles.FunctionKeyDescriptionStyle.Render("Fullscreen"), true, func() tea.Cmd {
				a.showTerminal = true
				a.terminal.SetShowPanels(false)
				a.invalidateView("function bar fullscreen")
				a.invalidateFunctionBar("terminal enter")
				a.invalidateTerminalArea("terminal enter")
				return nil
			}},
		}
	}

	spans := make([]int, len(keys)+1)
	acc := 0
	for i, k := range keys {
		spans[i] = acc
		acc += lipgloss.Width(k.label)
	}
	spans[len(keys)] = acc

	for i := 0; i < len(keys); i++ {
		if x >= spans[i] && x < spans[i+1] {
			if keys[i].enabled && keys[i].action != nil {
				return keys[i].action()
			}
			return nil
		}
	}
	return nil
}

// renderToggleMessage renders the toggle message for fullscreen mode
func (a *App) renderToggleMessage() string {
	// Create the same layout as function keys
	key := uistyles.FunctionKeyStyle.Render("Ctrl+O") + uistyles.FunctionKeyDescriptionStyle.Render("Return to panels")
	title := uistyles.FunctionKeyTitleStyle.Render("Kubernetes Commander")

	// Calculate the exact spacing needed to push title to the right edge
	spacing := a.width - len(key) - len(title)
	if spacing < 0 {
		spacing = 1 // minimum spacing
	}

	content := key + strings.Repeat(" ", spacing) + title

	// Create a full-width container
	fullWidthStyle := uistyles.FunctionKeyBarStyle.
		Width(a.width).
		Align(lipgloss.Left)

	return fullWidthStyle.Render(content)
}
