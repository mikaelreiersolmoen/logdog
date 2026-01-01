package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mikaelreiersolmoen/logdog/internal/logcat"
)

const (
	tagColumnWidth       = 30
	timestampColumnWidth = 18
)

// FormatEntry returns a formatted string representation of the entry
func FormatEntry(e *logcat.Entry, style lipgloss.Style) string {
	return FormatEntryWithTag(e, style, true)
}

// FormatEntryWithTag returns a formatted string representation with optional tag display
func FormatEntryWithTag(e *logcat.Entry, style lipgloss.Style, showTag bool) string {
	return FormatEntryWithTagAndIndent(e, style, showTag, false)
}

// FormatEntryWithTagAndIndent returns a formatted string with optional indentation for stack traces
func FormatEntryWithTagAndIndent(e *logcat.Entry, style lipgloss.Style, showTag bool, indent bool) string {
	return FormatEntryWithTagAndMessageStyle(e, style, showTag, lipgloss.NewStyle(), indent)
}

// FormatEntryWithTimestampTagAndIndent returns a formatted string with optional timestamp display
func FormatEntryWithTimestampTagAndIndent(e *logcat.Entry, style lipgloss.Style, showTag bool, indent bool, showTimestamp bool) string {
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
		tagText := truncate(e.Tag, tagColumnWidth)
		tagStr = tagStyle.Render(fmt.Sprintf("%*s", tagColumnWidth, tagText))
	} else {
		tagStr = strings.Repeat(" ", tagColumnWidth)
	}

	message := e.Message
	if indent && logcat.IsStackTraceLine(message) {
		message = "    " + message
	}

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

// FormatEntryWithTagAndMessageStyle returns a formatted string with separate style for message
func FormatEntryWithTagAndMessageStyle(e *logcat.Entry, style lipgloss.Style, showTag bool, messageStyle lipgloss.Style, indent bool) string {
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

	messageStyle = messageStyle.Foreground(subtleColor)

	var tagStr string
	if showTag {
		tagText := truncate(e.Tag, tagColumnWidth)
		tagStr = tagStyle.Render(fmt.Sprintf("%*s", tagColumnWidth, tagText))
	} else {
		tagStr = strings.Repeat(" ", tagColumnWidth)
	}

	message := e.Message
	if indent && logcat.IsStackTraceLine(message) {
		message = "    " + message
	}

	return fmt.Sprintf("%s %s %s", tagStr, priorityStyle.Render(e.Priority.String()), messageStyle.Render(message))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
