package logcat

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/mikaelreiersolmoen/logdog/internal/adb"
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

		// Remove padding between priority column and tag but preserve message indentation
		trimmedRemainder := strings.TrimLeft(remainder, " ")

		// Tag ends with ':'; remove padding emitted by logcat so alignment stays consistent
		colonIdx := strings.Index(trimmedRemainder, ":")
		if colonIdx >= 0 {
			tag := strings.TrimSpace(trimmedRemainder[:colonIdx])
			entry.Tag = tag
			if colonIdx+1 < len(trimmedRemainder) {
				message := trimmedRemainder[colonIdx+1:]
				if len(message) > 0 && message[0] == ' ' {
					message = message[1:]
				}
				entry.Message = message
			}
		} else {
			entry.Message = strings.TrimLeft(remainder, " ")
		}
	}

	return entry, nil
}

// FormatPlain returns a plain text representation without any styling or ANSI codes
func (e *Entry) FormatPlain() string {
	tag := strings.TrimRight(e.Tag, " ")

	return fmt.Sprintf("%s %s %s %s",
		e.Timestamp,
		e.Priority.String(),
		tag,
		e.Message,
	)
}

// Manager manages the logcat process
type Manager struct {
	cmd             *exec.Cmd
	appID           string
	deviceSerial    string
	stopChan        chan struct{}
	monitorStopChan chan struct{}
	tailSize        int
	currentPID      string
	statusChan      chan string
	lineChan        chan<- string
	scanner         *bufio.Scanner
	readStop        chan struct{}
	readDone        chan struct{}
	readMu          sync.Mutex
	cmdMu           sync.Mutex
}

// TailAll indicates that all available log entries should be loaded.
const TailAll = -1

const (
	scannerBufferSize    = 64 * 1024
	maxScannerBufferSize = 1024 * 1024
	readBatchSize        = 100
	readTickInterval     = 33 * time.Millisecond
)

