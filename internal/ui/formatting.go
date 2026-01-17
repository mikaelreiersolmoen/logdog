package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mikaelreiersolmoen/logdog/internal/logcat"
	"github.com/muesli/reflow/wrap"
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

// FormatEntry returns a formatted string with optional timestamp display.
// When continuation is true, timestamp, tag, and priority columns are blanked
// to visually indicate that the entry belongs to the previous timestamp.
func FormatEntry(e *logcat.Entry, style lipgloss.Style, showTag bool, showTimestamp bool, logLevelBackground bool, continuation bool) string {
	lines := FormatEntryLines(e, style, showTag, showTimestamp, logLevelBackground, continuation, 0)
	return strings.Join(lines, "\n")
}

// FormatEntryLines returns formatted lines with ANSI-aware wrapping.
// maxWidth is the full line width; when <= 0, wrapping is disabled.
func FormatEntryLines(e *logcat.Entry, style lipgloss.Style, showTag bool, showTimestamp bool, logLevelBackground bool, continuation bool, maxWidth int) []string {
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

	priorityStyle := lipgloss.NewStyle().Bold(true)
	if logLevelBackground {
		priorityStyle = priorityStyle.
			Foreground(lipgloss.AdaptiveColor{Light: "255", Dark: "0"}).
			Background(subtleColor)
	} else {
		priorityStyle = priorityStyle.Foreground(subtleColor)
	}

	tagStyle := lipgloss.NewStyle().
		Foreground(TagColor(e.Tag))

	messageStyle := lipgloss.NewStyle().Foreground(subtleColor)

	var tagStr string
	if showTag && !continuation {
		tagText := truncate(e.Tag, TagColumnWidth())
		tagStr = tagStyle.Render(fmt.Sprintf("%*s", TagColumnWidth(), tagText))
	} else {
		tagStr = strings.Repeat(" ", TagColumnWidth())
	}

	priorityWidth := len(e.Priority.String()) + 2
	priorityStr := strings.Repeat(" ", priorityWidth)
	if !continuation {
		priorityStr = priorityStyle.Render(" " + e.Priority.String() + " ")
	}
	message := e.Message

	if showTimestamp {
		timestampStyle := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "238", Dark: "252"})
		timestampContent := strings.Repeat(" ", timestampColumnWidth)
		if !continuation {
			timestampContent = fmt.Sprintf("%-*s", timestampColumnWidth, e.Timestamp)
		}
		timestampStr := timestampStyle.Render(timestampContent)
		sep := " "
		prefix := timestampStr + sep + tagStr + sep + priorityStr + sep
		contPrefix := timestampStyle.Render(strings.Repeat(" ", timestampColumnWidth)) +
			sep +
			strings.Repeat(" ", TagColumnWidth()) +
			sep +
			strings.Repeat(" ", priorityWidth) +
			sep
		renderOne := func(s string) string { return messageStyle.Render(s) }
		return wrapWithPrefix(message, renderOne, prefix, contPrefix, maxWidth)
	}

	sep := " "
	prefix := tagStr + sep + priorityStr + sep
	contPrefix := strings.Repeat(" ", TagColumnWidth()) +
		sep +
		strings.Repeat(" ", priorityWidth) +
		sep
	renderOne := func(s string) string { return messageStyle.Render(s) }
	return wrapWithPrefix(message, renderOne, prefix, contPrefix, maxWidth)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func wrapWithPrefix(message string, render func(string) string, prefix, contPrefix string, maxWidth int) []string {
	if render == nil {
		render = func(s string) string { return s }
	}
	if maxWidth <= 0 {
		return []string{prefix + render(message)}
	}
	messageWidth := maxWidth - lipgloss.Width(prefix)
	if messageWidth < 1 {
		messageWidth = 1
	}
	wrapped := wrap.String(message, messageWidth)
	lines := strings.Split(wrapped, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		rendered := render(line)
		if i == 0 {
			out = append(out, prefix+rendered)
		} else {
			out = append(out, contPrefix+rendered)
		}
	}
	return out
}
