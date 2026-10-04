# Manual test: Windows

keepalive 2.0.0 compiles for Windows and its Windows code has been
reviewed, but it has **not yet run on Windows**. Treat every step as a first
run: an unexpected line, a console window or a crash is a finding, not
noise.

Run the commands in PowerShell (Windows Terminal is fine) unless a step
says "admin PowerShell". Keep the screen unlocked, and do not touch the
mouse or keyboard while a step says "hands off".

## Prerequisites

- Windows 10 or 11, signed in to Microsoft Teams (new Teams) and, if you use
  it, Slack.
- keepalive from the release: `scoop install keepalive` (after
  `scoop bucket add stigoleg https://github.com/stigoleg/scoop-bucket`), or
  the `keepalive_<version>_windows_amd64.zip` (or `arm64`) unpacked to a
  folder on your PATH, with `keepalive.exe` and `keepalivew.exe` side by
  side.
- An admin PowerShell (right-click Terminal → Run as administrator) for
  `powercfg /requests`. Keep it unfocused unless a step says otherwise.
- A second device (phone or another computer) or a colleague to watch your
  presence. Do not open Teams or Slack on your phone during the away test:
  the mobile apps mark you active themselves.
- Two monitors for step 13.
- No other keepalive running: `keepalive status` exits with 3
  (`$LASTEXITCODE`).

## 1. Version and doctor

```powershell
keepalive version
keepalive doctor; $LASTEXITCODE
```

- [ ] `version` prints `2.0.0` (or the snapshot version you installed).
- [ ] doctor exits 0. Sleep prevention: `PowerCreateRequest  kernel32 power
  requests`.
- [ ] Activity simulation: a `SendInput` row, an `idle time` row with
  `GetLastInputInfo`, and `screen lock  WTS session state, else the input
  desktop (unlocked now)`.
- [ ] Notifications: `toast notifications via Windows PowerShell`.
- [ ] Characters such as `✓` and `→` render correctly (or the output falls
  back to `ok`/`warn`/`FAIL`); note your terminal if they do not.

## 2. doctor --probe

```powershell
keepalive doctor --probe
```

Confirm, then hands off until it finishes.

- [ ] An `input` row (`moved the pointer via SendInput in …`), a
  `GetLastInputInfo  <before> → <after>` row where *after* is well under a
  second, and `✓ verdict  input resets the idle timer that Teams/Slack
  read`. Exit 0.
- [ ] The pointer is back exactly where it started.

## 3. Teams and Slack away test (15 minutes)

```powershell
Get-Date -Format 'yyyy-MM-ddTHH:mm'   # note the start time
keepalive -a                          # or: keepalive --plain -a
```

Hands off for at least 15 minutes, screen unlocked. Teams normally shows
Away after about 5 minutes without input.

- [ ] Teams (watched from the second device or a colleague) stayed
  Available the whole time.
- [ ] Slack stayed Active.
- [ ] The dashboard showed `simulating` with an advancing `last move`; no
  `not working` line.
- [ ] Admin PowerShell during the test: `powercfg /requests` lists
  `keepalive.exe` under `DISPLAY:` and `SYSTEM:` with
  `keepalive: keeping the system awake`.
- [ ] Optional evidence from the new Teams log (path not verified on
  Windows; adjust if it differs):

  ```powershell
  Select-String -Path "$env:LOCALAPPDATA\Packages\MSTeams_8wekyb3d8bbwe\LocalCache\Microsoft\MSTeams\Logs\MSTeams_*.log" -Pattern 'Received availability update' | Select-Object -Last 20
  ```

## 4. Power requests, including after taskkill /F

PowerShell:

```powershell
keepalive status; $LASTEXITCODE          # 3
Start-Process keepalive -ArgumentList '--plain','-d','30'
keepalive status                         # note the pid
```

Admin PowerShell: `powercfg /requests`

```powershell
keepalive stop
```

Admin PowerShell: `powercfg /requests` again. Then:

