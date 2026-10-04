# Manual test: Linux, GNOME on Wayland

So far the Linux code has been verified in containers only: the X11 path in
Docker (Xvfb and xdotool), the uinput device against a real kernel in a
privileged container, and the sleep inhibitors against python-dbusmock. This
is the first run on a real GNOME desktop.

Run every step in a terminal inside the GNOME session (not over SSH), with
the screen unlocked. Do not touch the mouse or keyboard while a step says
"hands off".

## Prerequisites

- A GNOME session on Wayland: `echo $XDG_SESSION_TYPE $XDG_CURRENT_DESKTOP`
  prints `wayland` and something containing `GNOME`.
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
cat /usr/lib/modules-load.d/keepalive-uinput.conf
ls -d /sys/module/uinput
getfacl -p /dev/uinput
```

- [ ] The install prints no errors; `keepalive version` and
  `man keepalive` work.
- [ ] The rule contains `TAG+="uaccess"` and the modules-load file
  `uinput`.
- [ ] `/sys/module/uinput` exists (module loaded or built in).
- [ ] Without logging out, `getfacl` lists `user:<you>:rw-`.
- [ ] After a reboot, `/sys/module/uinput` and the ACL are still there.
- [ ] `id -nG` does **not** list `input`, and nothing told you to add it.

## 2. doctor

```sh
keepalive doctor; echo "exit $?"
```

- [ ] Exit 0. Sleep prevention: `logind` ✓, and `org.gnome.SessionManager`
  ✓ on the session bus.
- [ ] Activity simulation: `uinput` ✓; `idle time` ✓ with
  `Mutter IdleMonitor`; `screen lock` ✓ with
  `logind LockedHint, org.gnome.ScreenSaver (unlocked now)`;
  `display server  Wayland (with XWayland)`; `desktop` GNOME.
- [ ] A `!` `XWayland` row explaining that apps under XWayland may not see
  the activity.

## 3. doctor --probe

```sh
keepalive doctor --probe
```

Confirm, then hands off until it finishes.

- [ ] `input` row: `moved the pointer via uinput in …`.
- [ ] `Mutter IdleMonitor  <before> → <after>`, *after* well under a second;
  verdict `✓ input resets the idle timer that Teams/Slack read`; exit 0.
- [ ] An `xprintidle (XWayland)` row too; note whether it was reset and
  what it says.
- [ ] The pointer ends roughly where it started, and no hot corner
  (Activities, top left) opened.

## 4. Slack away test (15 minutes)

Find out whether Slack runs under XWayland: `xlsclients | grep -i slack`
prints a line if it does.

```sh
date +%H:%M          # note the start time
keepalive -a         # or: keepalive --plain -a
```

Hands off for at least 15 minutes, screen unlocked.

- [ ] Slack (watched from the second device or a colleague) stayed Active.
- [ ] The dashboard showed `simulating` with an advancing `last move`; no
  `not working` line.
- [ ] Note whether Slack was under XWayland, and whether keepalive printed
  the hint `Apps running under XWayland (some Slack/Teams builds) may not
  see this activity; start them with --ozone-platform=wayland`.

## 5. XWayland Slack

Do this whether or not step 4 passed. Quit Slack completely (tray icon →
Quit), then start it under XWayland explicitly:

For a Flatpak Slack, use `flatpak run com.slack.Slack --ozone-platform=…`
in place of `slack …` below.

```sh
slack --ozone-platform=x11 &
xlsclients | grep -i slack          # prints a line
keepalive --plain -a -l
```

Hands off for 15 minutes.

- [ ] Record whether Slack went Away.
- [ ] Record whether keepalive printed the XWayland hint (after about two
  bursts if XWayland's idle counter is not reset), and whether
  `keepalive doctor` (Running instance) shows it as a `!` row.

Quit Slack again and start it natively:

```sh
slack --ozone-platform=wayland &
xlsclients | grep -i slack          # prints nothing
```

- [ ] Repeat the 15 minutes: Slack stays Active.

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
  `keepalive status` shows `logind(sleep:idle)` plus
  `org.gnome.SessionManager` (and any other desktop inhibitor it took).
- [ ] Gone after `keepalive stop` and after `kill -9`; a new keepalive then
  starts without an "already running" error.
- [ ] `--keep-display=false`: WHAT is `sleep`.
- [ ] Optional real effect: set Settings → Power → Screen Blank to 1 minute,
  run `keepalive --plain -d 5`, hands off: the screen stays on for 5
  minutes and blanks about a minute after keepalive stops.

## 7. Interactive UI: start, stop, attach

Terminal A: `keepalive`

- [ ] Home renders correctly; `enter` on "Until I stop it" shows the
  dashboard with `● AWAKE`.
- [ ] Terminal B: `keepalive status` says `started in a terminal`.
- [ ] `a` toggles activity; `keepalive status` in B follows.
- [ ] `s` goes back to Home; `keepalive status` in B exits 3. `q` quits,
  exit 0.

Attach. Terminal A: `keepalive --plain -d 30`. Terminal B: `keepalive`.

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
- [ ] Ctrl+C during `keepalive run -- sleep 30`: exit 130.

## 10. --while

```sh
cp "$(command -v sleep)" /tmp/kawait && /tmp/kawait 30 &
keepalive --plain --while kawait
```

- [ ] Start line ends `while kawait runs`; about 30 seconds later
  `stopped: kawait exited`, exit 0.
- [ ] Run `keepalive --plain --while kawait` again with nothing running:
  `keepalive: error: no process named "kawait" is running`, exit 2.

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
- [ ] `journalctl --user -u keepalive.service -n 20` shows no errors;
  `~/.cache/keepalive/keepalive.log` has the service's info log.
- [ ] Log out and back in: running again.
- [ ] `keepalive stop` stops it, and it stays stopped until the next login.
- [ ] `keepalive service install -d 30` and `keepalive service install -c
  17:00` are refused (exit 2) with a hint to use `--schedule`, and the
  installed service is left as it was.
- [ ] `keepalive service uninstall` prints `removed the login service
  (systemd --user)`; the unit file is gone; `keepalive status` exits 3.

## 12. Lock screen pauses activity

```sh
keepalive --plain -a --active-idle 10s --active-interval 10s
```

- [ ] Hands off: `active: simulating input via uinput` within about 20
  seconds.
- [ ] Super+L, wait a minute, unlock: `active: paused while the screen is
  locked`, then `simulating` again.
- [ ] Use the mouse for a while: `active: paused while you use the
  computer`, and `simulating` again once you are hands off for about 20
  seconds.

## 13. Notifications

```sh
keepalive --plain -d 1
keepalive --plain --notify=false -d 1
```

- [ ] After a minute, a notification `Keep-Alive stopped` /
  `duration reached` from `keepalive`.
- [ ] The second run shows none.

## Report back

Paste:

1. Distribution and version, GNOME version (`gnome-shell --version`),
   architecture.
2. The output of `keepalive doctor` and `keepalive doctor --probe`, and of
   `getfacl -p /dev/uinput`.
3. What Slack showed in steps 4 and 5 (XWayland and native), and whether
   the XWayland hint appeared.
4. For every box that failed: the step number, the command, its full
   output and what you expected.
5. For anything about activity, run it again with `-l` and attach
   `~/.cache/keepalive/keepalive.log`.
