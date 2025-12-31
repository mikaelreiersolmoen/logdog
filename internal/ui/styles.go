package ui

import "github.com/charmbracelet/lipgloss"

// Color palette for log levels
var (
	colorVerbose = lipgloss.AdaptiveColor{Light: "240", Dark: "247"} // Very subtle gray
	colorDebug   = lipgloss.AdaptiveColor{Light: "30", Dark: "109"}  // Moderate teal
	colorInfo    = lipgloss.AdaptiveColor{Light: "28", Dark: "114"}  // Vibrant green
	colorWarn    = lipgloss.AdaptiveColor{Light: "130", Dark: "178"} // Subtle orange
	colorError   = lipgloss.AdaptiveColor{Light: "124", Dark: "210"} // Subtle red
	colorFatal   = lipgloss.AdaptiveColor{Light: "126", Dark: "211"} // Subtle magenta
	colorDefault = lipgloss.AdaptiveColor{Light: "0", Dark: "255"}   // Black/White
)

// Color palette for tags - pastel colors that don't overlap with log levels
var tagColors = []lipgloss.AdaptiveColor{
	{Light: "30", Dark: "123"},  // Pastel teal
	{Light: "91", Dark: "183"},  // Pastel purple
	{Light: "130", Dark: "222"}, // Pastel peach
	{Light: "64", Dark: "151"},  // Pastel lime
	{Light: "97", Dark: "189"},  // Pastel lavender
	{Light: "37", Dark: "122"},  // Pastel cyan
	{Light: "90", Dark: "182"},  // Pastel violet
	{Light: "131", Dark: "217"}, // Pastel tan
	{Light: "65", Dark: "152"},  // Pastel mint
	{Light: "98", Dark: "190"},  // Pastel mauve
}

// Color palette for filter badges - very subtle muted colors
var filterColors = []lipgloss.AdaptiveColor{
	{Light: "109", Dark: "102"}, // Muted teal-gray
	{Light: "146", Dark: "139"}, // Muted purple-gray
	{Light: "181", Dark: "174"}, // Muted peach-gray
	{Light: "144", Dark: "108"}, // Muted lime-gray
	{Light: "182", Dark: "145"}, // Muted lavender-gray
	{Light: "116", Dark: "109"}, // Muted cyan-gray
	{Light: "140", Dark: "139"}, // Muted violet-gray
	{Light: "180", Dark: "144"}, // Muted tan-gray
	{Light: "151", Dark: "108"}, // Muted mint-gray
	{Light: "183", Dark: "146"}, // Muted mauve-gray
}

// UI accent color used in headers and selected items
var accentColor = lipgloss.AdaptiveColor{Light: "33", Dark: "110"}

// GetVerboseColor returns the color for verbose log level
func GetVerboseColor() lipgloss.TerminalColor { return colorVerbose }

// GetDebugColor returns the color for debug log level
func GetDebugColor() lipgloss.TerminalColor { return colorDebug }

// GetInfoColor returns the color for info log level
func GetInfoColor() lipgloss.TerminalColor { return colorInfo }

// GetWarnColor returns the color for warn log level
func GetWarnColor() lipgloss.TerminalColor { return colorWarn }

// GetErrorColor returns the color for error log level
func GetErrorColor() lipgloss.TerminalColor { return colorError }

// GetFatalColor returns the color for fatal log level
func GetFatalColor() lipgloss.TerminalColor { return colorFatal }

// GetAccentColor returns the UI accent color
func GetAccentColor() lipgloss.TerminalColor { return accentColor }

// TagColor returns a consistent color for a given tag name
func TagColor(tag string) lipgloss.TerminalColor {
	if tag == "" {
		return colorDefault
	}

	// Simple hash function to map tag to color index
	var hash uint32
	for i := 0; i < len(tag); i++ {
		hash = hash*31 + uint32(tag[i])
	}

	colorIndex := int(hash) % len(tagColors)
	return tagColors[colorIndex]
}

// FilterColor returns a consistent color for filter badges (more subtle than tag colors)
func FilterColor(filterText string) lipgloss.TerminalColor {
	if filterText == "" {
		return colorDefault
	}

	// Simple hash function to map filter to color index
	var hash uint32
	for i := 0; i < len(filterText); i++ {
		hash = hash*31 + uint32(filterText[i])
	}

	colorIndex := int(hash) % len(filterColors)
	return filterColors[colorIndex]
}
