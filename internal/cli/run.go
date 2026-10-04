package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
	"github.com/stigoleg/keep-alive/v2/internal/cli/output"
	"github.com/stigoleg/keep-alive/v2/internal/clock"
	"github.com/stigoleg/keep-alive/v2/internal/ipc"
	"github.com/stigoleg/keep-alive/v2/internal/logging"
	"github.com/stigoleg/keep-alive/v2/internal/power"
	"github.com/stigoleg/keep-alive/v2/internal/session"
	"github.com/stigoleg/keep-alive/v2/internal/tui"
)

// execute runs a planned session in the TUI or headless.
func (a *App) execute(ctx context.Context, p *Plan) error {
	closeLog, err := a.startLogging(p)
	if err != nil {
		return err
	}
	defer closeLog()

	ctx, stop := signal.NotifyContext(ctx, stopSignals()...)
	defer stop()

	if p.TUI {
		return a.runTUI(ctx, p, a.deps(p))
	}
	srv, err := a.claimInstance(ctx, p)
	if err != nil {
		return err
	}
	return a.runHeadless(ctx, p, a.deps(p), srv)
}

// startLogging sets up the log file the plan asks for and says where it is.
func (a *App) startLogging(p *Plan) (func() error, error) {
	logPath, closeLog, err := a.logSetup(p.Logging)
	if err != nil {
		return nil, runtimeErr(fmt.Errorf("open log file: %w", err), "pass --log-file to choose another location")
	}
	if logPath != "" && p.Origin != ipc.OriginService {
		fmt.Fprintf(a.Stderr, "keepalive: logging to %s\n", logPath)
	}
	slog.Info("keepalive starting", "version", a.Version, "origin", p.Origin, "tui", p.TUI, "json", p.JSON,
		"notify", p.Notify, "config", p.Config.Path)
	return closeLog, nil
}

// deps are the production session dependencies for p.
func (a *App) deps(p *Plan) session.Deps {
	d := session.Deps{
		Clock:     clock.Real(),
		Power:     a.newPower(),
		Activity:  activity.New(),
		Battery:   a.Battery,
		Processes: a.Processes,
	}
	if p.Notify {
		d.Notifier = a.notifier()
	}
	return d
}

// runHeadless runs the session and prints its events until it ends; srv
// (may be nil) controls it.
func (a *App) runHeadless(ctx context.Context, p *Plan, deps session.Deps, srv *ipc.Server) error {
	s := session.New(p.Session, deps)
	events, unsub := s.Subscribe()
	defer unsub()
	defer serve(ctx, srv, s)()

	var pr output.Printer = output.NewHuman(a.Stdout, a.StdoutTTY && a.colorAllowed())
	if p.JSON {
		pr = output.NewJSON(a.Stdout)
	}
	printed := make(chan struct{})
	go func() {
		defer close(printed)
		for ev := range events {
			if err := pr.Print(ev); err != nil {
				slog.Warn("cannot write event", "err", err)
			}
		}
	}()

	res := s.Run(ctx)
	<-printed
	return resultError(res)
}

// resultError maps a finished session to the command's error. User stop,
// signals and reached limits are normal ends. A power error's hint is shown.
func resultError(res session.Result) error {
	if res.Err != nil {
		return runtimeErr(res.Err, power.HintOf(res.Err))
	}
	return nil
}

func (a *App) colorAllowed() bool {
	v, ok := a.LookupEnv("NO_COLOR")
	return !ok || v == ""
}

// runTUI runs the interactive UI. Signals stop the running session (through
// ctx) and quit the program. When another keepalive is running, the UI
// attaches to it instead of failing (--replace takes over as before); the
// control socket follows whichever session the UI runs, and a stop request
// without one quits the UI.
func (a *App) runTUI(ctx context.Context, p *Plan, deps session.Deps) error {
	ctrl := &switchController{}
	inst := &tuiInstance{ctx: ctx, info: ipc.ServerInfo{Version: a.Version, Origin: p.Origin}, ctrl: ctrl, stderr: a.Stderr}
	defer inst.close()
	logPath := p.Logging.Path
	if logPath == "" {
		logPath, _ = logging.DefaultPath()
	}
	opts := tui.Options{
		Version:    a.Version,
		Context:    ctx,
		Deps:       deps,
		Base:       p.Session,
		Start:      p.AutoStart,
		OnSession:  ctrl.set,
		Renderer:   tui.NewRenderer(a.Stdout, a.colorAllowed()),
		Claim:      inst.claim,
		LogPath:    logPath,
		LogEnabled: p.Logging.Enabled,
		Battery:    a.Battery,
	}
	if p.Replace {
		srv, err := a.claimInstance(ctx, p)
		if err != nil {
			return err
		}
		inst.adopt(srv)
		opts.Claimed = true
	} else {
		other, err := inst.claim()
		opts.Attach, opts.Warning, opts.Claimed = other, err, other == nil && err == nil
	}
	model := tui.New(opts)

	prog := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithoutSignalHandler(),
		tea.WithInput(a.Stdin),
		tea.WithOutput(a.Stdout),
	)
	ctrl.mu.Lock()
	ctrl.quit = prog.Quit
	ctrl.mu.Unlock()
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-ctx.Done():
			prog.Quit()
		case <-finished:
		}
	}()

	final, runErr := prog.Run()
	stopErr := model.Shutdown()
	if fm, ok := final.(tui.Model); ok && fm.FinalMessage() != "" {
		fmt.Fprintf(a.Stdout, "keepalive: %s\n", fm.FinalMessage())
	}
	if runErr != nil {
		return runtimeErr(fmt.Errorf("terminal UI: %w", runErr), "use --plain to run without the UI")
	}
	if stopErr != nil {
		return runtimeErr(stopErr, "")
	}
	return nil
}
