package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sttts/kc/internal/ui/styles"
	"github.com/sttts/kc/pkg/appconfig"
)

type CommandSelectorModel struct {
	entries  []commandSelectorEntry
	selected int
	scroll   int
	width    int
	height   int
	onSelect func(appconfig.CommandConfig) tea.Cmd
	onCancel func() tea.Cmd
}

type commandSelectorEntry struct {
	header  string
	command *appconfig.CommandConfig
}

func NewCommandSelectorModel(commands []appconfig.CommandConfig, onSelect func(appconfig.CommandConfig) tea.Cmd, onCancel func() tea.Cmd) *CommandSelectorModel {
	entries := buildCommandEntries(commands)
	return &CommandSelectorModel{
		entries:  entries,
		selected: firstSelectable(entries),
		scroll:   0,
		onSelect: onSelect,
		onCancel: onCancel,
	}
}

func (m *CommandSelectorModel) Init() tea.Cmd { return nil }

func (m *CommandSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.onCancel != nil {
				return m, m.onCancel()
			}
			return m, nil
		case "up", "k":
			m.moveSelection(-1)
		case "down", "j":
			m.moveSelection(1)
		case "enter":
			if m.selected >= 0 && m.selected < len(m.entries) && m.entries[m.selected].command != nil {
				return m, m.onSelect(*m.entries[m.selected].command)
			}
		}
	}
	return m, nil
}

func (m *CommandSelectorModel) View() tea.View {
	var s strings.Builder

	if len(m.entries) == 0 {
		return tea.NewView("No commands available")
	}

	if m.height <= 0 {
		m.height = len(m.entries)
	}
	visible := max(1, m.height)

	sectionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(styles.ColorModalFg)).
		Background(lipgloss.Color(styles.ColorModalBg)).
		Bold(true)
	rowStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(styles.ColorModalBg)).
		Foreground(lipgloss.Color(styles.ColorModalFg))
	focusStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(styles.ColorModalSelBg)).
		Foreground(lipgloss.Color(styles.ColorModalFg)).
		Bold(true)

	start := m.scroll
	end := start + visible
	if end > len(m.entries) {
		end = len(m.entries)
	}

	for i := start; i < end; i++ {
		entry := m.entries[i]
		if entry.header != "" {
			s.WriteString(sectionStyle.Width(m.width).Render(entry.header))
			if i < end-1 {
				s.WriteString("\n")
			}
			continue
		}
		if entry.command == nil {
			continue
		}
		cursor := "  "
		style := rowStyle
		if i == m.selected {
			cursor = "> "
			style = focusStyle
		}
		name := entry.command.Name
		line := fmt.Sprintf("%s%s", cursor, name)
		s.WriteString(style.Width(m.width).Render(line))
		if i < end-1 {
			s.WriteString("\n")
		}
	}

	return tea.NewView(s.String())
}

func (m *CommandSelectorModel) SetDimensions(w, h int) {
	m.width = w
	m.height = h
	m.ensureSelectionVisible()
}

func (m *CommandSelectorModel) Filter(query string) {
	// TODO: Implement filtering if needed
}

func (m *CommandSelectorModel) moveSelection(delta int) {
	if len(m.entries) == 0 {
		return
	}
	next := m.selected
	for {
		next += delta
		if next < 0 || next >= len(m.entries) {
			break
		}
		if m.entries[next].command != nil {
			m.selected = next
			break
		}
	}
	m.ensureSelectionVisible()
}

func buildCommandEntries(commands []appconfig.CommandConfig) []commandSelectorEntry {
	grouped := map[appconfig.CommandType][]appconfig.CommandConfig{}
	for _, c := range commands {
		grouped[c.Type] = append(grouped[c.Type], c)
	}
	order := []appconfig.CommandType{
		appconfig.CommandTypeGlobal,
		appconfig.CommandTypeNamespace,
		appconfig.CommandTypeSticky,
		appconfig.CommandTypeSelector,
	}
	var entries []commandSelectorEntry
	for _, t := range order {
		cmds := grouped[t]
		if len(cmds) == 0 {
			continue
		}
		sort.Slice(cmds, func(i, j int) bool {
			return cmds[i].Name < cmds[j].Name
		})
		entries = append(entries, commandSelectorEntry{header: sectionTitle(t)})
		for i := range cmds {
			cmd := cmds[i]
			entries = append(entries, commandSelectorEntry{command: &cmd})
		}
	}
	return entries
}

func sectionTitle(t appconfig.CommandType) string {
	switch t {
	case appconfig.CommandTypeGlobal:
		return "Global Commands"
	case appconfig.CommandTypeNamespace:
		return "Namespace Commands"
	case appconfig.CommandTypeSticky:
		return "Sticky Commands"
	case appconfig.CommandTypeSelector:
		return "Selector Commands"
	default:
		return "Commands"
	}
}

func firstSelectable(entries []commandSelectorEntry) int {
	for i, e := range entries {
		if e.command != nil {
			return i
		}
	}
	return 0
}

// PreferredSize implements ModalSizer to suggest a compact size.
func (m *CommandSelectorModel) PreferredSize(maxContentWidth, maxContentHeight int) (int, int) {
	maxLen := 0
	for _, e := range m.entries {
		if e.header != "" {
			maxLen = max(maxLen, lipgloss.Width(e.header))
			continue
		}
		if e.command != nil {
			line := fmt.Sprintf("> %s", e.command.Name)
			if w := lipgloss.Width(line); w > maxLen {
				maxLen = w
			}
		}
	}
	width := min(maxContentWidth, maxLen+2)
	height := min(maxContentHeight, max(len(m.entries), 6))
	return width, height
}

// FooterHints renders hints consistent with other modals.
func (m *CommandSelectorModel) FooterHints() []FooterHint {
	return []FooterHint{
		{Key: "Up/Down", Label: "Move", Enabled: true},
		{Key: "Enter", Label: "Run", Enabled: true},
		{Key: "Esc", Label: "Close", Enabled: true},
	}
}

func (m *CommandSelectorModel) ensureSelectionVisible() {
	if m.height <= 0 {
		return
	}
	visible := max(1, m.height)
	if m.selected < m.scroll {
		m.scroll = m.selected
	}
	if m.selected >= m.scroll+visible {
		m.scroll = m.selected - visible + 1
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}
