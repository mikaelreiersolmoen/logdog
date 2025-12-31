package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mikaelreiersolmoen/logdog/internal/logcat"
)

const tagColumnWidth = 30

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

	// Priority indicator uses subtle color
	priorityStyle := lipgloss.NewStyle().
		Foreground(subtleColor).
		Bold(true)

	// Tag uses hash-based color
	tagStyle := lipgloss.NewStyle().
		Foreground(TagColor(e.Tag))

	// Message uses subtle color
	messageStyle := lipgloss.NewStyle().Foreground(subtleColor)

	var tagStr string
	if showTag {
		tagText := truncate(e.Tag, tagColumnWidth)
		tagStr = tagStyle.Render(fmt.Sprintf("%*s", tagColumnWidth, tagText))
	} else {
		tagStr = strings.Repeat(" ", tagColumnWidth)
	}

	// Add indentation if requested (determined by caller based on timestamp matching)
	message := e.Message
	if indent && logcat.IsStackTraceLine(message) {
		message = "    " + message // 4 spaces indentation
	}

	if showTimestamp {
		// Timestamp is preserved in the model for future toggling but not rendered.
	}

	return fmt.Sprintf("%s %s %s",
		tagStr,
		priorityStyle.Render(e.Priority.String()),
		messageStyle.Render(message),
	)
}

// FormatEntryWithTagAndMessageStyle returns a formatted string with separate style for message
func FormatEntryWithTagAndMessageStyle(e *logcat.Entry, style lipgloss.Style, showTag bool, messageStyle lipgloss.Style, indent bool) string {
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

	// Priority indicator uses subtle color
	priorityStyle := lipgloss.NewStyle().
		Foreground(subtleColor).
		Bold(true)

	// Tag uses hash-based color
	tagStyle := lipgloss.NewStyle().
		Foreground(TagColor(e.Tag))

	// Message uses subtle color
	messageStyle = messageStyle.Foreground(subtleColor)

	var tagStr string
	if showTag {
		tagText := truncate(e.Tag, tagColumnWidth)
		tagStr = tagStyle.Render(fmt.Sprintf("%*s", tagColumnWidth, tagText))
	} else {
		tagStr = strings.Repeat(" ", tagColumnWidth)
	}

	// Add indentation if requested (determined by caller based on timestamp matching)
	message := e.Message
	if indent && logcat.IsStackTraceLine(message) {
		message = "    " + message // 4 spaces indentation
	}

	return fmt.Sprintf("%s %s %s",
		tagStr,
		priorityStyle.Render(e.Priority.String()),
		messageStyle.Render(message),
	)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
