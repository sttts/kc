package ui

import (
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
)

// Modal represents a modal dialog
type Modal struct {
	title   string
	content tea.Model
	width   int
	height  int
	visible bool
	onClose func() tea.Cmd
}

// Init initializes the modal
func (m *Modal) Init() tea.Cmd {
	return m.content.Init()
}

// NewModal creates a new modal dialog
func NewModal(title string, content tea.Model) *Modal {
	return &Modal{
		title:   title,
		content: content,
		visible: false,
	}
}

// Show shows the modal
func (m *Modal) Show() {
	m.visible = true
}

// Hide hides the modal
func (m *Modal) Hide() {
	m.visible = false
}

// IsVisible returns true if the modal is visible
func (m *Modal) IsVisible() bool {
	return m.visible
}

// SetDimensions sets the modal dimensions
func (m *Modal) SetDimensions(width, height int) {
	m.width = width
	m.height = height
}

// SetOnClose sets the callback for when the modal is closed
func (m *Modal) SetOnClose(callback func() tea.Cmd) {
	m.onClose = callback
}

// Update handles messages and updates the modal state
func (m *Modal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	var cmd tea.Cmd
	var cmds []tea.Cmd

	// Handle modal-specific keys
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.Hide()
			if m.onClose != nil {
				cmd = m.onClose()
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
	}

	// Update the content
	model, cmd := m.content.Update(msg)
	m.content = model
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the modal
func (m *Modal) View() string {
	if !m.visible {
		return ""
	}

	// Calculate modal dimensions (centered)
	modalWidth := m.width * 3 / 4
	modalHeight := m.height * 3 / 4

	// Set content dimensions
	if setter, ok := m.content.(interface{ SetDimensions(int, int) }); ok {
		setter.SetDimensions(modalWidth-4, modalHeight-4) // -4 for border
	}

	// Create border
	border := ModalBorderStyle.
		Width(modalWidth).
		Height(modalHeight).
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2)

	// Create title
	title := ModalTitleStyle.
		Width(modalWidth - 4).
		Height(1).
		Align(lipgloss.Center).
		Render(m.title)

	// Create content
	var content string
	if m.content != nil {
		// Use type assertion to get the View method
		if viewable, ok := m.content.(interface{ View() string }); ok {
			content = viewable.View()
		} else {
			content = "Content not viewable"
		}
	}

	// Combine title and content
	modalContent := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		content,
	)

	// Apply border
	modal := border.Render(modalContent)

	// Center the modal
	centered := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(modal)

	return centered
}

// ModalManager manages multiple modals
type ModalManager struct {
    modals map[string]*Modal
    active string
}

// Init initializes the modal manager
func (mm *ModalManager) Init() tea.Cmd {
	return nil
}

// NewModalManager creates a new modal manager
func NewModalManager() *ModalManager {
	return &ModalManager{
		modals: make(map[string]*Modal),
	}
}

// Register registers a modal
func (mm *ModalManager) Register(name string, modal *Modal) {
	mm.modals[name] = modal
}

// Show shows a modal by name
func (mm *ModalManager) Show(name string) {
	if modal, exists := mm.modals[name]; exists {
		modal.Show()
		mm.active = name
	}
}

// Hide hides the active modal
func (mm *ModalManager) Hide() {
	if mm.active != "" {
		if modal, exists := mm.modals[mm.active]; exists {
			modal.Hide()
		}
		mm.active = ""
	}
}

// IsModalVisible returns true if any modal is visible
func (mm *ModalManager) IsModalVisible() bool {
	return mm.active != ""
}

// GetActiveModal returns the active modal
func (mm *ModalManager) GetActiveModal() *Modal {
	if mm.active != "" {
		return mm.modals[mm.active]
	}
	return nil
}

// Update handles messages and updates the modal manager
func (mm *ModalManager) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if mm.active != "" {
		if modal, exists := mm.modals[mm.active]; exists {
			model, cmd := modal.Update(msg)
			mm.modals[mm.active] = model.(*Modal)

			// Check if modal was closed
			if !modal.IsVisible() {
				mm.active = ""
			}

			return mm, cmd
		}
	}
	return mm, nil
}

// View renders the modal manager
func (mm *ModalManager) View() string {
	if mm.active != "" {
		if modal, exists := mm.modals[mm.active]; exists {
			return modal.View()
		}
	}
	return ""
}
