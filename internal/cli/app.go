// Package cli is the keepalive command line: the cobra command tree, the
// configuration precedence, mode selection (TUI or headless) and exit codes.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/clock"
	"github.com/stigoleg/keep-alive/v2/internal/config"
	"github.com/stigoleg/keep-alive/v2/internal/ipc"
	"github.com/stigoleg/keep-alive/v2/internal/logging"
	"github.com/stigoleg/keep-alive/v2/internal/notify"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/proc"
	"github.com/stigoleg/keep-alive/v2/internal/schedule"
	"github.com/stigoleg/keep-alive/v2/internal/session"
	"github.com/stigoleg/keep-alive/v2/internal/util"
)

// App is one invocation of the CLI. Its fields are the process environment,
// so tests can substitute every one of them.
type App struct {
	Version                        string
	Stdin                          io.Reader
	Stdout, Stderr                 io.Writer
	StdinTTY, StdoutTTY, StderrTTY bool
	LookupEnv                      func(string) (string, bool)
	Now                            func() time.Time
	Battery                        func() (platform.BatteryStatus, error)
	DefaultConfigPath              func() (string, error)
	// Processes checks --pid and --while targets during planning.
	Processes proc.Lister
	// Notifier reports a service that cannot start; nil means notify.New().
	Notifier notify.Notifier

	// logSetup is logging.Setup; tests replace it.
	logSetup func(logging.Options) (string, func() error, error)

	// runSession executes a resolved plan; tests replace it.
	runSession func(ctx context.Context, p *Plan) error
	// runChild executes `keepalive run`; tests replace it.
	runChild func(ctx context.Context, p *Plan, argv []string) error
}

// Plan is a fully resolved request to run a session.
type Plan struct {
	Session   session.Config
	TUI       bool // interactive UI instead of headless output
	JSON      bool // NDJSON events (headless only)
	AutoStart bool // TUI: start the session right away, as v1 did for -d/-c/-b
	// Notify shows desktop notifications for unusual stops and problems:
	// on by default except in the TUI; an explicit setting wins.
	Notify  bool
	Logging logging.Options
	Config  config.Resolved
	// Origin is who started this process: "terminal", "service" or "run".
	Origin string
	// Replace stops another running instance instead of failing.
	Replace bool
}

// originRun marks `keepalive run` in the control socket's status.
const originRun = "run"

// Main runs the CLI against the real process environment and returns the exit
// code.
func Main(version string) int {
	return NewApp(version).Execute(os.Args[1:])
}

// NewApp returns an App wired to the real process.
func NewApp(version string) *App {
	return &App{
		Version:           ResolveVersion(version),
		Stdin:             os.Stdin,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		StdinTTY:          isTerminal(os.Stdin),
		StdoutTTY:         isTerminal(os.Stdout),
		StderrTTY:         isTerminal(os.Stderr),
		LookupEnv:         os.LookupEnv,
		Now:               time.Now,
		Battery:           platform.GetBatteryStatus,
		DefaultConfigPath: config.DefaultPath,
		Processes:         session.SystemProcesses(),
	}
}

// NewRootCommand returns the command tree; cmd/gen-docs uses it for man
// pages and completions.
func NewRootCommand(version string) *cobra.Command {
	return NewApp(version).Command()
}

// Execute runs the command line and returns the exit code. Errors are printed
// as one "keepalive: error: …" line plus an optional hint.
func (a *App) Execute(args []string) int {
	cmd := a.Command()
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		printError(a.Stderr, err, a.color())
	}
	code := ExitCode(err)
	if code == ExitUsage && a.isServiceRun(cmd, args) {
		// The service manager restarts on failure; a broken config would
		// loop forever. Report it once and exit cleanly instead.
		a.reportServiceFailure(err)
		return ExitOK
	}
	return code
}

// isServiceRun reports whether the login service started this root command.
func (a *App) isServiceRun(root *cobra.Command, args []string) bool {
	if c, _, err := root.Find(args); err != nil || c != root {
		return false
	}
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--origin="+ipc.OriginService || (arg == "--origin" && i+1 < len(args) && args[i+1] == ipc.OriginService) {
			return true
		}
	}
	v, ok := a.LookupEnv(ipc.EnvOrigin)
	return ok && v == ipc.OriginService
}