```powershell
Start-Process keepalive -ArgumentList '--plain','-d','30'
keepalive status                         # note the pid
taskkill /F /PID <pid>
keepalive status; $LASTEXITCODE          # 3
Start-Process keepalive -ArgumentList '--plain','--keep-display=false','-d','30'
```

Admin PowerShell: `powercfg /requests`, then `keepalive stop`.

- [ ] While running: `[PROCESS] …\keepalive.exe` with
  `keepalive: keeping the system awake` under both `DISPLAY:` and
  `SYSTEM:`; `keepalive status` shows
  `power  PowerRequest(DisplayRequired, SystemRequired)` or similar.
- [ ] After `keepalive stop`: no keepalive entry.
- [ ] After `taskkill /F`: no keepalive entry, and a new keepalive starts
  without an "already running" error.
- [ ] `--keep-display=false`: listed under `SYSTEM:` only.
- [ ] Ctrl+C in a `keepalive --plain -d 30` window prints
  `stopped: interrupted`, exit 0, and the entry is gone.
- [ ] Optional real effect: set "Turn off my screen after" to 1 minute
  (Settings → System → Power), run `keepalive --plain -d 5`, hands off: the
  screen stays on for 5 minutes and turns off about a minute after
  keepalive stops.

## 5. Interactive UI: start, stop, attach

Window A: `keepalive`

- [ ] Home renders without broken characters; `↑`/`↓` move the cursor.
- [ ] `enter` on "Until I stop it" shows the dashboard with `● AWAKE`.
- [ ] Window B: `keepalive status` says `started in a terminal`.
- [ ] `a` toggles activity; `keepalive status` in B follows.
- [ ] `s` goes back to Home; `keepalive status` in B exits 3.
- [ ] `q` quits; `$LASTEXITCODE` is 0.

Attach. Window A: `keepalive --plain -d 30`. Window B: `keepalive`.

- [ ] B shows the dashboard with `attached to terminal · pid N`.
- [ ] `+` in B: A prints a new `… left (until …)` line.
- [ ] `q` in B quits; A keeps running.
- [ ] `keepalive` in B again, `s`, `y`: A prints
  `stopped: stopped by "keepalive stop"`; B shows the final line, then
  Home.

## 6. Work hours starting in 2 minutes

```powershell
$from = (Get-Date).AddMinutes(2).ToString('HH:mm'); $to = (Get-Date).AddMinutes(5).ToString('HH:mm')
keepalive --plain --schedule "daily $from-$to"
```

Run `powercfg /requests` (admin) in each phase.

- [ ] Start line: `… during work hours (daily HH:MM-HH:MM); outside work
  hours until HH:MM`; no keepalive power request.
- [ ] At the start time: `work hours started (until HH:MM)`; requests
  present.
- [ ] At the end time: `outside work hours until <weekday> HH:MM`; requests
  gone; keepalive keeps running. Stop it with Ctrl+C.

## 7. keepalive run exit status

```powershell
keepalive run -- powershell -NoProfile -Command "Start-Sleep 5; exit 3"; $LASTEXITCODE
keepalive run -- no-such-command; $LASTEXITCODE
```

- [ ] First: `keepalive: keeping system and display awake while powershell
  runs`,
  then `3`. `powercfg /requests` (admin) lists keepalive during the 5
  seconds.
- [ ] Second: `keepalive: error: command not found: no-such-command`,
  `127`.
- [ ] No desktop notification appears when the command ends.
- [ ] Ctrl+C during `keepalive run -- ping -t 127.0.0.1` stops ping and
  keepalive; note the exit status.

## 8. --while

```powershell
Start-Process notepad
keepalive --plain --while notepad
```

- [ ] Start line ends `while notepad runs`.
- [ ] Close Notepad: within 2 seconds `stopped: notepad exited`, exit 0.
- [ ] With Notepad closed: `keepalive: error: no process named "notepad"
  is running` and `hint: start the app first, or check the name with
  "tasklist"`; exit 2.

## 9. Login service and keepalivew.exe

```powershell
keepalive service install -a
keepalive service status
schtasks /Query /TN keepalive /V /FO LIST | Select-String 'Task To Run','Status','Logon Mode'
```

