package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/cli/output"
	"github.com/stigoleg/keep-alive/v2/internal/ipc"
	"github.com/stigoleg/keep-alive/v2/internal/logging"
	"github.com/stigoleg/keep-alive/v2/internal/notify"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/power"
	"github.com/stigoleg/keep-alive/v2/internal/service"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

// Check results.
const (
	checkOK   = "ok"
	checkWarn = "warn"
	checkFail = "fail"
)

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix"`
}

type doctorSection struct {
	Title  string        `json:"title"`
	Checks []doctorCheck `json:"checks"`
}

type doctorReport struct {
	Sections []doctorSection `json:"sections"`
}

func (r *doctorReport) section(title string) *doctorSection {
	r.Sections = append(r.Sections, doctorSection{Title: title, Checks: []doctorCheck{}})
	return &r.Sections[len(r.Sections)-1]
}

func (s *doctorSection) add(name, status, detail, fix string) {
	s.Checks = append(s.Checks, doctorCheck{Name: name, Status: status, Detail: detail, Fix: fix})
}

func (r doctorReport) failed() bool {
	for _, s := range r.Sections {
		for _, c := range s.Checks {
			if c.Status == checkFail {
				return true
			}
		}
	}
	return false
}

// doctorFacts is everything doctor looks at; tests fill it with fakes.
type doctorFacts struct {
	Version, OS, Arch string
	Cgo               bool
	ConfigPath        string
	ConfigFound       bool
	ConfigErr         error
	LogEnabled        bool
	LogPath           string

	Mechanisms []power.Mechanism
	Activity   activity.Diagnostics
	Battery    platform.BatteryStatus
	BatteryErr error
	NotifyOK   bool
	NotifyHow  string

	ServiceName  string
	Service      service.State
	ServiceErr   error
	Instance     *ipc.Status
	InstanceErr  error
	InstanceTime time.Time // now, for "running since"
}

// collectDoctor gathers the facts from the real system. Nothing here moves
// the pointer or changes anything.
func (a *App) collectDoctor(cmd *cobra.Command) doctorFacts {
	f := doctorFacts{Version: a.Version, OS: runtime.GOOS, Arch: runtime.GOARCH, Cgo: cgoEnabled(), InstanceTime: a.Now()}
	res, err := a.resolveConfig(cmd)
	f.ConfigPath, f.ConfigFound, f.ConfigErr = res.Path, res.FileFound, err
	f.LogEnabled, f.LogPath = res.Log, res.LogFile
	if f.LogPath == "" {
		f.LogPath, _ = logging.DefaultPath()
	}
	f.Mechanisms = power.Mechanisms()
	f.Activity = activity.Diagnose()
	f.Battery, f.BatteryErr = a.Battery()
	f.NotifyOK, f.NotifyHow = notify.Available()
	if m, err := a.ServiceManager(); err != nil {
		f.ServiceErr = err
	} else {
		f.ServiceName = m.Name()
		f.Service, f.ServiceErr = m.Status()
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
	defer cancel()
	if c, err := ipc.Dial(ctx); err != nil {
		f.InstanceErr = err
	} else if st, err := c.Status(ctx); err != nil {
		f.InstanceErr = err
	} else {
		f.Instance = &st
	}
	return f
}

func cgoEnabled() bool {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "CGO_ENABLED" {
				return s.Value == "1"
			}
		}
	}
	return false
}