// reportServiceFailure logs a service start failure and shows one
// notification; failures to do either are ignored.
func (a *App) reportServiceFailure(err error) {
	msg := err.Error()
	var ee *ExitError
	if errors.As(err, &ee) && ee.Hint != "" {
		msg += " (" + ee.Hint + ")"
	}
	if _, closeLog, lerr := a.logSetup(logging.Options{Enabled: true}); lerr == nil {
		slog.Error("service: could not start", "err", msg)
		_ = closeLog()
	}
	n := a.Notifier
	if n == nil {
		n = notify.New()
	}
	ctx, cancel := context.WithTimeout(context.Background(), notify.Timeout)
	defer cancel()
	if nerr := n.Notify(ctx, "Keep-Alive service could not start", msg); nerr != nil {
		slog.Debug("service: notification failed", "err", nerr)
	}
}

func (a *App) color() bool {
	if v, ok := a.LookupEnv("NO_COLOR"); ok && v != "" {
		return false
	}
	return a.StderrTTY
}

func isTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// sessionFlags holds the flag values that are not config keys; config keys
// are read back from the flag set by config.Resolve.
type sessionFlags struct {
	duration config.DurationValue
	clock    string
	pids     []int
	while    string
	json     bool
	plain    bool
	replace  bool
}

// addSessionFlags defines the session flags shared by the root command,
// `run`, `service install` and `config show`.
func addSessionFlags(fs *pflag.FlagSet, f *sessionFlags) {
	def := config.Defaults()
	fs.VarP(&f.duration, "duration", "d", `keep awake for this long: minutes ("150") or a duration ("2h30m", "45m"); at least 1m`)
	fs.StringVarP(&f.clock, "clock", "c", "", `keep awake until this time of day ("22:00", "10:00PM"); alias --until`)
	fs.IntP("battery", "b", 0, "stop when the battery is at or below this percentage (1-100)")
	fs.BoolP("active", "a", false, "simulate user activity so chat apps (Slack, Teams) keep you active")
	fs.Duration("active-idle", def.ActiveIdle, "idle time before activity is simulated (at least 10s)")
	fs.Duration("active-interval", def.ActiveInterval, "mean gap between simulated activity bursts (at least 5s)")
	fs.Bool("active-keys", false, "also send a harmless key press with each activity burst")
	fs.String("schedule", "", `only keep awake during these work hours, e.g. "Mon-Fri 08:00-16:00"`)
	fs.IntSliceVar(&f.pids, "pid", nil, "keep awake until these processes exit (repeatable)")
	fs.StringVar(&f.while, "while", "", `keep awake while a process with this name runs, e.g. "zoom"`)
	fs.Bool("keep-display", def.KeepDisplay, "keep the display on too (false keeps only the system awake)")
	fs.BoolVar(&f.json, "json", false, "print NDJSON events on stdout (implies --plain)")
	fs.BoolVar(&f.plain, "plain", false, "run headless even on a terminal")
	fs.Bool("notify", false, "show a desktop notification when the session ends (not applied yet)")
	fs.BoolP("log", "l", false, "write a debug log (the path is printed at start)")
	fs.String("log-file", "", "log file path (default: keepalive/keepalive.log in the user cache directory)")
	fs.SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		if name == "until" {
			name = "clock"
		}
		return pflag.NormalizedName(name)
	})
}

// configPath returns the --config path or the default one.
func (a *App) configPath(cmd *cobra.Command) (path string, explicit bool, err error) {
	if f := cmd.Flags().Lookup("config"); f != nil && f.Changed {
		return f.Value.String(), true, nil
	}
	path, err = a.DefaultConfigPath()
	return path, false, err
}

func (a *App) resolveConfig(cmd *cobra.Command) (config.Resolved, error) {
	path, explicit, err := a.configPath(cmd)
	if err != nil {
		return config.Resolved{}, runtimeErr(err, "pass --config to choose a file")
	}
	res, err := config.Resolve(config.Options{Path: path, PathExplicit: explicit, Env: a.LookupEnv, Flags: cmd.Flags()})
	if err != nil {
		hint := "check the value, or run 'keepalive config show'"
		if strings.Contains(err.Error(), "schedule") || strings.Contains(err.Error(), "window") {
			hint = `example: --schedule "Mon-Fri 08:00-16:00" (days Mon-Sun, weekdays, weekends or daily; 24-hour times)`
		}
		return res, usageErr(err, hint)
	}
	return res, nil
}

// origin reports who started this process: the hidden --origin flag (the
// login service passes it, since a Windows task cannot set the environment),
// else KEEPALIVE_ORIGIN.
func (a *App) origin(cmd *cobra.Command) (string, error) {
	if f := cmd.Flags().Lookup("origin"); f != nil && f.Changed {
		switch v := f.Value.String(); v {
		case ipc.OriginService, ipc.OriginTerminal:
			return v, nil
		default:
			return "", usageErr(fmt.Errorf("--origin: unknown origin %q", v), `use "service" or "terminal"`)
		}
	}
	if v, ok := a.LookupEnv(ipc.EnvOrigin); ok && v == ipc.OriginService {
		return ipc.OriginService, nil
	}
	return ipc.OriginTerminal, nil
}

