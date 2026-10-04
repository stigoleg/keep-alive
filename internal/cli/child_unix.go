//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// runSignals are handled by `keepalive run`.
func runSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP}
}

// commandSignals passes keepalive run's signals on to the command so that
// each arrives once.
//
// In a terminal the command shares keepalive's process group, which it
// needs to read from the terminal. Ctrl+C and Ctrl+\ then reach both of
// them when that group is the terminal's foreground group, so SIGINT and
// SIGQUIT are only forwarded when it is not. SIGTERM and SIGHUP are always
// forwarded; one sent to the whole group (kill -TERM -<pgid>) reaches the
// command twice.
//
// Without a controlling terminal (CI, a supervisor, nohup) nothing needs
// the shared group, so the command gets its own and every signal is
// forwarded to that group exactly once, also one sent to keepalive's group.
type commandSignals struct {
	ownGroup bool // the command leads its own process group
}

// newCommandSignals prepares cmd before it starts.
func newCommandSignals(cmd *exec.Cmd) *commandSignals {
	c := &commandSignals{}
	if f, err := os.Open("/dev/tty"); err == nil {
		f.Close()
	} else {
		c.ownGroup = true
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	return c
}

// forward passes sig on to the started command p, unless it already got it.
func (c *commandSignals) forward(p *os.Process, sig os.Signal) bool {
	if c.ownGroup {
		if s, ok := sig.(syscall.Signal); ok {
			return syscall.Kill(-p.Pid, s) == nil
		}
	}
	if (sig == syscall.SIGINT || sig == syscall.SIGQUIT) && terminalForeground() {
		return false // the terminal sent it to the command too
	}
	return p.Signal(sig) == nil
}

// terminalForeground reports whether keepalive's process group is the
// foreground group of its controlling terminal.
func terminalForeground() bool {
	f, err := os.Open("/dev/tty")
	if err != nil {
		return false
	}
	defer f.Close()
	pgrp, err := unix.IoctlGetInt(int(f.Fd()), unix.TIOCGPGRP)
	return err == nil && pgrp == syscall.Getpgrp()
}

func childExitCode(ps *os.ProcessState) int {
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ps.ExitCode()
}

func signalExitCode(sig os.Signal) int {
	if s, ok := sig.(syscall.Signal); ok {
		return 128 + int(s)
	}
	return 1
}
