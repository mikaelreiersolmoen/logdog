package ui

import (
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mikaelreiersolmoen/logdog/internal/buffer"
	"github.com/mikaelreiersolmoen/logdog/internal/logcat"
)

type logLevelItem logcat.Priority

func (i logLevelItem) FilterValue() string { return "" }

type logLevelDelegate struct{}

func (d logLevelDelegate) Height() int                             { return 1 }
func (d logLevelDelegate) Spacing() int                            { return 0 }
func (d logLevelDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d logLevelDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(logLevelItem)
	if !ok {
		return
	}

	priority := logcat.Priority(i)

	// Map priority to keyboard shortcut
	var shortcut string
	switch priority {
	case logcat.Verbose:
		shortcut = "v"
	case logcat.Debug:
		shortcut = "d"
	case logcat.Info:
		shortcut = "i"
	case logcat.Warn:
		shortcut = "w"
	case logcat.Error:
		shortcut = "e"
	case logcat.Fatal:
		shortcut = "f"
	}

	str := fmt.Sprintf("(%s) %s", shortcut, priority.Name())

	// Get subtle message color for this priority
	var subtleColor lipgloss.TerminalColor
	switch priority {
	case logcat.Verbose:
		subtleColor = logcat.GetVerboseColor()
	case logcat.Debug:
		subtleColor = logcat.GetDebugColor()
	case logcat.Info:
		subtleColor = logcat.GetInfoColor()
	case logcat.Warn:
		subtleColor = logcat.GetWarnColor()
	case logcat.Error:
		subtleColor = logcat.GetErrorColor()
	case logcat.Fatal:
		subtleColor = logcat.GetFatalColor()
	default:
		subtleColor = logcat.GetVerboseColor()
	}

	itemStyle := lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle := lipgloss.NewStyle().PaddingLeft(2).Foreground(subtleColor)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

type deviceItem logcat.Device

func (i deviceItem) FilterValue() string { return "" }

type deviceDelegate struct{}

func (d deviceDelegate) Height() int                             { return 1 }
func (d deviceDelegate) Spacing() int                            { return 0 }
func (d deviceDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d deviceDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(deviceItem)
	if !ok {
		return
	}

	device := logcat.Device(i)
	str := fmt.Sprintf("%s - %s", device.Serial, device.Model)

	itemStyle := lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle := lipgloss.NewStyle().
		PaddingLeft(2).
		Foreground(lipgloss.AdaptiveColor{Light: "33", Dark: "117"})

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

type Model struct {
	viewport          viewport.Model
	buffer            *buffer.RingBuffer
	logManager        *logcat.Manager
	lineChan          chan string
	ready             bool
	width             int
	height            int
	appID             string
	appStatus         string
	terminating       bool
	showLogLevel      bool
	logLevelList      list.Model
	minLogLevel       logcat.Priority
	showFilter        bool
	filterInput       textinput.Model
	filters           []Filter
	parsedEntries     []*logcat.Entry
	needsUpdate       bool
	highlightedEntry  *logcat.Entry
	selectionMode     bool
	messageOnlySelect bool // true for message-only (v), false for whole-line (V)
	selectedEntries   map[*logcat.Entry]bool
	selectionAnchor   *logcat.Entry
	autoScroll        bool
	showDeviceSelect  bool
	deviceList        list.Model
	devices           []logcat.Device
	selectedDevice    string // Device serial or model
	errorMessage      string
}

type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

type Filter struct {
	isTag bool
	regex *regexp.Regexp
}

type logLineMsg string
type updateViewportMsg struct{}
type appStatusMsg string

func NewModel(appID string, tailSize int) Model {
	items := []list.Item{
		logLevelItem(logcat.Verbose),
		logLevelItem(logcat.Debug),
		logLevelItem(logcat.Info),
		logLevelItem(logcat.Warn),
		logLevelItem(logcat.Error),
		logLevelItem(logcat.Fatal),
	}

	logLevelList := list.New(items, logLevelDelegate{}, 30, len(items)+4)
	logLevelList.Title = "Select log level (v/d/i/w/e/f)"
	logLevelList.SetShowStatusBar(false)
	logLevelList.SetFilteringEnabled(false)
	logLevelList.SetShowPagination(false)
	logLevelList.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "33", Dark: "117"}).
		Padding(0, 1)

	filterInput := textinput.New()
	filterInput.Placeholder = "e.g., error|warning, tag:MyTag"
	filterInput.CharLimit = 500
	filterInput.Width = 80

	// Check for multiple devices
	devices, err := logcat.GetDevices()
	showDeviceSelect := false
	var deviceList list.Model

	if err == nil && len(devices) > 1 {
		// Multiple devices - show device selector
		showDeviceSelect = true
		deviceItems := make([]list.Item, len(devices))
		for i, device := range devices {
			deviceItems[i] = deviceItem(device)
		}
		deviceList = list.New(deviceItems, deviceDelegate{}, 50, len(devices)+4)
		deviceList.Title = "Select device"
		deviceList.SetShowStatusBar(false)
		deviceList.SetFilteringEnabled(false)
		deviceList.SetShowPagination(false)
		deviceList.Styles.Title = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "33", Dark: "117"}).
			Padding(0, 1)
	} else if err == nil && len(devices) == 1 {
		// Single device - use it automatically
		logManager := logcat.NewManager(appID, tailSize)
		logManager.SetDevice(devices[0].Serial)
		return Model{
			appID:             appID,
			buffer:            buffer.NewRingBuffer(10000),
			logManager:        logManager,
			lineChan:          make(chan string, 100),
			showLogLevel:      false,
			logLevelList:      logLevelList,
			minLogLevel:       logcat.Verbose,
			showFilter:        false,
			filterInput:       filterInput,
			filters:           []Filter{},
			parsedEntries:     make([]*logcat.Entry, 0, 10000),
			needsUpdate:       false,
			highlightedEntry:  nil,
			selectionMode:     false,
			messageOnlySelect: false,
			selectedEntries:   make(map[*logcat.Entry]bool),
			selectionAnchor:   nil,
			autoScroll:        true,
			showDeviceSelect:  false,
			deviceList:        list.Model{},
			devices:           devices,
			selectedDevice:    devices[0].Model,
		}
	}

	return Model{
		appID:             appID,
		buffer:            buffer.NewRingBuffer(10000),
		logManager:        logcat.NewManager(appID, tailSize),
		lineChan:          make(chan string, 100),
		showLogLevel:      false,
		logLevelList:      logLevelList,
		minLogLevel:       logcat.Verbose,
		showFilter:        false,
		filterInput:       filterInput,
		filters:           []Filter{},
		parsedEntries:     make([]*logcat.Entry, 0, 10000),
		needsUpdate:       false,
		highlightedEntry:  nil,
		selectionMode:     false,
		messageOnlySelect: false,
		selectedEntries:   make(map[*logcat.Entry]bool),
		selectionAnchor:   nil,
		autoScroll:        true,
		showDeviceSelect:  showDeviceSelect,
		deviceList:        deviceList,
		devices:           devices,
		selectedDevice:    "",
	}
}

