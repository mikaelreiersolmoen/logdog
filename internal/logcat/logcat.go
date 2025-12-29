package logcat

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Color palette
var (
	colorVerbose = lipgloss.AdaptiveColor{Light: "240", Dark: "250"}
	colorDebug   = lipgloss.AdaptiveColor{Light: "33", Dark: "117"}   // Pastel blue
	colorInfo    = lipgloss.AdaptiveColor{Light: "71", Dark: "114"}   // Pastel green
	colorWarn    = lipgloss.AdaptiveColor{Light: "172", Dark: "215"}  // Pastel orange/yellow
	colorError   = lipgloss.AdaptiveColor{Light: "160", Dark: "204"}  // Pastel red
	colorFatal   = lipgloss.AdaptiveColor{Light: "168", Dark: "213"}  // Pastel magenta
	colorDefault = lipgloss.AdaptiveColor{Light: "0", Dark: "255"}    // Black/White
)

// Color palette for tags - pastel colors that don't overlap with log levels
var tagColors = []lipgloss.AdaptiveColor{
	{Light: "75", Dark: "123"},   // Pastel teal
	{Light: "140", Dark: "183"},  // Pastel purple
	{Light: "180", Dark: "222"},  // Pastel peach
	{Light: "108", Dark: "151"},  // Pastel lime
	{Light: "146", Dark: "189"},  // Pastel lavender
	{Light: "79", Dark: "122"},   // Pastel cyan
	{Light: "139", Dark: "182"},  // Pastel violet
	{Light: "173", Dark: "217"},  // Pastel tan
	{Light: "109", Dark: "152"},  // Pastel mint
	{Light: "147", Dark: "190"},  // Pastel mauve
}

// Priority represents logcat priority levels
type Priority int

const (
	Verbose Priority = iota
	Debug
	Info
	Warn
	Error
	Fatal
	Unknown
)

// Entry represents a parsed logcat entry
type Entry struct {
	Timestamp string
	PID       string
	TID       string
	Priority  Priority
	Tag       string
	Message   string
	Raw       string
}

// PriorityFromChar converts a logcat priority character to Priority
func PriorityFromChar(c rune) Priority {
	switch c {
	case 'V':
		return Verbose
	case 'D':
		return Debug
	case 'I':
		return Info
	case 'W':
		return Warn
	case 'E':
		return Error
	case 'F':
		return Fatal
	default:
		return Unknown
	}
}

// String returns the string representation of the priority
func (p Priority) String() string {
	switch p {
	case Verbose:
		return "V"
	case Debug:
		return "D"
	case Info:
		return "I"
	case Warn:
		return "W"
	case Error:
		return "E"
	case Fatal:
		return "F"
	default:
		return "?"
	}
}

// Name returns the full name of the priority
func (p Priority) Name() string {
	switch p {
	case Verbose:
		return "Verbose"
	case Debug:
		return "Debug"
	case Info:
		return "Info"
	case Warn:
		return "Warning"
	case Error:
		return "Error"
	case Fatal:
		return "Fatal"
	default:
		return "Unknown"
	}
}

// Color returns the lipgloss color for the priority
func (p Priority) Color() lipgloss.TerminalColor {
	switch p {
	case Verbose:
		return colorVerbose
	case Debug:
		return colorDebug
	case Info:
		return colorInfo
	case Warn:
		return colorWarn
	case Error:
		return colorError
	case Fatal:
		return colorFatal
	default:
		return colorDefault
	}
}

// TagColor returns a consistent color for a given tag name
func TagColor(tag string) lipgloss.TerminalColor {
	if tag == "" {
		return colorDefault
	}
	
	// Simple hash function to map tag to color index
	var hash uint32
	for i := 0; i < len(tag); i++ {
		hash = hash*31 + uint32(tag[i])
	}
	
	colorIndex := int(hash) % len(tagColors)
	return tagColors[colorIndex]
}

