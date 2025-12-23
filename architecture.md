## Keep-Alive Architecture Overview

### Purpose
A lightweight, cross-platform CLI/TUI for preventing system sleep. This document orients new contributors and tools (like Cursor) to the project’s structure, patterns, and core flows.

### Tech Stack
- Language: Go (go 1.23)
- TUI: Bubble Tea runtime with custom M-U-V (Model/Update/View) implementation
- Styling: Lip Gloss
- Testing: Go test + Testify
- OS integration: platform-specific processes/APIs selected at build-time via tags

### Repository Structure (high-level)
- `cmd/keepalive/`
  - `main.go` — Entrypoint: parses flags, configures logging, wires Bubble Tea program, handles OS signals
- `internal/config/`
  - `flags.go` — Flag parsing (`-d/--duration`, `-c/--clock`, `-v/--version`); styled errors; printing help
  - `flags_test.go` — Flag parsing tests and time calculation checks
- `internal/keepalive/`
  - `keepalive.go` — Core `Keeper` orchestrating lifecycle: start/stop (indefinite/timed), context/timers, thread safety
  - `keepalive_test.go` — Lifecycle tests (start/stop, timed)
  - `keepalive_windows.go` / `keepalive_other.go` — Platform stubs for Windows or other when needed
- `internal/platform/`
  - `platform_interface.go` — `KeepAlive` interface: `Start(ctx)`/`Stop()`
  - `platform_darwin.go` — macOS impl: `caffeinate` + periodic assertions (`pmset touch`, `caffeinate -u`); careful process cleanup
  - `platform_windows.go` — Windows impl: `SetThreadExecutionState` primary + PowerShell fallback; periodic refresh
  - `platform_linux.go` — Linux impl: prefer `systemd-inhibit`; fall back to `xset`/`xdotool` and GNOME gsettings; restore on stop
  - `platform_other.go` — Unsupported platforms return clear error
  - `platform_test.go` — Darwin-focused process count/assertion checks
- `internal/ui/`
  - `model.go` — Bubble Tea `Model`, states, initializers (with/without preset duration)
  - `update.go` — Pure update functions per state (Menu, Timed Input, Running, Help); key handling; ticker
  - `view.go` — Pure views per state; custom gradient progress; countdown text
  - `style.go` — Centralized Lip Gloss styles and colors (`ui.Current`)
  - `ui.go` / `state.go` / `ui_test.go` — Small helpers, public wrappers, and tests
- `internal/util/`
  - `duration.go` — Parse minutes or Go duration strings with helpful error text
  - `time.go` — Parse 24h/12h clock strings (with `WithNow` variant for tests)
  - `time_test.go` — Extensive time parsing tests
- `internal/integration/`
  - `integration_test.go` — Keeper <> Platform integration (timed run, stop/cleanup)
  - `system_test.go` — Longer system-behavior checks (optional/skip in short mode)
- `docs/` — Demo assets; future: completions/manpage artifacts
- `README.md` — Features, install, usage, and behavior overview

### Core Patterns
- M-U-V (Model/Update/View):
  - `Model` holds UI state machine and session values
  - `Update` is functional and split into handlers per state
  - `View` is pure rendering using `ui.Current` styles
- Concurrency and Cleanup:
  - `internal/keepalive.Keeper` owns a platform-specific `KeepAlive` impl
  - Uses contexts (`WithCancel`/`WithTimeout`) and a `time.Timer` for timed runs
  - Mutex guards (`running`, `endTime`, timer) ensure thread safety
  - Platform adapters use tickers and `WaitGroup`s; stop ensures processes/settings are restored
- Cross-Platform via Build Tags:
  - `//go:build darwin|windows|linux` selects implementation at compile time
  - `platform_other.go` provides a safe unsupported fallback

### Key Flows
- Program lifecycle (`cmd/keepalive/main.go`):
  1. Parse flags via `config.ParseFlags(appVersion)`; version/usage handled and may exit early
  2. Initialize Bubble Tea program with `ui.InitialModel()` or `ui.InitialModelWithDuration()`
  3. Set up SIGINT/SIGTERM handler to stop keeper and kill the program
  4. Run program; Bubble Tea manages event loop and renders views
- TUI state machine (`internal/ui`):
  - Menu → (Indefinite) Running
  - Menu → Timed Input → validate minutes → Running timed
  - Running: shows active status; timed sessions show countdown and progress; Enter stops back to Menu; q/esc quits
  - Help: toggled; displays usage and examples; returns on q/esc
- Platform behavior (`internal/platform`):
  - macOS: `caffeinate -s -d -m -i -u`; periodic `pmset touch` and `caffeinate -u -t 1`; robust termination sequence
  - Windows: `SetThreadExecutionState` (`ES_CONTINUOUS|ES_SYSTEM_REQUIRED|ES_DISPLAY_REQUIRED`); PowerShell fallback; periodic refresh
  - Linux: `systemd-inhibit` preferred; otherwise `xset`/`xdotool` or GNOME settings; restore defaults on stop

### Validation & Error UX
- Duration strings: parse minutes (e.g., `150`) or Go durations (`2h30m`); rich error text
- Clock strings: parse 24h (`22:00`) and 12h (`10:00PM`), including WithNow variants for predictable tests
- Config `formatError` styles known parse errors with Lip Gloss boxes to aid usability

### Logging
- Bubble Tea debug logs to `debug.log` via `tea.LogToFile`
- Future (see `new_spec.md`): structured logs with levels and file destination flags

### Testing Strategy
- Unit tests: config, UI views/update transitions, time parsing, keeper lifecycle
- Integration: keeper timed run with platform; assertions on running/remaining/stop
- Platform/system: best-effort checks per OS; skipped in short mode or when unavailable

### External Dependencies (from `go.mod`)
- Bubble Tea: TUI runtime and message loop
- Lip Gloss: ANSI styling and layout
- Testify: assertions and require helpers
- x/sys: Windows syscall interop

### Notes on Bubbles Components
- Current UI uses custom input/help/progress; the spec proposes adopting `bubbles` components incrementally (textinput, help, timer, progress, key bindings) for improved UX and maintainability.
- Reference docs:
  - Bubbles components: `https://github.com/charmbracelet/bubbles`
  - Bubble Tea tutorials:
    - Basics: `https://github.com/charmbracelet/bubbletea/blob/main/tutorials/basics/README.md`
    - Commands: `https://github.com/charmbracelet/bubbletea/blob/main/tutorials/commands/README.md`

### Extension Guidance
- Add new UI states by extending the state enum and adding handlers in `update.go` and `view.go`
- Add platform variants by implementing `KeepAlive` and wiring `NewKeepAlive()` in a build-tagged file
- Maintain thread-safety in `Keeper`; prefer single responsibility and early returns; avoid swallowing errors

### Build & Run
- Build: `go build -o keepalive ./cmd/keepalive`
- Run TUI: `keepalive`
- Examples:
  - Timed: `keepalive -d 2h30m` or `keepalive -d 150`
  - Until: `keepalive -c 22:00`
  - Version: `keepalive --version`