func (m Model) Init() tea.Cmd {
	// If showing device selector, don't start logcat yet
	if m.showDeviceSelect {
		return nil
	}

	cmds := []tea.Cmd{
		startLogcat(m.logManager, m.lineChan),
		waitForLogLine(m.lineChan),
		tickViewportUpdate(),
	}

	// If filtering by app, listen for status updates
	if m.appID != "" {
		cmds = append(cmds, waitForStatus(m.logManager.StatusChan()))
	}

	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Calculate header height based on what will be shown
		headerHeight := 4 // Base header (log level line + app/device line + border)
		footerHeight := 2
		verticalMargin := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMargin)
			m.viewport.YPosition = headerHeight
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMargin
		}

		m.width = msg.Width
		m.height = msg.Height

	case logLineMsg:
		m.buffer.Add(string(msg))
		entry, _ := logcat.ParseLine(string(msg))
		if entry != nil {
			m.parsedEntries = append(m.parsedEntries, entry)
			if len(m.parsedEntries) > 10000 {
				m.parsedEntries = m.parsedEntries[1:]
			}
		}
		m.needsUpdate = true

		if !m.terminating {
			cmds = append(cmds, waitForLogLine(m.lineChan))
		}

	case appStatusMsg:
		m.appStatus = string(msg)
		if !m.terminating {
			cmds = append(cmds, waitForStatus(m.logManager.StatusChan()))
		}

	case updateViewportMsg:
		if m.needsUpdate && m.ready {
			m.updateViewportWithScroll(m.autoScroll)
			m.needsUpdate = false
		}
		if !m.terminating {
			cmds = append(cmds, tickViewportUpdate())
		}

	case errMsg:
		// Handle errors from logcat start
		m.errorMessage = msg.Error()
		m.terminating = true
		return m, tea.Quit

	case tea.KeyMsg:
		if m.showDeviceSelect {
			switch msg.String() {
			case "q", "ctrl+c", "esc":
				m.terminating = true
				return m, tea.Quit
			case "enter":
				if i, ok := m.deviceList.SelectedItem().(deviceItem); ok {
					device := logcat.Device(i)
					m.logManager.SetDevice(device.Serial)
					m.selectedDevice = device.Model
					m.showDeviceSelect = false
					// Start logcat now that device is selected
					cmds := []tea.Cmd{
						startLogcat(m.logManager, m.lineChan),
						waitForLogLine(m.lineChan),
						tickViewportUpdate(),
					}
					if m.appID != "" {
						cmds = append(cmds, waitForStatus(m.logManager.StatusChan()))
					}
					return m, tea.Batch(cmds...)
				}
				return m, nil
			}
		} else if m.showLogLevel {
			switch msg.String() {
			case "esc":
				m.showLogLevel = false
				return m, nil
			case "enter":
				if i, ok := m.logLevelList.SelectedItem().(logLevelItem); ok {
					m.minLogLevel = logcat.Priority(i)
					m.showLogLevel = false
					m.updateViewport()
				}
				return m, nil
			case "v":
				m.minLogLevel = logcat.Verbose
				m.showLogLevel = false
				m.updateViewport()
				return m, nil
			case "d":
				m.minLogLevel = logcat.Debug
				m.showLogLevel = false
				m.updateViewport()
				return m, nil
			case "i":
				m.minLogLevel = logcat.Info
				m.showLogLevel = false
				m.updateViewport()
				return m, nil
			case "w":
				m.minLogLevel = logcat.Warn
				m.showLogLevel = false
				m.updateViewport()
				return m, nil
			case "e":
				m.minLogLevel = logcat.Error
				m.showLogLevel = false
				m.updateViewport()
				return m, nil
			case "f":
				m.minLogLevel = logcat.Fatal
				m.showLogLevel = false
				m.updateViewport()
				return m, nil
			}
		} else if m.showFilter {
			switch msg.String() {
			case "esc":
				m.showFilter = false
				m.filterInput.Blur()
				return m, nil
			case "enter":
				m.parseFilters(m.filterInput.Value())
				m.showFilter = false
				m.filterInput.Blur()
				m.updateViewport()
				return m, nil
			}
		} else {
			switch msg.String() {
			case "q", "ctrl+c":
				m.terminating = true
				m.logManager.Stop()
				return m, tea.Quit
			case "l":
				m.showLogLevel = true
				return m, nil
			case "f":
				m.showFilter = true
				m.filterInput.Focus()
				return m, textinput.Blink
			case "esc":
				if m.selectionMode {
					m.selectionMode = false
					m.clearSelection()
				}
				m.highlightedEntry = nil
				m.autoScroll = true
				m.updateViewportWithScroll(false)
				return m, nil
			case "v": // v to enter message-only selection mode
				m.autoScroll = false
				m.enterSelectionMode(true)
				m.updateViewportWithScroll(false)
				return m, nil
			case "V": // Shift+v to enter whole-line selection mode
				m.autoScroll = false
				m.enterSelectionMode(false)
				m.updateViewportWithScroll(false)
				return m, nil
			case "c":
				if m.selectionMode && len(m.selectedEntries) > 0 {
					m.copySelectedMessages()
					m.clearSelection()
					m.selectionMode = false
					m.autoScroll = true
					m.updateViewportWithScroll(false)
				}
				return m, nil
			case "j", "down":
				m.autoScroll = false
				if m.selectionMode {
					m.extendSelectionDown()
				} else {
					m.moveHighlightDown()
				}
				m.updateViewportWithScroll(false)
				return m, nil
			case "k", "up":
				m.autoScroll = false
				if m.selectionMode {
					m.extendSelectionUp()
				} else {
					m.moveHighlightUp()
				}
				m.updateViewportWithScroll(false)
				return m, nil
			}
		}

	case tea.MouseMsg:
		if msg.Type == tea.MouseLeft && !m.showLogLevel && !m.showFilter && !m.showDeviceSelect {
			m.autoScroll = false
			m.handleMouseClick(msg.Y)
			m.updateViewportWithScroll(false)
			return m, nil
		}
	}

	if m.showDeviceSelect {
		m.deviceList, cmd = m.deviceList.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.showLogLevel {
		m.logLevelList, cmd = m.logLevelList.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.showFilter {
		m.filterInput, cmd = m.filterInput.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		// Track viewport position before update
		wasAtBottom := m.viewport.AtBottom()
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)

		// Re-enable auto-scroll if user scrolled to bottom
		if !wasAtBottom && m.viewport.AtBottom() {
			m.autoScroll = true
		} else if wasAtBottom && !m.viewport.AtBottom() {
			// Disable auto-scroll if user scrolled away from bottom
			m.autoScroll = false
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.showDeviceSelect {
		return "\n" + m.deviceList.View()
	}

	if !m.ready {
		return "\n  Initializing..."
	}

	if m.showLogLevel {
		return "\n" + m.logLevelList.View()
	}

	headerStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		BorderBottom(true).
		PaddingLeft(1).
		Width(m.width)

	headerStyleNoBorder := lipgloss.NewStyle().
		PaddingLeft(1).
		Width(m.width)

	filterInfo := ""
	if len(m.filters) > 0 {
		var filterStrs []string
		for _, f := range m.filters {
			var filterText string
			if f.isTag {
				filterText = "tag:" + f.regex.String()
			} else {
				filterText = f.regex.String()
			}

			// Use filter colors for filter badges
			filterColor := logcat.FilterColor(filterText)
			filterBadge := lipgloss.NewStyle().
				Background(filterColor).
				Foreground(lipgloss.AdaptiveColor{Light: "0", Dark: "0"}).
				Padding(0, 1).
				Render(filterText)
			filterStrs = append(filterStrs, filterBadge)
		}
		filterInfo = " | filters: " + strings.Join(filterStrs, " ")
	}

	appInfo := m.appID
	if appInfo == "" {
		appInfo = "all"
	}

	statusStyle := lipgloss.NewStyle()
	var statusText string

	switch m.appStatus {
	case "running":
		statusStyle = statusStyle.Foreground(lipgloss.AdaptiveColor{Light: "71", Dark: "114"}) // Green
		statusText = "running"
	case "stopped":
		statusStyle = statusStyle.Foreground(lipgloss.AdaptiveColor{Light: "172", Dark: "215"}) // Orange
		statusText = "disconnected"
	case "reconnecting":
		statusStyle = statusStyle.Foreground(lipgloss.AdaptiveColor{Light: "172", Dark: "215"}) // Orange
		statusText = "disconnected"
	}

	// Get color for current log level
	var logLevelColor lipgloss.TerminalColor
	switch m.minLogLevel {
	case logcat.Verbose:
		logLevelColor = logcat.GetVerboseColor()
	case logcat.Debug:
		logLevelColor = logcat.GetDebugColor()
	case logcat.Info:
		logLevelColor = logcat.GetInfoColor()
	case logcat.Warn:
		logLevelColor = logcat.GetWarnColor()
	case logcat.Error:
		logLevelColor = logcat.GetErrorColor()
	case logcat.Fatal:
		logLevelColor = logcat.GetFatalColor()
	default:
		logLevelColor = logcat.GetVerboseColor()
	}

	logLevelStyle := lipgloss.NewStyle().Foreground(logLevelColor)

	// Build header lines
	var headerLines []string

	// First line: log level and filters
	logLevelLine := fmt.Sprintf("log level: %s%s",
		logLevelStyle.Render(strings.ToLower(m.minLogLevel.Name())), filterInfo)
	headerLines = append(headerLines, headerStyle.Render(logLevelLine))

	// Second line: app and device info (always show)
	if !m.showFilter {
		var infoParts []string
		if m.appID != "" {
			infoParts = append(infoParts, fmt.Sprintf("app: %s (%s)", appInfo, statusStyle.Render(statusText)))
		} else {
			infoParts = append(infoParts, "app: all")
		}
		if m.selectedDevice != "" {
			infoParts = append(infoParts, fmt.Sprintf("device: %s", m.selectedDevice))
		}
		infoLine := strings.Join(infoParts, " | ")
		headerLines = append(headerLines, headerStyleNoBorder.Render(infoLine))
	}

	header := lipgloss.JoinVertical(lipgloss.Left, headerLines...)

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		PaddingLeft(1).
		Width(m.width)

	footerStyleNoBorder := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		PaddingLeft(1).
		Width(m.width)

	var footer string
	if m.showFilter {
		filterLabel := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "33", Dark: "117"}).
			Bold(true).
			Render("filter: ")

		filterHelp := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render("(comma-separated, tag: prefix for tags | enter: apply | esc: cancel)")

		filterLine := footerStyleNoBorder.Render(filterLabel + m.filterInput.View())
		helpLine := footerStyle.Render(filterHelp)
		footer = lipgloss.JoinVertical(lipgloss.Left, filterLine, helpLine)
	} else if m.selectionMode {
		modeType := "WHOLE-LINE"
		if m.messageOnlySelect {
			modeType = "MESSAGE"
		}
		selectionInfo := fmt.Sprintf("%s SELECTION | %d lines | j/k: extend | v/V: switch mode | c: copy | esc: exit", modeType, len(m.selectedEntries))
		footer = footerStyle.Render(selectionInfo)
	} else {
		baseHelp := "q: quit | j/k: highlight | v: select msg | V: select line | l: set log level | f: edit filter"
		footer = footerStyle.Render(baseHelp)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.viewport.View(),
		header,
		footer,
	)
}

