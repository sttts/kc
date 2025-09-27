package ui

import (
    tea "github.com/charmbracelet/bubbletea/v2"
    "github.com/charmbracelet/lipgloss/v2"
    "strings"
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
    if !m.visible { return "" }
    // Fullscreen modal styled like panel
    contentW := max(1, m.width-2)
    contentH := max(1, m.height-2)
    if setter, ok := m.content.(interface{ SetDimensions(int, int) }); ok {
        setter.SetDimensions(contentW, contentH)
    }
    // Render content
    var inner string
    if m.content != nil {
        if viewable, ok := m.content.(interface{ View() string }); ok {
            inner = viewable.View()
        } else { inner = "" }
    }
    // Build frame with overlay title (similar to app frame)
    boxStyle := lipgloss.NewStyle().
        Border(lipgloss.NormalBorder()).
        BorderForeground(lipgloss.Color(ColorGrey)).
        Background(lipgloss.Color(ColorDarkerBlue))

    labelStyle := PanelHeaderStyle
    label := labelStyle.Render(m.title)
    border := boxStyle.GetBorderStyle()
    topBorderStyler := lipgloss.NewStyle().
        Foreground(boxStyle.GetBorderTopForeground()).
        Background(boxStyle.GetBorderTopBackground()).
        Render

    topLeft := topBorderStyler(border.TopLeft)
    topRight := topBorderStyler(border.TopRight)
    available := m.width - lipgloss.Width(topLeft+topRight)
    lw := lipgloss.Width(label)
    var top string
    if lw >= available {
        gap := strings.Repeat(border.Top, max(0, available-lw))
        top = topLeft + label + topBorderStyler(gap) + topRight
    } else {
        total := available - lw
        left := total / 2
        right := total - left
        top = topLeft + topBorderStyler(strings.Repeat(border.Top, left)) + label + topBorderStyler(strings.Repeat(border.Top, right)) + topRight
    }

    // Box content under header
    bottom := boxStyle.Copy().
        BorderTop(false).
        Width(m.width).
        Height(m.height-1).
        Render(inner)

    // Replace bottom corners to T junction at the top border of bottom
    lines := strings.Split(bottom, "\n")
    if len(lines) >= 2 {
        // adjust last line corners visually if needed
        last := lines[len(lines)-1]
        last = strings.Replace(last, "└", "├", 1)
        last = strings.Replace(last, "┘", "┤", 1)
        lines[len(lines)-1] = last
    }
    return top + "\n" + strings.Join(lines, "\n")
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
