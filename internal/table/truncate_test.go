package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss/v2"
)

func TestAnsiTruncatePadWidth(t *testing.T) {
	st := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0"))
	colored := st.Render("abcdefghijklmnopqrstuvwxyz")
	for w := 1; w <= 30; w++ {
		out := ansiTruncatePad(colored, w)
		if lipgloss.Width(out) != w {
			t.Fatalf("width=%d got=%d for %q", w, lipgloss.Width(out), out)
		}
	}
}

func TestAnsiTruncatePadKeepsANSI(t *testing.T) {
	st := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0")).Bold(true)
	colored := st.Render("hello world")
	out := ansiTruncatePad(colored, 8)
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI sequences to remain")
	}
	if lipgloss.Width(out) != 8 {
		t.Fatalf("expected width 8, got %d", lipgloss.Width(out))
	}
}

func TestAnsiTruncatePadShortTail(t *testing.T) {
	out := ansiTruncatePad("abc", 2)
	if lipgloss.Width(out) != 2 {
		t.Fatalf("expected width 2, got %d", lipgloss.Width(out))
	}
}
