# Manual test: macOS

Checks keepalive 2.0.0 on a real Mac. Items marked `[x]` were verified on
2026-10-04; the rest are open. Run every step in a normal terminal app with
the screen unlocked, and do not touch the mouse or keyboard while a step
says "hands off".

## Prerequisites

- macOS 13 or later, signed in to Microsoft Teams (new Teams) and, if you
  use it, Slack.
- keepalive from the release: `brew install --cask stigoleg/tap/keepalive`,
  or the `keepalive_<version>_darwin_universal.tar.gz` archive.
- A checkout of the repository for `scripts/verify-teams-presence.py`
  (Python 3.10 or later).
- A second device (phone or another computer) or a colleague to watch your
  presence. Do not open Teams or Slack on your phone during the away tests:
  the mobile apps mark you active themselves.
- No other keepalive running: `keepalive status` exits with 3.

## 1. Install and Gatekeeper

```sh
keepalive version
spctl --assess --type open --context context:primary-signature -v "$(command -v keepalive)"
```

- [x] 2026-10-04: the notarized universal binary prints
  `accepted` and `source=Notarized Developer ID`.
- [ ] The installed copy prints the same, and `keepalive version` runs
  without a Gatekeeper dialog.

## 2. doctor

```sh
keepalive doctor
```

- [ ] Every row is `✓` (or `ok` when not on a terminal); exit status 0.
- [ ] Sleep prevention: `IOPMAssertion  IOKit power assertions`.
- [ ] Activity simulation: `CoreGraphics  Accessibility granted to "<your
  terminal app>"`, an `idle time` row that names CombinedSessionState, and
  `screen lock … (unlocked now)`.
- [ ] If Accessibility is off: the row says so and names the terminal app;
  grant it, run `keepalive doctor` again, and the row turns `✓`.

## 3. doctor --probe

```sh
keepalive doctor --probe
```

Confirm, then hands off until it finishes.

- [x] 2026-10-04: CombinedSessionState idle fell from 56 s to 0.2 s, verdict
  `✓ input resets the idle timer that Teams/Slack read`.
- [ ] Same on your Mac: an `input` row (`moved the pointer via CoreGraphics
  in …`), one `before → after` row per idle source, a `✓` verdict, exit 0.
- [ ] The pointer is back where it started.

## 4. Teams and Slack away test (15 minutes)

```sh
date +%Y-%m-%dT%H:%M     # note the start time
keepalive -a             # interactive UI; or: keepalive --plain -a
```

Hands off for at least 15 minutes, screen unlocked. Then note the end time,
stop keepalive and run:

```sh
scripts/verify-teams-presence.py <start> <end>
```

- [x] 2026-10-04: with real Microsoft Teams and default settings, presence
  stayed Available for 25 minutes of absence.
- [ ] The script exits 0 and lists no `Away` while the screen was unlocked.
- [ ] Slack (watched from the second device or a colleague) stayed Active.
- [ ] The dashboard (or the `--plain` lines) showed `simulating` with a
  `last move` that kept advancing; no `not working` / `unavailable` line.

## 5. Power holds, including after kill -9

```sh
keepalive --plain -d 30 & sleep 1
pmset -g assertions | grep keepalive
keepalive stop
pmset -g assertions | grep keepalive        # no output
keepalive --plain -d 30 & sleep 1
kill -9 $!
pmset -g assertions | grep keepalive        # no output
keepalive --plain --keep-display=false -d 30 & sleep 1
pmset -g assertions | grep keepalive
keepalive stop
```

- [x] 2026-10-04 (development build): while running, three lines
  `pid N(keepalive): … PreventUserIdleSystemSleep / PreventUserIdleDisplaySleep /
  PreventSystemSleep named: "keepalive: keeping the system awake"`; none after
  `keepalive stop`; none after `kill -9`; `--keep-display=false` holds only
  `PreventUserIdleSystemSleep` and `PreventSystemSleep`.
- [ ] Same with the release binary.
- [ ] Optional real effect: set the display to turn off after 1 minute
  (System Settings → Lock Screen), run `keepalive --plain -d 5`, hands off:
  the display stays on for 5 minutes and turns off about a minute after
  keepalive stops.

## 6. Interactive UI: start, stop, attach

Terminal A:

```sh
keepalive
```

- [ ] Home shows the four choices and the options; `Simulate activity` is
  unchecked unless your config turns it on.
- [ ] `enter` on "Until I stop it" shows the dashboard with `● AWAKE`.
- [ ] Terminal B: `keepalive status` says `started in a terminal`.
- [ ] `a` toggles activity; `keepalive status` in terminal B follows.
- [ ] `s` goes back to Home; `keepalive status` in terminal B exits 3.
- [ ] `q` quits with exit status 0.

Attach. Terminal A: `keepalive --plain -d 30`. Terminal B: `keepalive`.

- [ ] Terminal B shows the dashboard with `attached to terminal · pid N`.
- [ ] `+` in B: A prints a new `… left (until …)` line, 15 minutes later.
- [ ] `q` in B quits; A keeps running.
- [ ] `keepalive` in B again, then `s`: it asks `Stop the running
  keepalive? y/n`; `y` stops A (`stopped: stopped by "keepalive stop"`),
  and B shows the final line, then Home.

