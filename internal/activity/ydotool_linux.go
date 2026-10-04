//go:build linux

package activity

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ydotoolStep coalesces moves: ydotool starts a process per move.
const ydotoolStep = 40 * time.Millisecond

const ydotoolDaemonHint = "start the ydotool daemon, e.g. systemctl --user enable --now ydotool"

// ydotool drives ydotoold's virtual device through the ydotool 1.x client.
// 0.1.x (Ubuntu 22.04/24.04, Debian 12) has an incompatible command line
// and is refused.
type ydotool struct {
	run      cmdRunner
	lookPath func(string) (string, error)
	getenv   func(string) string
	isSocket func(string) bool

	versionOK bool
	socket    string
}

func newYdotool() *ydotool {
	return &ydotool{run: runCmd, lookPath: exec.LookPath, getenv: os.Getenv, isSocket: isSocket}
}

func isSocket(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode()&os.ModeSocket != 0
}

func (y *ydotool) Name() string { return "ydotool" }

func (y *ydotool) MinStep() time.Duration { return ydotoolStep }

func (y *ydotool) Available() error {
	if _, err := y.lookPath("ydotool"); err != nil {
		return &Unavailable{Reason: "ydotool is not installed", Hint: "install ydotool 1.x and run its daemon (ydotoold)"}
	}
	if !y.versionOK {
		out, errOut, _ := y.run(context.Background(), cmdTimeout, nil, "ydotool", "help")
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
	sock, candidates := ydotoolSocket(y.getenv, y.isSocket)
	if sock == "" {
		return &Unavailable{
			Reason: "ydotoold is not running (no socket at " + strings.Join(candidates, ", ") + ")",
			Hint:   ydotoolDaemonHint,
		}
	}
	y.socket = sock
	return nil
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

// ydotoolSocket finds the daemon socket where the 1.x client looks for it:
// $YDOTOOL_SOCKET, $XDG_RUNTIME_DIR/.ydotool_socket, /tmp/.ydotool_socket.
func ydotoolSocket(getenv func(string) string, isSocket func(string) bool) (string, []string) {
	var candidates []string
	if s := getenv("YDOTOOL_SOCKET"); s != "" {
		candidates = append(candidates, s)
	}
	if dir := getenv("XDG_RUNTIME_DIR"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, ".ydotool_socket"))
	}
	candidates = append(candidates, "/tmp/.ydotool_socket")
	for _, c := range candidates {
		if isSocket(c) {
			return c, candidates
		}
	}
	return "", candidates
}

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
	if sock, _ := ydotoolSocket(y.getenv, y.isSocket); sock == "" {
		return "ydotoold stopped", ydotoolDaemonHint
	}
	return "ydotool input did not reset the idle timer",
		"check that ydotoold runs as your user and that the desktop accepts its virtual device"
}

func (y *ydotool) Close() error { return nil }
