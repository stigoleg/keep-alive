# Changelog

All notable changes to keepalive. Releases before 2.0.0 are described on the
[GitHub releases page](https://github.com/stigoleg/keep-alive/releases).

## [2.0.0] - 2026-10-04

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
- **A new interactive UI.** Pick how long to stay awake, toggle activity,
  display and battery options, and follow a dashboard with the time left,
  the activity state, the work hours and the power hold. Opened while
  another keepalive runs (the login service, for example), it attaches to
  that one instead of refusing to start.
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
- **New keys in the interactive UI.** `↑`/`↓` choose (`j`/`k` are gone),
  `b` turns the battery limit on and off (`B` is gone), `esc` no longer
  quits, and `+`/`-` move the end of a running session by 15 minutes.
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
- Activity simulation kept no distance from display edges, so a burst could
  trigger a hot corner (GNOME Activities, Plasma Overview, macOS). Bursts
  now stay 3 px off every edge and out of a 24 px square at each corner;
  where the pointer position cannot be read (uinput, ydotool), they head
  right and down and barely move up or left.
- Activity simulation kept moving the pointer while you took the mouse in
  the middle of a burst. On macOS and Windows a burst now stops at once and
  waits for the next idle period.
- Windows: the pointer could end a pixel or two away from where it started;
  it now returns exactly. Bursts stay on the monitor under the cursor. When
  the session query fails, the lock screen is detected through the input
  desktop. System DLLs load from System32 only.
- Linux: on Wayland, XWayland's idle counter could gate or verify activity,
  although it only sees input over XWayland windows. It is now only
  watched: when it misses bursts, keepalive warns that apps under XWayland
  may go Away and suggests `--ozone-platform=wayland`.
- Linux: the fix for `/dev/uinput` access told you to join the `input`
  group, which lets every program read your keyboard. It now grants access
  to the user at the seat with a `uaccess` udev rule.
- `-d`, `-c`, `extend` and work hours follow the wall clock, also after the
  machine has slept.
- Linux: input methods are checked for real (keepalive opens `/dev/uinput`
  and connects to `ydotoold`), and when one fails during a session
  keepalive switches to the next and replays the burst on it.
- Linux: the screen counts as locked when logind or any running desktop
  screensaver (GNOME, KDE, MATE, Cinnamon, XFCE) says so.
- Stopping keepalive now also ends activity setup, idle and lock queries
  and a burst in progress at once, even when a D-Bus call or a helper
  process hangs.

### New

- `keepalive run -- CMD` keeps the machine awake while a command runs and
  always exits with the command's status. If the machine cannot be kept
  awake it prints a warning and runs the command anyway. It sends no
  desktop notification when the command ends. On a terminal, Ctrl+C and
  Ctrl+\ reach the command once; without a controlling terminal the command
  gets its own process group and keepalive passes signals on to it (so a
  SIGKILL to keepalive's group, as `timeout -k` sends, does not reach the
  command).
- `keepalive service install` takes the session flags except the one-shot
  limits (`-d`, `-c`/`--until`, `--pid`, `--while`), which it refuses with a
  hint to use `--schedule` (exit status 2); the service ignores a
  `duration` from the config file or `KEEPALIVE_DURATION`, with a warning,
  and install warns about one in the config file. When another keepalive is
  running it asks whether to stop it on a terminal, stops it with
  `--replace`, and otherwise installs and says why the service cannot start
  yet. In the service, `-b` pauses keeping awake while the battery is at or
  below the threshold and resumes once it is 5 points above it or on
  external power, instead of ending the service; `-b 100` keeps the machine
  awake only while it is plugged in.
- A service that keeps failing shows each kind of notification at most once
  every 10 minutes, also across restarts (`keepalive/notified.json` in the
  user cache directory), and `keepalive doctor` reports it as restarting
  repeatedly.
- `--json` snapshots say why nothing is held right now (`paused`:
  `schedule` or `battery`) and whether the battery threshold pauses
  (`battery.pause`).
- A mistyped command (`keepalive statsu`, `keepalive service instal`) is a
  usage error (exit status 2) with a suggestion.
- `KEEPALIVE_RUNTIME_DIR` moves the control socket. An existing directory
  that other users can read (such as `/tmp`) is left as it is; keepalive
  uses a private `keepalive-<uid>` directory inside it. Symlinks are
  resolved (`/tmp` on macOS), and a keepalive that cannot create its control
  socket does not start instead of running out of reach of `status` and
  `stop`.
- Linux: on desktops without an idle source (KDE Plasma and Wayland
  compositors other than GNOME) activity is simulated on a fixed schedule,
  reported as working with a hint that it cannot pause while you use the
  computer.
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
  yourself to the `input` group. The rule lets every process of that user
  write to `/dev/uinput`; if you never use `--active`, delete it or mask it
  with an empty `/etc/udev/rules.d/60-keepalive-uinput.rules`.
