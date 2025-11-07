package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	uistyles "github.com/sttts/kc/internal/ui/styles"
)

// ViewOptionsCommittedMsg aggregates the selections made in the sectioned view
// options dialog.
type ViewOptionsCommittedMsg struct {
	PanelIndex        int
	PanelMode         PanelViewMode
	SetPanelMode      bool
	PanelWidthPercent int
	SetPanelWidth     bool
	TableMode         string
	HasTableMode      bool
	Resources         *ViewOptionsResourcesPayload
	Objects           *ViewOptionsObjectsPayload
	SaveDefault       bool
	Accept            bool
	Close             bool
}

// ViewOptionsResourcesPayload carries Resources section toggles.
type ViewOptionsResourcesPayload struct {
	HasInclude      bool
	IncludeEmpty    bool
	HasOrder        bool
	Order           string
	ShowTableOption bool
}

// ViewOptionsObjectsPayload carries Objects section toggles.
type ViewOptionsObjectsPayload struct {
	ShowTableOption bool
	Columns         string
	Order           string
}

// ViewOptionsConfig seeds the sectioned view options model.
type ViewOptionsConfig struct {
	PanelIndex int

	PanelModes        []PanelViewMode
	ActivePanelMode   PanelViewMode
	PanelWidthPercent int

	TableMode string

	Resources *ViewOptionsResourcesConfig
	Objects   *ViewOptionsObjectsConfig
}

// ViewOptionsResourcesConfig configures the Resources section.
type ViewOptionsResourcesConfig struct {
	ShowInclude   bool
	IncludeEmpty  bool
	ShowOrder     bool
	Order         string
	ShowTableMode bool
}

// ViewOptionsObjectsConfig configures the Objects section.
type ViewOptionsObjectsConfig struct {
	ShowTableMode bool
	Columns       string
	Order         string
}

type viewOptionEntryKind int

const (
	viewOptionEntryOption viewOptionEntryKind = iota
	viewOptionEntrySection
	viewOptionEntrySpacer
)

type viewOptionKind int

const (
	viewOptionPanelMode viewOptionKind = iota
	viewOptionPanelWidth
	viewOptionIncludeEmpty
	viewOptionResourceOrder
	viewOptionResourceTableMode
	viewOptionObjectTableMode
	viewOptionObjectColumns
	viewOptionObjectOrder
)

type viewOptionEntry struct {
	kind   viewOptionEntryKind
	title  string
	option viewOptionKind
}

// ViewOptionsModel renders the sectioned View Options dialog, covering general
// panel mode settings plus contextual Resources/Objects options.
type ViewOptionsModel struct {
	panelIdx int

	width  int
	height int

	entries []viewOptionEntry
	focus   int
	scroll  int

	panelModes      []PanelViewMode
	panelModeIndex  int
	hasPanelWidth   bool
	panelWidthIndex int

	tableModeIndex int
	hasTableMode   bool

	resources struct {
		enabled    bool
		hasInclude bool
		include    bool
		hasOrder   bool
		orderIndex int
		showTable  bool
	}

	objects struct {
		enabled    bool
		showTable  bool
		columnsIdx int
		orderIdx   int
	}
}

