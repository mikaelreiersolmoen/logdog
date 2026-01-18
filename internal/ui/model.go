package ui

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mikaelreiersolmoen/logdog/internal/adb"
	"github.com/mikaelreiersolmoen/logdog/internal/config"
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
		subtleColor = GetVerboseColor()
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

type deviceItem adb.Device

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

	device := adb.Device(i)
	str := fmt.Sprintf("%s - %s", device.Serial, device.Model)

	itemStyle := lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle := lipgloss.NewStyle().
		PaddingLeft(2).
		Foreground(GetAccentColor())

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

type Model struct {
	viewport           viewport.Model
	logManager         *logcat.Manager
	lineChan           chan string
	ready              bool
	width              int
	height             int
	appID              string
	appStatus          string
	deviceStatus       string
	terminating        bool
	showLogLevel       bool
	logLevelList       list.Model
	minLogLevel        logcat.Priority
	showFilter         bool
	filterInput        textinput.Model
	filters            []Filter
	parsedEntries      []*logcat.Entry
	needsUpdate        bool
	highlightedEntry   *logcat.Entry
	selectionMode      bool
	selectedEntries    map[*logcat.Entry]bool
	selectionAnchor    *logcat.Entry
	lineEntries        []*logcat.Entry
	entryLineRanges    map[*logcat.Entry]entryLineRange
	renderedLines      []string
	renderedUpTo       int
	renderReset        bool
	viewportContent    string
	lastRenderedTag    string
	lastRenderedTime   string
	lastRenderedCont   bool
	lastRenderedPrio   logcat.Priority
	lastRenderedPID    string
	lastRenderedTID    string
	lastRenderedPrev   *logcat.Entry
	lastRenderedLast   *logcat.Entry
	renderScheduled    bool
	wrapLines          bool
	autoScroll         bool
	showDeviceSelect   bool
	deviceList         list.Model
	devices            []adb.Device
	selectedDevice     string // Device serial or model
	errorMessage       string
	showTimestamp      bool
	logLevelBackground bool
	coloredMessages    bool
	showClearConfirm   bool
	clearInput         textinput.Model
}

type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

type Filter struct {
	isTag   bool
	pattern string
	regex   *regexp.Regexp
}

type logLineMsg struct {
	lines []string
}
type updateViewportMsg struct{}
type appStatusMsg string
type deviceStatusMsg string

type entryLineRange struct {
	start int
	end   int
}

func NewModel(appID string, tailSize int) Model {
	prefs, prefsLoaded, prefsErr := config.Load()
	if prefsErr != nil {
		prefsLoaded = false
	}

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
		Foreground(accentColor).
		Padding(0, 1)

	filterInput := textinput.New()
	filterInput.Placeholder = "e.g., tag:MyTag, some message"
	filterInput.CharLimit = 500
	filterInput.Width = 80

	clearInput := textinput.New()
	clearInput.Placeholder = "y/n"
	clearInput.CharLimit = 10
	clearInput.Width = 40

	entryCapacity := 10000
	if tailSize > 0 {
		entryCapacity = tailSize
	}

	// Check for multiple devices
	devices, deviceErr := adb.GetDevices()
	showDeviceSelect := false
	var deviceList list.Model

	if deviceErr == nil && len(devices) > 1 {
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
			Foreground(GetAccentColor()).
			Padding(0, 1)
	} else if deviceErr == nil && len(devices) == 1 {
		// Single device - use it automatically
		logManager := logcat.NewManager(appID, tailSize)
		logManager.SetDevice(devices[0].Serial)
		model := Model{
			appID:              appID,
			logManager:         logManager,
			lineChan:           make(chan string, 100),
			showLogLevel:       false,
			logLevelList:       logLevelList,
			minLogLevel:        logcat.Verbose,
			showFilter:         false,
			filterInput:        filterInput,
			filters:            []Filter{},
			parsedEntries:      make([]*logcat.Entry, 0, entryCapacity),
			needsUpdate:        false,
			highlightedEntry:   nil,
			selectionMode:      false,
			selectedEntries:    make(map[*logcat.Entry]bool),
			selectionAnchor:    nil,
			autoScroll:         true,
			showDeviceSelect:   false,
			deviceList:         list.Model{},
			devices:            devices,
			selectedDevice:     devices[0].Model,
			deviceStatus:       "connected",
			showClearConfirm:   false,
			clearInput:         clearInput,
			showTimestamp:      false,
			logLevelBackground: false,
			coloredMessages:    true,
			wrapLines:          false,
		}
		if prefsLoaded {
			model.applyPreferences(prefs)
		}
		return model
	}

	model := Model{
		appID:              appID,
		logManager:         logcat.NewManager(appID, tailSize),
		lineChan:           make(chan string, 100),
		showLogLevel:       false,
		logLevelList:       logLevelList,
		minLogLevel:        logcat.Verbose,
		showFilter:         false,
		filterInput:        filterInput,
		filters:            []Filter{},
		parsedEntries:      make([]*logcat.Entry, 0, entryCapacity),
		needsUpdate:        false,
		highlightedEntry:   nil,
		selectionMode:      false,
		selectedEntries:    make(map[*logcat.Entry]bool),
		selectionAnchor:    nil,
		autoScroll:         true,
		showDeviceSelect:   showDeviceSelect,
		deviceList:         deviceList,
		devices:            devices,
		selectedDevice:     "",
		showClearConfirm:   false,
		clearInput:         clearInput,
		showTimestamp:      false,
		logLevelBackground: false,
		coloredMessages:    true,
		wrapLines:          false,
	}

	if prefsLoaded {
		model.applyPreferences(prefs)
	}

	return model
}

