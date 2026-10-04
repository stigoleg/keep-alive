# Manual test: Linux on X11

So far the Linux code has been verified in containers only: this X11 path in
Docker (Xvfb and xdotool, with idle resets measured by xprintidle), the
uinput device against a real kernel in a privileged container, and the
sleep inhibitors against python-dbusmock. This is the first run on a real
X11 desktop.

This checklist starts from the tarball, so it covers `xdotool` and the
`/dev/uinput` fix that `keepalive doctor` suggests; the GNOME and KDE
checklists cover the packaged udev rule. Run every step in a terminal inside
the desktop session (not over SSH), with the screen unlocked. Do not touch
the mouse or keyboard while a step says "hands off".

## Prerequisites

- An X11 session (Xfce, Cinnamon, MATE, GNOME on Xorg, Plasma (X11), …):
  `echo $XDG_SESSION_TYPE $XDG_CURRENT_DESKTOP` prints `x11` and the
  desktop.
- The `keepalive_<version>_linux_<arch>.tar.gz` archive, **no** keepalive
  package installed, and no `/etc/udev/rules.d/60-keepalive-uinput.rules`.
- `xdotool`, `xprintidle` and `jq` installed.
- The Slack desktop app, signed in.
- A second device or a colleague to watch your Slack presence. Do not open
  Slack on your phone during the away test: the mobile app marks you active
  itself.
- No other keepalive running: `keepalive status` exits with 3.

## 1. Install from the tarball

```sh
mkdir -p ~/.local/bin
tar xzf keepalive_<version>_linux_<arch>.tar.gz -C ~/.local/bin keepalive
keepalive version
```

- [ ] `keepalive version` prints the release version (put `~/.local/bin`
  on your PATH if the shell cannot find it).

## 2. doctor with xdotool

```sh
keepalive doctor; echo "exit $?"
```

- [ ] Exit 0. Sleep prevention: `logind` ✓, plus any desktop inhibitor
  your desktop offers.
- [ ] Activity simulation: `xdotool` ✓; `not needed  not available here:
  uinput (…)`; `idle time` ✓ with `xprintidle` (or `Mutter IdleMonitor` on
  GNOME); `screen lock` ✓ with `logind LockedHint` and/or a desktop
  screensaver; `display server  X11`.
- [ ] No `XWayland` row.

## 3. doctor --probe with xdotool

```sh
keepalive doctor --probe
```

Confirm, then hands off until it finishes.

- [ ] `input` row: `moved the pointer via xdotool in …`; an
  `xprintidle  <before> → <after>` row with *after* well under a second;
  verdict ✓; exit 0.

## 4. Return to the origin and hot corners (xdotool)

In one terminal, print the pointer position every second:

```sh
while sleep 1; do xdotool getmouselocation; done
```

In another: `keepalive --plain -a --active-idle 10s --active-interval 10s`.
Put the pointer in the middle of the screen, hands off for a minute.

- [ ] During bursts the position moves by at most about 130 px.
- [ ] Between bursts it is back at the position you left it at.
- [ ] Park the pointer a few pixels from a screen corner (one with a hot
  corner action if your desktop has one), hands off for a minute: the
  pointer never reaches the corner, and no hot corner action fires.
- [ ] On two monitors, park it near the edge between them: bursts stay on
  that monitor.

## 5. The /dev/uinput fix from doctor

Hide xdotool so doctor shows the uinput hint:

```sh
env PATH=/usr/sbin:/sbin "$(command -v keepalive)" doctor
```

- [ ] A `!` `uinput` row about access to `/dev/uinput`, whose fix is the
  `echo 'KERNEL=="uinput", … TAG+="uaccess" …' | sudo tee
  /etc/udev/rules.d/60-keepalive-uinput.rules` one-liner followed by
  `sudo udevadm control --reload && sudo udevadm trigger`. It does not
  suggest `usermod` or the `input` group as the fix.

Run the fix exactly as printed, then:

```sh
getfacl -p /dev/uinput
keepalive doctor
```

- [ ] `getfacl` lists `user:<you>:rw-` without logging out (if not, try
  `sudo modprobe uinput` first, as the hint says, and note it).
