# Manual test: Linux, KDE Plasma on Wayland

So far the Linux code has been verified in containers only: the X11 path in
Docker (Xvfb and xdotool), the uinput device against a real kernel in a
privileged container, and the sleep inhibitors against python-dbusmock. This
is the first run on a real Plasma desktop.

Plasma on Wayland does not share idle time with other programs, so
keepalive cannot tell whether you are using the computer. It then simulates
activity on a fixed schedule (the first burst after `--active-idle`, then
every `--active-interval`) and reports it as working, with a hint that it
cannot pause while you use the computer. This checklist tests that mode.

Run every step in a terminal (Konsole) inside the Plasma session (not over
SSH), with the screen unlocked. Do not touch the mouse or keyboard while a
step says "hands off".

## Prerequisites

- A Plasma session on Wayland: `echo $XDG_SESSION_TYPE $XDG_CURRENT_DESKTOP`
  prints `wayland KDE`.
- The `.deb` or `.rpm` for your architecture from the release, **not** the
  tarball: this checklist tests the udev rule it installs.
- `jq`, `getfacl` (package `acl`), `xprintidle` and `xlsclients` (Debian and
  Ubuntu: `x11-utils`; Fedora: `xlsclients`).
- The Slack desktop app, signed in.
- A second device or a colleague to watch your Slack presence. Do not open
  Slack on your phone during the away test: the mobile app marks you active
  itself.
- No other keepalive running: `keepalive status` exits with 3.

## 1. Package and udev rule

```sh
sudo apt install ./keepalive_<version>_linux_<arch>.deb    # or: sudo dnf install ./….rpm
cat /usr/lib/udev/rules.d/60-keepalive-uinput.rules
ls -d /sys/module/uinput
getfacl -p /dev/uinput
```

- [ ] The install prints no errors; `keepalive version` and
  `man keepalive` work.
- [ ] The rule contains `TAG+="uaccess"`; `/sys/module/uinput` exists.
- [ ] Without logging out, `getfacl` lists `user:<you>:rw-`; after a reboot
  it still does.
- [ ] `id -nG` does **not** list `input`.

## 2. doctor

```sh
keepalive doctor; echo "exit $?"
```

- [ ] Exit 0. Sleep prevention: `logind` ✓, and `org.freedesktop.ScreenSaver`
  and/or `org.freedesktop.PowerManagement` ✓ on the session bus.
- [ ] Activity simulation: `uinput` ✓; `screen lock` ✓ with
  `logind LockedHint, org.freedesktop.ScreenSaver (unlocked now)`;
  `display server  Wayland (with XWayland)`; `desktop  KDE`.
- [ ] `! idle time  no idle source; activity is simulated on a fixed
  schedule (read for information only: xprintidle (XWayland) …)` with the
  fix `simulated activity cannot pause while you use the computer; KDE
  Plasma on Wayland does not share idle time with other programs; log in to
  a Plasma (X11) session for idle-aware simulation`.
- [ ] A `!` `XWayland` row.

## 3. doctor --probe

```sh
keepalive doctor --probe
```

Confirm, then hands off until it finishes.

- [ ] `input` row: `moved the pointer via uinput in …`.
- [ ] Verdict `! moved the pointer, but this desktop exposes no idle time to
  verify it`; exit 0.
- [ ] The pointer ends roughly where it started; no screen edge or corner
  action fired.

## 4. Fixed-cadence mode

```sh
keepalive --json -a | jq -r 'select(.type=="activity") | [.time, .snapshot.activity.state, .snapshot.activity.last_burst // ""] | @tsv'
```

Hands off for 4 minutes, then use the computer normally for 2 minutes.

- [ ] The activity lines go from `waiting_idle` to `simulating`, never
  `degraded`.
- [ ] The first burst comes 2 minutes (`--active-idle`) after the start;
  after that `last_burst` advances every 20 to 40 seconds, both while hands
  off and while you work (expected in this mode: keepalive cannot see your
  input).
- [ ] In another terminal, `keepalive status` shows `activity  simulating
  input via uinput on a fixed schedule`, and `keepalive doctor` has a `!`
  `instance` row whose fix starts with `no idle source on this desktop, so
  simulated activity cannot pause while you use the computer`.
