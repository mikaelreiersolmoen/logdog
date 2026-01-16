package adb

import (
	"fmt"
	"os/exec"
	"time"
)

// GetPID gets the PID for an app package name on the specified device
func GetPID(deviceSerial, appID string) (string, error) {
	devices, err := GetDevices()
	if err != nil {
		return "", err
	}
	if deviceSerial != "" {
		var target *Device
		for i := range devices {
			if devices[i].Serial == deviceSerial {
				target = &devices[i]
				break
			}
		}
		if target == nil {
			return "", fmt.Errorf("device %s not found", deviceSerial)
		}
		if target.Status != "device" {
			return "", fmt.Errorf("device %s not online (status: %s)", target.Serial, target.Status)
		}
	} else {
		onlineCount := 0
		for _, device := range devices {
			if device.Status == "device" {
				onlineCount++
			}
		}
		if onlineCount == 0 {
			return "", fmt.Errorf("no online devices found - connect a device or start an emulator")
		}
		if onlineCount > 1 {
			return "", fmt.Errorf("multiple devices connected - select a device")
		}
	}

	// Get PID
	args := []string{}
	if deviceSerial != "" {
		args = append(args, "-s", deviceSerial)
	}
	args = append(args, "shell", "pidof", appID)
	cmd := exec.Command("adb", args...)
	output, err = cmd.Output()
	if err != nil {
		return "", fmt.Errorf("app not running or package name not found - is '%s' installed and running?", appID)
	}

	pid := strings.TrimSpace(string(output))
	if pid == "" {
		return "", fmt.Errorf("app not running or package name not found - is '%s' installed and running?", appID)
	}

	return pid, nil
}

// IsPIDRunning checks if a PID is still running on the specified device
func IsPIDRunning(deviceSerial, pid string) bool {
	args := []string{}
	if deviceSerial != "" {
		args = append(args, "-s", deviceSerial)
	}
	args = append(args, "shell", "ps", "-p", pid)
	cmd := exec.Command("adb", args...)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	// If ps returns output with the PID, the process is running
	return strings.Contains(string(output), pid)
}

// WaitForPID polls for a PID to appear, returning when found or context cancelled
// Returns the PID when found, or empty string if cancelled
func WaitForPID(deviceSerial, appID string, pollInterval time.Duration, stopChan <-chan struct{}) string {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stopChan:
			return ""
		case <-ticker.C:
			pid, err := GetPID(deviceSerial, appID)
			if err == nil && pid != "" {
				return pid
			}
		}
	}
}

// MonitorPID monitors a PID and returns when it stops running
func MonitorPID(deviceSerial, pid string, checkInterval time.Duration, stopChan <-chan struct{}) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stopChan:
			return
		case <-ticker.C:
			if !IsPIDRunning(deviceSerial, pid) {
				return
			}
		}
	}
}
