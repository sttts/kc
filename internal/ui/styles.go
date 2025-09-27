package ui

import "github.com/charmbracelet/lipgloss/v2"

// Color constants (ANSI base codes)
// Note: Prefer lipgloss built-in ANSI constants where possible in code.
const (
	ColorBlack      = "0"  // prefer lipgloss.Black
	ColorDarkerBlue = "4"  // prefer lipgloss.Blue
	ColorCyan       = "6"  // prefer lipgloss.Cyan
	ColorGrey       = "7"  // prefer lipgloss.White (base white)
	ColorDarkGrey   = "8"  // bright black (grey)
	ColorWhite      = "15" // bright white
)

// Common styles
var (
	// Panel styles
	PanelHeaderStyle = lipgloss.NewStyle().
				Background(lipgloss.Blue).
				Foreground(lipgloss.White)

	PanelContentStyle = lipgloss.NewStyle().
				Background(lipgloss.Blue).
				Foreground(lipgloss.White)

	PanelFooterStyle = lipgloss.NewStyle().
				Background(lipgloss.Blue).
				Foreground(lipgloss.White)

	PanelItemStyle = lipgloss.NewStyle().
			Background(lipgloss.Blue).
			Foreground(lipgloss.White)

	PanelItemSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Cyan).
				Foreground(lipgloss.Black)

	// Table header style for server-side Table rendering in panel content
	PanelTableHeaderStyle = lipgloss.NewStyle().
				Background(lipgloss.Blue).
				Foreground(lipgloss.White).
				Bold(true)

		// Border and frame styles
	BorderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Cyan)

		// Panel frame styles (use lipgloss borders)
	PanelFrameLeftStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Cyan).
				BorderRight(false)

	PanelFrameRightStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Cyan).
				BorderLeft(false)

		// Function key styles
	FunctionKeyStyle = lipgloss.NewStyle().
				Background(lipgloss.Black).
				Padding(0, 0, 0, 1)

	FunctionKeyDescriptionStyle = lipgloss.NewStyle().
					Background(lipgloss.Cyan).
					Foreground(lipgloss.Black).
					Padding(0, 1, 0, 0)

	FunctionKeyDisabledStyle = lipgloss.NewStyle().
					Background(lipgloss.Color(ColorDarkGrey)).
					Foreground(lipgloss.White).
					Padding(0, 1, 0, 0)

	FunctionKeyBarStyle = lipgloss.NewStyle().
				Background(lipgloss.Black).
				Foreground(lipgloss.White)

	FunctionKeyTitleStyle = lipgloss.NewStyle().
				Background(lipgloss.Blue).
				Foreground(lipgloss.Color(ColorWhite)).
				Bold(true)

		// Toggle message styles
	ToggleMessageStyle = lipgloss.NewStyle().
				Background(lipgloss.Blue).
				Foreground(lipgloss.White).
				Padding(0, 1)

	ToggleMessageTitleStyle = lipgloss.NewStyle().
				Background(lipgloss.Blue).
				Foreground(lipgloss.White).
				Bold(true)

		// Modal styles
	ModalBorderStyle = lipgloss.NewStyle().
				BorderForeground(lipgloss.Cyan)

	ModalTitleStyle = lipgloss.NewStyle().
			Background(lipgloss.Blue).
			Foreground(lipgloss.White)

		// Resource selector styles
	ResourceSelectorHeaderStyle = lipgloss.NewStyle().
					Background(lipgloss.Blue).
					Foreground(lipgloss.White)

	ResourceSelectorFooterStyle = lipgloss.NewStyle().
					Background(lipgloss.Blue).
					Foreground(lipgloss.White)
)
