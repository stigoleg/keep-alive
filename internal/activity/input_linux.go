//go:build linux

package activity

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

func linuxMonitor(env linuxEnv) func(context.Context) (inputMonitor, error) {
	if env.x11() {
		return openXInputMonitor
	}
	return openEvdevMonitor
}

// These sources carry synthetic input, not physical-device activity. Unknown
// virtual devices are deliberately NOT excluded (e.g. remote-control input).
func syntheticDevice(name string) bool {
	return name == uinputName || name == "ydotoold virtual device" ||
		name == "Virtual core XTEST pointer" || name == "Virtual core XTEST keyboard"
}

type xInputMonitor struct {
	*inputState
	cancel  context.CancelFunc
	done    chan struct{}
	pending atomic.Int64
}

func (*xInputMonitor) Name() string { return "XInput2 raw user input" }
func (m *xInputMonitor) Close() error {
	m.cancel()
	<-m.done
	return nil
}

var xInputDeviceLine = regexp.MustCompile(`^.*?([^\t]*?)\s+id=(\d+)\s+\[([^\]]+)\]`)
var xInputSourceLine = regexp.MustCompile(`^device:\s+(\d+)\s+\((\d+)\)$`)
var xInputEventLine = regexp.MustCompile(`^EVENT type [0-9]+ \([[:alnum:]_]+\)$`)
var xInputHeldLine = regexp.MustCompile(`^(key|button)\[(\d+)\]=(down|up)$`)

func parseXInputDevices(out string) (map[int]bool, error) {
	devices := make(map[int]bool)
	for _, line := range strings.Split(out, "\n") {
		match := xInputDeviceLine.FindStringSubmatch(line)
		if match == nil {
			if strings.TrimSpace(line) != "" {
				return nil, errors.New("cannot decode the XInput device list")
			}
			continue
		}
		if strings.Contains(match[3], "floating") {
			return nil, errors.New("floating XInput devices prevent complete user-input monitoring")
		}
		if !strings.HasPrefix(match[3], "slave ") {
			continue // master devices duplicate their source's events
		}
		id, err := strconv.Atoi(match[2])
		if err != nil {
			return nil, errors.New("invalid XInput device identity")
		}
		name := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(match[1]), "⎡⎜⎣↳ "))
		devices[id] = syntheticDevice(name)
	}
	if len(devices) == 0 {
		return nil, errors.New("no XInput source devices were found")
	}
	return devices, nil
}

type xInputParser struct {
	state   *inputState
	devices map[int]bool
	event   string
	source  int
	pending bool
}

// line consumes only event kind, source identity and transient held state.
// It never includes raw event text in an error or a log message.
func (p *xInputParser) line(line string) error {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "EVENT type ") {
		if !xInputEventLine.MatchString(line) {
			return errors.New("invalid XInput event header")
		}
		if p.pending {
			return errors.New("incomplete XInput raw event")
		}
		p.event, p.source = "", 0
		switch {
		case strings.Contains(line, "(HierarchyChanged)"):
			return errors.New("XInput devices changed; input coverage must be refreshed")
		case strings.Contains(line, "(Raw"):
			start, end := strings.IndexByte(line, '('), strings.IndexByte(line, ')')
			if start < 0 || end < start {
				return errors.New("invalid XInput event header")
			}
			p.event, p.pending = line[start+1:end], true
		}
		return nil
	}
	if !p.pending {
		return nil
	}
	if strings.HasPrefix(line, "device:") {
		match := xInputSourceLine.FindStringSubmatch(line)
		if match == nil {
			return errors.New("invalid XInput event source")
		}
		p.source, _ = strconv.Atoi(match[2])
		synthetic, known := p.devices[p.source]
		if !known {
			return errors.New("unknown XInput source; input coverage must be refreshed")
		}
		if synthetic {
			p.pending = false
		} else if p.event == "RawMotion" {
			p.state.input(0, false)
			p.pending = false
		}
		return nil
	}
	if strings.HasPrefix(line, "detail:") {
		if p.source == 0 {
			return errors.New("XInput event has no source")
		}
		detail, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "detail:")), 10, 32)
		if err != nil {
			return errors.New("invalid XInput input state")
		}
		key := uint64(p.source)<<32 | detail + 1
		switch p.event {
		case "RawKeyPress", "RawButtonPress", "RawTouchBegin":
			// X11 scroll-wheel buttons are momentary, not held buttons.
			if p.event == "RawButtonPress" && detail >= 4 && detail <= 7 {
				key = 0
			}
			p.state.input(key, true)
		case "RawKeyRelease", "RawButtonRelease", "RawTouchEnd":
			p.state.input(key, false)
		case "RawTouchUpdate":
			p.state.input(0, false)
		default:
			return errors.New("unsupported XInput raw event")
		}
		p.pending = false
	}
	return nil
}