- [ ] Install prints `installed the login service (Task Scheduler)`, a
  command line that runs `keepalivew.exe`, and `state: running`. No
  warning that keepalivew.exe was not found.
- [ ] With Scoop: `Task To Run` points at the real
  `…\scoop\apps\keepalive\current\keepalivew.exe` (or a versioned folder),
  not a shim. Note which.
- [ ] `keepalive service status` shows running and a status that says
  `started by the login service`.
- [ ] Sign out and back in: **no console window flashes** at logon, and
  `keepalive status` shows it running again.
- [ ] Task Manager → Details lists `keepalivew.exe`, not `keepalive.exe`.
- [ ] `keepalive stop` stops it; it stays stopped until the next sign-in.
- [ ] `keepalive service install -d 30` and `keepalive service install -c
  17:00` are refused (exit 2) with a hint to use `--schedule`, and the
  installed service is left as it was.
- [ ] `keepalive service uninstall` prints `removed the login service (Task
  Scheduler)`; `schtasks /Query /TN keepalive` reports that the task does
  not exist; `keepalive status` exits 3.

## 10. Lock screen pauses activity

```powershell
keepalive --plain -a --active-idle 10s --active-interval 10s
```

- [ ] Hands off: `active: simulating input via SendInput` within about 20
  seconds.
- [ ] Win+L, wait a minute, sign back in: the output has
  `active: paused while the screen is locked` while locked, then
  `simulating` again. No error lines.
- [ ] Move the mouse while the pointer moves on its own: the burst stops
  at once; `active: paused while you use the computer`.

## 11. Elevated window: degraded hint

Run keepalive in a normal (not admin) PowerShell:

```powershell
keepalive --plain -a --active-idle 10s --active-interval 10s
```

Click into the admin PowerShell window so it has focus, then hands off
for a minute.

- [ ] Within about a minute: `active: unavailable (Windows dropped the
  synthetic input); input is blocked while an elevated (administrator)
  window is focused; run keepalive elevated or focus another window`.
- [ ] A toast `Keep-Alive: activity simulation not working` appears.
- [ ] Click a normal window, hands off: `simulating` comes back. Note it if
  it does not.

## 12. Notifications

```powershell
keepalive --plain -d 1
keepalive --plain --notify=false -d 1
```

- [ ] After the first run's minute, a toast `Keep-Alive stopped` /
  `duration reached` appears (sent through Windows PowerShell; check that
  Do not disturb is off).
- [ ] No PowerShell window flashes when it is sent.
- [ ] The second run shows none.

## 13. Multi-monitor: exact return to the origin

In one PowerShell window, print the pointer position every second:

```powershell
Add-Type -AssemblyName System.Windows.Forms
while ($true) { $p = [System.Windows.Forms.Cursor]::Position; '{0:HH:mm:ss} {1},{2}' -f (Get-Date), $p.X, $p.Y; Start-Sleep 1 }
```

In another: `keepalive --plain -a --active-idle 10s --active-interval 10s`.
Put the pointer in the middle of the **second** monitor, hands off for a
minute.

- [ ] During bursts the position moves by at most about 130 px and stays on
  that monitor.
- [ ] Between bursts the position is exactly the one you left it at (same
  X,Y every time).
- [ ] Repeat with the pointer about 10 px from the edge that borders the
  other monitor, and about 10 px from a screen corner: the pointer never
  crosses to the other monitor or into the corner, and still returns
  exactly.
- [ ] If the monitors use different scaling (e.g. 100 % and 150 %), repeat
  on each.

## Report back

Paste:

1. Windows version (`winver`), CPU architecture, and how you installed
   (Scoop or zip).
2. The output of `keepalive version`, `keepalive doctor` and
   `keepalive doctor --probe`.
3. What Teams and Slack showed during the away test, and the
   `powercfg /requests` output.
4. For every box that failed: the step number, the command, its full
   output and what you expected.
5. For anything about activity or the service, run it again with `-l` and
   attach `%LOCALAPPDATA%\keepalive\keepalive.log`.