// NewViewOptionsModel constructs a sectioned view options dialog model.
func NewViewOptionsModel(cfg ViewOptionsConfig) *ViewOptionsModel {
	model := &ViewOptionsModel{
		panelIdx:     cfg.PanelIndex,
		panelModes:   append([]PanelViewMode(nil), cfg.PanelModes...),
		hasTableMode: cfg.Resources != nil && cfg.Resources.ShowTableMode || cfg.Objects != nil && cfg.Objects.ShowTableMode,
	}
	if len(model.panelModes) == 0 {
		model.panelModes = []PanelViewMode{PanelModeList}
	}
	model.panelModeIndex = indexOfPanelMode(model.panelModes, cfg.ActivePanelMode)
	model.tableModeIndex = tableModeIndexFor(cfg.TableMode)

	model.entries = append(model.entries, viewOptionEntry{
		kind:   viewOptionEntryOption,
		option: viewOptionPanelMode,
	})
	model.hasPanelWidth = cfg.PanelWidthPercent >= 0
	if model.hasPanelWidth {
		model.panelWidthIndex = viewWidthIndexFor(cfg.PanelWidthPercent)
		model.entries = append(model.entries, viewOptionEntry{
			kind:   viewOptionEntryOption,
			option: viewOptionPanelWidth,
		})
	}

	if cfg.Resources != nil {
		model.resources.enabled = true
		model.resources.hasInclude = cfg.Resources.ShowInclude
		model.resources.include = cfg.Resources.IncludeEmpty
		model.resources.hasOrder = cfg.Resources.ShowOrder
		model.resources.orderIndex = resourceOrderIndexFor(cfg.Resources.Order)
		model.resources.showTable = cfg.Resources.ShowTableMode

		model.appendSectionSpacerIfNeeded()
		model.entries = append(model.entries, viewOptionEntry{
			kind:  viewOptionEntrySection,
			title: "Resources:",
		})
		if model.resources.hasInclude {
			model.entries = append(model.entries, viewOptionEntry{
				kind:   viewOptionEntryOption,
				option: viewOptionIncludeEmpty,
			})
		}
		if model.resources.hasOrder {
			model.entries = append(model.entries, viewOptionEntry{
				kind:   viewOptionEntryOption,
				option: viewOptionResourceOrder,
			})
		}
		if model.resources.showTable {
			model.entries = append(model.entries, viewOptionEntry{
				kind:   viewOptionEntryOption,
				option: viewOptionResourceTableMode,
			})
		}
	}

	if cfg.Objects != nil {
		model.objects.enabled = true
		model.objects.showTable = cfg.Objects.ShowTableMode
		model.objects.columnsIdx = objectColumnsIndexFor(cfg.Objects.Columns)
		model.objects.orderIdx = objectOrderIndexFor(cfg.Objects.Order)

		model.appendSectionSpacerIfNeeded()
		model.entries = append(model.entries, viewOptionEntry{
			kind:  viewOptionEntrySection,
			title: "Objects:",
		})
		if model.objects.showTable {
			model.entries = append(model.entries, viewOptionEntry{
				kind:   viewOptionEntryOption,
				option: viewOptionObjectTableMode,
			})
		}
		model.entries = append(model.entries,
			viewOptionEntry{kind: viewOptionEntryOption, option: viewOptionObjectColumns},
			viewOptionEntry{kind: viewOptionEntryOption, option: viewOptionObjectOrder},
		)
	}

	if len(model.entries) == 0 {
		// Should never happen, but keep at least the panel mode row.
		model.entries = append(model.entries, viewOptionEntry{
			kind:   viewOptionEntryOption,
			option: viewOptionPanelMode,
		})
	}

	model.focus = model.firstOptionIndex()
	model.scroll = 0

	return model
}

func (m *ViewOptionsModel) appendSectionSpacerIfNeeded() {
	if len(m.entries) == 0 {
		return
	}
	last := m.entries[len(m.entries)-1]
	if last.kind == viewOptionEntrySpacer {
		return
	}
	if last.kind == viewOptionEntryOption {
		m.entries = append(m.entries, viewOptionEntry{kind: viewOptionEntrySpacer})
	}
}

func indexOfPanelMode(modes []PanelViewMode, mode PanelViewMode) int {
	for i, candidate := range modes {
		if candidate == mode {
			return i
		}
	}
	return 0
}

func tableModeIndexFor(mode string) int {
	for i, key := range tableModeKeys {
		if strings.EqualFold(key, mode) {
			return i
		}
	}
	return 0
}

func resourceOrderIndexFor(order string) int {
	for i, key := range orderKeys {
		if strings.EqualFold(key, order) {
			return i
		}
	}
	return 0
}

