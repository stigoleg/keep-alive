# Renders docs/demo.gif with VHS (https://github.com/charmbracelet/vhs).
#
#   cd docs && KEEPALIVE_DEMO_BIN=/path/to/dir/with/keepalive vhs demo.tape
#
# The recording runs the real keepalive with activity simulation, so it moves
# the pointer of the machine that records it: do not touch the mouse or the
# keyboard while it runs (about a minute). A short idle threshold and interval
# make the activity visible within the recording.

Output demo.gif

Require bash

Set Shell "bash"
Set FontFamily "Menlo"
Set FontSize 17
Set LineHeight 1.25
Set Width 820
Set Height 540
Set Padding 28
Set Framerate 30
Set TypingSpeed 70ms
Set CursorBlink false
Set Theme { "name": "keepalive", "background": "#0f1117", "foreground": "#e4e6ee", "cursor": "#a78bfa", "selection": "#1e2232", "black": "#0f1117", "red": "#f87171", "green": "#4ade80", "yellow": "#fbbf24", "blue": "#a78bfa", "magenta": "#f472b6", "cyan": "#2dd4bf", "white": "#e4e6ee", "brightBlack": "#5b6172", "brightRed": "#f87171", "brightGreen": "#4ade80", "brightYellow": "#fbbf24", "brightBlue": "#a78bfa", "brightMagenta": "#f472b6", "brightCyan": "#2dd4bf", "brightWhite": "#ffffff" }

Env COLORTERM "truecolor"
Env KEEPALIVE_ACTIVE_IDLE "10s"
Env KEEPALIVE_ACTIVE_INTERVAL "6s"

Hide
Type 'export PATH="${KEEPALIVE_DEMO_BIN:-$PATH}:$PATH" PS1="$ "; clear'
Enter
Sleep 500ms
Show

Sleep 800ms
Type "keepalive -a"
Sleep 600ms
Enter
Sleep 2.5s

# Pick "For a duration…" and keep the machine awake for two hours.
Down
Sleep 700ms
Enter
Sleep 900ms
Type "2h"
Sleep 1.5s
Enter

# Idle for the threshold: the dashboard counts down, then activity starts.
Sleep 16s

# Extend by 15 minutes, look at the keys, then stop and quit.
Type "+"
Sleep 2.5s
Type "?"
Sleep 3.5s
Type "?"
Sleep 1.5s
Type "s"
Sleep 2s
Type "q"
Sleep 1s
