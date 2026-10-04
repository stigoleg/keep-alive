package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"github.com/stigoleg/keep-alive/v2/internal/cli/output"
	"github.com/stigoleg/keep-alive/v2/internal/power"
	"github.com/stigoleg/keep-alive/v2/internal/session"
)

// executeRun keeps the machine awake while argv runs. The child inherits
// stdin, stdout and stderr; keepalive's own few lines go to stderr. Signals
// are passed on to the child, and the session ends when it exits. The exit
// code is the child's (128+N when a signal killed it), also when the
// machine could not be kept awake.
func (a *App) executeRun(ctx context.Context, p *Plan, argv []string) error {
	closeLog, err := a.startLogging(p)
	if err != nil {
		return err
	}
	defer closeLog()
	if _, err := exec.LookPath(argv[0]); err != nil {
		return startError(argv[0], err) // before anything is held
	}

	// From here on signals are ours: before the command starts they cancel
	// the run, afterwards they are forwarded to it.
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, runSignals()...)
	defer signal.Stop(sigs)

	srv, err := a.claimInstance(ctx, p)
	if err != nil {
		return err
	}
	s := session.New(p.Session, a.deps(p))
	events, unsub := s.Subscribe()
	defer unsub()
	stopServing := serve(ctx, srv, s)
	var pr output.Printer = output.NewRun(a.Stderr, a.StderrTTY && a.colorAllowed())
	if p.JSON {
		pr = output.NewJSON(a.Stderr)
	}
	started := make(chan bool, 1) // true once the session runs, false if it ended first
	printed := make(chan struct{})
	go func() {
		defer close(printed)
		first := true
		for ev := range events {
			if first && (ev.Type == session.EventStarted || ev.Type == session.EventStopped) {
				started <- ev.Type == session.EventStarted
				first = false
			}
			if err := pr.Print(ev); err != nil {
				slog.Warn("cannot write event", "err", err)
			}
		}
		if first {
			started <- false
		}
	}()

	result := make(chan session.Result, 1)
	go func() {
		res := s.Run(context.WithoutCancel(ctx))
		// The session is over, though the command may still run: give up
		// the lock and the socket, so "keepalive stop" and "status" see no
		// instance and another keepalive can start.
		stopServing()
		result <- res
	}()
	var ended *session.Result
	finish := func(r session.Reason) session.Result { // may be called twice
		if ended == nil {
			s.Stop(r)
			res := <-result
			<-printed
			ended = &res
		}
		return *ended
	}

	select {
	case ok := <-started:
		if !ok {
			// Nothing keeps the machine awake, but the command matters
			// more: run it anyway and say so.
			res := finish(session.ReasonError)
			if res.Err != nil && !p.JSON { // --json printed the error event
				a.warnNoPower(res.Err)
			}
		}
	case sig := <-sigs:
		finish(session.ReasonSignal)
		return &ExitError{Code: signalExitCode(sig)}
	}

	child := exec.Command(argv[0], argv[1:]...)
	child.Stdin, child.Stdout, child.Stderr = a.Stdin, a.Stdout, a.Stderr
	csigs := newCommandSignals(child)
	if err := child.Start(); err != nil {
		finish(session.ReasonCommandExited)
		return startError(argv[0], err)
	}
	slog.Info("run: command started", "pid", child.Process.Pid, "argv", argv)

	forwarded := make(chan struct{})
	waited := make(chan struct{})
	go func() {
		defer close(forwarded)
		for {
			select {
			case sig := <-sigs:
				if csigs.forward(child.Process, sig) {
					slog.Debug("run: forwarded signal", "signal", sig)
				}
			case <-waited:
				return
			}
		}
	}()
	waitErr := child.Wait()
	close(waited)
	<-forwarded

	code := childExitCode(child.ProcessState)
	slog.Info("run: command exited", "code", code, "err", waitErr)
	if res := finish(session.ReasonCommandExited); res.Err != nil {
		slog.Warn("run: session ended with an error", "err", res.Err)
	}
	if code == 0 {
		return nil
	}
	return &ExitError{Code: code}
}

// warnNoPower reports a session that could not start, so the command runs
// without keeping the machine awake.
func (a *App) warnNoPower(err error) {
	msg := strings.TrimPrefix(err.Error(), "keep the system awake: ")
	hint := power.HintOf(err)
	if hint == "" {
		hint = `"keepalive doctor" shows what can keep this machine awake`
	}
	fmt.Fprintf(a.Stderr, "keepalive: warning: could not keep the system awake: %s\n", msg)
	fmt.Fprintf(a.Stderr, "hint: %s; the command runs anyway, but the machine may sleep\n", hint)
}

// startError maps a command that could not be started to the shell's exit
// codes: 127 not found, 126 not executable.
func startError(name string, err error) error {
	switch {
	case errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist):
		return &ExitError{Code: 127, Err: fmt.Errorf("command not found: %s", name), Hint: "check the spelling, or give the full path"}
	case errors.Is(err, fs.ErrPermission):
		return &ExitError{Code: 126, Err: fmt.Errorf("cannot run %s: permission denied", name), Hint: "check that the file is executable"}
	}
	return &ExitError{Code: 126, Err: fmt.Errorf("cannot run %s: %w", name, err)}
}