- [ ] doctor now shows `uinput` ✓; keepalive prefers it over xdotool.
- [ ] `keepalive doctor --probe` (hands off): `moved the pointer via
  uinput`, verdict ✓.

## 6. Slack away test (15 minutes)

```sh
date +%H:%M          # note the start time
keepalive -a         # or: keepalive --plain -a
```

Hands off for at least 15 minutes, screen unlocked.

- [ ] Slack (watched from the second device or a colleague) stayed Active.
- [ ] The dashboard showed `simulating` with an advancing `last move`; no
  `not working` line.
- [ ] Note which method it used (`uinput` after step 5).

## 7. Sleep inhibitor, including after kill -9

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
  `keepalive status` shows `logind(sleep:idle)` and any desktop inhibitors.
- [ ] Gone after `keepalive stop` and after `kill -9`; a new keepalive then
  starts without an "already running" error.
- [ ] `--keep-display=false`: WHAT is `sleep`.
- [ ] `xset q` shows the same DPMS and screen saver settings before, during
  and after (keepalive no longer changes them).
- [ ] Optional real effect: set the screen to blank after 1 minute in your
  desktop's power settings, run `keepalive --plain -d 5`, hands off: the
  screen stays on for 5 minutes and blanks about a minute after keepalive
  stops.

## 8. Interactive UI: start, stop, attach

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

## 9. Work hours starting in 2 minutes

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

## 10. keepalive run exit status

```sh
keepalive run -- sh -c 'sleep 5; exit 3'; echo "exit $?"
keepalive run -- no-such-command; echo "exit $?"
```

- [ ] `keepalive: keeping system and display awake while sh runs`, then
  `exit 3`; the inhibitor is listed during the 5 seconds.
- [ ] `keepalive: error: command not found: no-such-command`, `exit 127`.
- [ ] No desktop notification appears when the command ends.
- [ ] Ctrl+C during `keepalive run -- sleep 30`: exit 130.

## 11. --while

```sh
cp "$(command -v sleep)" /tmp/kawait && /tmp/kawait 30 &
keepalive --plain --while kawait
```

- [ ] Start line ends `while kawait runs`; about 30 seconds later
  `stopped: kawait exited`, exit 0.
- [ ] Again with nothing running: `keepalive: error: no process named
  "kawait" is running`, exit 2.

## 12. Login service

```sh
keepalive service install -a
keepalive service status
```

- [ ] Install prints `installed the login service (systemd --user)` (or
  `XDG autostart` without systemd), the file, the command line and the
  state.
- [ ] systemd: `systemctl --user status keepalive.service` shows it active;
  `keepalive status` says `started by the login service`.
- [ ] Log out and back in: running again.
- [ ] `keepalive stop` stops it, and it stays stopped until the next login.
- [ ] `keepalive service install -d 30` and `keepalive service install -c
  17:00` are refused (exit 2) with a hint to use `--schedule`, and the
  installed service is left as it was.
- [ ] `keepalive service uninstall` prints `removed the login service (…)`;
  the unit or autostart file is gone; `keepalive status` exits 3.

## 13. Lock screen and your own input pause activity

```sh
keepalive --plain -a --active-idle 10s --active-interval 10s
```

- [ ] Hands off: `active: simulating input via …` within about 20 seconds.
- [ ] Lock the screen, wait a minute, unlock: `active: paused while the
  screen is locked`, then `simulating` again. If no lock line appears, note
  your screen locker.
- [ ] Use the mouse for a while: `active: paused while you use the
  computer`, and `simulating` again once you are hands off for about 20
  seconds.

## 14. Notifications

```sh
keepalive --plain -d 1
keepalive --plain --notify=false -d 1
```

- [ ] After a minute, a notification `Keep-Alive stopped` /
  `duration reached` from `keepalive`.
- [ ] The second run shows none.

## Report back

Paste:

1. Distribution and version, desktop and version, architecture.
2. The output of `keepalive doctor` and `keepalive doctor --probe` before
   and after step 5, and of `getfacl -p /dev/uinput`.
3. What Slack showed in step 6, and the positions from step 4 if a burst
   did not return exactly.
4. For every box that failed: the step number, the command, its full
   output and what you expected.
5. For anything about activity, run it again with `-l` and attach
   `~/.cache/keepalive/keepalive.log`.
