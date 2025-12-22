# Logdog

A text-based interface for viewing and filtering Android logcat logs, built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Features

- **Real-time log viewing**: Continuously reads logs from a spawned `adb logcat` process
- **Application filtering**: Filter logs by application ID via CLI flag
- **Configurable tail size**: Load recent N entries (default 1000) for fast startup
- **Vim-like selection mode**: Click to select rows, ctrl+click (or shift+click) to extend selection, `c` to copy messages
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
# View all logs (last 1000 entries by default)
./logdog

# View last 5000 entries
./logdog --tail 5000

# Filter by application ID
./logdog --app com.example.app

# Filter by app with custom tail size
./logdog --app com.example.app --tail 500

# Using shorthand flags
./logdog -a com.example.app -t 2000
```

### Prerequisites

- Android Debug Bridge (ADB) must be installed and in your PATH
- An Android device connected via USB or an emulator running

### Controls

- `↑`/`↓` or `PgUp`/`PgDn`: Scroll through logs
- **Selection**:
  - Click on a row to select it
  - Ctrl+click (or Shift+click) on another row to extend selection (selects all rows between)
  - `x`: Toggle extend mode (for terminals like Warp that don't support modifier keys) - next click will extend
  - `c`: Copy selected messages to clipboard
  - `Esc`: Clear selection
- `l`: Open log level selector
  - Arrow keys or shortcuts (`v`/`d`/`i`/`w`/`e`/`f`) to select level
  - `Enter`: Confirm selection
  - `Esc`: Cancel
- `f`: Open filter input
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
