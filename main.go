package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mikaelreiersolmoen/logdog/internal/ui"
)

func main() {
	var appID string
	flag.StringVar(&appID, "app", "", "Application ID to filter logcat logs")
	flag.StringVar(&appID, "a", "", "Application ID to filter logcat logs (shorthand)")
	flag.Parse()

	if appID == "" {
		fmt.Fprintln(os.Stderr, "Error: --app flag is required")
		flag.Usage()
		os.Exit(1)
	}

	m := ui.NewModel(appID)

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
