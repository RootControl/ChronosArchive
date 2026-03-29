package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorGreen  = lipgloss.Color("#00d787")
	colorYellow = lipgloss.Color("#ffd700")
	colorRed    = lipgloss.Color("#ff5f5f")
	colorBlue   = lipgloss.Color("#5f87ff")
	colorGray   = lipgloss.Color("#626262")
	colorWhite  = lipgloss.Color("#d0d0d0")
	colorBorder = lipgloss.Color("#3a3a3a")

	styleGreen  = lipgloss.NewStyle().Foreground(colorGreen)
	styleYellow = lipgloss.NewStyle().Foreground(colorYellow)
	styleRed    = lipgloss.NewStyle().Foreground(colorRed)
	styleBlue   = lipgloss.NewStyle().Foreground(colorBlue)
	styleGray   = lipgloss.NewStyle().Foreground(colorGray)
	styleBold   = lipgloss.NewStyle().Bold(true)

	styleLeftPanel = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderRight(true).
			BorderForeground(colorBorder).
			PaddingRight(1)

	styleRightPanel = lipgloss.NewStyle().
			PaddingLeft(1)

	styleSelectedRow = lipgloss.NewStyle().
				Background(lipgloss.Color("#1c1c1c")).
				Bold(true)

	stylePermBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorYellow).
			Foreground(colorWhite).
			Padding(0, 1).
			MarginBottom(1)

	stylePermTitle = lipgloss.NewStyle().
			Foreground(colorYellow).
			Bold(true)

	styleToolName = lipgloss.NewStyle().Foreground(colorBlue)
	styleResult   = lipgloss.NewStyle().Foreground(colorGray)

	styleHeader = lipgloss.NewStyle().
			Foreground(colorWhite).
			Bold(true).
			Background(lipgloss.Color("#1a1a2e")).
			Padding(0, 1)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorGray).
			Padding(0, 1)
)
