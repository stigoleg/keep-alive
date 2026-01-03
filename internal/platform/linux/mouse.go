//go:build linux

package linux

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/stigoleg/keep-alive/internal/platform/patterns"
)

// GetIdleTime returns the system idle time on Linux using xprintidle.
// Note: xprintidle only works on X11, not Wayland.
func GetIdleTime() (time.Duration, error) {
	displayServer := DetectDisplayServer()
	if displayServer == DisplayServerWayland {
		return 0, fmt.Errorf("xprintidle does not work on Wayland (only X11)")
	}
	if !hasCommand("xprintidle") {
		return 0, fmt.Errorf("xprintidle not found")
	}
	out, err := runVerbose("xprintidle")
	if err != nil {
		return 0, err
	}
	millis, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse xprintidle output %q: %v", out, err)
	}
	return time.Duration(millis) * time.Millisecond, nil
}

// MouseMover defines an interface for executing mouse movements.
type MouseMover interface {
	Move(dx, dy int) error
	Name() string
}

// UinputMover implements MouseMover for uinput.
type UinputMover struct {
	Sim *UinputSimulator
}

func (u *UinputMover) Move(dx, dy int) error {
	return u.Sim.Move(int32(dx), int32(dy))
}

func (u *UinputMover) Name() string {
	return "uinput"
}

// CommandMover implements MouseMover for command-line tools.
type CommandMover struct {
	Cmd  string
	Args []string
}

func (c *CommandMover) Move(dx, dy int) error {
	args := append(c.Args, fmt.Sprintf("%d", dx), fmt.Sprintf("%d", dy))
	_, err := runVerbose(c.Cmd, args...)
	return err
}

func (c *CommandMover) Name() string {
	return c.Cmd
}

// ExecutePattern executes a mouse pattern using the provided mover.
func ExecutePattern(points []patterns.Point, mover MouseMover, patternGen *patterns.Generator) bool {
	if mover == nil || len(points) == 0 || patternGen == nil {
		return false
	}

	for i, pt := range points {
		dx := int(pt.X)
		dy := int(pt.Y)
		if err := mover.Move(dx, dy); err != nil {
			log.Printf("linux: %s move failed: %v", mover.Name(), err)
			return false
		}

		distance := patterns.SegmentDistance(points, i)
		delay := patternGen.MovementDelay(distance)
		time.Sleep(delay)

		if patternGen.ShouldPause() {
			time.Sleep(patternGen.PauseDelay())
		}

		if patternGen.ShouldAddIntermediate(points, i, distance) {
			midPt, midDelay := patternGen.IntermediatePoint(points, i, delay)
			if err := mover.Move(int(midPt.X), int(midPt.Y)); err != nil {
				log.Printf("linux: %s move failed: %v", mover.Name(), err)
				return false
			}
			time.Sleep(midDelay)
		}
	}

	// Return to origin
	lastPt := points[len(points)-1]
	returnDelay := patternGen.ReturnDelay()
	if err := mover.Move(-int(lastPt.X), -int(lastPt.Y)); err != nil {
		log.Printf("linux: %s move failed: %v", mover.Name(), err)
		return false
	}
	time.Sleep(returnDelay)
	return true
}

// SimulateSystemActivity uses DBus to simulate user activity.
func SimulateSystemActivity() {
	displayServer := DetectDisplayServer()
	runBestEffort("dbus-send", "--dest=org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver", "org.freedesktop.ScreenSaver.SimulateUserActivity")
	runBestEffort("dbus-send", "--dest=org.gnome.ScreenSaver", "/org/gnome/ScreenSaver", "org.gnome.ScreenSaver.SimulateUserActivity")

	if displayServer == DisplayServerWayland {
		if hasCommand("loginctl") {
			runBestEffort("loginctl", "user-status")
		}
	}
}