func (m *Model) applyPreferences(prefs config.Preferences) {
	if priority, ok := priorityFromConfig(prefs.MinLogLevel); ok {
		m.minLogLevel = priority
		if priority >= logcat.Verbose && priority <= logcat.Fatal {
			m.logLevelList.Select(int(priority))
		}
	}

	m.showTimestamp = prefs.ShowTimestamp
	m.wrapLines = prefs.WrapLines
	if prefs.LogLevelBackground != nil {
		m.logLevelBackground = *prefs.LogLevelBackground
	} else {
		m.logLevelBackground = false
	}
	if prefs.ColoredMessages != nil {
		m.coloredMessages = *prefs.ColoredMessages
	} else {
		m.coloredMessages = true
	}

	if prefs.TagColumnWidth > 0 {
		SetTagColumnWidth(prefs.TagColumnWidth)
	} else {
		SetTagColumnWidth(DefaultTagColumnWidth)
	}

	if len(prefs.Filters) == 0 {
		m.filters = []Filter{}
		m.filterInput.SetValue("")
		return
	}

	m.filters = make([]Filter, 0, len(prefs.Filters))
	filterStrings := make([]string, 0, len(prefs.Filters))

	for _, pref := range prefs.Filters {
		if pref.Pattern == "" {
			continue
		}

		regex, err := regexp.Compile("(?i)" + pref.Pattern)
		if err != nil {
			continue
		}

		m.filters = append(m.filters, Filter{
			isTag:   pref.IsTag,
			pattern: pref.Pattern,
			regex:   regex,
		})
		filterStrings = append(filterStrings, formatFilterPreference(pref))
	}

	if len(filterStrings) > 0 {
		m.filterInput.SetValue(strings.Join(filterStrings, ", "))
	} else {
		m.filterInput.SetValue("")
	}
}

func (m *Model) resetRenderCache() {
	m.renderedLines = nil
	m.lineEntries = nil
	m.entryLineRanges = nil
	m.viewportContent = ""
	m.renderedUpTo = 0
	m.lastRenderedTag = ""
	m.lastRenderedTime = ""
	m.lastRenderedCont = false
	m.lastRenderedPrio = logcat.Unknown
	m.lastRenderedPID = ""
	m.lastRenderedTID = ""
	m.lastRenderedPrev = nil
	m.lastRenderedLast = nil
	m.renderReset = true
}

func priorityFromConfig(value string) (logcat.Priority, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false
	}

	switch strings.ToUpper(trimmed) {
	case "V", "VERBOSE":
		return logcat.Verbose, true
	case "D", "DEBUG":
		return logcat.Debug, true
	case "I", "INFO":
		return logcat.Info, true
	case "W", "WARN", "WARNING":
		return logcat.Warn, true
	case "E", "ERROR":
		return logcat.Error, true
	case "F", "FATAL":
		return logcat.Fatal, true
	default:
		return 0, false
	}
}

func formatFilterPreference(pref config.FilterPreference) string {
	pattern := strings.ReplaceAll(pref.Pattern, ",", "\\,")
	if pref.IsTag {
		return "tag:" + pattern
	}
	return pattern
}

func isStackTraceLine(message string) bool {
	trimmed := strings.TrimLeft(message, " \t")
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "at ") {
		return true
	}
	if strings.HasPrefix(trimmed, "Caused by:") {
		return true
	}
	if strings.HasPrefix(trimmed, "Suppressed:") {
		return true
	}
	if strings.HasPrefix(trimmed, "...") {
		return true
	}
	if strings.HasPrefix(trimmed, "Stack trace:") {
		return true
	}
	return false
}

func sameEntryMeta(a, b *logcat.Entry) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Timestamp == b.Timestamp &&
		a.Tag == b.Tag &&
		a.Priority == b.Priority &&
		a.PID == b.PID &&
		a.TID == b.TID
}

func shouldContinue(prev, curr, next *logcat.Entry) bool {
	if !sameEntryMeta(prev, curr) {
		return false
	}
	if isStackTraceLine(curr.Message) {
		return true
	}
	if sameEntryMeta(curr, next) && isStackTraceLine(next.Message) {
		return true
	}
	return false
}

