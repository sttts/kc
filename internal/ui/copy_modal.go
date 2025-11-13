package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	uistyles "github.com/sttts/kc/internal/ui/styles"
)

// CopyToLocalResultMsg captures the outcome of the copy dialog.
type CopyToLocalResultMsg struct {
	Path    string
	Confirm bool
	Close   bool
}

// CopyToLocalModel configures a TextInputModal for file path selection.
type CopyToLocalModel struct {
	modal         *TextInputModal
	hint          string
	lastValidPath string
}

// NewCopyToLocalModel constructs an empty copy dialog model.
func NewCopyToLocalModel() *CopyToLocalModel {
	model := &CopyToLocalModel{
		hint: "Enter an absolute file path. Existing files will be overwritten.",
	}
	confirmStyle := lipgloss.NewStyle().
		Padding(0, 3).
		Background(lipgloss.Color(uistyles.ColorModalButtonBg)).
		Foreground(lipgloss.Color(uistyles.ColorGrey))
	confirmFocused := confirmStyle.Copy().
		Background(lipgloss.Color(uistyles.ColorModalButtonSelBg)).
		Foreground(lipgloss.Color(uistyles.ColorBlack)).
		Bold(true)
	cfg := TextInputModalConfig{
		Title:        "Copy to Local File",
		Label:        "File path",
		Description:  model.hint,
		ConfirmLabel: "Copy",
		CancelLabel:  "Cancel",
		MinWidth:     24,
		MaxWidth:     80,
		MinHeight:    6,
		Validate:     model.validatePath,
		ResultMsg:    model.resultMsg,
		ButtonStyles: TextInputModalButtonStyles{
			Confirm:        &confirmStyle,
			ConfirmFocused: &confirmFocused,
		},
	}
	model.modal = NewTextInputModal(cfg)
	return model
}

func (m *CopyToLocalModel) Init() tea.Cmd          { return m.modal.Init() }
func (m *CopyToLocalModel) SetDimensions(w, h int) { m.modal.SetDimensions(w, h) }
func (m *CopyToLocalModel) PreferredSize(maxW, maxH int) (int, int) {
	return m.modal.PreferredSize(maxW, maxH)
}
func (m *CopyToLocalModel) FooterHints() []FooterHint { return m.modal.FooterHints() }

func (m *CopyToLocalModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.modal.Update(msg)
	return m, cmd
}

func (m *CopyToLocalModel) View() (string, *tea.Cursor) {
	return m.modal.View()
}

// Configure sets the dialog subject and default path.
func (m *CopyToLocalModel) Configure(subject, path string) {
	title := fmt.Sprintf("Copy %s to local file", strings.TrimSpace(subject))
	m.modal.SetTitle(title)
	m.modal.SetDescription(m.hint)
	m.modal.SetValue(m.defaultPath(path))
	m.modal.ClearError()
	m.lastValidPath = ""
}

// FocusPath focuses the file path input.
func (m *CopyToLocalModel) FocusPath() tea.Cmd { return m.modal.FocusInput() }

// BlurInputs blurs the input control.
func (m *CopyToLocalModel) BlurInputs() { m.modal.BlurInput() }

func (m *CopyToLocalModel) defaultPath(path string) string {
	value := expandedPath(strings.TrimSpace(path))
	if value == "" {
		value = "resource.txt"
	}
	if abs, err := filepath.Abs(value); err == nil {
		value = abs
	}
	return value
}

func (m *CopyToLocalModel) validatePath(value string) error {
	normalized, err := m.normalizePath(value)
	if err != nil {
		m.lastValidPath = ""
		return err
	}
	m.lastValidPath = normalized
	return nil
}

func (m *CopyToLocalModel) normalizePath(value string) (string, error) {
	target := expandedPath(strings.TrimSpace(value))
	if target == "" {
		return "", fmt.Errorf("File path is required")
	}
	absPath, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("File path: %w", err)
	}
	if !filepath.IsAbs(absPath) {
		return "", fmt.Errorf("File path must be absolute")
	}
	dir := filepath.Dir(absPath)
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("Directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("Directory is not valid")
	}
	if info, err := os.Stat(absPath); err == nil && info.IsDir() {
		return "", fmt.Errorf("File path points to a directory")
	}
	return absPath, nil
}

func (m *CopyToLocalModel) resultMsg(res TextInputModalResult) tea.Msg {
	path := m.lastValidPath
	if path == "" {
		if normalized, err := m.normalizePath(res.Value); err == nil {
			path = normalized
		} else {
			path = res.Value
		}
	}
	return CopyToLocalResultMsg{
		Path:    path,
		Confirm: res.Confirm,
		Close:   res.Close,
	}
}

func expandedPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if value[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			if value == "~" {
				return home
			}
			if len(value) > 1 && (value[1] == '/' || value[1] == '\\') {
				return filepath.Join(home, value[2:])
			}
		}
	}
	return value
}