// buildDoctor turns the facts into the report.
func buildDoctor(f doctorFacts) doctorReport {
	var r doctorReport

	s := r.section("keepalive")
	cgo := "cgo off"
	if f.Cgo {
		cgo = "cgo on"
	}
	build := fmt.Sprintf("%s (%s/%s, %s)", f.Version, f.OS, f.Arch, cgo)
	if f.OS == "darwin" && !f.Cgo {
		s.add("version", checkWarn, build+": activity simulation is not available and sleep prevention uses caffeinate",
			"install a release build, or build with CGO_ENABLED=1")
	} else {
		s.add("version", checkOK, build, "")
	}
	switch {
	case f.ConfigErr != nil:
		s.add("config", checkFail, f.ConfigErr.Error(), `fix the value, or start over with "keepalive config init --force"`)
	case f.ConfigFound:
		s.add("config", checkOK, f.ConfigPath, "")
	default:
		s.add("config", checkOK, f.ConfigPath+" (not found; using defaults)", "")
	}
	if f.LogEnabled {
		s.add("log", checkOK, f.LogPath, "")
	} else {
		s.add("log", checkOK, "off (turn on with --log); file: "+f.LogPath, "")
	}

	s = r.section("Sleep prevention")
	var unused []string
	usable := 0
	for _, m := range f.Mechanisms {
		if m.Available {
			usable++
		}
	}
	for _, m := range f.Mechanisms {
		switch {
		case m.Available:
			s.add(m.Name, checkOK, orDefault(m.Detail, "available"), "")
		case usable > 0:
			unused = append(unused, m.Name)
		default:
			s.add(m.Name, checkFail, orDefault(m.Detail, "not available"), powerFix(f.OS))
		}
	}
	if len(f.Mechanisms) == 0 {
		s.add("sleep prevention", checkFail, "not supported on "+f.OS, powerFix(f.OS))
	}
	if len(unused) > 0 {
		s.add("not needed", checkOK, "not available here: "+strings.Join(unused, ", "), "")
	}

	activitySection(r.section("Activity simulation"), f.Activity)

	s = r.section("Battery")
	if f.BatteryErr == nil && f.Battery.Available {
		s.add("battery", checkOK, fmt.Sprintf("%d%%", f.Battery.Percentage), "")
	} else {
		why := "not found"
		if f.BatteryErr != nil {
			why = f.BatteryErr.Error()
		}
		s.add("battery", checkWarn, "no battery ("+why+")", "--battery only works on a machine with a battery; nothing to do on a desktop")
	}

	s = r.section("Notifications")
	if f.NotifyOK {
		detail := f.NotifyHow
		if f.OS == "darwin" {
			detail += `; they show up as "Script Editor" in System Settings → Notifications`
		}
		s.add("notifications", checkOK, detail, "")
	} else {
		s.add("notifications", checkWarn, f.NotifyHow, notifyFix(f.OS))
	}

	s = r.section("Background service")
	switch {
	case errors.Is(f.ServiceErr, errors.ErrUnsupported):
		s.add("service", checkWarn, "not supported on "+f.OS, "run keepalive from your session's autostart instead")
	case f.ServiceErr != nil:
		s.add("service", checkWarn, f.ServiceErr.Error(), service.Hint(f.ServiceErr))
	case !f.Service.Installed:
		s.add(f.ServiceName, checkOK, `not installed (optional: "keepalive service install")`, "")
	case f.Service.Running:
		s.add(f.ServiceName, checkOK, joinDetail(f.Service.Detail, f.Service.Path), "")
	case f.ServiceName == "XDG autostart":
		s.add(f.ServiceName, checkOK, joinDetail(f.Service.Detail, f.Service.Path), "")
	default:
		s.add(f.ServiceName, checkWarn, joinDetail(f.Service.Detail, f.Service.Path),
			`reinstall it with "keepalive service install", or read its log`)
	}

	s = r.section("Running instance")
	switch {
	case f.Instance != nil:
		instanceCheck(s, *f.Instance, f.InstanceTime)
	case errors.Is(f.InstanceErr, ipc.ErrNotRunning):
		s.add("instance", checkOK, "none running", "")
	default:
		s.add("instance", checkWarn, fmt.Sprintf("cannot reach the control socket: %v", f.InstanceErr),
			"set KEEPALIVE_RUNTIME_DIR to a short, writable directory")
	}
	return r
}

