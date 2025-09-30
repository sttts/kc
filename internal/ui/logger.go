package ui

import (
    "fmt"
    "strings"
    "sync"
    "time"
    tea "github.com/charmbracelet/bubbletea/v2"
)

// ToastLogger is a minimal adapter that wraps log-style calls and emits
// transient toasts in the UI for errors, with rate limiting to avoid storms.
type ToastLogger struct {
    app         *App
    mu          sync.Mutex
    lastToast   time.Time
    minInterval time.Duration
    lastText    string
}

func NewToastLogger(app *App, minInterval time.Duration) *ToastLogger {
    return &ToastLogger{app: app, minInterval: minInterval}
}

// Errorf shows a red toast for the formatted error message if allowed by the
// rate limiter. It also returns the message as a Cmd for easy composition.
func (l *ToastLogger) Errorf(format string, args ...any) tea.Cmd {
    msg := fmt.Sprintf(format, args...)
    msg = strings.TrimSpace(msg)
    l.mu.Lock()
    now := time.Now()
    // Suppress duplicates of the same text for 30s to avoid storms.
    suppressDup := (msg == l.lastText) && now.Sub(l.lastToast) < 30*time.Second
    allow := now.Sub(l.lastToast) >= l.minInterval && !suppressDup
    if allow {
        l.lastToast = now
        l.lastText = msg
    }
    l.mu.Unlock()
    if !allow {
        return nil
    }
    return l.app.ShowToast(msg, 5*time.Second)
}
