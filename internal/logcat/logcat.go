package logcat

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

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
func (p Priority) Color() lipgloss.Color {
	switch p {
	case Verbose:
		return lipgloss.Color("240") // Gray
	case Debug:
		return lipgloss.Color("33") // Blue
	case Info:
		return lipgloss.Color("40") // Green
	case Warn:
		return lipgloss.Color("214") // Orange
	case Error:
		return lipgloss.Color("196") // Red
	case Fatal:
		return lipgloss.Color("201") // Magenta
	default:
		return lipgloss.Color("255") // White
	}
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
	priorityStyle := lipgloss.NewStyle().
		Foreground(e.Priority.Color()).
		Bold(true)

	tagStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("45")) // Cyan

	return fmt.Sprintf("%s %s %s %s",
		e.Timestamp,
		priorityStyle.Render(e.Priority.String()),
		tagStyle.Render(fmt.Sprintf("%-20s", truncate(e.Tag, 20))),
		e.Message,
	)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// Manager manages the logcat process
type Manager struct {
	cmd      *exec.Cmd
	scanner  *bufio.Scanner
	appID    string
	stopChan chan struct{}
}

// NewManager creates a new logcat manager
func NewManager(appID string) *Manager {
	return &Manager{
		appID:    appID,
		stopChan: make(chan struct{}),
	}
}

// Start starts the logcat process
func (m *Manager) Start() error {
	// Build logcat command with app ID filter
	args := []string{"logcat", "-v", "threadtime"}
	if m.appID != "" {
		pid, err := m.getPID()
		if err != nil {
			return fmt.Errorf("failed to get PID for app %s: %w", m.appID, err)
		}
		if pid != "" {
			args = append(args, "--pid="+pid)
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
	return nil
}

// getPID gets the PID for the app package name
func (m *Manager) getPID() (string, error) {
	cmd := exec.Command("adb", "shell", "pidof", m.appID)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get PID: %w", err)
	}
	
	pid := strings.TrimSpace(string(output))
	if pid == "" {
		return "", fmt.Errorf("app not running or package name not found")
	}
	
	return pid, nil
}

// ReadLines reads lines from logcat and sends them on the channel
// Returns when Stop() is called or logcat process ends
func (m *Manager) ReadLines(lineChan chan<- string) {
	defer close(lineChan)

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
					lineChan <- line
				}
			}
			return
		case <-ticker.C:
			// Flush batch periodically
			if len(batch) > 0 {
				for _, line := range batch {
					lineChan <- line
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
						lineChan <- line
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

// Stop stops the logcat process
func (m *Manager) Stop() error {
	close(m.stopChan)
	if m.cmd != nil && m.cmd.Process != nil {
		return m.cmd.Process.Kill()
	}
	return nil
}