func (m *Model) updateViewport() {
	m.updateViewportWithScroll(true)
}

func (m *Model) updateViewportWithScroll(scrollToBottom bool) {
	lines := make([]string, 0, len(m.parsedEntries))
	var lastTag string
	var lastTimestamp string

	selectedStyle := lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "251", Dark: "240"})
	highlightStyle := lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "254", Dark: "237"}) // Subtle highlight

	for _, entry := range m.parsedEntries {
		if entry.Priority >= m.minLogLevel && m.matchesFilters(entry) {
			var line string

			// Check if this should be indented (stack trace continuation with same timestamp)
			shouldIndent := entry.Timestamp == lastTimestamp &&
				logcat.IsStackTraceLine(entry.Message)

			// Apply styles based on selection/highlight state
			if m.selectedEntries[entry] {
				// Strong selection style
				if m.messageOnlySelect {
					// Message-only: only highlight the message part
					line = entry.FormatWithTagAndMessageStyle(lipgloss.NewStyle(), entry.Tag != lastTag, selectedStyle, shouldIndent)
				} else {
					// Whole-line: highlight everything by passing the background to all parts
					line = m.formatEntryWithBackground(entry, entry.Tag != lastTag, selectedStyle, shouldIndent)
				}
			} else if entry == m.highlightedEntry {
				// Subtle highlight style - whole line background
				line = m.formatEntryWithBackground(entry, entry.Tag != lastTag, highlightStyle, shouldIndent)
			} else {
				line = entry.FormatWithTagAndIndent(lipgloss.NewStyle(), entry.Tag != lastTag, shouldIndent)
			}

			lines = append(lines, line)
			lastTag = entry.Tag
			lastTimestamp = entry.Timestamp
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	m.viewport.SetContent(content)

	if scrollToBottom {
		m.viewport.GotoBottom()
	}
}

// formatEntryWithBackground formats an entry with background color applied to all parts
func (m *Model) formatEntryWithBackground(entry *logcat.Entry, showTag bool, bgStyle lipgloss.Style, indent bool) string {
	// Get color for this priority
	var priorityColor lipgloss.TerminalColor
	switch entry.Priority {
	case logcat.Verbose:
		priorityColor = logcat.GetVerboseColor()
	case logcat.Debug:
		priorityColor = logcat.GetDebugColor()
	case logcat.Info:
		priorityColor = logcat.GetInfoColor()
	case logcat.Warn:
		priorityColor = logcat.GetWarnColor()
	case logcat.Error:
		priorityColor = logcat.GetErrorColor()
	case logcat.Fatal:
		priorityColor = logcat.GetFatalColor()
	default:
		priorityColor = logcat.GetVerboseColor()
	}

	priorityStyle := lipgloss.NewStyle().
		Foreground(priorityColor).
		Background(bgStyle.GetBackground()).
		Bold(true)

	tagStyle := lipgloss.NewStyle().
		Foreground(logcat.TagColor(entry.Tag)).
		Background(bgStyle.GetBackground())

	messageStyle := lipgloss.NewStyle().
		Background(bgStyle.GetBackground())

	timestampStyle := lipgloss.NewStyle().
		Background(bgStyle.GetBackground())

	var tagStr string
	if showTag {
		tagStr = tagStyle.Render(fmt.Sprintf("%-20s", truncateString(entry.Tag, 20)))
	} else {
		tagStr = bgStyle.Render(strings.Repeat(" ", 20))
	}

	// Add indentation if requested (for stack traces with matching timestamps)
	message := entry.Message
	if indent && logcat.IsStackTraceLine(message) {
		message = "    " + message
	}

	return fmt.Sprintf("%s %s %s %s",
		timestampStyle.Render(entry.Timestamp),
		priorityStyle.Render(entry.Priority.String()),
		tagStr,
		messageStyle.Render(message),
	)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func (m *Model) parseFilters(filterStr string) {
	m.filters = []Filter{}
	if filterStr == "" {
		return
	}

	parts := splitByUnescapedComma(filterStr)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		var filter Filter
		if strings.HasPrefix(part, "tag:") {
			filter.isTag = true
			part = strings.TrimPrefix(part, "tag:")
		}

		// Unescape commas
		part = strings.ReplaceAll(part, "\\,", ",")

		regex, err := regexp.Compile(part)
		if err == nil {
			filter.regex = regex
			m.filters = append(m.filters, filter)
		}
	}
}

func splitByUnescapedComma(s string) []string {
	var parts []string
	var current strings.Builder
	escaped := false

	for _, char := range s {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}

		if char == '\\' {
			escaped = true
			current.WriteRune(char)
			continue
		}

		if char == ',' {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}

		current.WriteRune(char)
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

func (m *Model) matchesFilters(entry *logcat.Entry) bool {
	if len(m.filters) == 0 {
		return true
	}

	for _, filter := range m.filters {
		if filter.isTag {
			if filter.regex.MatchString(entry.Tag) {
				return true
			}
		} else {
			if filter.regex.MatchString(entry.Message) {
				return true
			}
		}
	}

	return false
}

func startLogcat(manager *logcat.Manager, lineChan chan string) tea.Cmd {
	return func() tea.Msg {
		if err := manager.Start(); err != nil {
			return errMsg{err}
		}
		go manager.ReadLines(lineChan)
		return nil
	}
}

func waitForLogLine(lineChan <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-lineChan
		if !ok {
			return nil
		}
		return logLineMsg(line)
	}
}

func waitForStatus(statusChan <-chan string) tea.Cmd {
	return func() tea.Msg {
		status, ok := <-statusChan
		if !ok {
			return nil
		}
		return appStatusMsg(status)
	}
}

func tickViewportUpdate() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return updateViewportMsg{}
	})
}

