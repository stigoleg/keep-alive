package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/stigoleg/keep-alive/v2/internal/cli/output"
	"github.com/stigoleg/keep-alive/v2/internal/logging"
	"github.com/stigoleg/keep-alive/v2/internal/platform"
	"github.com/stigoleg/keep-alive/v2/internal/power"
	"github.com/stigoleg/keep-alive/v2/internal/session"
	"github.com/stigoleg/keep-alive/v2/internal/tui"
)

// execute runs a planned session in the TUI or headless.
func (a *App) execute(ctx context.Context, p *Plan) error {
	logPath, closeLog, err := logging.Setup(p.Logging)
	if err != nil {
		return runtimeErr(fmt.Errorf("open log file: %w", err), "pass --log-file to choose another location")
	}
	defer closeLog()
	if logPath != "" {
		fmt.Fprintf(a.Stderr, "keepalive: logging to %s\n", logPath)
	}
	slog.Info("keepalive starting", "version", a.Version, "tui", p.TUI, "json", p.JSON, "config", p.Config.Path)
	if dep := platform.GetDependencyMessage(); dep != "" {
		slog.Warn("missing dependencies", "detail", dep)
	}

	ctx, stop := signal.NotifyContext(ctx, stopSignals()...)
	defer stop()

	if p.TUI {
		return a.runTUI(ctx, p, realDeps())
	}
	return a.runHeadless(ctx, p, realDeps())
}

// runHeadless runs the session and prints its events until it ends.
func (a *App) runHeadless(ctx context.Context, p *Plan, deps session.Deps) error {
	s := session.New(p.Session, deps)
	events, unsub := s.Subscribe()
	defer unsub()

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
// ctx) and quit the program.
func (a *App) runTUI(ctx context.Context, p *Plan, deps session.Deps) error {
	model := tui.New(tui.Options{
		Version: a.Version,
		Context: ctx,
		Deps:    deps,
		Base:    p.Session,
		Start:   p.AutoStart,
	})
	if dep := platform.GetDependencyMessage(); dep != "" {
		model.SetDependencyWarning(dep)
	}
	if p.Session.Active {
		if st := platform.GetActivitySimulationStatus(); !st.Available {
			model.SetActivityWarning(st.Message)
		}
	}

	prog := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithoutSignalHandler(),
		tea.WithInput(a.Stdin),
		tea.WithOutput(a.Stdout),
	)
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-ctx.Done():
			prog.Quit()
		case <-finished:
		}
	}()

	_, runErr := prog.Run()
	stopErr := model.Shutdown()
	if runErr != nil {
		return runtimeErr(fmt.Errorf("terminal UI: %w", runErr), "use --plain to run without the UI")
	}
	if stopErr != nil {
		return runtimeErr(stopErr, "")
	}
	return nil
}