- [ ] No desktop notification `Keep-Alive: activity simulation not working`
  appears.
- [ ] Ctrl+C stops it; the pointer is not left somewhere odd.

## 5. Slack away test (15 minutes)

Find out whether Slack runs under XWayland: `xlsclients | grep -i slack`
prints a line if it does.

```sh
date +%H:%M          # note the start time
keepalive -a         # or: keepalive --plain -a
```

Hands off for at least 15 minutes, screen unlocked.

- [ ] Slack (watched from the second device or a colleague) stayed Active.
- [ ] Note whether Slack ran under XWayland. If it went Away, quit it
  completely, start it with `slack --ozone-platform=wayland` (Flatpak:
  `flatpak run com.slack.Slack --ozone-platform=wayland`), check that
  `xlsclients` no longer lists it, and repeat.

## 6. Sleep inhibitor, including after kill -9

```sh
keepalive --plain -d 30 & sleep 1
systemd-inhibit --list | grep keepalive
keepalive status | grep power
keepalive stop
systemd-inhibit --list | grep keepalive     # no output
keepalive --plain -d 30 & sleep 1
kill -9 $!
systemd-inhibit --list | grep keepalive     # no output
keepalive --plain --keep-display=false -d 30 & sleep 1
systemd-inhibit --list | grep keepalive
keepalive stop
```

- [ ] While running: a line with WHO `keepalive`, WHAT `sleep:idle`, WHY
  `keepalive: keeping the system awake`, MODE `block`; `power` in
  `keepalive status` shows `logind(sleep:idle)` plus the desktop
  inhibitors it took.
- [ ] The Power and Battery widget in the panel lists keepalive as blocking
  sleep and screen locking while it runs. Note it if it does not.
- [ ] Gone after `keepalive stop` and after `kill -9`, also from the widget;
  a new keepalive then starts without an "already running" error.
- [ ] `--keep-display=false`: WHAT is `sleep`.
- [ ] Optional real effect: set System Settings → Power Management → Turn off
  screen to 1 minute, run `keepalive --plain -d 5`, hands off: the screen
  stays on for 5 minutes and turns off about a minute after keepalive
  stops.

## 7. Interactive UI: start, stop, attach

Konsole tab A: `keepalive`

- [ ] Home renders correctly; the activity warning on Home (if any) is
  readable. `enter` on "Until I stop it" shows the dashboard with
  `● AWAKE`.
- [ ] Tab B: `keepalive status` says `started in a terminal`.
- [ ] `a` toggles activity; `keepalive status` in B follows.
- [ ] `s` goes back to Home; `keepalive status` in B exits 3. `q` quits,
  exit 0.

Attach. Tab A: `keepalive --plain -d 30`. Tab B: `keepalive`.

- [ ] B shows the dashboard with `attached to terminal · pid N`.
- [ ] `+` in B: A prints a new `… left (until …)` line.
- [ ] `q` in B quits; A keeps running.
- [ ] `keepalive` in B, `s`, `y`: A prints
  `stopped: stopped by "keepalive stop"`; B shows the final line, then Home.

## 8. Work hours starting in 2 minutes

```sh
from=$(date -d '+2 min' +%H:%M); to=$(date -d '+5 min' +%H:%M)
keepalive --plain --schedule "daily $from-$to"
```

Run `systemd-inhibit --list | grep keepalive` in each phase.

- [ ] Start line: `… during work hours (daily HH:MM-HH:MM); outside work
  hours until HH:MM`; no inhibitor.
- [ ] At the start time: `work hours started (until HH:MM)`; inhibitor
  present.
- [ ] `keepalive status --json` has `"paused":"schedule"` before the start
  time and `"paused":""` inside the window.
- [ ] At the end time: `outside work hours until <weekday> HH:MM`;
  inhibitor gone; keepalive keeps running. Ctrl+C stops it, exit 0.

## 9. keepalive run exit status

```sh
keepalive run -- sh -c 'sleep 5; exit 3'; echo "exit $?"
keepalive run -- no-such-command; echo "exit $?"
```

- [ ] `keepalive: keeping system and display awake while sh runs`, then
  `exit 3`; the inhibitor is listed during the 5 seconds.
