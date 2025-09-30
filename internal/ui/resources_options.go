package ui

import (
    "strings"
    tea "github.com/charmbracelet/bubbletea/v2"
    "github.com/charmbracelet/lipgloss/v2"
)

// ResourcesOptionsModel is the content for the F2 Resources dialog.
// It exposes two toggles:
// - Show only non-empty resources
// - Order: alpha | group | favorites
// It emits ResourcesOptionsChangedMsg on changes and on Enter (with Close=true).
type ResourcesOptionsModel struct {
    width, height int
    // state
    focus int // 0: include empty toggle, 1: order, 2: table mode, 3: columns
    includeEmpty bool
    orderIdx int // 0=alpha, 1=group, 2=favorites
    modeIdx int  // 0=scroll, 1=fit
    columnsIdx int // 0=normal, 1=wide
    objOrderIdx int // 0=name,1=-name,2=creation,3=-creation
}

var orderLabels = []string{"Alphabetic", "Group", "Favorites"}
var orderKeys   = []string{"alpha", "group", "favorites"}
var modeLabels  = []string{"Scroll", "Fit"}
var modeKeys    = []string{"scroll", "fit"}
var columnsLabels = []string{"Normal", "Wide"}
var columnsKeys   = []string{"normal", "wide"}

func NewResourcesOptionsModel(showNonEmpty bool, order string) *ResourcesOptionsModel {
    idx := 0
    switch order {
    case "group": idx = 1
    case "favorites": idx = 2
    default: idx = 0
    }
    return &ResourcesOptionsModel{includeEmpty: !showNonEmpty, orderIdx: idx, modeIdx: 0, columnsIdx: 0, objOrderIdx: 0}
}

func (m *ResourcesOptionsModel) Init() tea.Cmd { return nil }
func (m *ResourcesOptionsModel) SetDimensions(w, h int) { m.width, m.height = w, h }

// ResourcesOptionsChangedMsg notifies the App of a change/persist/close.
type ResourcesOptionsChangedMsg struct {
    ShowNonEmptyOnly bool
    Order string
    TableMode string // "scroll" or "fit"
    Columns string   // "normal" or "wide"
    ObjectsOrder string // name|-name|creation|-creation
    Accept bool // true when user confirmed (Enter)
    Close bool  // request to close the dialog
    SaveDefault bool // persist current values as defaults in config
}

func (m *ResourcesOptionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch t := msg.(type) {
    case tea.KeyMsg:
        switch t.String() {
        case "up", "k":
            if m.focus > 0 { m.focus-- }
        case "down", "j":
            if m.focus < 4 { m.focus++ }
        case "left", "right", " ", "space":
            if m.focus == 0 {
                m.includeEmpty = !m.includeEmpty
            } else if m.focus == 1 {
                if t.String() == "left" {
                    if m.orderIdx > 0 { m.orderIdx-- } else { m.orderIdx = len(orderKeys)-1 }
                } else {
                    if m.orderIdx < len(orderKeys)-1 { m.orderIdx++ } else { m.orderIdx = 0 }
                }
            } else if m.focus == 2 { // table mode
                if t.String() == "left" || t.String() == "right" || t.String() == " " || t.String() == "space" {
                    if m.modeIdx == 0 { m.modeIdx = 1 } else { m.modeIdx = 0 }
                }
            } else if m.focus == 3 { // columns mode
                if t.String() == "left" || t.String() == "right" || t.String() == " " || t.String() == "space" {
                    if m.columnsIdx == 0 { m.columnsIdx = 1 } else { m.columnsIdx = 0 }
                }
            } else { // objects order
                if t.String() == "left" {
                    if m.objOrderIdx > 0 { m.objOrderIdx-- } else { m.objOrderIdx = len(objOrderKeys)-1 }
                } else {
                    if m.objOrderIdx < len(objOrderKeys)-1 { m.objOrderIdx++ } else { m.objOrderIdx = 0 }
                }
            }
            // No immediate apply; wait for Enter
            return m, nil
        case "ctrl+s":
            // Save as defaults (persist to config) but do not close
            return m, func() tea.Msg { return ResourcesOptionsChangedMsg{ShowNonEmptyOnly: !m.includeEmpty, Order: orderKeys[m.orderIdx], TableMode: modeKeys[m.modeIdx], Columns: columnsKeys[m.columnsIdx], ObjectsOrder: objOrderKeys[m.objOrderIdx], SaveDefault: true} }
        case "enter":
            return m, func() tea.Msg { return ResourcesOptionsChangedMsg{ShowNonEmptyOnly: !m.includeEmpty, Order: orderKeys[m.orderIdx], TableMode: modeKeys[m.modeIdx], Columns: columnsKeys[m.columnsIdx], ObjectsOrder: objOrderKeys[m.objOrderIdx], Accept: true, Close: true} }
        }
    }
    return m, nil
}

func (m *ResourcesOptionsModel) View() string {
    labels := []string{"Include empty", "Order", "Table mode", "Columns", "Objects order"}
    values := []string{func() string { if m.includeEmpty { return "Yes" } ; return "No" }(), orderLabels[m.orderIdx], modeLabels[m.modeIdx], columnsLabels[m.columnsIdx], objOrderLabels[m.objOrderIdx]}
    // Compute label width for alignment
    maxLabel := 0
    for _, l := range labels { if w := lipgloss.Width(l); w > maxLabel { maxLabel = w } }
    rowStyle := lipgloss.NewStyle().Background(lipgloss.Color("250")).Foreground(lipgloss.Black)
    // Focused row gets a dark bar with white text
    focusStyle := lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.White).Bold(true)
    // Build rows
    rows := make([]string, 0, len(labels))
    for i := range labels {
        marker := " "
        st := rowStyle
        if i == m.focus { marker = ">"; st = focusStyle }
        lpad := labels[i]
        // pad label to maxLabel
        if w := lipgloss.Width(lpad); w < maxLabel { lpad = lpad + strings.Repeat(" ", maxLabel-w) }
        s := marker + " " + lpad + ": " + values[i]
        rows = append(rows, st.Width(m.width).Render(s))
    }
    // Fill remaining height
    for len(rows) < m.height { rows = append(rows, rowStyle.Width(m.width).Render("")) }
    return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// FooterHints implements ModalFooterHints to show key help in the modal footer.
func (m *ResourcesOptionsModel) FooterHints() [][2]string {
    return [][2]string{{"Up/Down", "Move"}, {"Left/Right/Space", "Toggle"}, {"Ctrl+S", "Save as defaults"}, {"Enter", "Apply & Close"}, {"Esc", "Cancel"}}
}