func activitySection(s *doctorSection, d activity.Diagnostics) {
	usable := d.Usable()
	var unused []string
	for _, c := range d.Injectors {
		switch {
		case c.Available:
			s.add(c.Name, checkOK, orDefault(c.Detail, "available"), "")
		case usable:
			unused = append(unused, c.Name+" ("+c.Reason+")")
		default:
			s.add(c.Name, checkWarn, c.Reason, orDefault(c.Hint, "activity simulation (-a) cannot work on this system"))
		}
	}
	if len(unused) > 0 {
		s.add("not needed", checkOK, "not available here: "+strings.Join(unused, "; "), "")
	}
	var parts []string
	for _, i := range d.Idle {
		parts = append(parts, fmt.Sprintf("%s %s", i.Name, output.FormatDuration(i.Idle.Truncate(time.Second))))
	}
	if d.Gate == "" {
		// Counters that answer but cannot be trusted (XWayland on Wayland)
		// are listed for information.
		detail := "no idle source; activity is simulated on a fixed schedule"
		if len(parts) > 0 {
			detail += " (read for information only: " + strings.Join(parts, ", ") + ")"
		}
		s.add("idle time", checkWarn, detail,
			orDefault(d.NoIdleHint, "simulation still works, but also while you use the computer"))
	} else {
		detail := "idle: " + strings.Join(parts, ", ")
		if d.Verifier != "" {
			detail += "; bursts are checked against " + d.Verifier + " (what Teams/Slack read)"
		}
		s.add("idle time", checkOK, detail, "")
	}
	switch {
	case d.Lock.Method == "":
		s.add("screen lock", checkWarn, "cannot tell when the screen is locked",
			"activity keeps running while the screen is locked; on Linux this needs logind or a desktop screensaver (GNOME, KDE, MATE, Cinnamon, XFCE)")
	case !d.Lock.Known:
		s.add("screen lock", checkWarn, d.Lock.Method+": "+orDefault(d.Lock.Err, "state unknown"),
			"activity may keep running while the screen is locked")
	default:
		state := "unlocked now"
		if d.Lock.Locked {
			state = "locked now"
		}
		s.add("screen lock", checkOK, d.Lock.Method+" ("+state+")", "")
	}
	for _, e := range d.Environment {
		name, value, ok := strings.Cut(e, ": ")
		if !ok {
			name, value = "desktop", e
		}
		s.add(name, checkOK, value, "")
	}
	for _, n := range d.Notes {
		name, value, ok := strings.Cut(n, ": ")
		if !ok {
			name, value = "note", n
		}
		s.add(name, checkWarn, value, "")
	}
}

func instanceCheck(s *doctorSection, st ipc.Status, now time.Time) {
	snap := st.Snapshot
	if !snap.Running {
		s.add("instance", checkOK, fmt.Sprintf("the interactive UI is open, no session (pid %d)", st.PID), "")
		return
	}
	detail := fmt.Sprintf("pid %d", st.PID)
	if snap.StartedAt != nil {
		if t, err := time.Parse(time.RFC3339Nano, *snap.StartedAt); err == nil {
			detail = fmt.Sprintf("running since %s (%s, pid %d)", session.ClockText(now.Local(), t.Local()), orDefault(st.Origin, "terminal"), st.PID)
		}
	}
	if snap.Active {
		detail += ", active: " + output.ActivityText(activity.Status{
			State: activity.State(snap.Activity.State), Method: snap.Activity.Method, Reason: snap.Activity.Reason,
		}, false)
	}
	if snap.Activity.State == string(activity.StateDegraded) {
		s.add("instance", checkWarn, detail, orDefault(snap.Activity.Hint, "see the activity section above"))
		return
	}
	if snap.Activity.State == string(activity.StateSimulating) && snap.Activity.Hint != "" {
		// Input works, but not for everything (e.g. apps under XWayland).
		s.add("instance", checkWarn, detail, snap.Activity.Hint)
		return
	}
	s.add("instance", checkOK, detail, "")
}

func powerFix(goos string) string {
	switch goos {
	case "linux":
		return "keepalive needs systemd-logind on the system bus or a desktop session (GNOME, KDE, XFCE) on the session bus"
	case "darwin":
		return "check `pmset -g assertions`"
	case "windows":
		return "check `powercfg /requests`"
	}
	return "keepalive cannot keep this operating system awake"
}

func notifyFix(goos string) string {
	if goos == "linux" {
		return "notifications are optional; install a notification daemon (most desktops have one) or notify-send"
	}
	return "notifications are optional; keepalive works without them"
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func joinDetail(a, b string) string {
	if b == "" {
		return a
	}
	return a + "; " + b
}

// renderDoctor prints the report: ✓/!/✗ on a UTF-8 terminal, ok/warn/FAIL
// otherwise.
func renderDoctor(w io.Writer, r doctorReport, unicode, color bool) {
	width := 0
	for _, s := range r.Sections {
		for _, c := range s.Checks {
			width = max(width, len(c.Name))
		}
	}
	for i, s := range r.Sections {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, s.Title)
		for _, c := range s.Checks {
			mark, pad := doctorMark(c.Status, unicode, color)
			fmt.Fprintf(w, "  %s %-*s  %s\n", mark, width, c.Name, c.Detail)
			if c.Fix != "" && c.Status != checkOK {
				fmt.Fprintf(w, "  %s %-*s  fix: %s\n", strings.Repeat(" ", pad), width, "", c.Fix)
			}
		}
	}
}

