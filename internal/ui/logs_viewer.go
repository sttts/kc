package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/sttts/kc/internal/ui/viewer"
)

type logStreamMsg struct {
	lines []string
	done  bool
	err   error
}

// LogsViewer renders a streaming pod log in a fullscreen modal.
type LogsViewer struct {
	inner      *viewer.Widget
	width      int
	height     int
	reader     io.ReadCloser
	buffer     *bufio.Reader
	cancel     context.CancelFunc
	autoFollow bool
	done       bool
	err        error
	onTheme    func() tea.Cmd
	onClose    func() tea.Cmd
}

func NewLogsViewer(title string, reader io.ReadCloser, cancel context.CancelFunc, theme string) *LogsViewer {
	w := viewer.New(theme)
	w.SetPlainMode(true)
	w.SetContent("", viewer.Metadata{
		Title:    title,
		MimeType: "text/plain",
		Filename: strings.ReplaceAll(title, "/", "_") + ".log",
	})
	lv := &LogsViewer{
		inner:      w,
		reader:     reader,
		buffer:     bufio.NewReader(reader),
		cancel:     cancel,
		autoFollow: true,
	}
	lv.refreshCallbacks()
	return lv
}

func (v *LogsViewer) Init() tea.Cmd {
	if v.reader == nil || v.done {
		return nil
	}
	return v.readNextChunk()
}

func (v *LogsViewer) SetDimensions(w, h int) {
	v.width = w
	v.height = h
}

func (v *LogsViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case logStreamMsg:
		if len(m.lines) > 0 {
			v.inner.AppendLines(m.lines)
			if v.autoFollow || v.inner.AtEnd() {
				v.inner.ScrollToEnd()
				v.autoFollow = true
			}
		}
		if m.err != nil && !errors.Is(m.err, io.EOF) && !errors.Is(m.err, context.Canceled) {
			v.err = m.err
			v.done = true
			return v, nil
		}
		if m.done {
			v.done = true
			return v, nil
		}
		if v.reader != nil {
			return v, v.readNextChunk()
		}
	case tea.KeyMsg, tea.MouseWheelMsg:
		if cmd, handled := v.inner.Update(m); handled {
			v.autoFollow = v.inner.AtEnd()
			return v, cmd
		}
	default:
		if cmd, handled := v.inner.Update(msg); handled {
			v.autoFollow = v.inner.AtEnd()
			return v, cmd
		}
	}
	return v, nil
}

func (v *LogsViewer) View() string {
	if v.height <= 0 || v.width <= 0 {
		return ""
	}
	return v.inner.View(viewer.Frame{Width: v.width, Height: v.height, Focused: true})
}

func (v *LogsViewer) FooterHints() []FooterHint {
	if v.inner.SearchMode() {
		return []FooterHint{
			{Key: "Enter", Label: "Search", Enabled: true},
			{Key: "Esc", Label: "Cancel", Enabled: true},
		}
	}
	return []FooterHint{
		{Key: "F2", Label: "Next", Enabled: v.inner.HasSearchMatches()},
		{Key: "F6", Label: "Theme", Enabled: v.onTheme != nil},
		{Key: "F7", Label: "Search", Enabled: true},
		{Key: "F10", Label: "Close", Enabled: v.onClose != nil},
	}
}

func (v *LogsViewer) FooterStatus(width int) string {
	if width <= 0 {
		return ""
	}
	base := v.inner.FooterStatusText(width)
	state := "FOLLOW"
	if !v.autoFollow {
		state = "PAUSED"
	}
	status := appendFooterStatus(base, state, width)
	switch {
	case v.err != nil:
		status = appendFooterStatus(status, fmt.Sprintf("ERR: %v", v.err), width)
	case v.done:
		status = appendFooterStatus(status, "END", width)
	}
	return status
}

func (v *LogsViewer) FooterCursor(width int) *tea.Cursor {
	return v.inner.FooterCursor(width)
}

func (v *LogsViewer) HandleModalEscape(key tea.KeyMsg) (bool, tea.Cmd) {
	if key.String() != "esc" {
		return false, nil
	}
	if cmd, handled := v.inner.HandleEscape(); handled {
		return true, cmd
	}
	return false, nil
}

func (v *LogsViewer) SetOnTheme(fn func() tea.Cmd) {
	v.onTheme = fn
	v.refreshCallbacks()
}

func (v *LogsViewer) SetOnClose(fn func() tea.Cmd) {
	v.onClose = fn
	v.refreshCallbacks()
}

func (v *LogsViewer) SetTheme(name string) {
	v.inner.SetTheme(name)
}

func (v *LogsViewer) Close() {
	if v.reader == nil && v.cancel == nil {
		return
	}
	v.done = true
	if v.cancel != nil {
		v.cancel()
		v.cancel = nil
	}
	if v.reader != nil {
		_ = v.reader.Close()
		v.reader = nil
		v.buffer = nil
	}
}

func (v *LogsViewer) readNextChunk() tea.Cmd {
	if v.reader == nil || v.done {
		return nil
	}
	if v.buffer == nil {
		return nil
	}
	return func() tea.Msg {
		line, err := v.buffer.ReadString('\n')
		if len(line) > 0 {
			text := strings.TrimRight(line, "\r\n")
			msg := logStreamMsg{lines: []string{text}}
			if errors.Is(err, io.EOF) {
				msg.done = true
			}
			return msg
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return logStreamMsg{done: true}
			}
			return logStreamMsg{err: err}
		}
		return logStreamMsg{}
	}
}

func appendFooterStatus(base, extra string, width int) string {
	if width <= 0 {
		return ""
	}
	if extra == "" {
		return trimStatusToWidth(base, width)
	}
	if base == "" {
		return trimStatusToWidth(extra, width)
	}
	combined := fmt.Sprintf("%s • %s", base, extra)
	return trimStatusToWidth(combined, width)
}

func trimStatusToWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}
	display := lipgloss.Width(text)
	if display <= width {
		return text
	}
	if width <= 1 {
		return "…"
	}
	limit := width - 1
	var b strings.Builder
	current := 0
	for _, r := range text {
		seg := string(r)
		rw := lipgloss.Width(seg)
		if current+rw > limit {
			break
		}
		b.WriteRune(r)
		current += rw
	}
	return b.String() + "…"
}

func (v *LogsViewer) refreshCallbacks() {
	if v.inner == nil {
		return
	}
	v.inner.SetCallbacks(nil, func() tea.Cmd {
		if v.onTheme != nil {
			return v.onTheme()
		}
		return nil
	}, func() tea.Cmd {
		v.Close()
		if v.onClose != nil {
			return v.onClose()
		}
		return nil
	})
}
