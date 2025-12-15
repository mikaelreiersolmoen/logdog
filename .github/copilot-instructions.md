# Logdog Development Instructions

## Project Overview

Logdog is a TUI (text-based user interface) application for viewing and filtering Android logcat logs in real-time. Built with Go and the Bubble Tea framework, it provides an efficient, color-coded interface for monitoring application logs.

## Architecture

The project follows clean architecture principles with clear separation of concerns:

- **main.go**: Entry point, CLI argument parsing, and program initialization
- **internal/logcat/**: Manages `adb logcat` process spawning, log parsing, and PID resolution
- **internal/buffer/**: Circular ring buffer implementation for efficient log storage
- **internal/ui/**: Bubble Tea UI model handling rendering, user input, and state management

## Key Design Principles

1. **Efficiency First**: All components are optimized for performance (circular buffer, batch updates, viewport rendering)
2. **Pre-filtering**: Leverage logcat's native filtering capabilities rather than post-processing
3. **Real-time Updates**: Non-blocking log reading with batched UI updates (~30 FPS)
4. **Separation of Concerns**: Clear boundaries between process management, data storage, and UI

## Development Workflow

### Building
```bash
make build          # Produces ./logdog binary
go build -o logdog . # Alternative direct build
```

### Running
```bash
make run                               # Run without filter (last 1000 entries)
make run ARGS="--tail 5000"            # Load last 5000 entries
make run ARGS="--app com.example.app"  # Via Makefile with filter
go run . --app com.example.app         # Direct execution with filter
go run . --tail 2000                   # Direct execution with custom tail
./logdog -a com.example.app            # From built binary with filter
./logdog -t 500                        # From built binary with custom tail
./logdog                               # From built binary (default 1000 entries)
```

### Testing
```bash
make test    # Run all tests
go test ./... # Alternative
```

### Installing
```bash
make install  # Install to GOPATH/bin
go install    # Alternative
```

## Code Conventions

### Error Handling
- Fatal errors should exit gracefully with helpful messages
- Non-fatal errors should be logged/displayed to the user
- Always check error returns from external processes (adb commands)

### Styling
- Use Lipgloss for all terminal styling and colors
- Priority levels have consistent color coding (Debug=cyan, Info=green, Warn=yellow, Error=red, Fatal=magenta)
- Maintain clean, readable terminal output

### Performance Considerations
- **Buffer Size**: Currently 10,000 entries - adjust if memory constraints change
- **Batch Timing**: 30ms batching interval balances responsiveness and CPU usage
- **Viewport**: Only renders visible rows, never the entire buffer
- **Logcat Filtering**: Always prefer `--pid` flag over post-processing

## Adding New Features

### Adding New Filters
1. Update logcat package to support new filter parameters (optional if appID is empty)
2. Modify main.go CLI flags if needed
3. Pass filters to logcat initialization
4. Update UI to display active filters

### Adding UI Controls
1. Add key bindings in ui/model.go Update() method
2. Update README controls section
3. Consider adding status bar indicator for new actions

### Extending Log Parsing
1. Modify logcat package's parsing logic
2. Update LogEntry struct if new fields needed
3. Adjust UI rendering to display new information

## Dependencies

- **github.com/charmbracelet/bubbletea**: TUI framework (event loop, updates, rendering)
- **github.com/charmbracelet/bubbles**: Reusable UI components (viewport)
- **github.com/charmbracelet/lipgloss**: Styling and layout

Keep dependencies minimal. Carefully evaluate any new additions.

## Testing Strategy

- Unit tests for parsing logic and buffer operations
- Mock adb interactions for logcat tests
- UI tests should verify state transitions, not rendering output
- Integration tests should verify end-to-end flow with test fixtures

## Prerequisites for Development

- Go 1.21 or later
- Android SDK Platform Tools (adb) in PATH
- Connected Android device or emulator for testing
- Familiarity with Bubble Tea's Model-View-Update pattern

## Common Issues

### ADB Not Found
Ensure `adb` is in PATH. Test with `adb devices` before running logdog.

### Process Termination
The logcat process must be properly cleaned up. Verify signal handling and defer statements.

### Buffer Wraparound
Ring buffer wraps when full. UI must handle this gracefully with correct offset calculations.

### PID Resolution
App ID to PID mapping can fail if app isn't running. Handle this error clearly.

## Performance Profiling

```bash
go build -o logdog .
./logdog --app com.example.app # Run with heavy log output
# Profile with pprof if needed
go test -cpuprofile=cpu.prof -memprofile=mem.prof ./...
```

## Release Process

1. Update version in code if versioned
2. Run tests: `make test`
3. Build: `make build`
4. Test binary with real device
5. Tag release: `git tag v0.x.x`
6. Push: `git push origin v0.x.x`
7. GitHub Actions or manual release

## Contributing Guidelines

- Follow existing code style and conventions
- Add tests for new functionality
- Update README for user-facing changes
- Keep commits atomic and well-described
- Ensure `go fmt` and `go vet` pass