- [ ] `keepalive: error: command not found: no-such-command`, `exit 127`.
- [ ] No desktop notification appears when the command ends.
- [ ] `keepalive run -- sh -c 'trap "echo got INT; exit 5" INT; sleep 30 & wait'`,
  then Ctrl+C: `got INT` is printed exactly once, and the exit status is 5.

## 10. --while

```sh
cp "$(command -v sleep)" /tmp/kawait && /tmp/kawait 30 &
keepalive --plain --while kawait
```

- [ ] Start line ends `while kawait runs`; about 30 seconds later
  `stopped: kawait exited`, exit 0.
- [ ] Again with nothing running: `keepalive: error: no process named
  "kawait" is running`, exit 2.

## 11. Login service

```sh
keepalive service install -a
keepalive service status
systemctl --user status keepalive.service
```

- [ ] Install prints `installed the login service (systemd --user)`, the
  unit path, the command line and `state: running`.
- [ ] `systemctl --user status` shows it active; `keepalive status` says
  `started by the login service`.
- [ ] `journalctl --user -u keepalive.service -n 20` shows no errors.
- [ ] Log out and back in: running again.
- [ ] `keepalive stop` stops it, and it stays stopped until the next login.
- [ ] `keepalive service install -d 30`, `… -c 17:00`, `… --pid 1` and
  `… --while x` each exit 2 with `hint: a service runs at every login; use
  --schedule for work hours`, and the installed service is left as it was.
- [ ] With `keepalive --plain -d 30` running in another terminal,
  `keepalive service install` asks `Stop the running keepalive now so the
  service can start? [y/N]`. `n`: it installs anyway and warns `the
  service will not start while it runs`. Again with `y`: `stopped
  keepalive (pid N, started in a terminal)`, and the service runs. Start
  the other one again: `keepalive service install --replace` stops it
  without asking.
- [ ] Laptop only: `keepalive service install -b 100`, then unplug the
  power. Within a minute `keepalive status` shows `power  released while
  the battery is low` and `battery  NN% · pauses at 100%, resumes at 105%
  or when charging`, and the service is still running. Plug in: within a
  minute the power hold is back. Install again without `-b` afterwards.
- [ ] `keepalive service uninstall` prints `removed the login service
  (systemd --user)`; the unit file is gone; `keepalive status` exits 3.

## 12. Lock screen pauses activity

```sh
keepalive --plain -a --active-idle 10s --active-interval 10s
```

- [ ] At once: `active: waiting for idle (first burst after 10s); no idle
  source on this desktop, …`; about 10 seconds later `active: simulating
  input via uinput on a fixed schedule; …`.
- [ ] Meta+L, wait a minute, unlock: `active: paused while the screen is
  locked` while locked, then `simulating … on a fixed schedule` again.

## 13. Notifications

```sh
keepalive --plain -d 1
keepalive --plain --notify=false -d 1
```

- [ ] After a minute, a notification `Keep-Alive stopped` /
  `duration reached` from `keepalive`.
- [ ] The second run shows none.

## 14. Optional: Plasma (X11) cross-check

Log in to a "Plasma (X11)" session and run `keepalive doctor`.

- [ ] `idle time` is ✓ (KDE ScreenSaver or xprintidle) and
  `keepalive --plain -a` waits for idle instead of using a fixed schedule.

## 15. Shared runtime directory

```sh
KEEPALIVE_RUNTIME_DIR=/tmp keepalive --plain -d 5 & sleep 1
ls -ld /tmp /tmp/keepalive-$(id -u)
KEEPALIVE_RUNTIME_DIR=/tmp keepalive status
KEEPALIVE_RUNTIME_DIR=/tmp keepalive stop
```

- [ ] `/tmp` keeps its mode (`drwxrwxrwt`); `/tmp/keepalive-<uid>` is
  `drwx------`; `status` and `stop` find the running keepalive.

## Report back

Paste:

1. Distribution and version, Plasma version (`plasmashell --version`),
   architecture.
2. The output of `keepalive doctor` and `keepalive doctor --probe`, and of
   `getfacl -p /dev/uinput`.
3. The `jq` output of step 4, whether any "not working" notification
   appeared, and what Slack showed in step 5.
4. For every box that failed: the step number, the command, its full
   output and what you expected.
5. For anything about activity, run it again with `-l` and attach
   `~/.cache/keepalive/keepalive.log`.