// ParseLine parses a logcat line in threadtime format
// Format: MM-DD HH:MM:SS.mmm PID TID P TAG: MESSAGE
func ParseLine(line string) (*Entry, error) {
	if len(line) == 0 {
		return nil, fmt.Errorf("empty line")
	}

	// Store raw line
	entry := &Entry{Raw: line}

	// Split by spaces, but be careful with the message part
	parts := strings.Fields(line)
	if len(parts) < 6 {
		// Malformed line, return as-is with Unknown priority
		entry.Priority = Unknown
		entry.Message = line
		return entry, nil
	}

	// Parse timestamp (MM-DD HH:MM:SS.mmm)
	if len(parts) >= 2 {
		entry.Timestamp = parts[0] + " " + parts[1]
	}

	// Parse PID, TID
	if len(parts) >= 4 {
		entry.PID = parts[2]
		entry.TID = parts[3]
	}

	// Parse priority
	if len(parts) >= 5 && len(parts[4]) > 0 {
		entry.Priority = PriorityFromChar(rune(parts[4][0]))
	}

	// Parse tag and message
	// Find the position after priority to get tag+message
	tagMsgIdx := strings.Index(line, parts[4])
	if tagMsgIdx >= 0 && tagMsgIdx+len(parts[4]) < len(line) {
		remainder := line[tagMsgIdx+len(parts[4]):]
		remainder = strings.TrimSpace(remainder)

		// Tag ends with ':'
		colonIdx := strings.Index(remainder, ":")
		if colonIdx >= 0 {
			entry.Tag = remainder[:colonIdx]
			if colonIdx+1 < len(remainder) {
				entry.Message = strings.TrimSpace(remainder[colonIdx+1:])
			}
		} else {
			entry.Message = remainder
		}
	}

	return entry, nil
}

// Format returns a formatted string representation of the entry
func (e *Entry) Format(style lipgloss.Style) string {
	return e.FormatWithTag(style, true)
}

// FormatWithTag returns a formatted string representation with optional tag display
func (e *Entry) FormatWithTag(style lipgloss.Style, showTag bool) string {
	return e.FormatWithTagAndMessageStyle(style, showTag, lipgloss.NewStyle())
}

// FormatWithTagAndMessageStyle returns a formatted string with separate style for message
func (e *Entry) FormatWithTagAndMessageStyle(style lipgloss.Style, showTag bool, messageStyle lipgloss.Style) string {
	priorityStyle := lipgloss.NewStyle().
		Foreground(e.Priority.Color()).
		Bold(true)

	tagStyle := lipgloss.NewStyle().
		Foreground(TagColor(e.Tag))

	var tagStr string
	if showTag {
		tagStr = tagStyle.Render(fmt.Sprintf("%-20s", truncate(e.Tag, 20)))
	} else {
		tagStr = strings.Repeat(" ", 20)
	}

	return fmt.Sprintf("%s %s %s %s",
		e.Timestamp,
		priorityStyle.Render(e.Priority.String()),
		tagStr,
		messageStyle.Render(e.Message),
	)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

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

// Manager manages the logcat process
type Manager struct {
	cmd             *exec.Cmd
	scanner         *bufio.Scanner
	appID           string
	deviceSerial    string
	stopChan        chan struct{}
	monitorStopChan chan struct{}
	tailSize        int
	currentPID      string
	statusChan      chan string
	lineChan        chan<- string
}

// NewManager creates a new logcat manager
func NewManager(appID string, tailSize int) *Manager {
	if tailSize <= 0 {
		tailSize = 1000 // Default to 1000 entries
	}
	return &Manager{
		appID:           appID,
		stopChan:        make(chan struct{}),
		monitorStopChan: make(chan struct{}),
		tailSize:        tailSize,
		statusChan:      make(chan string, 10),
	}
}

// SetDevice sets the device serial for this manager
func (m *Manager) SetDevice(serial string) {
	m.deviceSerial = serial
}

// Start starts the logcat process
func (m *Manager) Start() error {
	// Build logcat command with app ID filter
	args := []string{}
	if m.deviceSerial != "" {
		args = append(args, "-s", m.deviceSerial)
	}
	args = append(args, "logcat", "-v", "threadtime", "-T", fmt.Sprintf("%d", m.tailSize))
	if m.appID != "" {
		pid, err := m.getPID()
		if err != nil {
			return err
		}
		if pid != "" {
			m.currentPID = pid
			args = append(args, "--pid="+pid)
			m.statusChan <- "running"
		}
	}

	m.cmd = exec.Command("adb", args...)
	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start logcat: %w", err)
	}

	m.scanner = bufio.NewScanner(stdout)

	// Start PID monitoring if filtering by app
	if m.appID != "" && m.currentPID != "" {
		go m.monitorPID()
	}

	return nil
}

