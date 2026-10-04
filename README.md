# keepalive

[![GitHub release (latest by date)](https://img.shields.io/github/v/release/stigoleg/keep-alive)](https://github.com/stigoleg/keep-alive/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/stigoleg/keep-alive)](https://goreportcard.com/report/github.com/stigoleg/keep-alive)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

keepalive keeps your computer awake for a while, until a time, during work
hours, while an app or a command runs, or until the battery runs low. It
holds the operating system's own sleep inhibitors, so nothing changes in
your power settings and the hold ends when keepalive does, even if it
crashes. With `-a` it also simulates activity, so Microsoft Teams and Slack
keep showing you as Active. It runs on macOS, Windows and Linux, as an
interactive terminal UI, a headless command or a login service.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [Activity simulation](#activity-simulation)
- [Platform setup](#platform-setup)
- [Work hours](#work-hours)
- [Keep awake while a process runs](#keep-awake-while-a-process-runs)
- [Background service](#background-service)
- [Controlling a running keepalive](#controlling-a-running-keepalive)
- [Configuration](#configuration)
- [Output and scripting](#output-and-scripting)
- [Troubleshooting](#troubleshooting)
- [Logs](#logs)
- [Migrating from 1.x](#migrating-from-1x)
- [Building from source](#building-from-source)

## Install

### Homebrew (macOS)

```sh
brew install --cask stigoleg/tap/keepalive
```

Upgrading from 1.x, which was a formula: run `brew uninstall keepalive`
first.

### Scoop (Windows)

```powershell
scoop bucket add stigoleg https://github.com/stigoleg/scoop-bucket
scoop install keepalive
```

### Linux packages

Download the package for your architecture (`amd64`, `arm64`, `386` or
`armv7`) from the [releases page](https://github.com/stigoleg/keep-alive/releases/latest):

```sh
sudo apt install ./keepalive_2.0.0_linux_amd64.deb            # Debian, Ubuntu
sudo dnf install ./keepalive_2.0.0_linux_amd64.rpm            # Fedora, RHEL
sudo apk add --allow-untrusted keepalive_2.0.0_linux_amd64.apk  # Alpine
```

The packages install a udev rule (`/usr/lib/udev/rules.d/60-keepalive-uinput.rules`)
that gives the user at the seat access to `/dev/uinput` and load the
`uinput` module at boot, so `-a` works on Wayland without further setup
(see [Linux](#linux) for what that rule allows). They also install man
pages and bash, zsh and fish completions.

### Direct download

Every release has these archives, plus `checksums.txt`:

| Platform | Archive |
|---|---|
| macOS 13+ (Intel and Apple silicon) | `keepalive_<version>_darwin_universal.tar.gz` |
| Linux | `keepalive_<version>_linux_<arch>.tar.gz` (`amd64`, `arm64`, `386`, `armv7`) |
| Windows | `keepalive_<version>_windows_<arch>.zip` (`amd64`, `arm64`) |

```sh
VERSION=2.0.0
curl -LO "https://github.com/stigoleg/keep-alive/releases/download/v$VERSION/keepalive_${VERSION}_darwin_universal.tar.gz"
tar xzf "keepalive_${VERSION}_darwin_universal.tar.gz" keepalive
sudo install -m 755 keepalive /usr/local/bin/
```

Each archive also holds the man pages and shell completions. The macOS
binary is signed with a Developer ID and notarized. The Windows zip has
`keepalive.exe` and `keepalivew.exe`; keep them in the same folder (the
login service uses `keepalivew.exe` so no console window opens at logon).

### Go

```sh
go install github.com/stigoleg/keep-alive/v2/cmd/keepalive@latest
```

Needs Go 1.25 or later. On macOS, build with cgo (the default when the Xcode
command-line tools are installed): without it, `-a` is unavailable and sleep
prevention falls back to `caffeinate`.

## Quick start

```sh
keepalive                                       # open the interactive UI
keepalive -d 2h                                 # keep awake for 2 hours
keepalive -c 17:00 -a                           # until 17:00, and simulate activity
keepalive --schedule "Mon-Fri 08:00-16:00" -a   # during work hours
keepalive run -- make release                   # while a command runs
```

On a terminal keepalive opens the interactive UI: `-d`, `-c` and `-b` start
right away, anything else waits for Enter. With `--plain` (or without a
terminal) it runs headless and prints one line per event. Ctrl+C stops it.

| Flag | Meaning |
|---|---|
| `-d, --duration` | for this long: minutes (`150`) or a duration (`2h30m`); at least 1m |
| `-c, --clock`, `--until` | until a time of day (`17:00`, `5:00PM`); a time already past today means tomorrow |
| `-b, --battery` | stop when the battery is at or below this percentage |
| `--keep-display=false` | keep only the system awake; the display may sleep |
| `-a, --active` | simulate activity (see below) |
| `--replace` | stop a keepalive that is already running and take over |

`-d` or `-c` can be combined with `-b`; keepalive stops at whichever comes
first. `keepalive --help` lists everything, and every command has `--help`
and a man page.

### The interactive UI

```
 keepalive 2.0.0                                        ● AWAKE

 Keeping system and display awake
 ██████████████████░░░░░░░░░░░░  1h 12m left · until 17:00

 Activity     ● simulating · last move 12s ago · CoreGraphics
 Work hours   Mon-Fri 08:00-16:00 · ends 16:00
 Battery      76% · stops at 20%
 Holding      IOPMAssertion(PreventUserIdleSystemSleep, +2)

 a activity off · +/- 15 min · s stop · ? help · q quit
```

| Screen | Keys |
|---|---|
| Home | `↑` `↓` choose, `enter` start, `a` simulate activity, `d` keep display on, `b` stop at a battery level, `k` tap Shift too, `?` help, `q` quit |
| Running | `a` activity on/off, `+` `-` 15 minutes more or less, `s` or `esc` stop and go back, `q` stop and quit |
| Attached to another keepalive | `a`, `+`, `-` act on it, `s` asks before stopping it, `q` detaches and leaves it running |

## Activity simulation

Teams and Slack mark you Away after a few minutes without input. With `-a`,
keepalive waits until you have been idle for 2 minutes (`--active-idle`),
then about every 30 seconds (`--active-interval`, ±35 %) moves the pointer
along a short, curved, human-like path and back to where it was. A burst
takes 1 to 2.5 seconds, stays within about 130 px and, when keepalive can
see the pointer position, keeps clear of screen edges and hot corners.
`--active-keys` also taps Shift with each burst.

- **It gets out of your way.** It pauses as soon as you use the mouse or
  keyboard (on macOS and Windows even in the middle of a burst), and while
  the screen is locked.
- **It checks its own work.** After every burst keepalive reads the system
  idle time that chat apps read (CombinedSessionState on macOS,
  `GetLastInputInfo` on Windows, Mutter, KDE or `xprintidle` on Linux). If
  two bursts in a row did not reset it, keepalive reports activity as not
  working, with the reason and a fix, and shows a desktop notification
  (see `--notify`).
- **It is tested against the real thing.** On macOS with Microsoft Teams and
  default settings, presence stayed Available through 25 minutes away from
  the computer.

`keepalive doctor` shows which input method and idle source will be used.
`keepalive doctor --probe` moves the pointer once and checks that the idle
time was reset.

## Platform setup

| | Sleep prevention | Activity input | Idle check |
|---|---|---|---|
| macOS | IOKit power assertions | CoreGraphics | CombinedSessionState |
| Windows | power requests | `SendInput` | `GetLastInputInfo` |
| Linux | logind inhibitor, plus GNOME/KDE/XFCE over D-Bus | `/dev/uinput`, `ydotool` 1.x or `xdotool` (X11) | Mutter (GNOME), `xprintidle` (X11), KDE (X11) |

### macOS

`-a` needs the Accessibility permission (System Settings → Privacy &
Security → Accessibility). macOS asks for it the first time; which app gets
the entry depends on how keepalive runs:

- **In a terminal:** the terminal app (Terminal, iTerm2, Ghostty, …).
- **As the login service:** `keepalive` itself.

`keepalive doctor` names the app. keepalive rechecks every 60 seconds, so
there is no need to restart it after granting. The grant is tied to the
release binary's signature and survives updates; a binary you build
yourself needs the grant again after every rebuild.

### Linux

Sleep prevention works out of the box on any desktop with systemd-logind.
Over SSH, logind may refuse the inhibitor (polkit); run keepalive in your
desktop session instead.

For `-a`, keepalive tries `/dev/uinput` first (works on Wayland and X11),
then `ydotool` 1.x (with `ydotoold` running), then `xdotool` (X11 only).
The Linux packages set up `/dev/uinput` for you. Otherwise, give your login
session access to it with the same udev rule (this is what `keepalive
doctor` suggests):

```sh
echo 'KERNEL=="uinput", SUBSYSTEM=="misc", TAG+="uaccess", OPTIONS+="static_node=uinput"' | sudo tee /etc/udev/rules.d/60-keepalive-uinput.rules
sudo udevadm control --reload && sudo udevadm trigger
sudo modprobe uinput
echo uinput | sudo tee /etc/modules-load.d/keepalive-uinput.conf   # load it at boot
```

Do not add yourself to the `input` group instead: that lets every program
you run read your keyboard.

The trade-off of the rule: every process of the user at the active seat can
write to `/dev/uinput`, so any of them could inject input into your
session. If you never use `-a`, remove the packaged rule
(`/usr/lib/udev/rules.d/60-keepalive-uinput.rules`), or mask it with an
empty file of the same name in `/etc/udev/rules.d/`.

| Session | Idle-aware? | Notes |
|---|---|---|
| GNOME on Wayland | yes (Mutter) | |
| KDE Plasma on Wayland | no | Plasma does not share idle time, so keepalive simulates activity on a fixed schedule (every `--active-interval`) and says so |
| Other Wayland compositors (sway, Hyprland, …) | no | fixed schedule, as above |
| X11 | yes | GNOME and KDE report idle time themselves; other desktops need `xprintidle` (`sudo apt install xprintidle`) |

**XWayland.** On Wayland, apps that run under XWayland (some Slack and Teams
builds) only see input while the pointer is over one of their windows, so
they may still go Away. keepalive warns when it detects this. Start the app
with `--ozone-platform=wayland` to fix it.

### Windows

No setup is needed. Windows blocks simulated input to a window that runs as
administrator (User Interface Privilege Isolation): while such a window has
focus, keepalive reports activity as not working. Focus another window, or
run keepalive as administrator too.

## Work hours

`--schedule` keeps the machine awake only inside weekly windows, in local
time. Outside them keepalive releases the sleep hold and pauses activity
simulation, and picks both up again when the next window starts.

```sh
keepalive --schedule "Mon-Fri 08:00-16:00"
keepalive --schedule "weekdays 08:00-11:30,12:00-16:00; Sat 10:00-12:00" -a
keepalive --schedule "daily 22:00-06:00"
```

```
SPEC     = rule { ";" rule }
rule     = days windows
days     = item { "," item }      item = Day | Day-Day | daily | weekdays | weekends
Day      = Mon Tue Wed Thu Fri Sat Sun (or Monday … Sunday), any case
windows  = HH:MM-HH:MM { "," HH:MM-HH:MM }   24-hour; 24:00 allowed as an end
```

- Day ranges may wrap: `Fri-Mon` is Friday to Monday. `weekdays` is
  `Mon-Fri`, `weekends` is `Sat,Sun`.
- A window that ends before it starts runs past midnight and belongs to its
  start day: `Fri 22:00-02:00` is Friday 22:00 to Saturday 02:00.
- Equal start and end is an error; use `00:00-24:00` for a whole day.
- Overlapping or touching windows count as one.
- `-d`, `-c` and `-b` still end the whole session (in the
  [login service](#background-service) `-b` pauses it instead).

## Keep awake while a process runs

```sh
keepalive --while zoom          # until no process named zoom is left
keepalive --pid 4242 --pid 4243 # until these processes have exited
keepalive run -- ./backup.sh    # run a command and keep awake until it exits
```

`--while` matches the process name without regard to case, with or without
its extension (`zoom` matches `zoom.us` and `Zoom.exe`). The process must be
running when keepalive starts.

`keepalive run` gives the command the terminal, forwards signals to it and
always exits with the command's exit code. If it cannot keep the machine
awake, it prints a warning and runs the command anyway. Session flags go
before `--` (`keepalive run -a -b 20 -- rsync -a ~/photos nas:/backup`). It
also works next to a keepalive that is already running, and sends no
desktop notification when the command ends.

## Background service

```sh
keepalive service install --schedule "Mon-Fri 08:00-16:00" -a
keepalive service status
keepalive service uninstall
```

`service install` starts keepalive now and at every login: a LaunchAgent on
macOS, a systemd user unit on Linux (an XDG autostart entry without
systemd), and a Task Scheduler task on Windows. It takes the session flags
except the one-shot limits (`-d`, `-c`/`--until`, `--pid`, `--while`), which
it refuses; use `--schedule` to limit when it keeps the machine awake.
Anything you do not pass is read from the config file each time the
service starts. Installing again replaces the service.

With `-b`, the service does not end at the battery threshold: it stops
keeping the machine awake until the battery is 5 points above the
threshold or charging, then carries on. The service logs to a file and
shows a desktop notification if it cannot start or stops on its own.

## Controlling a running keepalive

Only one keepalive runs per user. Wherever it was started (a terminal, the
service or `keepalive run`), these commands control it:

```sh
keepalive status            # what it is doing; exit status 3 if none is running
keepalive status --follow   # and keep printing its events
keepalive active on         # turn activity simulation on (or off)
keepalive extend 30m        # move the end; -15m shortens it
keepalive stop
```

```
keepalive 2.0.0 · pid 4242 · started in a terminal at 10:02
  session     1h12m left (until 12:02); keeps the system and display awake
  activity    simulating input via CoreGraphics
  power       IOPMAssertion(PreventUserIdleSystemSleep, PreventUserIdleDisplaySleep, PreventSystemSleep)
  battery     80% · stops at 20%
```

Running `keepalive` in a terminal while another one runs opens the UI
attached to it. Starting a second session fails and points at the running
one; `--replace` takes over instead.

## Configuration

Settings you always want go in a TOML file. Precedence: flag > `KEEPALIVE_*`
environment variable > config file > default.

```sh
keepalive config init     # write a commented template
keepalive config show     # the effective settings and where each comes from
keepalive config path
```

The file is `~/.config/keepalive/config.toml` (`$XDG_CONFIG_HOME` if set) on
macOS and Linux, and `%APPDATA%\keepalive\config.toml` on Windows. Keys are
the long flag names in snake_case: `duration`, `battery`, `active`,
`active_idle`, `active_interval`, `active_keys`, `schedule`,
`keep_display`, `notify`, `log`, `log_file`. The environment variable is
`KEEPALIVE_` plus the key in upper case.

```toml
active = true
active_idle = "90s"
schedule = "Mon-Fri 08:00-16:00"
```

```sh
KEEPALIVE_ACTIVE_IDLE=3m keepalive -a
```

## Output and scripting

Headless output is one line per event:

```
10:02:11 keeping system and display awake for 2h0m (until 12:02), stopping at 20% battery, simulating activity
10:04:11 active: waiting for idle (needs 2m0s)
10:07:11 active: simulating input via CoreGraphics mouse events
12:02:11 stopped: duration reached
```

`--json` prints NDJSON instead, one object per event:

```json
{"time":"…","type":"started","snapshot":{…},"message":"keeping system and display awake for 30m (until 14:34)","reason":""}
```

- `type`: `started`, `snapshot`, `activity`, `battery`, `warning`,
  `schedule`, `stopping`, `stopped`.
- `snapshot`: `running`, `started_at`, `ends_at`, `remaining` (seconds),
  `mode` (`indefinite`, `duration`, `until`), `active`, `activity`
  (`state`, `method`, `reason`, `hint`, `last_burst`, `idle`), `battery`
  (`percent`, `available`, `threshold`), `keep_display`, `power_hold`,
  `schedule`, `in_window`, `next_change`, `watching`.
- `activity.state`: `off`, `waiting_idle`, `simulating`, `paused_user`,
  `paused_locked`, `degraded`.
- `reason` on `stopping`/`stopped`: `user`, `duration`, `until`, `battery`,
  `signal`, `ipc`, `process_exited`, `command_exited`, `error`.

`keepalive status --json` prints `{pid, version, origin, snapshot}`, and
`keepalive doctor --json` the whole report.

| Exit status | Meaning |
|---|---|
| 0 | ended normally, including Ctrl+C and `keepalive stop` |
| 1 | runtime error, e.g. nothing can keep the machine awake; `doctor`: a check failed |
| 2 | usage error: a bad flag, value, schedule or process |
| 3 | `status`, `stop`, `active`, `extend`: no keepalive is running |
| *n* | `keepalive run`: the command's exit status (127 not found, 126 not executable) |

`--notify` controls desktop notifications when keepalive stops on its own or
activity simulation fails. They are on by default except in the interactive
UI; `keepalive run` sends none when its command ends.

## Troubleshooting

Start with:

```sh
keepalive doctor            # every check, each problem with a fix
keepalive doctor --probe    # also move the pointer once and verify the idle reset
```

| Problem | Fix |
|---|---|
| macOS: "Accessibility permission is missing" | Turn on the app `doctor` names under Privacy & Security → Accessibility. After an update, remove the entry and add it again. |
| macOS: no notifications | They come from Script Editor; allow it under System Settings → Notifications. |
| Linux: "no write access to /dev/uinput" | Install the package, or add the udev rule under [Linux](#linux). |
| Linux: "simulating on a fixed schedule" | KDE Plasma or another compositor on Wayland does not share idle time. Use GNOME, or a Plasma (X11) session. |
| Linux: the XWayland warning | Start Slack or Teams with `--ozone-platform=wayland`. |
| Linux: the inhibitor is refused | logind's polkit rules refuse it outside a desktop session; run keepalive from the desktop. |
| Windows: "Windows dropped the synthetic input" | An administrator window has focus; see [Windows](#windows). |
| "another keepalive is already running" | `keepalive status`, `keepalive stop`, or start with `--replace`. |

## Logs

`-l` writes a debug log and prints its path at start; `--log-file PATH`
picks another file. The default is `keepalive.log` in the user cache
directory:

| OS | Log file |
|---|---|
| macOS | `~/Library/Caches/keepalive/keepalive.log` |
| Linux | `~/.cache/keepalive/keepalive.log` (`$XDG_CACHE_HOME` if set) |
| Windows | `%LOCALAPPDATA%\keepalive\keepalive.log` |

The login service always writes an info log there. On macOS its console
output goes to `~/Library/Logs/keepalive/service.log`; on Linux see
`journalctl --user -u keepalive.service`.

## Migrating from 1.x

2.0.0 changes a few things. The [CHANGELOG](CHANGELOG.md) has the full
list; the ones that break scripts:

- `-l` writes its log to the user cache directory, not the current
  directory (see [Logs](#logs)).
- `-d` must be at least one minute; `-d 0` and `-d 30s` are errors.
- Only one keepalive runs at a time; use `status`/`stop`, or `--replace`.
- Release files are named `keepalive_<version>_<os>_<arch>`, and Homebrew
  installs a cask.
- The Go module is `github.com/stigoleg/keep-alive/v2`.
- The menu-bar GUI is gone, and macOS 13 or later is required.

## Building from source

```sh
make check                    # gofmt, go vet for every OS, go test -race
make build                    # bin/keepalive
make docs                     # man pages and shell completions
KEEPALIVE_SIGN=0 make snapshot  # every release artifact in dist/, unsigned
```

`make snapshot` needs [GoReleaser](https://goreleaser.com). The Linux
end-to-end tests run in Docker: `test/docker/power/run.sh` (sleep
inhibitors against a mocked D-Bus) and `test/docker/activity` (activity
simulation on X11). Releases are made with `make release`; see
[RELEASING.md](RELEASING.md). Manual test checklists per platform are in
[docs/testing](docs/testing).

## License

[MIT](LICENSE)
