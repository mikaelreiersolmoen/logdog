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

	itemStyle := lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle := lipgloss.NewStyle().PaddingLeft(2).Foreground(priority.Color())

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

type Model struct {
	viewport         viewport.Model
	buffer           *buffer.RingBuffer
	logManager       *logcat.Manager
	lineChan         chan string
	ready            bool
	width            int
	height           int
	appID            string
	appStatus        string
	terminating      bool
	showLogLevel     bool
	logLevelList     list.Model
	minLogLevel      logcat.Priority
	showFilter       bool
	filterInput      textinput.Model
	filters          []Filter
	parsedEntries    []*logcat.Entry
	needsUpdate      bool
	highlightedEntry *logcat.Entry
	selectionMode    bool
	selectedEntries  map[*logcat.Entry]bool
	selectionAnchor  *logcat.Entry
}

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
		Foreground(lipgloss.Color("39")).
		Padding(0, 1)

	filterInput := textinput.New()
	filterInput.Placeholder = "e.g., error|warning, tag:MyTag"
	filterInput.CharLimit = 500
	filterInput.Width = 80

	return Model{
		appID:            appID,
		buffer:           buffer.NewRingBuffer(10000),
		logManager:       logcat.NewManager(appID, tailSize),
		lineChan:         make(chan string, 100),
		showLogLevel:     false,
		logLevelList:     logLevelList,
		minLogLevel:      logcat.Verbose,
		showFilter:       false,
		filterInput:      filterInput,
		filters:          []Filter{},
		parsedEntries:    make([]*logcat.Entry, 0, 10000),
		needsUpdate:      false,
		highlightedEntry: nil,
		selectionMode:    false,
		selectedEntries:  make(map[*logcat.Entry]bool),
		selectionAnchor:  nil,
	}
}

func (m Model) Init() tea.Cmd {
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
		headerHeight := 3
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
		if m.needsUpdate {
			// Disable auto-scroll when in selection mode
			if m.selectionMode {
				m.updateViewportWithScroll(false)
			} else {
				m.updateViewport()
			}
			m.needsUpdate = false
		}
		if !m.terminating {
			cmds = append(cmds, tickViewportUpdate())
		}

	case tea.KeyMsg:
		if m.showLogLevel {
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
				m.updateViewportWithScroll(false)
				return m, nil
			case "V": // Shift+v to enter selection mode
				m.enterSelectionMode()
				m.updateViewportWithScroll(false)
				return m, nil
			case "c":
				if m.selectionMode && len(m.selectedEntries) > 0 {
					m.copySelectedMessages()
					m.clearSelection()
					m.selectionMode = false
					m.updateViewportWithScroll(false)
				}
				return m, nil
			case "j", "down":
				if m.selectionMode {
					m.extendSelectionDown()
				} else {
					m.moveHighlightDown()
				}
				m.updateViewportWithScroll(false)
				return m, nil
			case "k", "up":
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
		if msg.Type == tea.MouseLeft && !m.showLogLevel && !m.showFilter {
			m.handleMouseClick(msg.Y)
			m.updateViewportWithScroll(false)
			return m, nil
		}
	}

	if m.showLogLevel {
		m.logLevelList, cmd = m.logLevelList.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.showFilter {
		m.filterInput, cmd = m.filterInput.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	if m.showLogLevel {
		return "\n" + m.logLevelList.View()
	}

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		Width(m.width)

	filterInfo := ""
	if len(m.filters) > 0 {
		var filterStrs []string
		for _, f := range m.filters {
			if f.isTag {
				filterStrs = append(filterStrs, "tag:"+f.regex.String())
			} else {
				filterStrs = append(filterStrs, f.regex.String())
			}
		}
		filterInfo = " | Filters: " + strings.Join(filterStrs, ", ")
	}

	appInfo := m.appID
	if appInfo == "" {
		appInfo = "all"
	}

	header := headerStyle.Render(fmt.Sprintf("Logdog [app: %s | log level: %s%s]",
		appInfo, m.minLogLevel.Name(), filterInfo))

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		Width(m.width)

	var footer string
	if m.showFilter {
		filterLabel := lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true).
			Render("Filter: ")

		filterHelp := lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render(" (comma-separated, tag: prefix for tags | Enter: apply | Esc: cancel)")

		footer = footerStyle.Render(filterLabel + m.filterInput.View() + filterHelp)
	} else if m.selectionMode {
		selectionInfo := fmt.Sprintf("SELECTION MODE | %d lines selected | j/k: extend | click: extend | c: copy | Esc: exit", len(m.selectedEntries))
		footer = footerStyle.Render(selectionInfo)
	} else {
		baseHelp := "q: quit | j/k: highlight | shift+v: select | l: log level | f: filter"

		// Add app status if filtering by app
		if m.appID != "" && m.appStatus != "" {
			statusStyle := lipgloss.NewStyle()
			var statusText string

			switch m.appStatus {
			case "running":
				statusStyle = statusStyle.Foreground(lipgloss.Color("40")) // Green
				statusText = "running"
			case "stopped":
				statusStyle = statusStyle.Foreground(lipgloss.Color("214")) // Orange
				statusText = "disconnected"
			case "reconnecting":
				statusStyle = statusStyle.Foreground(lipgloss.Color("214")) // Orange
				statusText = "disconnected"
			}

			footer = footerStyle.Render(baseHelp + " | app status: " + statusStyle.Render(statusText))
		} else {
			footer = footerStyle.Render(baseHelp)
		}
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.viewport.View(),
		footer,
	)
}

func (m *Model) updateViewport() {
	m.updateViewportWithScroll(true)
}

func (m *Model) updateViewportWithScroll(scrollToBottom bool) {
	lines := make([]string, 0, len(m.parsedEntries))
	var lastTag string

	selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("240"))
	highlightStyle := lipgloss.NewStyle().Background(lipgloss.Color("235")) // Subtle highlight

	for _, entry := range m.parsedEntries {
		if entry.Priority >= m.minLogLevel && m.matchesFilters(entry) {
			var line string

			// Apply styles based on selection/highlight state
			if m.selectedEntries[entry] {
				// Strong selection style
				line = entry.FormatWithTagAndMessageStyle(lipgloss.NewStyle(), entry.Tag != lastTag, selectedStyle)
			} else if entry == m.highlightedEntry {
				// Subtle highlight style
				line = entry.FormatWithTagAndMessageStyle(lipgloss.NewStyle(), entry.Tag != lastTag, highlightStyle)
			} else {
				line = entry.FormatWithTag(lipgloss.NewStyle(), entry.Tag != lastTag)
			}

			lines = append(lines, line)
			lastTag = entry.Tag
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	m.viewport.SetContent(content)

	if scrollToBottom {
		m.viewport.GotoBottom()
	}
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
			panic(err)
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
func (m *Model) enterSelectionMode() {
	m.selectionMode = true

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
	var messages []string
	for _, entry := range visible {
		if m.selectedEntries[entry] {
			messages = append(messages, entry.Message)
		}
	}
	
	// Copy to clipboard using pbcopy (macOS) or similar
	clipboard := strings.Join(messages, "\n")
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(clipboard)
	cmd.Run()
}
