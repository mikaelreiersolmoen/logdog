# Logdog

A text-based interface for viewing and filtering Android logcat logs, built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Features

- **Real-time log viewing**: Continuously reads logs from a spawned `adb logcat` process
- **Application filtering**: Filter logs by application ID via CLI flag
- **Efficient rendering**: Uses circular buffer, renders only visible rows, and batches updates
- **Color-coded priority levels**: Visual distinction for Verbose, Debug, Info, Warn, Error, and Fatal logs
- **Viewport navigation**: Scroll through logs with keyboard controls
- **Pre-filtering at source**: Leverages logcat's built-in filtering capabilities for optimal performance

## Installation

```bash
go install github.com/mikaelreiersolmoen/logdog@latest
```

Or build from source:

```bash
git clone https://github.com/mikaelreiersolmoen/logdog.git
cd logdog
go build -o logdog .
```

## Usage

```bash
# Filter by application ID
./logdog --app com.example.app

# Using shorthand flag
./logdog -a com.example.app
```

### Prerequisites

- Android Debug Bridge (ADB) must be installed and in your PATH
- An Android device connected via USB or an emulator running

### Controls

- `↑`/`↓` or `PgUp`/`PgDn`: Scroll through logs
- `q` or `Ctrl+C`: Quit the application

## Architecture

Logdog follows a clean architecture inspired by [Gren](https://github.com/langtind/gren):

```
logdog/
├── main.go                    # Entry point and CLI parsing
├── internal/
│   ├── logcat/                # Logcat process management and parsing
│   │   └── logcat.go
│   ├── buffer/                # Circular buffer implementation
│   │   └── ringbuffer.go
│   └── ui/                    # Bubble Tea UI model
│       └── model.go
```

### Performance Optimizations

1. **Circular Buffer**: Fixed-size ring buffer (10,000 entries) prevents memory growth
2. **Batch Updates**: Logcat lines are batched at ~30 FPS to reduce rendering overhead
3. **Viewport Rendering**: Only visible rows are rendered using Bubbles' viewport component
4. **Source-level Filtering**: When an app ID is provided, logcat is pre-filtered using `--pid` flag
5. **Efficient Parsing**: Logcat entries are parsed once and cached

## Built With

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) - UI components (viewport)
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Styling and layout

## Future Enhancements

- Dynamic filtering (without restarting the process)
- Search functionality
- Multiple filter presets
- Log export capabilities
- Custom color schemes

## License

MIT