// doctorMark returns the status mark and its visible width.
func doctorMark(status string, unicode, color bool) (string, int) {
	if !unicode {
		switch status {
		case checkOK:
			return "ok  ", 4
		case checkWarn:
			return "warn", 4
		}
		return "FAIL", 4
	}
	mark, c := "✓", "\x1b[32m"
	switch status {
	case checkWarn:
		mark, c = "!", "\x1b[33m"
	case checkFail:
		mark, c = "✗", "\x1b[31m"
	}
	if color {
		return c + mark + "\x1b[0m", 1
	}
	return mark, 1
}

// unicodeTerminal reports a UTF-8 terminal without NO_COLOR.
func (a *App) unicodeTerminal() bool {
	if !a.StdoutTTY || !a.colorAllowed() {
		return false
	}
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v, ok := a.LookupEnv(k); ok && v != "" {
			v = strings.ToLower(v)
			return strings.Contains(v, "utf-8") || strings.Contains(v, "utf8")
		}
	}
	return runtime.GOOS == "windows"
}

// probeDelay is the pause before doctor --probe moves the pointer.
const probeDelay = 3 * time.Second

func (a *App) doctorCommand() *cobra.Command {
	var probe, asJSON, yes bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check what keepalive can do on this machine",
		Long: `Check everything keepalive depends on: how it keeps the machine awake, how it
simulates activity (input method, permissions, idle time, screen lock), the
battery, notifications, the login service and a running instance. Each
problem comes with a fix.

--probe also moves the mouse pointer once and checks that the idle timer
Teams and Slack read was reset. It needs a terminal, or --yes to confirm.

Exits with status 1 when a check fails.`,
		Example: `  keepalive doctor
  keepalive doctor --probe
  keepalive doctor --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if probe && !yes && !a.StdinTTY {
				return usageErr(errors.New("doctor --probe moves the mouse pointer and needs a terminal"),
					"run it in a terminal, or pass --yes to confirm")
			}
			report := buildDoctor(a.doctorFacts(cmd))
			if probe {
				ctx, stop := signal.NotifyContext(cmd.Context(), stopSignals()...)
				defer stop()
				fmt.Fprintln(a.Stderr, "moving the mouse in 3 seconds to verify activity simulation… (Ctrl+C to cancel)")
				select {
				case <-ctx.Done():
					return &ExitError{Code: ExitFailure, Err: errors.New("probe cancelled")}
				case <-time.After(a.probeWait):
				}
				probeSection(&report, a.probe(ctx))
			}
			if asJSON {
				enc := json.NewEncoder(a.Stdout)
				enc.SetEscapeHTML(false)
				enc.SetIndent("", "  ")
				if err := enc.Encode(report); err != nil {
					return runtimeErr(err, "")
				}
			} else {
				renderDoctor(a.Stdout, report, a.unicodeTerminal(), a.unicodeTerminal())
			}
			if report.failed() {
				return &ExitError{Code: ExitFailure}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&probe, "probe", false, "also move the pointer once to verify activity simulation")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm --probe without a terminal")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the report as one JSON document")
	return cmd
}

// probeSection adds the result of one verified burst.
func probeSection(r *doctorReport, p activity.ProbeResult) {
	s := r.section("Activity probe")
	if p.Method == "" {
		s.add("input", checkFail, orDefault(p.Reason, "no input method works"), p.Hint)
		return
	}
	s.add("input", checkOK, fmt.Sprintf("moved the pointer via %s in %s", p.Method, probeDuration(p.Burst)), "")
	if p.LockKnown && p.Locked {
		s.add("screen lock", checkWarn, "the screen is locked; the result may not apply to an unlocked session", "unlock the screen and probe again")
	}
	for _, rd := range p.Sources {
		if rd.Err != "" {
			s.add(rd.Source, checkWarn, rd.Err, "")
			continue
		}
		status := checkOK
		if rd.Note != "" {
			status = checkWarn
		}
		s.add(rd.Source, status, fmt.Sprintf("%s → %s", probeDuration(rd.Before), probeDuration(rd.After)), rd.Note)
	}
	switch {
	case p.Verifier == "":
		s.add("verdict", checkWarn, "moved the pointer, but this desktop exposes no idle time to verify it", "")
	case p.Effective:
		s.add("verdict", checkOK, "input resets the idle timer that Teams/Slack read", "")
	default:
		why := orDefault(p.Reason, "the idle timer did not change")
		s.add("verdict", checkFail, fmt.Sprintf("%s was not reset: %s", p.Verifier, why), p.Hint)
	}
}

// probeDuration is "210ms", "4.2s" or "38m12s".
func probeDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return d.Round(10 * time.Millisecond).String()
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return output.FormatDuration(d)
}