func (m Model) Init() tea.Cmd {
	// If showing device selector, don't start logcat yet
	if m.showDeviceSelect {
		return nil
	}

	cmds := []tea.Cmd{
		startLogcat(m.logManager, m.lineChan),
		waitForLogLine(m.lineChan),
	}

	// If filtering by app, listen for status updates
	if m.appID != "" {
		cmds = append(cmds, waitForStatus(m.logManager.StatusChan()))
	}
	if m.selectedDevice != "" {
		cmds = append(cmds, waitForDeviceStatus(m.logManager.DeviceStatusChan()))
	}

	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Calculate header height based on what will be shown
		headerHeight, footerHeight := m.layoutHeights()
		verticalMargin := headerHeight + footerHeight
		viewportHeight := msg.Height - verticalMargin
		if viewportHeight < 0 {
			viewportHeight = 0
		}

		if !m.ready {
			m.viewport = viewport.New(msg.Width, viewportHeight)
			m.viewport.YPosition = 0
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = viewportHeight
			m.viewport.YPosition = 0
		}

		m.width = msg.Width
		m.height = msg.Height
		m.renderReset = true
		m.needsUpdate = true
		if !m.renderScheduled {
			m.renderScheduled = true
			cmds = append(cmds, scheduleViewportUpdate())
		}

	case logLineMsg:
		for _, line := range msg.lines {
			entry, _ := logcat.ParseLine(line)
			if entry != nil {
				m.parsedEntries = append(m.parsedEntries, entry)
			}
		}
		m.needsUpdate = true
		if !m.renderScheduled {
			m.renderScheduled = true
			cmds = append(cmds, scheduleViewportUpdate())
		}

		if !m.terminating {
			cmds = append(cmds, waitForLogLine(m.lineChan))
		}

	case appStatusMsg:
		m.appStatus = string(msg)
		if !m.terminating {
			cmds = append(cmds, waitForStatus(m.logManager.StatusChan()))
		}
	case deviceStatusMsg:
		m.deviceStatus = string(msg)
		if !m.terminating {
			cmds = append(cmds, waitForDeviceStatus(m.logManager.DeviceStatusChan()))
		}

	case updateViewportMsg:
		m.renderScheduled = false
		if m.needsUpdate && m.ready {
			m.updateViewportWithScroll(m.autoScroll)
			m.needsUpdate = false
		}
		if m.needsUpdate && !m.renderScheduled {
			m.renderScheduled = true
			cmds = append(cmds, scheduleViewportUpdate())
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
					device := adb.Device(i)
					m.logManager.SetDevice(device.Serial)
					m.selectedDevice = device.Model
					m.deviceStatus = "connected"
					m.showDeviceSelect = false
					// Start logcat now that device is selected
					cmds := []tea.Cmd{
						startLogcat(m.logManager, m.lineChan),
						waitForLogLine(m.lineChan),
					}
					if m.appID != "" {
						cmds = append(cmds, waitForStatus(m.logManager.StatusChan()))
					}
					if m.selectedDevice != "" {
						cmds = append(cmds, waitForDeviceStatus(m.logManager.DeviceStatusChan()))
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
					m.resetRenderCache()
					m.updateViewport()
				}
				return m, nil
			case "v":
				m.minLogLevel = logcat.Verbose
				m.showLogLevel = false
				m.resetRenderCache()
				m.updateViewport()
				return m, nil
			case "d":
				m.minLogLevel = logcat.Debug
				m.showLogLevel = false
				m.resetRenderCache()
				m.updateViewport()
				return m, nil
			case "i":
				m.minLogLevel = logcat.Info
				m.showLogLevel = false
				m.resetRenderCache()
				m.updateViewport()
				return m, nil
			case "w":
				m.minLogLevel = logcat.Warn
				m.showLogLevel = false
				m.resetRenderCache()
				m.updateViewport()
				return m, nil
			case "e":
				m.minLogLevel = logcat.Error
				m.showLogLevel = false
				m.resetRenderCache()
				m.updateViewport()
				return m, nil
			case "f":
				m.minLogLevel = logcat.Fatal
				m.showLogLevel = false
				m.resetRenderCache()
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
				m.resetRenderCache()
				m.updateViewport()
				return m, nil
			}
		} else if m.showClearConfirm {
			switch msg.String() {
			case "esc":
				m.showClearConfirm = false
				m.clearInput.Blur()
				m.clearInput.SetValue("")
				return m, nil
			case "enter":
				input := strings.ToLower(strings.TrimSpace(m.clearInput.Value()))
				if input == "y" || input == "yes" {
					// Clear the log display
					m.parsedEntries = make([]*logcat.Entry, 0, 10000)
					m.highlightedEntry = nil
					m.clearSelection()
					m.resetRenderCache()
					m.updateViewport()
				}
				m.showClearConfirm = false
				m.clearInput.Blur()
				m.clearInput.SetValue("")
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
				m.renderReset = true
				m.updateViewportWithScroll(false)
				return m, nil
			case "v": // v to enter selection mode
				m.autoScroll = false
				m.enterSelectionMode()
				m.renderReset = true
				m.updateViewportWithScroll(false)
				return m, nil
			case "c":
				if m.selectionMode && len(m.selectedEntries) > 0 {
					m.copySelectedLines()
					m.clearSelection()
					m.selectionMode = false
					m.renderReset = true
					m.updateViewportWithScroll(false)
				} else if !m.selectionMode {
					// Show clear confirmation dialog
					m.showClearConfirm = true
					m.clearInput.Focus()
					return m, textinput.Blink
				}
				return m, nil
			case "C": // C to copy message only in selection mode
				if m.selectionMode && len(m.selectedEntries) > 0 {
					m.copySelectedMessagesOnly()
					m.clearSelection()
					m.selectionMode = false
					m.renderReset = true
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
				m.renderReset = true
				m.updateViewportWithScroll(false)
				return m, nil
			case "k", "up":
				m.autoScroll = false
				if m.selectionMode {
					m.extendSelectionUp()
				} else {
					m.moveHighlightUp()
				}
				m.renderReset = true
				m.updateViewportWithScroll(false)
				return m, nil
			case "z", "Z":
				m.showTimestamp = !m.showTimestamp
				m.resetRenderCache()
				m.updateViewportWithScroll(false)
				return m, nil
			case "w", "W":
				m.wrapLines = !m.wrapLines
				m.resetRenderCache()
				m.updateViewportWithScroll(m.autoScroll)
				return m, nil
			}
		}

	case tea.MouseMsg:
		// Only handle mouse release (not drag) to avoid performance issues
		if msg.Type == tea.MouseRelease && msg.Button == tea.MouseButtonLeft && !m.showLogLevel && !m.showFilter && !m.showDeviceSelect {
			m.autoScroll = false
			m.handleMouseClick(msg.Y)
			m.renderReset = true
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
	} else if m.showClearConfirm {
		m.clearInput, cmd = m.clearInput.Update(msg)
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

func (m Model) layoutHeights() (int, int) {
	headerHeight := 3
	if !m.showFilter && !m.showClearConfirm {
		headerHeight = 4
	}
	footerHeight := 2
	if m.showFilter || m.showClearConfirm {
		footerHeight = 3
	}
	return headerHeight, footerHeight
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
				filterText = "tag:" + f.pattern
			} else {
				filterText = f.pattern
			}

			// Use filter colors for filter badges
			filterColor := FilterColor(filterText)
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
	case "stopped":
		statusStyle = statusStyle.Foreground(lipgloss.AdaptiveColor{Light: "172", Dark: "215"}) // Orange
		statusText = "not running"
	case "reconnecting":
		statusStyle = statusStyle.Foreground(lipgloss.AdaptiveColor{Light: "172", Dark: "215"}) // Orange
		statusText = "not running"
	case "error":
		statusStyle = statusStyle.Foreground(GetErrorColor())
		statusText = "error"
	}

	deviceStatusStyle := lipgloss.NewStyle()
	var deviceStatusText string
	if m.deviceStatus == "disconnected" {
		deviceStatusStyle = deviceStatusStyle.Foreground(lipgloss.AdaptiveColor{Light: "172", Dark: "215"}) // Orange
		deviceStatusText = "disconnected"
	}

	// Get color for current log level
	var logLevelColor lipgloss.TerminalColor
	switch m.minLogLevel {
	case logcat.Verbose:
		logLevelColor = GetVerboseColor()
	case logcat.Debug:
		logLevelColor = GetDebugColor()
	case logcat.Info:
		logLevelColor = GetInfoColor()
	case logcat.Warn:
		logLevelColor = GetWarnColor()
	case logcat.Error:
		logLevelColor = GetErrorColor()
	case logcat.Fatal:
		logLevelColor = GetFatalColor()
	default:
		logLevelColor = GetVerboseColor()
	}

	logLevelStyle := lipgloss.NewStyle().Foreground(logLevelColor)

	// Build header lines
	var headerLines []string

	// First line: log level and filters
	logLevelLine := fmt.Sprintf("log level: %s%s",
		logLevelStyle.Render(strings.ToLower(m.minLogLevel.Name())), filterInfo)
	headerLines = append(headerLines, headerStyle.Render(logLevelLine))

	// Second line: app and device info (always show)
	if !m.showFilter && !m.showClearConfirm {
		var infoParts []string
		appStyle := lipgloss.NewStyle().Foreground(GetAccentColor())
		deviceStyle := lipgloss.NewStyle().Foreground(GetAccentColor())
		if m.appID != "" {
			appInfoText := fmt.Sprintf("app: %s", appStyle.Render(appInfo))
			if statusText != "" && m.deviceStatus != "disconnected" {
				appInfoText = fmt.Sprintf("app: %s (%s)", appStyle.Render(appInfo), statusStyle.Render(statusText))
			}
			infoParts = append(infoParts, appInfoText)
		} else {
			infoParts = append(infoParts, "app: all")
		}
		if m.selectedDevice != "" {
			deviceInfo := fmt.Sprintf("device: %s", deviceStyle.Render(m.selectedDevice))
			if deviceStatusText != "" {
				deviceInfo = fmt.Sprintf("device: %s (%s)", deviceStyle.Render(m.selectedDevice), deviceStatusStyle.Render(deviceStatusText))
			}
			infoParts = append(infoParts, deviceInfo)
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
			Foreground(GetAccentColor()).
			Bold(true).
			Render("filter: ")

		filterHelp := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render("comma-separated, tag: prefix for tags | enter: apply | esc: cancel")

		filterLine := footerStyleNoBorder.Render(filterLabel + m.filterInput.View())
		helpLine := footerStyle.Render(filterHelp)
		footer = lipgloss.JoinVertical(lipgloss.Left, filterLine, helpLine)
	} else if m.showClearConfirm {
		clearLabel := lipgloss.NewStyle().
			Foreground(GetAccentColor()).
			Bold(true).
			Render("clear log? ")

		clearHelp := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render("y/yes: clear | n/no: cancel | esc: cancel")

		clearLine := footerStyleNoBorder.Render(clearLabel + m.clearInput.View())
		helpLine := footerStyle.Render(clearHelp)
		footer = lipgloss.JoinVertical(lipgloss.Left, clearLine, helpLine)
	} else if m.selectionMode {
		selectionInfo := "SELECTION | j/k: extend | c: copy lines | C: copy messages | esc: cancel"
		footer = footerStyle.Render(selectionInfo)
	} else {
		baseHelp := "q: quit | c: clear | click: highlight | v: select | l: log level | f: filter | z: toggle timestamp | w: toggle line wrap"
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
	if m.renderReset || m.renderedUpTo > len(m.parsedEntries) {
		m.rebuildViewport(scrollToBottom)
		m.renderReset = false
		return
	}

	if m.renderedUpTo == len(m.parsedEntries) {
		if scrollToBottom {
			m.viewport.GotoBottom()
		}
		return
	}

	m.appendViewport(scrollToBottom)
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

func (m *Model) rebuildViewport(scrollToBottom bool) {
	lines := make([]string, 0, len(m.parsedEntries))
	lineEntries := make([]*logcat.Entry, 0, len(m.parsedEntries))
	entryLineRanges := make(map[*logcat.Entry]entryLineRange, len(m.parsedEntries))
	maxWidth := 0
	if m.wrapLines {
		maxWidth = m.viewport.Width
	}
	visible := make([]*logcat.Entry, 0, len(m.parsedEntries))
	for _, entry := range m.parsedEntries {
		if entry.Priority >= m.minLogLevel && m.matchesFilters(entry) {
			visible = append(visible, entry)
		}
	}

	var lastTag string
	var lastTimestamp string
	var lastWasContinuation bool
	var lastPriority = logcat.Unknown
	var lastPID string
	var lastTID string
	var lastPrevEntry *logcat.Entry
	var lastEntry *logcat.Entry

	selectedStyle := lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "251", Dark: "240"})
	highlightStyle := lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "254", Dark: "237"})

	for i, entry := range visible {
		var prev *logcat.Entry
		if i > 0 {
			prev = visible[i-1]
		}
		var next *logcat.Entry
		if i+1 < len(visible) {
			next = visible[i+1]
		}
		continuation := shouldContinue(prev, entry, next)
		showTag := false

		if !continuation {
			if lastWasContinuation {
				showTag = true
			} else {
				showTag = entry.Tag != lastTag
			}
		}

		var entryLines []string
		if m.selectedEntries[entry] {
			entryLines = m.formatEntryWithAllColumnsSelectedLines(entry, showTag, selectedStyle, continuation, maxWidth)
		} else if entry == m.highlightedEntry {
			entryLines = m.formatEntryWithAllColumnsSelectedLines(entry, showTag, highlightStyle, continuation, maxWidth)
		} else {
			entryLines = FormatEntryLines(entry, lipgloss.NewStyle(), showTag, m.showTimestamp, m.logLevelBackground, m.coloredMessages, continuation, maxWidth)
		}

		startLine := len(lineEntries)
		lines = append(lines, entryLines...)
		for range entryLines {
			lineEntries = append(lineEntries, entry)
		}
		if len(entryLines) > 0 {
			entryLineRanges[entry] = entryLineRange{start: startLine, end: len(lineEntries) - 1}
		}
		lastPrevEntry = lastEntry
		lastEntry = entry
		lastTag = entry.Tag
		lastTimestamp = entry.Timestamp
		lastWasContinuation = continuation
		lastPriority = entry.Priority
		lastPID = entry.PID
		lastTID = entry.TID
	}

	m.renderedLines = lines
	m.lineEntries = lineEntries
	m.entryLineRanges = entryLineRanges
	m.lastRenderedTag = lastTag
	m.lastRenderedTime = lastTimestamp
	m.lastRenderedCont = lastWasContinuation
	m.lastRenderedPrio = lastPriority
	m.lastRenderedPID = lastPID
	m.lastRenderedTID = lastTID
	m.lastRenderedPrev = lastPrevEntry
	m.lastRenderedLast = lastEntry
	m.renderedUpTo = len(m.parsedEntries)
	m.viewportContent = joinLines(lines)
	m.viewport.SetContent(m.viewportContent)

	if scrollToBottom {
		m.viewport.GotoBottom()
	}
}

