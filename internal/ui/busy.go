package ui

import (
    "time"
    tea "github.com/charmbracelet/bubbletea/v2"
)

// BusyShowMsg requests the app to show a spinner with a given token.
type BusyShowMsg struct{ token int }
// BusyTickMsg advances the spinner animation while active.
type BusyTickMsg struct{}
// BusyHideMsg requests the app to hide the spinner for a given token.
type BusyHideMsg struct{ token int }

// busyDoneMsg is emitted after work completes to hide spinner and forward
// the original result message.
type busyDoneMsg struct{ token int; msg tea.Msg }

// showToastMsg displays a transient notification for the given TTL.
type showToastMsg struct{ text string; ttl time.Duration }
type toastTickMsg struct{}

// ShowToast returns a Cmd to display a transient notification.
func (a *App) ShowToast(text string, ttl time.Duration) tea.Cmd {
    return func() tea.Msg { return showToastMsg{text: text, ttl: ttl} }
}

// withBusy runs work in a background command and shows a spinner if it takes
// longer than delay. When work completes, it hides the spinner.
func (a *App) withBusy(label string, delay time.Duration, work func() tea.Msg) tea.Cmd {
    tok := a.busyToken + 1
    a.busyToken = tok
    a.busyLabel = label
    show := tea.Tick(delay, func(time.Time) tea.Msg { return BusyShowMsg{token: tok} })
    run := func() tea.Msg { return busyDoneMsg{token: tok, msg: work()} }
    return tea.Batch(show, run)
}

// startBusyImmediately shows the spinner right away (no delay).
func (a *App) startBusyImmediately(label string) tea.Cmd {
    a.busyLabel = label
    a.busyActive = true
    a.busyFrame = 0
    return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return BusyTickMsg{} })
}
