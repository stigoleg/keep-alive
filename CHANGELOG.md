# Changelog

All notable changes to keepalive. Releases before 2.0.0 are described on the
[GitHub releases page](https://github.com/stigoleg/keep-alive/releases).

## [2.0.0] - Unreleased

### Highlights

- **Sleep prevention that dies with the process.** keepalive now holds the
  operating system's own sleep inhibitors: IOKit power assertions on macOS,
  power requests on Windows, and logind/D-Bus inhibitors on Linux. If
  keepalive crashes or is killed, the hold goes with it. Nothing is left
  behind in your power or screensaver settings.
- **Activity simulation you can trust.** `--active` keeps Teams and Slack
  showing you as Active with human-like pointer movement. It waits until you
  have been idle, pauses while you use the computer or the screen is locked,
  and checks that every burst actually reset the system idle time. When it
  cannot work, it says why and how to fix it.
- **Work hours and process watching.** `--schedule "Mon-Fri 08:00-16:00"`
  keeps the machine awake only during work hours. `--while zoom` and
  `--pid 4242` keep it awake while a process runs, and
  `keepalive run -- CMD` while a command runs.
- **Runs in the background.** `keepalive service install` starts keepalive
  at every login (LaunchAgent, systemd user unit or Task Scheduler).
  `keepalive status`, `stop`, `active on|off` and `extend 30m` control the
  running instance.
- **`keepalive doctor`** checks what works on this machine: power
  mechanisms, activity backends, idle sources, permissions and the lock
  screen, each with a fix.
- **Scriptable.** `--json` prints one NDJSON event per line. Settings can
  come from a config file (`keepalive config init`) and `KEEPALIVE_*`
  environment variables.
- **Signed releases.** The macOS binary is universal (Intel and Apple
  silicon), signed with a Developer ID and notarized, so the Accessibility
  permission survives updates. Linux has deb, rpm and apk packages, and
  Windows ships `keepalivew.exe` for the login service.

### Breaking changes

- **Go module path is now `github.com/stigoleg/keep-alive/v2`.** Install
  with `go install github.com/stigoleg/keep-alive/v2/cmd/keepalive@latest`.
- **No more `debug.log` in the current directory.** `-l` writes
  `keepalive/keepalive.log` in the user cache directory (for example
  `~/Library/Caches/keepalive/keepalive.log` on macOS) and prints the path
  at start. `--log-file` chooses another path.
- **Stricter `-d`.** The duration must be at least one minute. `-d 0`,
  negative values, `-d 30s` and unparsable values are now errors (exit
  status 2); they used to run forever. `-c` within the current minute means
  the same time tomorrow.
- **One keepalive per user.** Starting a second one fails and points at the
  running instance. Use `keepalive status`/`stop` to control it, or
  `--replace` to take over.
- **The GUI is gone.** keepalive is a command-line tool with an interactive
  terminal UI. The menu-bar app is no longer developed or shipped.
- **Release file names changed** to `keepalive_<version>_<os>_<arch>` (for
  example `keepalive_2.0.0_darwin_universal.tar.gz`,
  `keepalive_2.0.0_linux_amd64.tar.gz`). Download scripts that used
  `keep-alive_Darwin_x86_64.tar.gz` need updating.
- **Homebrew installs a cask** instead of a formula:
  `brew uninstall keepalive && brew install --cask stigoleg/tap/keepalive`.
- **macOS 13 or later** is required.
- **Exit status:** 0 when keepalive ends normally (also on Ctrl+C), 1 on a
  runtime error, 2 on a usage error, 3 when a control command finds no
  running keepalive.
- `-b` on a machine without a battery is now an error.

### Fixes

- Headless mode never exited when the battery threshold was reached.
- The battery was first checked 30 seconds after start; it is now checked
  immediately.
- `-c` crossed midnight with a fixed 24 hours, which was wrong on
  daylight-saving days.
- Linux: D-Bus inhibitors were released as soon as they were taken;
  gsettings and xset changes survived a crash and overwrote your settings;
  `systemd-inhibit` outlived a killed keepalive and blocked shutdown.
- Linux: GNOME idle time was always read as 64 ms, so `--active` never ran;
  the uinput device was ignored by libinput; ydotool was assumed to work
  without its daemon.
- Linux: peripheral batteries (mice, headsets) no longer count as the
  system battery; a missing `capacity` file falls back to energy or charge
  values; several system batteries are combined.
- Windows: sleep prevention could leak after stop because calls hopped
  between OS threads; the PowerShell fallback did nothing.
- Windows: simulated pointer movement drifted because of pointer
  acceleration, and input that was dropped on a locked desktop went
  unnoticed.

### New

- `keepalive run -- CMD` keeps the machine awake while a command runs and
  exits with its status.
- `--schedule`, `--pid`, `--while`, `--notify` (desktop notifications when
  keepalive stops on its own or activity simulation fails),
  `--active-idle`, `--active-interval`, `--active-keys`, `--keep-display`,
  `--replace`, `--plain`, `--json`, `--log-file`, `--config`.
- Commands: `doctor`, `status`, `stop`, `active`, `extend`,
  `service install|uninstall|status`, `config init|path|show`,
  `completion`, `version`.
- Man pages and bash, zsh, fish and PowerShell completions in every archive
  and package.
- Linux packages install a udev rule (`60-keepalive-uinput.rules`) that
  gives the user at the active seat access to `/dev/uinput`, and load the
  `uinput` module at boot, so `--active` works on Wayland without adding
  yourself to the `input` group.
