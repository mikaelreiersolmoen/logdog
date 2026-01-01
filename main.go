package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mikaelreiersolmoen/logdog/internal/adb"
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

	// Validate connectivity before starting UI (only if app filtering is requested and single device)
	if appID != "" {
		// Check device count first
		devices, err := adb.GetDevices()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Only validate if single device (multi-device validation happens after selection)
		if len(devices) == 1 {
			logManager := logcat.NewManager(appID, tailSize)
			logManager.SetDevice(devices[0].Serial)
			if err := logManager.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			logManager.Stop()
		}
	}

	m := ui.NewModel(appID, tailSize)

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	finalModel, err := p.Run()
	if err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}

	// Persist preferences and report any final error message
	if finalModel, ok := finalModel.(ui.Model); ok {
		if err := finalModel.PersistPreferences(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to save preferences: %v\n", err)
		}
		if finalModel.ErrorMessage() != "" {
			fmt.Fprintf(os.Stderr, "Error: %v\n", finalModel.ErrorMessage())
			os.Exit(1)
		}
	}
}