func (m *Model) appendViewport(scrollToBottom bool) {
	if m.entryLineRanges == nil {
		m.entryLineRanges = make(map[*logcat.Entry]entryLineRange)
	}
	maxWidth := 0
	if m.wrapLines {
		maxWidth = m.viewport.Width
	}

	selectedStyle := lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "251", Dark: "240"})
	highlightStyle := lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "254", Dark: "237"})

	newLines := make([]string, 0)
	lastTag := m.lastRenderedTag
	lastTimestamp := m.lastRenderedTime
	lastWasContinuation := m.lastRenderedCont
	lastPriority := m.lastRenderedPrio
	lastPID := m.lastRenderedPID
	lastTID := m.lastRenderedTID
	lastPrevEntry := m.lastRenderedPrev
	lastEntry := m.lastRenderedLast

	pendingVisible := make([]*logcat.Entry, 0)
	for i := m.renderedUpTo; i < len(m.parsedEntries); i++ {
		entry := m.parsedEntries[i]
		if entry.Priority >= m.minLogLevel && m.matchesFilters(entry) {
			pendingVisible = append(pendingVisible, entry)
		}
	}

	if len(pendingVisible) > 0 && m.lastRenderedLast != nil {
		if shouldContinue(m.lastRenderedPrev, m.lastRenderedLast, pendingVisible[0]) {
			m.rebuildViewport(scrollToBottom)
			return
		}
	}

	for i, entry := range pendingVisible {
		var prev *logcat.Entry
		if i == 0 {
			prev = lastEntry
		} else {
			prev = pendingVisible[i-1]
		}
		var next *logcat.Entry
		if i+1 < len(pendingVisible) {
			next = pendingVisible[i+1]
		}
		continuation := shouldContinue(prev, entry, next)
		showTag := false

		if !continuation {
			if lastWasContinuation {
				showTag = true
			} else {
				showTag = entry.Tag != lastTag
			}
		}

		var entryLines []string
		if m.selectedEntries[entry] {
			entryLines = m.formatEntryWithAllColumnsSelectedLines(entry, showTag, selectedStyle, continuation, maxWidth)
		} else if entry == m.highlightedEntry {
			entryLines = m.formatEntryWithAllColumnsSelectedLines(entry, showTag, highlightStyle, continuation, maxWidth)
		} else {
			entryLines = FormatEntryLines(entry, lipgloss.NewStyle(), showTag, m.showTimestamp, m.logLevelBackground, m.coloredMessages, continuation, maxWidth)
		}

		startLine := len(m.lineEntries)
		newLines = append(newLines, entryLines...)
		m.renderedLines = append(m.renderedLines, entryLines...)
		for range entryLines {
			m.lineEntries = append(m.lineEntries, entry)
		}
		if len(entryLines) > 0 {
			m.entryLineRanges[entry] = entryLineRange{start: startLine, end: len(m.lineEntries) - 1}
		}

		lastPrevEntry = lastEntry
		lastEntry = entry
		lastTag = entry.Tag
		lastTimestamp = entry.Timestamp
		lastWasContinuation = continuation
		lastPriority = entry.Priority
		lastPID = entry.PID
		lastTID = entry.TID
	}

	m.lastRenderedTag = lastTag
	m.lastRenderedTime = lastTimestamp
	m.lastRenderedCont = lastWasContinuation
	m.lastRenderedPrio = lastPriority
	m.lastRenderedPID = lastPID
	m.lastRenderedTID = lastTID
	m.lastRenderedPrev = lastPrevEntry
	m.lastRenderedLast = lastEntry
	m.renderedUpTo = len(m.parsedEntries)

	if len(newLines) > 0 {
		chunk := joinLines(newLines)
		if m.viewportContent == "" {
			m.viewportContent = chunk
		} else {
			m.viewportContent += "\n" + chunk
		}
		m.viewport.SetContent(m.viewportContent)
	}

	if scrollToBottom {
		m.viewport.GotoBottom()
	}
}

