package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mikaelreiersolmoen/logdog/internal/buffer"
	"github.com/mikaelreiersolmoen/logdog/internal/logcat"
)

type Model struct {
	viewport    viewport.Model
	buffer      *buffer.RingBuffer
	logManager  *logcat.Manager
	lineChan    chan string
	ready       bool
	width       int
	height      int
	appID       string
	err         error
	terminating bool
}

type logLineMsg string
type errMsg error

func NewModel(appID string) Model {
	return Model{
		appID:      appID,
		buffer:     buffer.NewRingBuffer(10000),
		logManager: logcat.NewManager(appID),
		lineChan:   make(chan string, 100),
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

	case errMsg:
		m.err = msg
		return m, tea.Quit

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.terminating = true
			m.logManager.Stop()
			return m, tea.Quit
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		Width(m.width)

	header := headerStyle.Render("Logdog - Logcat Viewer [App: " + m.appID + "]")

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		Width(m.width)

	footer := footerStyle.Render(fmt.Sprintf("q: quit | ↑/↓: scroll | Buffer: %d entries",
		m.buffer.Size()))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.viewport.View(),
		footer,
	)
}

func (m *Model) updateViewport() {
	entries := m.buffer.Get()
	lines := make([]string, len(entries))

	for i, line := range entries {
		entry, err := logcat.ParseLine(line)
		if err != nil {
			lines[i] = line
		} else {
			lines[i] = entry.Format(lipgloss.NewStyle())
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

func startLogcat(manager *logcat.Manager, lineChan chan string) tea.Cmd {
	return func() tea.Msg {
		if err := manager.Start(); err != nil {
			return errMsg(err)
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