Autostart: `keepalive -d 1`.

- [ ] The dashboard opens at once; after a minute the UI closes and prints
  `keepalive: stopped: duration reached at HH:MM`.

## 7. Work hours starting in 2 minutes

```sh
from=$(date -v+2M +%H:%M); to=$(date -v+5M +%H:%M)
keepalive --plain --schedule "daily $from-$to"
```

In a second terminal, run `pmset -g assertions | grep keepalive` in each
phase.

- [ ] Start line: `keeping system and display awake during work hours
  (daily HH:MM-HH:MM); outside work hours until HH:MM`; no assertions.
- [ ] At the start time: `work hours started (until HH:MM)`; assertions
  present; `keepalive status` shows `in work hours until HH:MM`.
- [ ] `keepalive status --json` has `"paused":"schedule"` before the start
  time and `"paused":""` inside the window.
- [ ] At the end time: `outside work hours until <weekday> HH:MM`;
  assertions gone; keepalive keeps running. Stop it with Ctrl+C (exit 0).

## 8. keepalive run exit status

```sh
keepalive run -- sh -c 'sleep 5; exit 3'; echo "exit $?"
keepalive run -- no-such-command; echo "exit $?"
```

- [ ] First: `keepalive: keeping system and display awake while sh runs`,
  then `exit 3`. During the 5 seconds `pmset -g assertions` shows keepalive.
- [ ] Second: `keepalive: error: command not found: no-such-command`,
  `exit 127`.
- [ ] No desktop notification appears when the command ends.
- [ ] `keepalive run -- sh -c 'trap "echo got INT; exit 5" INT; sleep 30 & wait'`,
  then Ctrl+C: `got INT` is printed exactly once, and the exit status is 5.

## 9. --while

```sh
open -a TextEdit
keepalive --plain --while TextEdit
```

- [ ] Start line ends `while TextEdit runs`.
- [ ] Quit TextEdit (⌘Q): within 2 seconds `stopped: TextEdit exited`,
  exit 0.
- [ ] With TextEdit not running: `keepalive: error: no process named
  "TextEdit" is running`, exit 2.

## 10. Login service

```sh
keepalive service install -a
keepalive service status
```

- [ ] Install prints `installed the login service (launchd)`, the plist
  path, the command line, `state: running (pid N)` and a note about
  Accessibility for "keepalive".
- [ ] `service status` shows `login service (launchd): running (pid N)` and
  a status that says `started by the login service`.
- [ ] After 2 minutes hands off, macOS asks for Accessibility for
  `keepalive` (not the terminal). After granting, `keepalive doctor`
  (Running instance) and `keepalive status` show `simulating`.
- [ ] Log out and back in: `keepalive status` shows it running again.
- [ ] `keepalive stop` stops it; it stays stopped until the next login.
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
  (launchd)` (and `stopped its keepalive` if it ran); the plist is gone;
  `keepalive status` exits 3.

## 11. Activity pauses: lock screen and your own input

```sh
keepalive --plain -a --active-idle 10s --active-interval 10s
```

- [ ] Hands off: `active: waiting for idle`, then `active: simulating input
  via CoreGraphics …` within about 20 seconds.
- [ ] Lock the screen (Ctrl+⌘+Q), wait a minute, unlock: the output has
  `active: paused while the screen is locked` and then resumes.
- [ ] Move the mouse while the pointer is moving on its own: the burst stops
  at once and the output says `active: paused while you use the computer`.
- [ ] With hands off for 2 minutes there is no `paused while you use the
  computer` line that you did not cause.
- [ ] Optional: set a hot corner, park the pointer a few pixels from that
  corner and keep hands off for a minute: the hot corner never fires.

## 12. Notifications

```sh
keepalive --plain -d 1
keepalive --plain --notify=false -d 1
```

- [ ] After the first run's minute, a notification `Keep-Alive stopped` /
  `duration reached` appears (from Script Editor; allow it under System
  Settings → Notifications if nothing shows).
- [ ] The second run shows none.

## 13. Shared runtime directory

```sh
KEEPALIVE_RUNTIME_DIR=/private/tmp keepalive --plain -d 5 & sleep 1
ls -ld /private/tmp /private/tmp/keepalive-$(id -u)
KEEPALIVE_RUNTIME_DIR=/private/tmp keepalive status
KEEPALIVE_RUNTIME_DIR=/private/tmp keepalive stop
```

- [ ] `/private/tmp` keeps its mode (`drwxrwxrwt`); `/private/tmp/keepalive-<uid>` is
  `drwx------`; `status` and `stop` find the running keepalive.

## Report back

Paste:

1. The output of `keepalive version`, `keepalive doctor` and
   `keepalive doctor --probe`.
2. The output and exit status of `scripts/verify-teams-presence.py`, and
   what Slack showed.
3. For every unticked box that failed: the step number, the command, its
   full output and what you expected.
4. For anything about activity, run it again with `-l` and attach
   `~/Library/Caches/keepalive/keepalive.log`.