func viewWidthIndexFor(percent int) int {
	options := panelWidthPercentOptions
	if len(options) == 0 {
		return 0
	}
	best := 0
	bestDelta := absInt(options[0] - percent)
	for i := 1; i < len(options); i++ {
		delta := absInt(options[i] - percent)
		if delta < bestDelta {
			best = i
			bestDelta = delta
		}
	}
	return best
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func objectColumnsIndexFor(columns string) int {
	for i, key := range objColumnsKeys {
		if strings.EqualFold(key, columns) {
			return i
		}
	}
	return 0
}

func objectOrderIndexFor(order string) int {
	for i, key := range objOrderKeys {
		if strings.EqualFold(key, order) {
			return i
		}
	}
	return 0
}

// Init implements tea.Model.
func (m *ViewOptionsModel) Init() tea.Cmd { return nil }

// SetDimensions updates the available width and height.
func (m *ViewOptionsModel) SetDimensions(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	m.width = w
	m.height = h
	m.ensureFocusVisible()
}

// ContentHeight returns the number of content rows required.
func (m *ViewOptionsModel) ContentHeight() int { return len(m.entries) }

func (m *ViewOptionsModel) firstOptionIndex() int {
	for i, entry := range m.entries {
		if entry.kind == viewOptionEntryOption {
			return i
		}
	}
	return 0
}

func (m *ViewOptionsModel) currentEntry() (viewOptionEntry, bool) {
	if m.focus < 0 || m.focus >= len(m.entries) {
		return viewOptionEntry{}, false
	}
	return m.entries[m.focus], true
}

func (m *ViewOptionsModel) ensureFocusVisible() {
	if m.height <= 0 {
		return
	}
	if m.focus < m.scroll {
		m.scroll = m.focus
	} else if m.focus >= m.scroll+m.height {
		m.scroll = m.focus - m.height + 1
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
	maxScroll := len(m.entries) - m.height
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
}

func (m *ViewOptionsModel) moveFocus(delta int) {
	if len(m.entries) == 0 {
		return
	}
	idx := m.focus
	for {
		idx += delta
		if idx < 0 {
			idx = 0
			break
		}
		if idx >= len(m.entries) {
			idx = len(m.entries) - 1
			break
		}
		if m.entries[idx].kind == viewOptionEntryOption {
			break
		}
	}
	m.focus = idx
	m.ensureFocusVisible()
}

// Update handles key navigation and edits.
func (m *ViewOptionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch t := msg.(type) {
	case tea.KeyMsg:
		switch t.String() {
		case "up", "k":
			m.moveFocus(-1)
		case "down", "j":
			m.moveFocus(1)
		case "left":
			m.adjustCurrent(-1)
		case "right":
			m.adjustCurrent(1)
		case " ", "space":
			m.adjustCurrent(0)
		case "ctrl+s":
			return m, func() tea.Msg {
				return m.commit(false, true)
			}
		case "enter":
			return m, func() tea.Msg {
				return m.commit(true, false)
			}
		}
	}
	return m, nil
}

func (m *ViewOptionsModel) adjustCurrent(delta int) {
	entry, ok := m.currentEntry()
	if !ok || entry.kind != viewOptionEntryOption {
		return
	}
	switch entry.option {
	case viewOptionPanelMode:
		if len(m.panelModes) == 0 {
			return
		}
		if delta < 0 {
			m.panelModeIndex = (m.panelModeIndex - 1 + len(m.panelModes)) % len(m.panelModes)
		} else {
			m.panelModeIndex = (m.panelModeIndex + 1) % len(m.panelModes)
		}
	case viewOptionPanelWidth:
		options := panelWidthPercentOptions
		if !m.hasPanelWidth || len(options) == 0 {
			return
		}
		switch {
		case delta < 0:
			if m.panelWidthIndex > 0 {
				m.panelWidthIndex--
			}
		case delta > 0:
			if m.panelWidthIndex < len(options)-1 {
				m.panelWidthIndex++
			}
		default:
			if m.panelWidthIndex < len(options)-1 {
				m.panelWidthIndex++
			}
		}
	case viewOptionIncludeEmpty:
		m.resources.include = !m.resources.include
	case viewOptionResourceOrder:
		if len(orderKeys) == 0 {
			return
		}
		if delta < 0 {
			m.resources.orderIndex--
			if m.resources.orderIndex < 0 {
				m.resources.orderIndex = len(orderKeys) - 1
			}
		} else {
			m.resources.orderIndex = (m.resources.orderIndex + 1) % len(orderKeys)
		}
	case viewOptionResourceTableMode, viewOptionObjectTableMode:
		if len(tableModeKeys) == 0 {
			return
		}
		if delta < 0 {
			m.tableModeIndex--
			if m.tableModeIndex < 0 {
				m.tableModeIndex = len(tableModeKeys) - 1
			}
		} else {
			m.tableModeIndex = (m.tableModeIndex + 1) % len(tableModeKeys)
		}
	case viewOptionObjectColumns:
		if len(objColumnsKeys) == 0 {
			return
		}
		m.objects.columnsIdx++
		if m.objects.columnsIdx >= len(objColumnsKeys) {
			m.objects.columnsIdx = 0
		}
	case viewOptionObjectOrder:
		if len(objOrderKeys) == 0 {
			return
		}
		if delta < 0 {
			m.objects.orderIdx--
			if m.objects.orderIdx < 0 {
				m.objects.orderIdx = len(objOrderKeys) - 1
			}
		} else {
			m.objects.orderIdx = (m.objects.orderIdx + 1) % len(objOrderKeys)
		}
	}
}

func (m *ViewOptionsModel) commit(accept, save bool) ViewOptionsCommittedMsg {
	msg := ViewOptionsCommittedMsg{
		PanelIndex:    m.panelIdx,
		SetPanelMode:  true,
		PanelMode:     m.panelModes[m.panelModeIndex],
		SetPanelWidth: m.hasPanelWidth,
		PanelWidthPercent: func() int {
			options := panelWidthPercentOptions
			if !m.hasPanelWidth || len(options) == 0 {
				return 0
			}
			return options[m.panelWidthIndex]
		}(),
		TableMode:    tableModeKeys[m.tableModeIndex],
		HasTableMode: m.hasTableMode,
		SaveDefault:  save,
		Accept:       accept,
		Close:        accept,
	}
	if !save {
		// Save keeps the dialog open in parity with previous behaviour.
		msg.Close = accept
	}
	if m.resources.enabled {
		msg.Resources = &ViewOptionsResourcesPayload{
			HasInclude:      m.resources.hasInclude,
			IncludeEmpty:    m.resources.include,
			HasOrder:        m.resources.hasOrder,
			Order:           orderKeys[m.resources.orderIndex],
			ShowTableOption: m.resources.showTable,
		}
	}
	if m.objects.enabled {
		msg.Objects = &ViewOptionsObjectsPayload{
			ShowTableOption: m.objects.showTable,
			Columns:         objColumnsKeys[m.objects.columnsIdx],
			Order:           objOrderKeys[m.objects.orderIdx],
		}
	}
	return msg
}

func (m *ViewOptionsModel) optionLabelAndValue(opt viewOptionKind) (string, string) {
	switch opt {
	case viewOptionPanelMode:
		return "Panel mode", modeLabel(m.panelModes[m.panelModeIndex])
	case viewOptionPanelWidth:
		options := panelWidthPercentOptions
		if !m.hasPanelWidth || len(options) == 0 {
			return "Panel width", ""
		}
		return "Panel width", fmt.Sprintf("%d%%", options[m.panelWidthIndex])
	case viewOptionIncludeEmpty:
		val := "No"
		if m.resources.include {
			val = "Yes"
		}
		return "Include empty", val
	case viewOptionResourceOrder:
		return "Order", orderLabels[m.resources.orderIndex]
	case viewOptionResourceTableMode, viewOptionObjectTableMode:
		return "Table mode", tableModeLabels[m.tableModeIndex]
	case viewOptionObjectColumns:
		return "Columns", objColumnsLabels[m.objects.columnsIdx]
	case viewOptionObjectOrder:
		return "Objects order", objOrderLabels[m.objects.orderIdx]
	default:
		return "", ""
	}
}

func (m *ViewOptionsModel) maxLabelWidth() int {
	maxWidth := 0
	for _, entry := range m.entries {
		if entry.kind != viewOptionEntryOption {
			continue
		}
		label, _ := m.optionLabelAndValue(entry.option)
		if w := lipgloss.Width(label); w > maxWidth {
			maxWidth = w
		}
	}
	return maxWidth
}

// View renders the dialog body (without frame).
func (m *ViewOptionsModel) View() string {
	if m.height <= 0 {
		m.height = len(m.entries)
		if m.height == 0 {
			m.height = 1
		}
	}
	rowStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Foreground(lipgloss.Color(uistyles.ColorModalFg))
	focusStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(uistyles.ColorModalSelBg)).
		Foreground(lipgloss.Color(uistyles.ColorModalFg)).
		Bold(true)
	sectionStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(uistyles.ColorModalBg)).
		Foreground(lipgloss.Color(uistyles.ColorModalFg)).
		Bold(true)

	maxLabel := m.maxLabelWidth()
	lines := make([]string, 0, m.height)
	for i := 0; i < m.height; i++ {
		idx := m.scroll + i
		if idx < 0 || idx >= len(m.entries) {
			lines = append(lines, rowStyle.Width(m.width).Render(""))
			continue
		}
		entry := m.entries[idx]
		switch entry.kind {
		case viewOptionEntrySpacer:
			lines = append(lines, rowStyle.Width(m.width).Render(""))
		case viewOptionEntrySection:
			lines = append(lines, sectionStyle.Width(m.width).Render(entry.title))
		case viewOptionEntryOption:
			label, value := m.optionLabelAndValue(entry.option)
			marker := " "
			st := rowStyle
			if idx == m.focus {
				marker = ">"
				st = focusStyle
			}
			if w := lipgloss.Width(label); w < maxLabel {
				label += strings.Repeat(" ", maxLabel-w)
			}
			line := marker + " " + label + ": " + value
			lines = append(lines, st.Width(m.width).Render(line))
		}
	}
	return strings.Join(lines, "\n")
}

// FooterHints exposes the keyboard hints for the modal footer.
func (m *ViewOptionsModel) FooterHints() []FooterHint {
	return []FooterHint{
		{Key: "Up/Down", Label: "Move", Enabled: true},
		{Key: "Left/Right/Space", Label: "Toggle", Enabled: true},
		{Key: "Ctrl+S", Label: "Save as defaults", Enabled: true},
		{Key: "Enter", Label: "Apply & Close", Enabled: true},
		{Key: "Esc", Label: "Cancel", Enabled: true},
	}
}

// PanelIndex identifies the panel the dialog was opened for.
func (m *ViewOptionsModel) PanelIndex() int { return m.panelIdx }
