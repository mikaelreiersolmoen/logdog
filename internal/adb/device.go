package adb

import (
	"fmt"
	"os/exec"
	"strings"
)

// Device represents an ADB device
type Device struct {
	Serial string
	Model  string
	Status string
}

// GetDevices returns a list of connected ADB devices
func GetDevices() ([]Device, error) {
	cmd := exec.Command("adb", "devices", "-l")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("adb command failed - is Android SDK installed?")
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) <= 1 {
		return nil, fmt.Errorf("no devices/emulators found")
	}

	var devices []Device
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Parse format: "serial device [model:...] ..."
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		device := Device{
			Serial: parts[0],
			Status: parts[1],
		}

		// Extract model from the rest of the line
		for j := 2; j < len(parts); j++ {
			if strings.HasPrefix(parts[j], "model:") {
				device.Model = strings.TrimPrefix(parts[j], "model:")
				break
			}
		}

		if device.Model == "" {
			device.Model = "Unknown"
		}

		devices = append(devices, device)
	}

	return devices, nil
}
