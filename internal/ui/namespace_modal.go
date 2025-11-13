package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	uistyles "github.com/sttts/kc/internal/ui/styles"
	"k8s.io/apimachinery/pkg/util/validation"
)

// NamespaceCreateResultMsg signals the outcome of the namespace creation dialog.
type NamespaceCreateResultMsg struct {
	Name    string
	Confirm bool
	Close   bool
}

// NamespaceCreateModel configures a TextInputModal for namespace creation.
type NamespaceCreateModel struct {
	modal *TextInputModal
	hint  string
}

// NewNamespaceCreateModel constructs an empty namespace creation dialog model.
func NewNamespaceCreateModel() *NamespaceCreateModel {
	model := &NamespaceCreateModel{
		hint: "Use lowercase letters, digits, or '-' (max 63 chars)",
	}
	confirmStyle := lipgloss.NewStyle().
		Padding(0, 3).
		Background(lipgloss.Color(uistyles.ColorModalButtonBg)).
		Foreground(lipgloss.Color(uistyles.ColorWhite))
	confirmFocused := confirmStyle.Copy().
		Background(lipgloss.Color(uistyles.ColorModalButtonSelBg)).
		Foreground(lipgloss.Color(uistyles.ColorBlack)).
		Bold(true)
	cfg := TextInputModalConfig{
		Title:        "Create Namespace",
		Label:        "Namespace",
		Description:  model.hint,
		ConfirmLabel: "Create",
		CancelLabel:  "Cancel",
		MinWidth:     24,
		MaxWidth:     60,
		MinHeight:    5,
		Validate:     model.validateName,
		ResultMsg:    model.resultMsg,
		HideTitle:    true,
		ButtonStyles: TextInputModalButtonStyles{
			Confirm:        &confirmStyle,
			ConfirmFocused: &confirmFocused,
		},
	}
	model.modal = NewTextInputModal(cfg)
	return model
}

func (m *NamespaceCreateModel) Init() tea.Cmd          { return m.modal.Init() }
func (m *NamespaceCreateModel) SetDimensions(w, h int) { m.modal.SetDimensions(w, h) }
func (m *NamespaceCreateModel) PreferredSize(maxW, maxH int) (int, int) {
	return m.modal.PreferredSize(maxW, maxH)
}
func (m *NamespaceCreateModel) FooterHints() []FooterHint { return m.modal.FooterHints() }

func (m *NamespaceCreateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.modal.Update(msg)
	return m, cmd
}

func (m *NamespaceCreateModel) View() (string, *tea.Cursor) {
	return m.modal.View()
}

// Reset clears the input state.
func (m *NamespaceCreateModel) Reset() {
	m.modal.SetValue("")
	m.modal.SetDescription(m.hint)
	m.modal.ClearError()
	m.modal.FocusInput()
}

func (m *NamespaceCreateModel) validateName(value string) error {
	name := strings.TrimSpace(value)
	if name == "" {
		return fmt.Errorf("Namespace is required")
	}
	if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
		return fmt.Errorf("Invalid namespace: %s", errs[0])
	}
	return nil
}

func (m *NamespaceCreateModel) resultMsg(res TextInputModalResult) tea.Msg {
	return NamespaceCreateResultMsg{
		Name:    strings.TrimSpace(res.Value),
		Confirm: res.Confirm,
		Close:   res.Close,
	}
}
