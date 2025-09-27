package ui

import "github.com/charmbracelet/lipgloss/v2"

// Color constants
const (
	ColorBlack      = "0"
	ColorDarkerBlue = "4"
	ColorCyan       = "6"
	ColorGrey       = "7"
	ColorWhite      = "15"
)

// Common styles
var (
	// Panel styles
	PanelHeaderStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(ColorDarkerBlue)).
		Foreground(lipgloss.Color(ColorGrey))

	PanelContentStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(ColorDarkerBlue)).
		Foreground(lipgloss.Color(ColorGrey))

	PanelFooterStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(ColorDarkerBlue)).
		Foreground(lipgloss.Color(ColorGrey))

	PanelItemStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(ColorDarkerBlue)).
		Foreground(lipgloss.Color(ColorGrey))

    PanelItemSelectedStyle = lipgloss.NewStyle().
        Background(lipgloss.Color(ColorCyan)).
        Foreground(lipgloss.Color(ColorBlack))

    // Table header style for server-side Table rendering in panel content
    PanelTableHeaderStyle = lipgloss.NewStyle().
        Background(lipgloss.Color(ColorDarkerBlue)).
        Foreground(lipgloss.Color(ColorGrey)).
        Bold(true)

	// Border and frame styles
	BorderStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorCyan))

	// Panel frame styles (use lipgloss borders)
	PanelFrameLeftStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(ColorCyan)).
		BorderRight(false)

	PanelFrameRightStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(ColorCyan)).
		BorderLeft(false)

	// Function key styles
	FunctionKeyStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(ColorBlack)).
		Padding(0, 0, 0, 1)

    FunctionKeyDescriptionStyle = lipgloss.NewStyle().
        Background(lipgloss.Color(ColorCyan)).
        Foreground(lipgloss.Color(ColorBlack)).
        Padding(0, 1, 0, 0)

    FunctionKeyDisabledStyle = lipgloss.NewStyle().
        Background(lipgloss.Color(ColorDarkerBlue)).
        Foreground(lipgloss.Color(ColorGrey)).
        Padding(0, 1, 0, 0)

	FunctionKeyBarStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(ColorBlack)).
		Foreground(lipgloss.Color(ColorGrey))

	FunctionKeyTitleStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(ColorDarkerBlue)).
		Foreground(lipgloss.Color(ColorWhite)).
		Bold(true)

	// Toggle message styles
	ToggleMessageStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(ColorDarkerBlue)).
		Foreground(lipgloss.Color(ColorGrey)).
		Padding(0, 1)

	ToggleMessageTitleStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(ColorDarkerBlue)).
		Foreground(lipgloss.Color(ColorGrey)).
		Bold(true)

	// Modal styles
	ModalBorderStyle = lipgloss.NewStyle().
		BorderForeground(lipgloss.Color(ColorCyan))

	ModalTitleStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(ColorDarkerBlue)).
		Foreground(lipgloss.Color(ColorGrey))

	// Resource selector styles
	ResourceSelectorHeaderStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(ColorDarkerBlue)).
		Foreground(lipgloss.Color(ColorGrey))

	ResourceSelectorFooterStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(ColorDarkerBlue)).
		Foreground(lipgloss.Color(ColorGrey))
)
