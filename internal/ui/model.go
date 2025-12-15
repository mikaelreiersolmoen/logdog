package ui

import (
	"fmt"
	"io"
	"regexp"
	"strings"

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
	str := fmt.Sprintf("%s", priority.Name())

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
	viewport       viewport.Model
	buffer         *buffer.RingBuffer
	logManager     *logcat.Manager
	lineChan       chan string
	ready          bool
	width          int
	height         int
	appID          string
	terminating    bool
	showLogLevel   bool
	logLevelList   list.Model
	minLogLevel    logcat.Priority
	showFilter     bool
	filterInput    textinput.Model
	filters        []Filter
}

type Filter struct {
	isTag bool
	regex *regexp.Regexp
}

type logLineMsg string

func NewModel(appID string) Model {
	items := []list.Item{
		logLevelItem(logcat.Verbose),
		logLevelItem(logcat.Debug),
		logLevelItem(logcat.Info),
		logLevelItem(logcat.Warn),
		logLevelItem(logcat.Error),
		logLevelItem(logcat.Fatal),
	}

	logLevelList := list.New(items, logLevelDelegate{}, 30, len(items)+4)
	logLevelList.Title = "Select log level"
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
		appID:        appID,
		buffer:       buffer.NewRingBuffer(10000),
		logManager:   logcat.NewManager(appID),
		lineChan:     make(chan string, 100),
		showLogLevel: false,
		logLevelList: logLevelList,
		minLogLevel:  logcat.Verbose,
		showFilter:   false,
		filterInput:  filterInput,
		filters:      []Filter{},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		startLogcat(m.logManager, m.lineChan),
		waitForLogLine(m.lineChan),
	)
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
		m.updateViewport()

		if !m.terminating {
			cmds = append(cmds, waitForLogLine(m.lineChan))
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
			}
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

	header := headerStyle.Render(fmt.Sprintf("Logdog [app: %s | log level: %s%s]",
		m.appID, m.minLogLevel.Name(), filterInfo))

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
	} else {
		footer = footerStyle.Render(fmt.Sprintf("q: quit | ↑/↓: scroll | l: log level | f: filter | buffer: %d entries",
			m.buffer.Size()))
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.viewport.View(),
		footer,
	)
}

func (m *Model) updateViewport() {
	entries := m.buffer.Get()
	lines := make([]string, 0, len(entries))
	var lastTag string

	for _, line := range entries {
		entry, err := logcat.ParseLine(line)
		if err != nil {
			lines = append(lines, line)
			lastTag = ""
		} else {
			if entry.Priority >= m.minLogLevel && m.matchesFilters(entry) {
				lines = append(lines, entry.FormatWithTag(lipgloss.NewStyle(), entry.Tag != lastTag))
				lastTag = entry.Tag
			}
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
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