// plan turns flags, env and config into a session request and validates it.
// forRun plans `keepalive run`: headless, never a TUI.
func (a *App) plan(cmd *cobra.Command, f *sessionFlags, forRun bool) (*Plan, error) {
	origin := originRun
	if !forRun {
		var err error
		if origin, err = a.origin(cmd); err != nil {
			return nil, err
		}
	}
	res, err := a.resolveConfig(cmd)
	if err != nil {
		return nil, err
	}
	fs := cmd.Flags()
	s := session.Config{
		Duration:         res.Duration,
		BatteryThreshold: res.Battery,
		Active:           res.Active,
		Activity:         activity.Config{IdleThreshold: res.ActiveIdle, Interval: res.ActiveInterval, Keys: res.ActiveKeys},
		KeepDisplay:      res.KeepDisplay,
		WatchPIDs:        f.pids,
		WatchProcess:     f.while,
	}
	if res.Schedule != "" {
		sched, err := schedule.Parse(res.Schedule)
		if err != nil { // config.Resolve validated it already
			return nil, usageErr(err, `example: --schedule "Mon-Fri 08:00-16:00"`)
		}
		s.Schedule = sched
	}
	if err := a.checkWatch(s.WatchPIDs, s.WatchProcess); err != nil {
		return nil, err
	}

	if fs.Changed("clock") {
		if fs.Changed("duration") {
			return nil, usageErr(errors.New("--duration and --clock cannot be used together"), "use one of them, e.g. -d 2h or -c 22:00")
		}
		until, err := util.NextClockTime(f.clock, a.Now())
		if err != nil {
			return nil, usageErr(err, "")
		}
		s.Until, s.Duration = until, 0 // an explicit --clock wins over a configured duration
	}

	if s.BatteryThreshold > 0 {
		if err := a.checkBattery(s.BatteryThreshold); err != nil {
			return nil, err
		}
	}

	p := &Plan{
		Session:   s,
		TUI:       a.StdinTTY && a.StdoutTTY && !f.plain && !f.json && origin == ipc.OriginTerminal,
		JSON:      f.json,
		AutoStart: s.Duration > 0 || !s.Until.IsZero() || s.BatteryThreshold > 0,
		Logging:   logging.Options{Enabled: res.Log, Debug: res.Log, Path: res.LogFile},
		Config:    res,
		Origin:    origin,
		Replace:   f.replace,
	}
	p.Notify = res.Notify
	if res.Sources["notify"] == config.SourceDefault {
		p.Notify = !p.TUI
	}
	if origin == ipc.OriginService && res.Sources["log"] == config.SourceDefault {
		p.Logging.Enabled, p.Logging.Debug = true, false // a service always keeps an info log
	}
	return p, nil
}

// checkWatch fails planning when nothing to watch is running, so a typo is
// a usage error instead of a session that ends at once.
func (a *App) checkWatch(pids []int, name string) error {
	if len(pids) == 0 && name == "" {
		return nil
	}
	for _, pid := range pids {
		if pid <= 0 {
			return usageErr(fmt.Errorf("--pid: invalid process ID %d", pid), "process IDs are positive whole numbers")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // only the start-up check; the session watches for real
	_, err := proc.NewWatcher(clock.Real(), session.WatchInterval, a.Processes).Watch(ctx, pids, name)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, proc.ErrNotRunning) && name != "":
		return usageErr(err, fmt.Sprintf("start the app first, or check the name with %q", processListCommand()))
	case errors.Is(err, proc.ErrNotRunning):
		return usageErr(err, fmt.Sprintf("check the process ID with %q", processListCommand()))
	}
	return runtimeErr(err, "")
}

func processListCommand() string {
	if runtime.GOOS == "windows" {
		return "tasklist"
	}
	return "ps"
}

func (a *App) checkBattery(threshold int) error {
	st, err := a.Battery()
	if err != nil || !st.Available {
		if err == nil {
			err = errors.New("not available")
		}
		return usageErr(fmt.Errorf("battery threshold %d%% set, but no battery was found (%v)", threshold, err),
			"--battery only works on machines with a battery; remove it (or the battery setting)")
	}
	if st.Percentage <= threshold {
		return usageErr(fmt.Errorf("battery threshold must be below the current level (current %d%%, threshold %d%%)", st.Percentage, threshold),
			"choose a lower --battery value")
	}
	return nil
}