// getVisibleEntries returns the list of entries currently visible after filtering
func (m *Model) getVisibleEntries() []*logcat.Entry {
	visible := make([]*logcat.Entry, 0)
	for _, entry := range m.parsedEntries {
		if entry.Priority >= m.minLogLevel && m.matchesFilters(entry) {
			visible = append(visible, entry)
		}
	}
	return visible
}

// handleMouseClick handles clicking on a row
func (m *Model) handleMouseClick(y int) {
	// Calculate which entry was clicked
	viewportStartY := 2

	// If click is before viewport content, ignore
	if y <= viewportStartY {
		return
	}

	// Calculate line within viewport (0-indexed)
	lineInViewport := y - viewportStartY - 1

	// If click is beyond viewport height, ignore (in footer area)
	if lineInViewport < 0 || lineInViewport >= m.viewport.Height {
		return
	}

	// Add viewport scroll offset to get actual line in content
	clickedLine := lineInViewport + m.viewport.YOffset

	visible := m.getVisibleEntries()
	if clickedLine >= 0 && clickedLine < len(visible) {
		clickedEntry := visible[clickedLine]

		if m.selectionMode {
			// In selection mode: extend selection to clicked entry
			m.extendSelectionTo(clickedEntry, visible)
		} else {
			// Not in selection mode: just highlight
			m.highlightedEntry = clickedEntry
		}
	}
}

