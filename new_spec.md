## Keep-Alive vNext Improvement Specification

### Overview
- **Goal**: Evolve Keep-Alive into a robust, ergonomic, and well-tested cross-platform utility with a polished TUI, resilient platform adapters, and first-class packaging.
- **Non-breaking principle**: Preserve current CLI and interactive behavior while adding optional enhancements.

### Current Architecture (snapshot)
- Entrypoint: `cmd/keepalive/main.go` wires flags, Bubble Tea program, and signal handling.
- Config: `internal/config` parses `-d/--duration` or `-c/--clock`, styles errors via `lipgloss` and UI help.
- Core: `internal/keepalive.Keeper` manages lifecycle and timing; thread-safe via mutex; uses `platform.NewKeepAlive()`.
- Platform adapters: `internal/platform` with build tags for darwin/windows/linux; implements `KeepAlive.Start/Stop` with periodic activity.
- TUI: `internal/ui` with Bubble Tea M-U-V functions, menus, timed input, running state, and a custom gradient progress bar.
- Utils: `internal/util` for duration/time parsing with user-friendly errors.
- Tests: Unit tests for config, UI, utils; platform/integration/system tests (skipped in short mode) verifying behavior per-OS.

### Key Decisions
- **Adopt Bubbles components incrementally** for input, help, progress, timer, and keybindings to streamline UI logic and improve accessibility and testability. Rationale: reduces custom UI code, aligns with Bubble Tea ecosystem best practices, and improves UX. See `charmbracelet/bubbles` for component coverage and patterns ([link](https://github.com/charmbracelet/bubbles)).
- **Keep the M-U-V structure** and pure function approach in `ui`, but replace bespoke elements (numeric input, help rendering, progress bar) with Bubbles.
- **Consolidate Windows keepalive code** into `internal/platform/windows` only; remove unused/duplicative low-level stubs in `internal/keepalive/*windows.go` if not required by tests.

### UX/TUI Improvements
1. Menu and navigation
   - Replace manual menu cursor handling with `bubbles/list` or a simple custom list paired with `bubbles/key` bindings for clarity.
   - Add visible key hints via `bubbles/help` tied to a `KeyMap` for consistent discoverability.
2. Timed input
   - Use `bubbles/textinput` for numeric minutes input (with validation) and add an optional free-form duration field (accept Go-style durations). Real-time validation feedback.
   - Add an alternative "until clock time" prompt (24h/12h) leveraging existing `util.ParseTimeString`.
3. Running view
   - Replace custom ticker with `bubbles/timer` for countdowns; drive updates via timer messages rather than manual 50ms tick.
   - Replace custom gradient with `bubbles/progress`, using a theme consistent with `lipgloss` styles. Maintain percentage and time remaining.
4. Help and accessibility
   - Migrate to `bubbles/help` for contextual help; show condensed vs expanded modes on `?`.
   - Improve focus states and high-contrast themes; ensure color fallback for low-color terminals.
5. Window size & layout
   - Handle `tea.WindowSizeMsg` to reflow content; optionally use `bubbles/viewport` for long help text.

### CLI/Config Improvements
- Accept both integer minutes and Go duration strings uniformly for `-d` (already supported), and add `--until` alias for `-c`.
- Add `--indefinite` flag as explicit mode (mutually exclusive with `-d`/`-c`).
- Provide `KEEPALIVE_DURATION`/`KEEPALIVE_CLOCK` env var defaults (flags override env).
- Add `--quiet` to suppress TUI and run headless with logs only.
- Produce machine-readable errors in non-TUI contexts (when not attached to a TTY or `--quiet`).

### Core/Keeper Enhancements
- Ensure idempotent `Start*`/`Stop` semantics and safe repeated calls; add unit tests.
- Make `TimeRemaining()` return zero when not running; already implemented — add guard tests.
- Introduce a single cancellation path (avoid both timer callback and timeout racing Stop): store an atomic state and check inside the timer callback before calling `Stop()`.
- Optional: expose a read-only channel or callback for state changes (Running/Stopped) to better coordinate UI transitions.

### Platform Reliability & Safety
- macOS
  - Keep `caffeinate -s -d -m -i -u`. Verify process group handling and ensure graceful shutdown before SIGKILL fallback; log detailed failures.
  - Add a startup self-check using `pmset -g assertions` to confirm active assertions; surface errors to UI.
- Windows
  - Use `SetThreadExecutionState` as primary and PowerShell as fallback; log which path is active.
  - On stop, ensure we restore `ES_CONTINUOUS` only once; add retry and error surfacing.
- Linux
  - Prefer `systemd-inhibit` when available; detect Wayland/X11 and gracefully skip `xset`/`xdotool` if unavailable.
  - Add GNOME settings fallback behind a clearly logged path and attempt to restore previous settings rather than assuming defaults.
  - Provide no-op paths with actionable guidance when no method is feasible; do not fail silently.
- Cross-cutting
  - Normalize tick cadence to 30s (configurable); backoff and retry when background assertions fail.
  - Guard against shell injection: never interpolate untrusted input into commands; all commands are fixed.
  - Add per-platform capability probing at start (recorded in logs and exposed in help/troubleshooting).

### Observability & Logging
- Replace ad-hoc logging with structured logs (`charmbracelet/log`) at adjustable levels. Redact sensitive info.
- Standardize log file name and rotation policy; add `--log-level` and `--log-file` flags.
- Emit lifecycle events (start/stop/errors) with platform and method metadata.

### Testing Strategy
- Unit tests
  - Keeper idempotence, timer race avoidance, and `TimeRemaining` edges.
  - Config/env precedence and mutually exclusive flags.
  - UI keymaps and validation logic (using model-level tests and component update tests).
- Integration tests
  - Mock platform layer via test doubles to simulate failures and retries.
  - Verify state transitions and cleanup signals.
- Platform tests
  - macOS: verify `caffeinate` process delta and `pmset` assertions.
  - Windows: verify `powercfg /requests` when feasible; otherwise assert calls were made (mocked).
  - Linux: verify `systemd-inhibit` process presence or safe fallbacks; avoid hard failures when utilities are missing.
- CI
  - Matrix on macOS/Windows/Linux (latest stable Go). Mark privileged/system tests as optional/nightly.

### Packaging & Distribution
- Integrate GoReleaser for multi-platform builds, checksums, archives, and brew formula.
- Codesign & notarize macOS binaries; sign Windows executables where possible.
- Provide `man` page (generated from help) and shell completions.
- Add Homebrew tap and Scoop bucket manifest; publish to GitHub Releases.

### Security & Privacy
- No secrets in code; use env var defaults for future features only.
- Validate and sanitize all external inputs (flags/env) prior to usage.
- Avoid excessive logging; configurable levels; never log environment values by default.

### Performance
- Avoid tight render loops; let component messages drive updates (timer/progress).
- Reduce external command invocations to the minimum cadence (30s) and reuse processes where possible (e.g., single `systemd-inhibit`).

### Migration Plan for Bubbles Adoption
- Phase 1: Introduce `key` + `help` components and define a `KeyMap`. Swap help rendering. Keep existing views.
- Phase 2: Replace numeric input with `textinput` and add validation feedback; introduce `timer`/`progress` in running view.
- Phase 3: Optional use of `viewport` for help and long content; consider `list` for main menu if complexity grows.
- Each phase gated behind small PRs with targeted tests.

### Detailed User Stories & Acceptance Criteria
1. As a user, I can see contextual key help
   - AC: Pressing `h`/`?` toggles concise/expanded help via `bubbles/help`.
   - AC: Key hints reflect actual `KeyMap` bindings.
2. As a user, I can enter minutes with validation feedback
   - AC: `textinput` restricts to digits by default; invalid/empty shows styled error.
   - AC: Hitting Enter on valid input starts the timer.
3. As a user, I can choose a clock time interactively
   - AC: A second prompt accepts `HH:MM` or `HH:MM[AM|PM]`; errors show guidance.
   - AC: If time is past, it targets the next day.
4. As a user, I see a smooth countdown and progress
   - AC: Running view shows `timer`-driven countdown and `progress` bar with theme.
   - AC: Indefinite mode hides countdown and shows active status.
5. As an operator, I can run headless with logs
   - AC: `--quiet` bypasses TUI, runs indefinitely/timed per flags, and emits structured logs only.
6. As a maintainer, I have structured logs
   - AC: Logs include platform, method, and lifecycle events at levels; configurable via flags.
7. As a maintainer, I trust platform cleanup
   - AC: Stop restores settings and terminates processes within 2s; retries are logged.
8. As a maintainer, CI verifies core flows across OSes
   - AC: CI matrix builds and runs unit tests; integration/system tests are gated/optional.

### Risks & Mitigations
- Platform command availability varies (e.g., Wayland without X11): probe capabilities at start; degrade gracefully with guidance.
- Tight coupling to shell commands on Linux: prefer `systemd-inhibit`; isolate command runners behind interfaces for testability.
- UI regressions from component swaps: phase adoption; snapshot tests for views where feasible.

### Milestones
- M1: KeyMap + Help + Logging; CI setup; docs update.
- M2: TextInput for minutes; validation; timer/progress in running view.
- M3: Clock-time interactive flow; env defaults; headless mode.
- M4: Platform resilience and restoration improvements; capability probes and troubleshooting section.
- M5: Packaging (GoReleaser, signing), completions, manpage.

### References
- Bubbles components and patterns: `https://github.com/charmbracelet/bubbles` (used to justify adopting textinput, help, timer, progress, keybindings)
- Bubble Tea model/update/view patterns and examples: 
  - `https://github.com/charmbracelet/bubbletea/blob/main/tutorials/basics/README.md`
  - `https://github.com/charmbracelet/bubbletea/blob/main/tutorials/commands/README.md`
