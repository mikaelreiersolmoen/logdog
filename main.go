package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mikaelreiersolmoen/logdog/internal/adb"
	"github.com/mikaelreiersolmoen/logdog/internal/config"
	"github.com/mikaelreiersolmoen/logdog/internal/logcat"
	"github.com/mikaelreiersolmoen/logdog/internal/ui"
)

func main() {
	var appID string
	var tailValue string
	defaultTailValue := resolveDefaultTailValue()
	flag.StringVar(&appID, "app", "", "Application ID to filter logcat logs (optional)")
	flag.StringVar(&appID, "a", "", "Application ID to filter logcat logs (shorthand)")
	flag.StringVar(&tailValue, "tail", defaultTailValue, "Number of recent log entries to load initially (0 = none, all = all)")
	flag.StringVar(&tailValue, "t", defaultTailValue, "Number of recent log entries to load initially (shorthand, 0 = none, all = all)")
	flag.Parse()

	tailSize, err := parseTailSize(tailValue)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}

	if err := config.EnsureExists(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to initialize preferences: %v\n", err)
	}

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

func parseTailSize(value string) (int, error) {
	if strings.EqualFold(value, "all") {
		return logcat.TailAll, nil
	}

	tailSize, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid --tail value %q (expected integer or \"all\")", value)
	}
	if tailSize < 0 {
		return 0, fmt.Errorf("invalid --tail value %d (must be >= 0 or \"all\")", tailSize)
	}
	return tailSize, nil
}

func resolveDefaultTailValue() string {
	defaultValue := config.DefaultTailSize
	prefs, _, err := config.Load()
	if err != nil {
		return strconv.Itoa(defaultValue)
	}

	if prefs.TailSize < 0 {
		return strconv.Itoa(defaultValue)
	}

	return strconv.Itoa(prefs.TailSize)
}