// extendSelectionDown extends selection downward
// extendSelectionTo extends selection from anchor to target entry
func (m *Model) extendSelectionTo(target *logcat.Entry, visible []*logcat.Entry) {
	if m.selectionAnchor == nil {
		return
	}

	anchorIdx := -1
	targetIdx := -1

	for i, entry := range visible {
		if entry == m.selectionAnchor {
			anchorIdx = i
		}
		if entry == target {
			targetIdx = i
		}
	}

	if anchorIdx < 0 || targetIdx < 0 {
		return
	}

	// Clear and rebuild selection
	m.selectedEntries = make(map[*logcat.Entry]bool)

	start := anchorIdx
	end := targetIdx
	if start > end {
		start, end = end, start
	}

	for i := start; i <= end; i++ {
		m.selectedEntries[visible[i]] = true
	}
}

// enterSelectionMode enters selection mode
func (m *Model) enterSelectionMode(messageOnly bool) {
	// If already in selection mode, just switch the mode type
	if m.selectionMode {
		m.messageOnlySelect = messageOnly
		return
	}

	m.selectionMode = true
	m.messageOnlySelect = messageOnly

	// If there's a highlighted entry, use it as the anchor
	if m.highlightedEntry != nil {
		m.selectedEntries = make(map[*logcat.Entry]bool)
		m.selectedEntries[m.highlightedEntry] = true
		m.selectionAnchor = m.highlightedEntry
	} else {
		// Otherwise, select the last visible entry
		visible := m.getVisibleEntries()
		if len(visible) > 0 {
			lastEntry := visible[len(visible)-1]
			m.selectedEntries = make(map[*logcat.Entry]bool)
			m.selectedEntries[lastEntry] = true
			m.selectionAnchor = lastEntry
			m.highlightedEntry = lastEntry
		}
	}
}