func openXInputMonitor(parent context.Context) (inputMonitor, error) {
	if _, err := exec.LookPath("xinput"); err != nil {
		return nil, &Unavailable{Reason: "xinput is required to monitor keyboard and mouse activity on X11", Hint: "install xinput; activity simulation stays paused until monitoring works"}
	}
	out, _, err := runCmd(parent, cmdTimeout, []string{"LC_ALL=C"}, "xinput", "list", "--short")
	if err != nil {
		return nil, &Unavailable{Reason: "cannot enumerate XInput devices", Hint: "check DISPLAY and that xinput can connect to your X11 session"}
	}
	devices, err := parseXInputDevices(out)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	m := &xInputMonitor{inputState: newInputState(time.Now), cancel: cancel, done: make(chan struct{})}
	cmd := exec.CommandContext(ctx, "xinput", "test-xi2", "--root")
	// Raw data is parsed in memory only; never capture stderr or stdout in logs.
	cmd.Stderr = io.Discard
	pipe, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		cancel()
		if pipe != nil {
			_ = pipe.Close()
		}
		return nil, &Unavailable{Reason: "cannot start the XInput observer"}
	}
	firstOutput := make(chan struct{})
	go func() {
		defer close(m.done)
		defer pipe.Close()
		parser := xInputParser{state: m.inputState, devices: devices}
		scanner := bufio.NewScanner(pipe)
		first := true
		for scanner.Scan() {
			if first {
				close(firstOutput)
				first = false
			}
			if err := parser.line(scanner.Text()); err != nil {
				m.fail(err)
				cancel()
				break
			}
			if parser.pending {
				if m.pending.Load() == 0 {
					m.pending.Store(time.Now().UnixNano())
				}
			} else {
				m.pending.Store(0)
			}
		}
		m.fail(&Unavailable{Reason: "XInput observer stopped; keyboard and mouse coverage is unavailable", Hint: "check xinput and your X11 session; keepalive retries in 60 seconds"})
		cancel()
		_ = cmd.Wait()
	}()
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if since := m.pending.Load(); since != 0 && time.Since(time.Unix(0, since)) > 50*time.Millisecond {
					m.fail(errors.New("incomplete XInput raw event; monitoring must be refreshed"))
					cancel()
					return
				}
			}
		}
	}()
	startup := time.NewTimer(cmdTimeout)
	defer startup.Stop()
	select {
	case <-firstOutput:
	case <-ctx.Done():
		_ = m.Close()
		return nil, errors.New("XInput observer did not start")
	case <-startup.C:
		_ = m.Close()
		return nil, errors.New("XInput observer startup timed out")
	}
	initial := m.Snapshot().Generation
	var held []uint64
	// Observe held state after starting the event stream, so a held key or
	// button cannot be mistaken for idle when keepalive starts.
	for id, synthetic := range devices {
		if synthetic {
			continue
		}
		out, _, err := runCmd(ctx, cmdTimeout, []string{"LC_ALL=C"}, "xinput", "query-state", strconv.Itoa(id))
		if err != nil {
			_ = m.Close()
			return nil, errors.New("cannot read initial XInput held state")
		}
		for _, line := range strings.Split(out, "\n") {
			match := xInputHeldLine.FindStringSubmatch(strings.TrimSpace(line))
			if match != nil && match[3] == "down" {
				detail, _ := strconv.ParseUint(match[2], 10, 32)
				held = append(held, uint64(id)<<32|detail+1)
			}
		}
	}
	if !m.SeedHeld(held, initial) {
		_ = m.Close()
		return nil, errors.New("input changed during the initial XInput state query; monitoring must be refreshed")
	}
	if err := m.Snapshot().Err; err != nil {
		_ = m.Close()
		return nil, fmt.Errorf("XInput observer unavailable: %w", err)
	}
	return m, nil
}
