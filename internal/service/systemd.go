package service

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// systemd manages a systemd --user unit, or an XDG autostart entry when
// systemctl --user cannot reach a user manager.
type systemd struct {
	run        runner
	configHome string // $XDG_CONFIG_HOME, else ~/.config

	once  sync.Once
	avail bool
}

func (m *systemd) unitPath() string    { return filepath.Join(m.configHome, "systemd", "user", UnitName) }
func (m *systemd) desktopPath() string { return filepath.Join(m.configHome, "autostart", DesktopName) }

// available reports whether systemctl --user works (systemd present and a
// user bus reachable); checked once.
func (m *systemd) available() bool {
	m.once.Do(func() {
		_, err := m.systemctl("show-environment")
		m.avail = err == nil
	})
	return m.avail
}

func (m *systemd) systemctl(args ...string) ([]byte, error) {
	return m.run.Run("systemctl", append([]string{"--user"}, args...)...)
}

// query returns a one-word systemctl answer (is-active, is-enabled), which
// is printed even when the exit status is non-zero.
func (m *systemd) query(verb string) string {
	out, _ := m.systemctl(verb, UnitName)
	return strings.TrimSpace(string(out))
}

const systemdHint = `check "systemctl --user status keepalive.service" and "journalctl --user -u keepalive.service"`

func (m *systemd) Name() string {
	if m.available() {
		return "systemd --user"
	}
	return "XDG autostart"
}

func (m *systemd) Install(spec Spec) error {
	if err := spec.validate(unixAbs); err != nil {
		return err
	}
	if !m.available() {
		if err := writeFile(m.desktopPath(), desktopEntry(spec), 0o644); err != nil {
			return err
		}
		return removeIfExists(m.unitPath())
	}
	wasActive := m.query("is-active") == "active"
	if err := writeFile(m.unitPath(), unit(spec), 0o644); err != nil {
		return err
	}
	if err := removeIfExists(m.desktopPath()); err != nil {
		return err
	}
	if _, err := m.systemctl("daemon-reload"); err != nil {
		return hinted(err, systemdHint)
	}
	if _, err := m.systemctl("enable", "--now", UnitName); err != nil {
		return hinted(err, systemdHint)
	}
	if wasActive { // enable --now leaves a running old instance alone
		if _, err := m.systemctl("restart", UnitName); err != nil {
			return hinted(err, systemdHint)
		}
	}
	return nil
}

func (m *systemd) Uninstall() error {
	unitFile, desktopFile := exists(m.unitPath()), exists(m.desktopPath())
	if !unitFile && !desktopFile {
		return ErrNotInstalled
	}
	if unitFile {
		if m.available() {
			if _, err := m.systemctl("disable", "--now", UnitName); err != nil {
				return hinted(err, systemdHint)
			}
		}
		if err := removeIfExists(m.unitPath()); err != nil {
			return err
		}
		if m.available() {
			if _, err := m.systemctl("daemon-reload"); err != nil {
				return hinted(err, systemdHint)
			}
		}
	}
	return removeIfExists(m.desktopPath())
}

func (m *systemd) Status() (State, error) {
	switch {
	case exists(m.unitPath()):
		st := State{Installed: true, Path: m.unitPath()}
		if !m.available() {
			st.Detail = "unit file present, but systemctl --user is unavailable"
			return st, nil
		}
		active := m.query("is-active")
		st.Running = active == "active"
		st.Detail = m.query("is-enabled") + ", " + active
		if out, err := m.systemctl("show", "--property=NRestarts", "--value", UnitName); err == nil {
			st.Restarts, _ = strconv.Atoi(strings.TrimSpace(string(out)))
		}
		return st, nil
	case exists(m.desktopPath()):
		return State{Installed: true, Path: m.desktopPath(), Detail: "autostart entry (starts at next login)"}, nil
	}
	path := m.desktopPath()
	if m.available() {
		path = m.unitPath()
	}
	return State{Path: path, Detail: "not installed"}, nil
}

// unit renders the systemd --user unit.
func unit(spec Spec) []byte {
	var exec []string
	for _, a := range append([]string{spec.Executable}, spec.programArgs()...) {
		exec = append(exec, systemdQuote(a))
	}
	return []byte(fmt.Sprintf(`# Installed by "keepalive service install"; remove with "keepalive service uninstall".
[Unit]
Description=keepalive
PartOf=graphical-session.target
After=graphical-session.target

[Service]
ExecStart=%s
Restart=on-failure
RestartSec=5
Environment=%s=%s

[Install]
WantedBy=graphical-session.target
`, strings.Join(exec, " "), OriginEnv, OriginValue))
}

// systemdQuote quotes one ExecStart word: "%" and "$" are doubled (specifier
// and variable expansion apply inside quotes too); words with blanks, quotes,
// backslashes or ";" are double-quoted with C-style escapes.
func systemdQuote(s string) string {
	s = strings.ReplaceAll(s, "%", "%%")
	s = strings.ReplaceAll(s, "$", "$$")
	if s != "" && !strings.ContainsAny(s, " \t\n\r\"'\\;") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// desktopEntry renders the XDG autostart entry. Exec cannot set the
// environment itself, so it goes through env(1).
func desktopEntry(spec Spec) []byte {
	words := []string{"env", OriginEnv + "=" + OriginValue}
	for _, a := range append([]string{spec.Executable}, spec.programArgs()...) {
		words = append(words, desktopQuote(a))
	}
	return []byte(`# Installed by "keepalive service install"; remove with "keepalive service uninstall".
[Desktop Entry]
Type=Application
Name=keepalive
Comment=Keeps the computer awake
Exec=` + strings.Join(words, " ") + `
Terminal=false
NoDisplay=true
X-GNOME-Autostart-enabled=true
`)
}

// desktopReserved are the characters that force quoting in an Exec argument
// (Desktop Entry Specification, "The Exec key").
const desktopReserved = " \t\n\"'\\><~|&;$*?#()`"

// desktopQuote quotes one Exec argument. Inside quotes '"', '`', '$' and '\'
// take a backslash; then the string-value escapes apply on top (so a literal
// backslash ends up as four) and "%" is doubled because of field codes.
func desktopQuote(s string) string {
	arg := s
	if s == "" || strings.ContainsAny(s, desktopReserved) {
		var b strings.Builder
		b.WriteByte('"')
		for _, r := range s {
			if strings.ContainsRune("\"`$\\", r) {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
		b.WriteByte('"')
		arg = b.String()
	}
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\t", `\t`, "\r", `\r`, "%", "%%").Replace(arg)
}
