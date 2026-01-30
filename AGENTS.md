# AGENTS

## Project
- rtty is a Go CLI that exposes a local terminal in a browser via WebSocket.
- Go 1.23+ is required (see README).

## Structure
- `main.go` wires the CLI entry point.
- `cmd/` contains the CLI implementation.
- `cmd/public/` serves static frontend assets (xterm.js, CSS).

## Build and run
- Build: `go build ./...` or `go build -o rtty .`
- Run: `go run . run zsh -p 8080 -v` (swap the shell/flags as needed)

## Tests
- `go test ./...`

## Notes
- Files like `cmd/public/js/*.min.js` are vendored/minified; avoid editing unless required.