// NewManager creates a new logcat manager
func NewManager(appID string, tailSize int) *Manager {
	if tailSize < TailAll {
		tailSize = 1000 // Fallback when an invalid tail size is provided.
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
	devices, err := adb.GetDevices()
	if err != nil {
		return err
	}
	if m.deviceSerial != "" {
		var target *adb.Device
		for i := range devices {
			if devices[i].Serial == m.deviceSerial {
				target = &devices[i]
				break
			}
		}
		if target == nil {
			return fmt.Errorf("device %s not found", m.deviceSerial)
		}
		if target.Status != "device" {
			return fmt.Errorf("device %s not online (status: %s)", target.Serial, target.Status)
		}
	} else {
		hasOnline := false
		for _, device := range devices {
			if device.Status == "device" {
				hasOnline = true
				break
			}
		}
		if !hasOnline {
			return fmt.Errorf("no online devices found")
		}
	}

	// Build logcat command with app ID filter
	args := []string{}
	if m.deviceSerial != "" {
		args = append(args, "-s", m.deviceSerial)
	}
	args = append(args, "logcat", "-v", "threadtime")
	if m.tailSize > 0 {
		args = append(args, "-T", fmt.Sprintf("%d", m.tailSize))
	} else if m.tailSize == 0 {
		args = append(args, "-T", "0")
	}
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

	cmd := exec.Command("adb", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start logcat: %w", err)
	}

	scanner := newScanner(stdout)

	m.cmdMu.Lock()
	m.cmd = cmd
	m.cmdMu.Unlock()

	m.setScanner(scanner)

	// Start PID monitoring if filtering by app
	if m.appID != "" && m.currentPID != "" {
		go m.monitorPID()
	}

	return nil
}

// getPID gets the PID for the app package name
func (m *Manager) getPID() (string, error) {
	return adb.GetPID(m.deviceSerial, m.appID)
}

// monitorPID monitors the current PID and restarts logcat when the app restarts
func (m *Manager) monitorPID() {
	checkInterval := 2 * time.Second
	pollInterval := 1 * time.Second

	for {
		// Monitor until PID stops
		adb.MonitorPID(m.deviceSerial, m.currentPID, checkInterval, m.monitorStopChan)

		select {
		case <-m.monitorStopChan:
			return
		default:
			// App has stopped
			m.statusChan <- "stopped"
			m.statusChan <- "reconnecting"

			// Wait for app to restart
			newPID := adb.WaitForPID(m.deviceSerial, m.appID, pollInterval, m.monitorStopChan)
			if newPID == "" {
				// Monitoring stopped
				return
			}

			// App has restarted with new PID
			m.currentPID = newPID
			if err := m.restart(); err != nil {
				m.statusChan <- "error"
				return
			}
			m.statusChan <- "running"
		}
	}
}

// restart stops the current logcat process and starts a new one with the current PID
func (m *Manager) restart() error {
	// Stop the current process
	m.stopProcess()

	// Build new logcat command with updated PID
	args := []string{}
	if m.deviceSerial != "" {
		args = append(args, "-s", m.deviceSerial)
	}
	args = append(args, "logcat", "-v", "threadtime", "-T", "0") // Use -T 0 for restarts to avoid duplicates
	if m.currentPID != "" {
		args = append(args, "--pid="+m.currentPID)
	}

	cmd := exec.Command("adb", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start logcat: %w", err)
	}

	scanner := newScanner(stdout)

	m.cmdMu.Lock()
	m.cmd = cmd
	m.cmdMu.Unlock()

	m.setScanner(scanner)

	return nil
}

// StatusChan returns the channel for receiving status updates
func (m *Manager) StatusChan() <-chan string {
	return m.statusChan
}

// ReadLines reads lines from logcat and sends them on the channel
// Returns when Stop() is called or logcat process ends
func (m *Manager) ReadLines(lineChan chan<- string) {
	m.readMu.Lock()
	m.lineChan = lineChan
	scanner := m.scanner
	m.readMu.Unlock()

	if scanner == nil {
		return
	}

	if done := m.startReader(scanner); done != nil {
		<-done
	}
}

// readLinesInternal is the internal implementation of ReadLines.
func (m *Manager) readLinesInternal(scanner *bufio.Scanner, lineChan chan<- string, readStop <-chan struct{}, done chan<- struct{}) {
	defer close(done)

	rawLines := make(chan string, readBatchSize*2)
	errChan := make(chan error, 1)

	go func() {
		defer close(rawLines)
		defer func() {
			select {
			case errChan <- scanner.Err():
			default:
			}
		}()
		for scanner.Scan() {
			line := scanner.Text()
			select {
			case rawLines <- line:
			case <-readStop:
				return
			case <-m.stopChan:
				return
			}
		}
	}()

	// Use a buffer to batch lines
	batch := make([]string, 0, readBatchSize)
	ticker := time.NewTicker(readTickInterval) // ~30 FPS
	defer ticker.Stop()

	flush := func() bool {
		if len(batch) == 0 {
			return true
		}
		for _, line := range batch {
			select {
			case lineChan <- line:
			case <-readStop:
				return false
			case <-m.stopChan:
				return false
			}
		}
		batch = batch[:0]
		return true
	}

	for {
		select {
		case <-m.stopChan:
			_ = flush()
			return
		case <-readStop:
			_ = flush()
			return
		case line, ok := <-rawLines:
			if !ok {
				_ = flush()
				select {
				case <-errChan:
				default:
				}
				return
			}
			batch = append(batch, line)
			if len(batch) >= readBatchSize {
				if !flush() {
					return
				}
			}
		case <-ticker.C:
			if !flush() {
				return
			}
		}
	}
}

// Stop stops the logcat process and monitoring
func (m *Manager) Stop() error {
	m.readMu.Lock()
	if m.readStop != nil {
		close(m.readStop)
		m.readStop = nil
	}
	m.readMu.Unlock()

	close(m.stopChan)
	close(m.monitorStopChan)
	return m.stopProcess()
}

func newScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, scannerBufferSize)
	scanner.Buffer(buf, maxScannerBufferSize)
	return scanner
}

func (m *Manager) setScanner(scanner *bufio.Scanner) {
	m.readMu.Lock()
	m.scanner = scanner
	m.readMu.Unlock()

	if scanner == nil {
		return
	}
	_ = m.startReader(scanner)
}

func (m *Manager) startReader(scanner *bufio.Scanner) <-chan struct{} {
	if scanner == nil {
		return nil
	}

	m.readMu.Lock()
	if m.lineChan == nil {
		m.readMu.Unlock()
		return nil
	}
	oldStop := m.readStop
	oldDone := m.readDone
	stop := make(chan struct{})
	done := make(chan struct{})
	lineChan := m.lineChan
	m.readStop = stop
	m.readDone = done
	m.readMu.Unlock()

	if oldStop != nil {
		close(oldStop)
		if oldDone != nil {
			<-oldDone
		}
	}

	go m.readLinesInternal(scanner, lineChan, stop, done)
	return done
}

func (m *Manager) stopProcess() error {
	m.cmdMu.Lock()
	cmd := m.cmd
	m.cmdMu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}

	_ = cmd.Process.Kill()
	return cmd.Wait()
}