// moveHighlightDown moves the highlight down one line
func (m *Model) moveHighlightDown() {
	visible := m.getVisibleEntries()
	if len(visible) == 0 {
		return
	}

	if m.highlightedEntry == nil {
		// Start at the first visible entry
		m.highlightedEntry = visible[0]
		return
	}

	// Find current highlight and move down
	for i, entry := range visible {
		if entry == m.highlightedEntry && i < len(visible)-1 {
			m.highlightedEntry = visible[i+1]
			return
		}
	}
}

// moveHighlightUp moves the highlight up one line
func (m *Model) moveHighlightUp() {
	visible := m.getVisibleEntries()
	if len(visible) == 0 {
		return
	}

	if m.highlightedEntry == nil {
		// Start at the last visible entry
		m.highlightedEntry = visible[len(visible)-1]
		return
	}

	// Find current highlight and move up
	for i, entry := range visible {
		if entry == m.highlightedEntry && i > 0 {
			m.highlightedEntry = visible[i-1]
			return
		}
	}
}

// extendSelectionDown extends the selection downward
func (m *Model) extendSelectionDown() {
	visible := m.getVisibleEntries()
	if len(visible) == 0 || m.selectionAnchor == nil {
		return
	}

	anchorIdx := -1
	highestIdx := -1
	lowestIdx := -1

	for i, entry := range visible {
		if entry == m.selectionAnchor {
			anchorIdx = i
		}
		if m.selectedEntries[entry] {
			if highestIdx == -1 || i < highestIdx {
				highestIdx = i
			}
			if lowestIdx == -1 || i > lowestIdx {
				lowestIdx = i
			}
		}
	}

	if anchorIdx == -1 || lowestIdx == -1 {
		return
	}

	// If we have selection above the anchor, shrink from top first
	if highestIdx < anchorIdx {
		delete(m.selectedEntries, visible[highestIdx])
	} else if lowestIdx < len(visible)-1 {
		// Otherwise extend downward
		m.selectedEntries[visible[lowestIdx+1]] = true
	}
}

