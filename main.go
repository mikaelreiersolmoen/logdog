package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mikaelreiersolmoen/logdog/internal/logcat"
	"github.com/mikaelreiersolmoen/logdog/internal/ui"
)

func main() {
	var appID string
	var tailSize int
	flag.StringVar(&appID, "app", "", "Application ID to filter logcat logs (optional)")
	flag.StringVar(&appID, "a", "", "Application ID to filter logcat logs (shorthand)")
	flag.IntVar(&tailSize, "tail", 1000, "Number of recent log entries to load initially")
	flag.IntVar(&tailSize, "t", 1000, "Number of recent log entries to load initially (shorthand)")
	flag.Parse()

	// Validate connectivity before starting UI (only if app filtering is requested)
	if appID != "" {
		logManager := logcat.NewManager(appID, tailSize)
		if err := logManager.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		logManager.Stop()
	}

	m := ui.NewModel(appID, tailSize)

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
