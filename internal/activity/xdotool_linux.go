//go:build linux

package activity

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// xdotool moves the pointer with XTest, which resets the X11 screensaver
// idle counter. A whole burst runs as one chained xdotool process.
type xdotool struct {
	run      cmdRunner
	lookPath func(string) (string, error)
	env      linuxEnv
}

func newXdotool(env linuxEnv) *xdotool {
	return &xdotool{run: runCmd, lookPath: exec.LookPath, env: env}
}

func (x *xdotool) Name() string { return "xdotool" }

func (x *xdotool) Available() error {
	if _, err := x.lookPath("xdotool"); err != nil {
		return &Unavailable{Reason: "xdotool is not installed", Hint: "install xdotool"}
	}
	if x.env.wayland != "" {
		return &Unavailable{Reason: "xdotool cannot move the pointer on Wayland", Hint: uinputPermissionHint}
	}
	if x.env.display == "" {
		return &Unavailable{Reason: "no X11 display (DISPLAY is not set)"}
	}
	if _, _, ok := x.Position(); !ok {
		return &Unavailable{Reason: "xdotool cannot reach the X server on " + x.env.display}
	}
	return nil
}

func (x *xdotool) xdo(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	out, _, err := x.run(ctx, timeout, nil, "xdotool", args...)
	return out, err
}

func (x *xdotool) Position() (float64, float64, bool) {
	out, err := x.xdo(context.Background(), cmdTimeout, "getmouselocation", "--shell")
	if err != nil {
		return 0, 0, false
	}
	return parseMouseLocation(out)
}

// parseMouseLocation reads `xdotool getmouselocation --shell` (X=, Y=, ...).
func parseMouseLocation(out string) (float64, float64, bool) {
	var x, y float64
	var gotX, gotY bool
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			continue
		}
		switch k {
		case "X":
			x, gotX = float64(n), true
		case "Y":
			y, gotY = float64(n), true
		}
	}
	return x, y, gotX && gotY
}

func (x *xdotool) Bounds() (Rect, bool) {
	out, err := x.xdo(context.Background(), cmdTimeout, "getdisplaygeometry")
	if err != nil {
		return Rect{}, false
	}
	return parseDisplayGeometry(out)
}

// parseDisplayGeometry reads `xdotool getdisplaygeometry` ("1920 1080").
func parseDisplayGeometry(out string) (Rect, bool) {
	f := strings.Fields(out)
	if len(f) != 2 {
		return Rect{}, false
	}
	w, err1 := strconv.Atoi(f[0])
	h, err2 := strconv.Atoi(f[1])
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return Rect{}, false
	}
	return Rect{W: float64(w), H: float64(h)}, true
}

func (x *xdotool) MoveTo(px, py float64) error {
	_, err := x.xdo(context.Background(), cmdTimeout, "mousemove", "--",
		strconv.Itoa(int(math.Round(px))), strconv.Itoa(int(math.Round(py))))
	return err
}

// Play runs the burst as one chained process; an interrupted or failed
// chain still puts the pointer back.
func (x *xdotool) Play(ctx context.Context, ox, oy float64, p Path) error {
	_, err := x.xdo(ctx, p.Duration()+cmdTimeout, xdotoolChain(ox, oy, p)...)
	if err != nil {
		_ = x.MoveTo(ox, oy)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("xdotool burst: %w", err)
	}
	return nil
}

// xdotoolChain builds "sleep S mousemove -- X Y ..." for the burst. Steps
// that round to the same pixel are merged; the last move (back to the
// origin) is always sent.
func xdotoolChain(ox, oy float64, p Path) []string {
	var args []string
	px, py := int(math.Round(ox)), int(math.Round(oy))
	var pending time.Duration
	for i, s := range p {
		pending += s.Delay
		x, y := int(math.Round(ox+s.X)), int(math.Round(oy+s.Y))
		if x == px && y == py && i < len(p)-1 {
			continue
		}
		args = append(args, "sleep", strconv.FormatFloat(pending.Seconds(), 'f', 3, 64),
			"mousemove", "--", strconv.Itoa(x), strconv.Itoa(y))
		px, py, pending = x, y, 0
	}
	return args
}

func (x *xdotool) Tap() error {
	_, err := x.xdo(context.Background(), cmdTimeout, "key", "Shift_R")
	return err
}

func (x *xdotool) Diagnose() (string, string) {
	return "xdotool input did not reset the X11 idle timer",
		"under XWayland xdotool only reaches X11 apps; use uinput instead: " + uinputPermissionHint
}

func (x *xdotool) Close() error { return nil }