// extendSelectionUp extends the selection upward
func (m *Model) extendSelectionUp() {
	visible := m.getVisibleEntries()
	if len(visible) == 0 || m.selectionAnchor == nil {
		return
	}

	anchorIdx := -1
	highestIdx := -1
	lowestIdx := -1

	for i, entry := range visible {
		if entry == m.selectionAnchor {
			anchorIdx = i
		}
		if m.selectedEntries[entry] {
			if highestIdx == -1 || i < highestIdx {
				highestIdx = i
			}
			if lowestIdx == -1 || i > lowestIdx {
				lowestIdx = i
			}
		}
	}

	if anchorIdx == -1 || highestIdx == -1 {
		return
	}

	// If we have selection below the anchor, shrink from bottom first
	if lowestIdx > anchorIdx {
		delete(m.selectedEntries, visible[lowestIdx])
	} else if highestIdx > 0 {
		// Otherwise extend upward
		m.selectedEntries[visible[highestIdx-1]] = true
	}
}

// clearSelection clears the selection
func (m *Model) clearSelection() {
	m.selectedEntries = make(map[*logcat.Entry]bool)
	m.selectionAnchor = nil
}

// copySelectedMessages copies selected messages to clipboard
func (m *Model) copySelectedMessages() {
	if len(m.selectedEntries) == 0 {
		return
	}

	// Get selected entries in order
	visible := m.getVisibleEntries()
	var lines []string
	for _, entry := range visible {
		if m.selectedEntries[entry] {
			if m.messageOnlySelect {
				// Copy only the message
				lines = append(lines, entry.Message)
			} else {
				// Copy the whole line (without styling)
				lines = append(lines, entry.Format(lipgloss.NewStyle()))
			}
		}
	}

	// Copy to clipboard using pbcopy (macOS) or similar
	clipboard := strings.Join(lines, "\n")
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(clipboard)
	cmd.Run()
}

// ErrorMessage returns any error message from the model
func (m Model) ErrorMessage() string {
	return m.errorMessage
}
