package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mikaelreiersolmoen/logdog/internal/logcat"
)

const (
	DefaultTagColumnWidth = 30
	timestampColumnWidth  = 18
)

var tagColumnWidth = DefaultTagColumnWidth

// SetTagColumnWidth allows adjusting the global tag column width used for rendering.
func SetTagColumnWidth(width int) {
	if width <= 0 {
		tagColumnWidth = DefaultTagColumnWidth
		return
	}
	tagColumnWidth = width
}

// TagColumnWidth returns the current tag column width.
func TagColumnWidth() int {
	return tagColumnWidth
}

// FormatEntry returns a formatted string with optional timestamp display
func FormatEntry(e *logcat.Entry, style lipgloss.Style, showTag bool, showTimestamp bool) string {
	// Get subtle color based on log level
	var subtleColor lipgloss.TerminalColor
	switch e.Priority {
	case logcat.Verbose:
		subtleColor = GetVerboseColor()
	case logcat.Debug:
		subtleColor = GetDebugColor()
	case logcat.Info:
		subtleColor = GetInfoColor()
	case logcat.Warn:
		subtleColor = GetWarnColor()
	case logcat.Error:
		subtleColor = GetErrorColor()
	case logcat.Fatal:
		subtleColor = GetFatalColor()
	default:
		subtleColor = colorDefault
	}

	priorityStyle := lipgloss.NewStyle().
		Foreground(subtleColor).
		Bold(true)

	tagStyle := lipgloss.NewStyle().
		Foreground(TagColor(e.Tag))

	messageStyle := lipgloss.NewStyle().Foreground(subtleColor)

	var tagStr string
	if showTag {
		tagText := truncate(e.Tag, TagColumnWidth())
		tagStr = tagStyle.Render(fmt.Sprintf("%*s", TagColumnWidth(), tagText))
	} else {
		tagStr = strings.Repeat(" ", TagColumnWidth())
	}

	message := e.Message

	priorityStr := priorityStyle.Render(e.Priority.String())
	messageStr := messageStyle.Render(message)

	if showTimestamp {
		timestampStyle := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "238", Dark: "252"})
		timestampStr := timestampStyle.Render(fmt.Sprintf("%-*s", timestampColumnWidth, e.Timestamp))
		return fmt.Sprintf("%s %s %s %s", timestampStr, tagStr, priorityStr, messageStr)
	}

	return fmt.Sprintf("%s %s %s", tagStr, priorityStr, messageStr)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
