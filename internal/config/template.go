package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Template is what `keepalive config init` writes. Every key is commented
// out, so the file as written changes nothing.
const Template = `# keepalive configuration file.
#
# Precedence: command-line flag > KEEPALIVE_* environment variable > this
# file > built-in default. The variable for a key is KEEPALIVE_ plus the key
# in upper case, e.g. KEEPALIVE_ACTIVE_IDLE=90s.
#
# Every setting is optional. Remove the leading "# " to set one.

# How long to keep the system awake: minutes (150) or a duration ("2h30m").
# Unset means until stopped.
# duration = "2h30m"

# Stop once the battery is at or below this percentage (1-100).
# battery = 20

# Simulate user activity so chat apps (Slack, Teams) keep you active.
# active = false

# Idle time required before activity is simulated (at least 10s).
# active_idle = "2m"

# Mean gap between simulated activity bursts (at least 5s).
# active_interval = "30s"

# Also send a harmless key press with each burst.
# active_keys = false

# Only keep awake during these work hours (local time); outside them keepalive
# pauses. Days: Mon-Sun, ranges (Mon-Fri), weekdays, weekends, daily.
# schedule = "Mon-Fri 08:00-16:00"

# Keep the display on too; false keeps only the system awake.
# keep_display = true

# Show a desktop notification when a session ends.
# notify = false

# Write a debug log.
# log = false

# Log file location; defaults to keepalive/keepalive.log in the user cache
# directory.
# log_file = "~/keepalive.log"
`

// TOML renders the effective configuration with a comment per key naming the
// layer it came from. Keys without a value are emitted as comments so the
// output stays loadable as a config file.
func (r Resolved) TOML() string {
	var b strings.Builder
	b.WriteString("# Effective keepalive configuration.\n")
	if r.Path != "" {
		state := "not found"
		if r.FileFound {
			state = "loaded"
		}
		fmt.Fprintf(&b, "# Config file: %s (%s)\n", r.Path, state)
	}
	b.WriteString("\n")

	lines := make([][2]string, 0, len(Keys))
	width := 0
	for _, k := range Keys {
		val, set := r.value(k)
		line := k + " = " + val
		if !set {
			line = "# " + k + " is not set"
		}
		width = max(width, len(line))
		lines = append(lines, [2]string{line, "# " + string(r.Sources[k])})
	}
	for _, l := range lines {
		fmt.Fprintf(&b, "%-*s  %s\n", width, l[0], l[1])
	}
	return b.String()
}

func (r Resolved) value(k string) (string, bool) {
	c := r.Config
	switch k {
	case "duration":
		return quoteDuration(c.Duration), c.Duration > 0
	case "battery":
		return strconv.Itoa(c.Battery), c.Battery > 0
	case "active":
		return strconv.FormatBool(c.Active), true
	case "active_idle":
		return quoteDuration(c.ActiveIdle), true
	case "active_interval":
		return quoteDuration(c.ActiveInterval), true
	case "active_keys":
		return strconv.FormatBool(c.ActiveKeys), true
	case "schedule":
		return strconv.Quote(c.Schedule), c.Schedule != ""
	case "keep_display":
		return strconv.FormatBool(c.KeepDisplay), true
	case "notify":
		return strconv.FormatBool(c.Notify), true
	case "log":
		return strconv.FormatBool(c.Log), true
	case "log_file":
		return strconv.Quote(c.LogFile), c.LogFile != ""
	}
	return "", false
}

func quoteDuration(d time.Duration) string { return strconv.Quote(d.String()) }
