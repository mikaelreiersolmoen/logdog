package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FilterPreference captures a single filter setting for persistence.
type FilterPreference struct {
	IsTag   bool   `json:"isTag"`
	Pattern string `json:"pattern"`
}

// Preferences holds persisted UI preferences.
type Preferences struct {
	Filters        []FilterPreference `json:"filters"`
	MinLogLevel    string             `json:"minLogLevel"`
	ShowTimestamp  bool               `json:"showTimestamp"`
	TagColumnWidth int                `json:"tagColumnWidth"`
}

// Load reads preferences from ~/.config/logdog/config.json.
func Load() (Preferences, bool, error) {
	path, err := configFilePath()
	if err != nil {
		return Preferences{}, false, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Preferences{}, false, nil
	}
	if err != nil {
		return Preferences{}, false, fmt.Errorf("read config: %w", err)
	}
	if len(data) == 0 {
		return Preferences{}, true, nil
	}

	var prefs Preferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return Preferences{}, false, fmt.Errorf("decode config: %w", err)
	}

	return prefs, true, nil
}

// Save writes preferences to ~/.config/logdog/config.json.
func Save(prefs Preferences) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func configFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "logdog", "config.json"), nil
}
