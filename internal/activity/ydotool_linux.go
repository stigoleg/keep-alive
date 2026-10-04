//go:build linux

package activity

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ydotoolStep coalesces moves: ydotool starts a process per move.
const ydotoolStep = 40 * time.Millisecond

const ydotoolDaemonHint = "start the ydotool daemon, e.g. systemctl --user enable --now ydotool"

const ydotoolSocketHint = "ydotoold runs as another user; run it as yours (systemctl --user enable --now ydotool) or make its socket writable for you"

// ydotool drives ydotoold's virtual device through the ydotool 1.x client.
// 0.1.x (Ubuntu 22.04/24.04, Debian 12) has an incompatible command line
// and is refused.
type ydotool struct {
	// ctx bounds the availability check; moves use their own timeout so an
	// interrupted burst can still be undone.
	ctx      context.Context
	run      cmdRunner
	lookPath func(string) (string, error)
	getenv   func(string) string
	dial     func(path string) error

	versionOK bool
	socket    string
}

func newYdotool(ctx context.Context) *ydotool {
	return &ydotool{ctx: ctx, run: runCmd, lookPath: exec.LookPath, getenv: os.Getenv, dial: dialYdotoold}
}

// dialYdotoold connects to the daemon the way the ydotool 1.x client does,
// with a datagram socket. A socket file left by a daemon that died refuses
// the connection; one owned by another user denies it.
func dialYdotoold(path string) error {
	c, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		return err
	}
	return c.Close()
}

func (y *ydotool) Name() string { return "ydotool" }

func (y *ydotool) MinStep() time.Duration { return ydotoolStep }

func (y *ydotool) Available() error {
	if _, err := y.lookPath("ydotool"); err != nil {
		return &Unavailable{Reason: "ydotool is not installed", Hint: "install ydotool 1.x and run its daemon (ydotoold)"}
	}
	if !y.versionOK {
		out, errOut, _ := y.run(y.ctx, cmdTimeout, nil, "ydotool", "help")
		switch ydotoolGeneration(out + "\n" + errOut) {
		case 1:
			y.versionOK = true
		case 0:
			return &Unavailable{
				Reason: "ydotool 0.1.x is not supported",
				Hint:   "install ydotool 1.x (the 0.1.8 package in Ubuntu and Debian is too old), or use uinput directly: " + uinputPermissionHint,
			}
		default:
			return &Unavailable{Reason: "cannot tell which ydotool version is installed", Hint: "install ydotool 1.x"}
		}
	}
	sock, candidates, err := ydotoolSocket(y.getenv, y.dial)
	switch {
	case sock != "":
		y.socket = sock
		return nil
	case errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM):
		return &Unavailable{Reason: "cannot connect to ydotoold at " + err.Error(), Hint: ydotoolSocketHint}
	case err != nil:
		return &Unavailable{Reason: "ydotoold is not running (cannot connect to " + err.Error() + ")", Hint: ydotoolDaemonHint}
	}
	return &Unavailable{
		Reason: "ydotoold is not running (no socket at " + strings.Join(candidates, ", ") + ")",
		Hint:   ydotoolDaemonHint,
	}
}

// ydotoolGeneration reads `ydotool help`: 0.1.x lists recorder, 1.x lists
// bakers/debug and mentions YDOTOOL_SOCKET. -1 when it is neither.
func ydotoolGeneration(help string) int {
	switch {
	case strings.Contains(help, "recorder") || strings.Contains(help, "mousemove_relative"):
		return 0
	case strings.Contains(help, "mousemove"):
		return 1
	}
	return -1
}

// ydotoolSocket finds a daemon that accepts connections where the 1.x
// client looks for it: $YDOTOOL_SOCKET, $XDG_RUNTIME_DIR/.ydotool_socket,
// /tmp/.ydotool_socket. Without one, err is the first socket that exists
// but refused ("path: reason"); nil when none exists.
func ydotoolSocket(getenv func(string) string, dial func(string) error) (sock string, candidates []string, err error) {
	if s := getenv("YDOTOOL_SOCKET"); s != "" {
		candidates = append(candidates, s)
	}
	if dir := getenv("XDG_RUNTIME_DIR"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, ".ydotool_socket"))
	}
	candidates = append(candidates, "/tmp/.ydotool_socket")
	for _, c := range candidates {
		derr := dial(c)
		if derr == nil {
			return c, candidates, nil
		}
		if err == nil && !errors.Is(derr, os.ErrNotExist) {
			var errno syscall.Errno
			if errors.As(derr, &errno) {
				derr = errno
			}
			err = &socketError{path: c, err: derr}
		}
	}
	return "", candidates, err
}

// socketError is a daemon socket that exists but cannot be used.
type socketError struct {
	path string
	err  error
}

func (e *socketError) Error() string { return e.path + ": " + e.err.Error() }
func (e *socketError) Unwrap() error { return e.err }

func (y *ydotool) env() []string { return []string{"YDOTOOL_SOCKET=" + y.socket} }

func (y *ydotool) MoveBy(dx, dy int) error {
	_, _, err := y.run(context.Background(), cmdTimeout, y.env(), "ydotool", "mousemove",
		"-x", strconv.Itoa(dx), "-y", strconv.Itoa(dy))
	return err
}

func (y *ydotool) Tap() error {
	_, _, err := y.run(context.Background(), cmdTimeout, y.env(), "ydotool", "key", "54:1", "54:0")
	return err
}

func (y *ydotool) Diagnose() (string, string) {
	if sock, _, _ := ydotoolSocket(y.getenv, y.dial); sock == "" {
		return "ydotoold stopped", ydotoolDaemonHint
	}
	return "ydotool input did not reset the idle timer",
		"check that ydotoold runs as your user and that the desktop accepts its virtual device"
}

func (y *ydotool) Close() error { return nil }
