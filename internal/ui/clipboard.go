package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func copyToClipboard(text string) error {
	switch runtime.GOOS {
	case "darwin":
		return runClipboardCommand("pbcopy", nil, text)
	case "windows":
		if _, err := exec.LookPath("clip"); err == nil {
			return runClipboardCommand("cmd", []string{"/c", "clip"}, text)
		}
		if _, err := exec.LookPath("powershell"); err == nil {
			return runClipboardCommand("powershell", []string{"-NoProfile", "-Command", "Set-Clipboard"}, text)
		}
		return fmt.Errorf("no clipboard tool found")
	case "linux":
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return runClipboardCommand("wl-copy", nil, text)
		}
		if _, err := exec.LookPath("xclip"); err == nil {
			return runClipboardCommand("xclip", []string{"-selection", "clipboard"}, text)
		}
		if _, err := exec.LookPath("xsel"); err == nil {
			return runClipboardCommand("xsel", []string{"--clipboard", "--input"}, text)
		}
		return fmt.Errorf("no clipboard tool found")
	default:
		return fmt.Errorf("unsupported platform")
	}
}

func runClipboardCommand(name string, args []string, text string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