// getPID gets the PID for the app package name
func (m *Manager) getPID() (string, error) {
	// First check if device is connected
	cmd := exec.Command("adb", "devices")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("adb command failed - is Android SDK installed?")
	}
	
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) <= 1 {
		return "", fmt.Errorf("no devices/emulators found - connect a device or start an emulator")
	}
	
	// Get PID
	args := []string{}
	if m.deviceSerial != "" {
		args = append(args, "-s", m.deviceSerial)
	}
	args = append(args, "shell", "pidof", m.appID)
	cmd = exec.Command("adb", args...)
	output, err = cmd.Output()
	if err != nil {
		return "", fmt.Errorf("app not running or package name not found - is '%s' installed and running?", m.appID)
	}
	
	pid := strings.TrimSpace(string(output))
	if pid == "" {
		return "", fmt.Errorf("app not running or package name not found - is '%s' installed and running?", m.appID)
	}
	
	return pid, nil
}

// isPIDRunning checks if a PID is still running
func (m *Manager) isPIDRunning(pid string) bool {
	args := []string{}
	if m.deviceSerial != "" {
		args = append(args, "-s", m.deviceSerial)
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

// monitorPID monitors the current PID and restarts logcat when the app restarts
func (m *Manager) monitorPID() {
	checkInterval := 2 * time.Second
	pollInterval := 1 * time.Second

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.monitorStopChan:
			return
		case <-ticker.C:
			// Check if current PID is still running
			if !m.isPIDRunning(m.currentPID) {
				// App has stopped
				m.statusChan <- "stopped"

				// Start polling for app restart
				ticker.Stop()
				ticker = time.NewTicker(pollInterval)

				// Poll for new PID
				for {
					select {
					case <-m.monitorStopChan:
						return
					case <-ticker.C:
						m.statusChan <- "reconnecting"
						newPID, err := m.getPID()
						if err == nil && newPID != "" && newPID != m.currentPID {
							// App has restarted with new PID
							m.currentPID = newPID
							if err := m.restart(); err == nil {
								m.statusChan <- "running"
								// Resume normal check interval
								ticker.Stop()
								ticker = time.NewTicker(checkInterval)
								goto continueMonitoring
							}
						}
					}
				}
			}
		continueMonitoring:
		}
	}
}

// restart stops the current logcat process and starts a new one with the current PID
func (m *Manager) restart() error {
	// Stop the current process
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
	}

	// Build new logcat command with updated PID
	args := []string{}
	if m.deviceSerial != "" {
		args = append(args, "-s", m.deviceSerial)
	}
	args = append(args, "logcat", "-v", "threadtime", "-T", "0") // Use -T 0 for restarts to avoid duplicates
	if m.currentPID != "" {
		args = append(args, "--pid="+m.currentPID)
	}

	m.cmd = exec.Command("adb", args...)
	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start logcat: %w", err)
	}

	m.scanner = bufio.NewScanner(stdout)

	// Restart the ReadLines goroutine with the new scanner
	if m.lineChan != nil {
		go m.readLinesInternal()
	}

	return nil
}

// StatusChan returns the channel for receiving status updates
func (m *Manager) StatusChan() <-chan string {
	return m.statusChan
}

// ReadLines reads lines from logcat and sends them on the channel
// Returns when Stop() is called or logcat process ends
func (m *Manager) ReadLines(lineChan chan<- string) {
	m.lineChan = lineChan
	m.readLinesInternal()
}

// readLinesInternal is the internal implementation of ReadLines
func (m *Manager) readLinesInternal() {
	// Use a buffer to batch lines
	batch := make([]string, 0, 100)
	ticker := time.NewTicker(33 * time.Millisecond) // ~30 FPS
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			// Send any remaining lines
			if len(batch) > 0 {
				for _, line := range batch {
					m.lineChan <- line
				}
			}
			return
		case <-ticker.C:
			// Flush batch periodically
			if len(batch) > 0 {
				for _, line := range batch {
					m.lineChan <- line
				}
				batch = batch[:0]
			}
		default:
			// Try to read a line (non-blocking via select)
			if m.scanner.Scan() {
				batch = append(batch, m.scanner.Text())
				// If batch is full, send immediately
				if len(batch) >= 100 {
					for _, line := range batch {
						m.lineChan <- line
					}
					batch = batch[:0]
				}
			} else {
				// Scanner done or error
				if err := m.scanner.Err(); err != nil {
					// Could send error on a separate channel
					return
				}
				return
			}
		}
	}
}

// Stop stops the logcat process and monitoring
func (m *Manager) Stop() error {
	close(m.stopChan)
	close(m.monitorStopChan)
	if m.cmd != nil && m.cmd.Process != nil {
		return m.cmd.Process.Kill()
	}
	return nil
}
