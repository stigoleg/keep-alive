package service

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// launchd manages a LaunchAgent in the gui/<uid> domain.
type launchd struct {
	run   runner
	uid   int
	dir   string // ~/Library/LaunchAgents
	sleep func(time.Duration)
}

func (m *launchd) Name() string { return "launchd" }

func (m *launchd) path() string   { return filepath.Join(m.dir, Label+".plist") }
func (m *launchd) domain() string { return "gui/" + strconv.Itoa(m.uid) }
func (m *launchd) target() string { return m.domain() + "/" + Label }

func (m *launchd) print() ([]byte, error) { return m.run.Run("launchctl", "print", m.target()) }

func (m *launchd) loaded() bool {
	_, err := m.print()
	return err == nil
}

func (m *launchd) Install(spec Spec) error {
	if err := spec.validate(unixAbs); err != nil {
		return err
	}
	if spec.LogPath != "" {
		if err := os.MkdirAll(filepath.Dir(spec.LogPath), 0o700); err != nil {
			return fmt.Errorf("service: log directory: %w", err)
		}
	}
	if err := writeFile(m.path(), plist(spec), 0o644); err != nil {
		return err
	}
	if m.loaded() {
		if _, err := m.run.Run("launchctl", "bootout", m.target()); err != nil {
			return hinted(err, fmt.Sprintf(`stop the old job with "launchctl bootout %s" and try again`, m.target()))
		}
		m.waitUnloaded()
	}
	if _, err := m.run.Run("launchctl", "bootstrap", m.domain(), m.path()); err != nil {
		return hinted(err, fmt.Sprintf(`check the file with "plutil -lint %s"; the %s domain exists only for a user logged in at the desktop`, m.path(), m.domain()))
	}
	return nil
}

// waitUnloaded gives launchd up to 5 s to finish a bootout; bootstrapping
// the label again before that fails with an I/O error.
func (m *launchd) waitUnloaded() {
	for range 50 {
		if !m.loaded() {
			return
		}
		m.sleep(100 * time.Millisecond)
	}
}

func (m *launchd) Uninstall() error {
	loaded := m.loaded()
	file := exists(m.path())
	if !loaded && !file {
		return ErrNotInstalled
	}
	if loaded {
		if _, err := m.run.Run("launchctl", "bootout", m.target()); err != nil {
			return hinted(err, fmt.Sprintf(`inspect the job with "launchctl print %s"`, m.target()))
		}
	}
	return removeIfExists(m.path())
}

func (m *launchd) Status() (State, error) {
	st := State{Path: m.path(), Installed: exists(m.path())}
	out, err := m.print()
	if err != nil {
		if st.Installed {
			st.Detail = `installed but not loaded (it loads at next login, or run "keepalive service install" again)`
		} else {
			st.Detail = "not installed"
		}
		return st, nil
	}
	info := parseLaunchctlPrint(out)
	st.Restarts = launchdRestarts(info)
	if info["state"] == "running" {
		st.Running = true
		st.Detail = "running"
		if pid := info["pid"]; pid != "" {
			st.Detail += " (pid " + pid + ")"
		}
	} else {
		st.Detail = "loaded, not running"
		if code := info["last exit code"]; code != "" {
			st.Detail += " (last exit code " + code + ")"
		}
	}
	if !st.Installed {
		st.Installed = true
		st.Detail += "; the plist file is missing"
	}
	return st, nil
}

// launchdRestarts is how often launchd restarted the job: every run after
// the first, as long as the last one failed (launchd only restarts it after
// a failed exit; "keepalive service install" loads it afresh).
func launchdRestarts(info map[string]string) int {
	runs, err := strconv.Atoi(info["runs"])
	if err != nil || runs < 2 {
		return 0
	}
	if code, err := strconv.Atoi(info["last exit code"]); err != nil || code == 0 {
		return 0
	}
	return runs - 1
}

// parseLaunchctlPrint returns the "key = value" pairs at the top level of
// the job's block, skipping nested blocks (endpoints, environment, …).
func parseLaunchctlPrint(out []byte) map[string]string {
	info := map[string]string{}
	depth := 0
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasSuffix(line, "{"):
			depth++
			continue
		case line == "}":
			depth--
			continue
		}
		if depth != 1 {
			continue
		}
		if k, v, ok := strings.Cut(line, " = "); ok {
			info[k] = v
		}
	}
	return info
}

// plist renders the LaunchAgent property list.
func plist(spec Spec) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`)
	key := func(k string) { b.WriteString("\t<key>" + xmlText(k) + "</key>\n") }
	str := func(indent, v string) { b.WriteString(indent + "<string>" + xmlText(v) + "</string>\n") }

	key("Label")
	str("\t", Label)
	key("ProgramArguments")
	b.WriteString("\t<array>\n")
	for _, a := range append([]string{spec.Executable}, spec.programArgs()...) {
		str("\t\t", a)
	}
	b.WriteString("\t</array>\n")
	key("EnvironmentVariables")
	b.WriteString("\t<dict>\n\t\t<key>" + OriginEnv + "</key>\n")
	str("\t\t", OriginValue)
	b.WriteString("\t</dict>\n")
	key("RunAtLoad")
	b.WriteString("\t<true/>\n")
	key("KeepAlive") // restart after a crash, not after a normal exit
	b.WriteString("\t<dict>\n\t\t<key>SuccessfulExit</key>\n\t\t<false/>\n\t</dict>\n")
	key("ProcessType") // no App Nap or timer throttling
	str("\t", "Interactive")
	if spec.LogPath != "" {
		key("StandardOutPath")
		str("\t", spec.LogPath)
		key("StandardErrorPath")
		str("\t", spec.LogPath)
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes()
}

func xmlText(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s)) // a strings.Builder never fails
	return b.String()
}