// formatEntryWithAllColumnsSelected formats an entry with background applied to all columns while preserving colors.
// When continuation is true, timestamp, tag, and priority columns are rendered as blank spaces to visually
// connect entries sharing the same timestamp.
func (m *Model) formatEntryWithAllColumnsSelectedLines(entry *logcat.Entry, showTag bool, bgStyle lipgloss.Style, continuation bool, maxWidth int) []string {
	// Get colors for this priority
	var priorityColor lipgloss.TerminalColor
	var priorityBgColor lipgloss.TerminalColor
	switch entry.Priority {
	case logcat.Verbose:
		priorityColor = GetVerboseColor()
		priorityBgColor = GetVerboseBgColor()
	case logcat.Debug:
		priorityColor = GetDebugColor()
		priorityBgColor = GetDebugBgColor()
	case logcat.Info:
		priorityColor = GetInfoColor()
		priorityBgColor = GetInfoBgColor()
	case logcat.Warn:
		priorityColor = GetWarnColor()
		priorityBgColor = GetWarnBgColor()
	case logcat.Error:
		priorityColor = GetErrorColor()
		priorityBgColor = GetErrorBgColor()
	case logcat.Fatal:
		priorityColor = GetFatalColor()
		priorityBgColor = GetFatalBgColor()
	default:
		priorityColor = GetVerboseColor()
		priorityBgColor = GetVerboseBgColor()
	}

	priorityStyle := lipgloss.NewStyle().Bold(true)
	if m.logLevelBackground {
		priorityStyle = priorityStyle.
			Foreground(lipgloss.AdaptiveColor{Light: "255", Dark: "0"}).
			Background(priorityBgColor)
	} else {
		priorityStyle = priorityStyle.
			Foreground(priorityColor).
			Background(bgStyle.GetBackground())
	}

	tagStyle := lipgloss.NewStyle().
		Foreground(TagColor(entry.Tag)).
		Background(bgStyle.GetBackground())

	messageColor := lipgloss.TerminalColor(lipgloss.AdaptiveColor{Light: "0", Dark: "254"})
	if m.coloredMessages {
		messageColor = priorityColor
	}
	messageStyle := lipgloss.NewStyle().
		Foreground(messageColor).
		Background(bgStyle.GetBackground())

	var tagStr string
	if showTag && !continuation {
		tagText := truncateString(entry.Tag, TagColumnWidth())
		tagStr = tagStyle.Render(fmt.Sprintf("%*s", TagColumnWidth(), tagText))
	} else {
		tagStr = bgStyle.Render(strings.Repeat(" ", TagColumnWidth()))
	}

	message := entry.Message

	priorityWidth := len(entry.Priority.String()) + 2
	priorityStr := bgStyle.Render(strings.Repeat(" ", priorityWidth))
	if !continuation {
		priorityStr = priorityStyle.Render(" " + entry.Priority.String() + " ")
	}
	if m.showTimestamp {
		sep := bgStyle.Render(" ")
		timestampStyle := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "238", Dark: "250"}).
			Background(bgStyle.GetBackground())
		timestampContent := strings.Repeat(" ", timestampColumnWidth)
		if !continuation {
			timestampContent = fmt.Sprintf("%-*s", timestampColumnWidth, entry.Timestamp)
		}
		timestampStr := timestampStyle.Render(timestampContent)
		prefix := timestampStr + sep + tagStr + sep + priorityStr + sep
		contPrefix := timestampStyle.Render(strings.Repeat(" ", timestampColumnWidth)) +
			sep +
			bgStyle.Render(strings.Repeat(" ", TagColumnWidth())) +
			sep +
			bgStyle.Render(strings.Repeat(" ", priorityWidth)) +
			sep
		renderOne := func(s string) string { return messageStyle.Render(s) }
		return wrapWithPrefix(message, renderOne, prefix, contPrefix, maxWidth)
	}

	sep := bgStyle.Render(" ")
	prefix := tagStr + sep + priorityStr + sep
	contPrefix := bgStyle.Render(strings.Repeat(" ", TagColumnWidth())) +
		sep +
		bgStyle.Render(strings.Repeat(" ", priorityWidth)) +
		sep
	renderOne := func(s string) string { return messageStyle.Render(s) }
	return wrapWithPrefix(message, renderOne, prefix, contPrefix, maxWidth)
}

func truncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
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

		regex, err := regexp.Compile("(?i)" + part)
		if err == nil {
			filter.pattern = part
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

	// Separate tag and message filters
	var tagFilters, messageFilters []Filter
	for _, filter := range m.filters {
		if filter.isTag {
			tagFilters = append(tagFilters, filter)
		} else {
			messageFilters = append(messageFilters, filter)
		}
	}

	// Tag filters: entry tag must match ANY tag filter (OR logic)
	if len(tagFilters) > 0 {
		tagMatched := false
		for _, filter := range tagFilters {
			if filter.regex.MatchString(entry.Tag) {
				tagMatched = true
				break
			}
		}
		if !tagMatched {
			return false
		}
	}

	// Message filters: entry message must match ALL message filters (AND logic)
	for _, filter := range messageFilters {
		if !filter.regex.MatchString(entry.Message) {
			return false
		}
	}

	return true
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

const maxLogBatch = 200

func waitForLogLine(lineChan <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-lineChan
		if !ok {
			return nil
		}
		lines := []string{line}
		for i := 1; i < maxLogBatch; i++ {
			select {
			case next, ok := <-lineChan:
				if !ok {
					return logLineMsg{lines: lines}
				}
				lines = append(lines, next)
			default:
				return logLineMsg{lines: lines}
			}
		}
		return logLineMsg{lines: lines}
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

func waitForDeviceStatus(statusChan <-chan string) tea.Cmd {
	return func() tea.Msg {
		status, ok := <-statusChan
		if !ok {
			return nil
		}
		return deviceStatusMsg(status)
	}
}

const renderDebounce = 200 * time.Millisecond

func scheduleViewportUpdate() tea.Cmd {
	return tea.Tick(renderDebounce, func(time.Time) tea.Msg {
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
	// Mouse Y is 1-indexed, and viewport is rendered first (before header)
	// So viewport starts at Y=1

	// Calculate line within viewport (0-indexed)
	lineInViewport := y

	// If click is beyond viewport height, ignore (in footer area)
	if lineInViewport < 0 || lineInViewport >= m.viewport.Height {
		return
	}

	// Add viewport scroll offset to get actual line in content
	clickedLine := lineInViewport + m.viewport.YOffset

	if clickedLine < 0 || clickedLine >= len(m.lineEntries) {
		return
	}

	clickedEntry := m.lineEntries[clickedLine]
	if clickedEntry == nil {
		return
	}

	visible := m.getVisibleEntries()
	if m.selectionMode {
		// In selection mode: extend selection to clicked entry
		m.extendSelectionTo(clickedEntry, visible)
	} else {
		// Not in selection mode: just highlight
		m.highlightedEntry = clickedEntry
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

func (m *Model) entryLineRange(entry *logcat.Entry) (int, int, bool) {
	if entry == nil {
		return 0, 0, false
	}
	if m.entryLineRanges != nil {
		if r, ok := m.entryLineRanges[entry]; ok {
			return r.start, r.end, true
		}
	}

	start := -1
	end := -1
	for i, e := range m.lineEntries {
		if e == entry {
			if start == -1 {
				start = i
			}
			end = i
		}
	}

	if start == -1 {
		return 0, 0, false
	}

	return start, end, true
}

// ensureLineVisible scrolls the viewport to ensure the entry at the given index is visible.
func (m *Model) ensureLineVisible(entryIndex int) {
	visible := m.getVisibleEntries()
	if len(visible) == 0 || entryIndex < 0 || entryIndex >= len(visible) {
		return
	}

	startLine, endLine, ok := m.entryLineRange(visible[entryIndex])
	if !ok {
		return
	}

	viewportTop := m.viewport.YOffset
	viewportBottom := m.viewport.YOffset + m.viewport.Height - 1

	// If line is above viewport, scroll up to show it
	if startLine < viewportTop {
		m.viewport.SetYOffset(startLine)
	}

	// If line is below viewport, scroll down to show it at the bottom
	if endLine > viewportBottom {
		newOffset := endLine - m.viewport.Height + 1
		if newOffset < 0 {
			newOffset = 0
		}
		m.viewport.SetYOffset(newOffset)
	}
}

// ensureEntryVisible scrolls the viewport to ensure the given entry is visible,
// positioning it roughly in the center to allow movement in both directions
func (m *Model) ensureEntryVisible(entry *logcat.Entry) {
	if entry == nil {
		return
	}

	startLine, endLine, ok := m.entryLineRange(entry)
	if !ok {
		return
	}

	// Check if the line is currently visible in the viewport
	viewportTop := m.viewport.YOffset
	viewportBottom := m.viewport.YOffset + m.viewport.Height - 1

	// If the line is not visible, center it in the viewport
	if startLine < viewportTop || endLine > viewportBottom {
		centerLine := startLine + (endLine-startLine)/2
		// Calculate offset to center the line in the viewport
		centerOffset := centerLine - m.viewport.Height/2

		// Ensure we don't scroll before the start
		if centerOffset < 0 {
			centerOffset = 0
		}

		// Ensure we don't scroll past the end
		maxOffset := len(m.lineEntries) - m.viewport.Height
		if maxOffset < 0 {
			maxOffset = 0
		}
		if centerOffset > maxOffset {
			centerOffset = maxOffset
		}

		m.viewport.SetYOffset(centerOffset)
	}
}

// enterSelectionMode enters selection mode
func (m *Model) enterSelectionMode() {
	// If already in selection mode, do nothing
	if m.selectionMode {
		return
	}

	m.selectionMode = true

	// If there's a highlighted entry, use it as the anchor
	if m.highlightedEntry != nil {
		m.selectedEntries = make(map[*logcat.Entry]bool)
		m.selectedEntries[m.highlightedEntry] = true
		m.selectionAnchor = m.highlightedEntry
		// Ensure the highlighted entry is visible
		m.ensureEntryVisible(m.highlightedEntry)
	} else {
		// Otherwise, select the last visible entry
		visible := m.getVisibleEntries()
		if len(visible) > 0 {
			lastEntry := visible[len(visible)-1]
			m.selectedEntries = make(map[*logcat.Entry]bool)
			m.selectedEntries[lastEntry] = true
			m.selectionAnchor = lastEntry
			m.highlightedEntry = lastEntry
			// Ensure the selected entry is visible
			m.ensureEntryVisible(lastEntry)
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
		m.ensureLineVisible(0)
		return
	}

	// Find current highlight and move down
	for i, entry := range visible {
		if entry == m.highlightedEntry && i < len(visible)-1 {
			m.highlightedEntry = visible[i+1]
			m.ensureLineVisible(i + 1)
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
		m.ensureLineVisible(len(visible) - 1)
		return
	}

	// Find current highlight and move up
	for i, entry := range visible {
		if entry == m.highlightedEntry && i > 0 {
			m.highlightedEntry = visible[i-1]
			m.ensureLineVisible(i - 1)
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
		newEntry := visible[lowestIdx+1]
		m.selectedEntries[newEntry] = true
		// Scroll to ensure the new entry is visible
		m.ensureLineVisible(lowestIdx + 1)
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
		newEntry := visible[highestIdx-1]
		m.selectedEntries[newEntry] = true
		// Scroll to ensure the new entry is visible
		m.ensureLineVisible(highestIdx - 1)
	}
}

// clearSelection clears the selection
func (m *Model) clearSelection() {
	m.selectedEntries = make(map[*logcat.Entry]bool)
	m.selectionAnchor = nil
}

// copySelectedLines copies selected lines (whole entries) to clipboard
func (m *Model) copySelectedLines() {
	if len(m.selectedEntries) == 0 {
		return
	}

	// Get selected entries in order
	visible := m.getVisibleEntries()
	var lines []string
	for _, entry := range visible {
		if m.selectedEntries[entry] {
			// Copy the whole line without any styling or ANSI codes
			lines = append(lines, entry.FormatPlain())
		}
	}

	clipboard := strings.Join(lines, "\n")
	_ = copyToClipboard(clipboard)
}

// copySelectedMessagesOnly copies only the message column of selected entries to clipboard
func (m *Model) copySelectedMessagesOnly() {
	if len(m.selectedEntries) == 0 {
		return
	}

	// Get selected entries in order
	visible := m.getVisibleEntries()
	var lines []string
	for _, entry := range visible {
		if m.selectedEntries[entry] {
			lines = append(lines, entry.Message)
		}
	}

	clipboard := strings.Join(lines, "\n")
	_ = copyToClipboard(clipboard)
}

func (m Model) PersistPreferences() error {
	filterPrefs := make([]config.FilterPreference, 0, len(m.filters))
	for _, filter := range m.filters {
		filterPrefs = append(filterPrefs, config.FilterPreference{
			IsTag:   filter.isTag,
			Pattern: filter.pattern,
		})
	}

	logLevelBackground := m.logLevelBackground
	coloredMessages := m.coloredMessages
	prefs := config.Preferences{
		Filters:            filterPrefs,
		MinLogLevel:        m.minLogLevel.String(),
		ShowTimestamp:      m.showTimestamp,
		TagColumnWidth:     TagColumnWidth(),
		WrapLines:          m.wrapLines,
		LogLevelBackground: &logLevelBackground,
		ColoredMessages:    &coloredMessages,
	}

	existingPrefs, exists, prefsErr := config.Load()
	if prefsErr == nil && exists {
		prefs.TailSize = existingPrefs.TailSize
	} else {
		prefs.TailSize = config.DefaultTailSize
	}

	return config.Save(prefs)
}

// ErrorMessage returns any error message from the model
func (m Model) ErrorMessage() string {
	return m.errorMessage
}
