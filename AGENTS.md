# Repository Guidelines

## Project Structure & Module Organization
- `main.go` wires up the CLI/TUI and starts the app.
- `internal/adb/` handles ADB device and PID discovery.
- `internal/logcat/` contains logcat stream parsing and filtering logic (tests live here).
- `internal/ui/` holds Bubble Tea models, styles, formatting, and clipboard helpers.
- `internal/config/` loads and persists user configuration.
- `build/` is used for release artifacts created by Make targets.

## Build, Test, and Development Commands
- `make build` builds the `logdog` binary in the repo root.
- `make run ARGS="--app com.example.app"` runs locally with CLI args.
- `make test` or `go test ./...` runs the Go test suite.
- `make build-all` builds cross‑platform release archives in `build/`.

## Coding Style & Naming Conventions
- Use standard Go formatting: `gofmt -w .` before committing.
- Package names are lowercase; files are lowercase with underscores when needed (e.g., `logcat_test.go`).
- Exported identifiers use `CamelCase`; unexported use `camelCase`.
- Keep UI styling in `internal/ui/` and avoid cross‑layer imports (UI should not depend on ADB).

## Commit & Pull Request Guidelines
- Commit messages are short, imperative, sentence case (e.g., “Update Makefile”).
- PRs should explain the what/why, link related issues, and include screenshots for UI changes.

## Configuration & Prerequisites
- Requires ADB in `PATH` and a connected device/emulator.
- User config is stored at `~/.config/logdog/config.json` (log level, filters, tail size, etc.).
