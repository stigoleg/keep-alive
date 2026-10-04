package service

import (
	"bytes"
	"encoding/xml"
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s mismatch\n got:\n%s\nwant:\n%s", name, got, want)
	}
}

// fakeRunner records commands; respond scripts their results.
type fakeRunner struct {
	calls   [][]string
	respond func(cmd []string) (string, error)
}

func (f *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	cmd := append([]string{name}, args...)
	f.calls = append(f.calls, cmd)
	if f.respond == nil {
		return nil, nil
	}
	out, err := f.respond(cmd)
	return []byte(out), err
}

func (f *fakeRunner) commands() []string {
	var s []string
	for _, c := range f.calls {
		s = append(s, strings.Join(c, " "))
	}
	return s
}

func failed(stderr string) error {
	return &CommandError{Command: "fake", Stderr: stderr, Err: errors.New("exit status 1")}
}

func assertCalls(t *testing.T, f *fakeRunner, want ...string) {
	t.Helper()
	if got := f.commands(); !slices.Equal(got, want) {
		t.Fatalf("commands:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func wellFormedXML(t *testing.T, data []byte) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	for {
		if _, err := dec.Token(); err == io.EOF {
			return
		} else if err != nil {
			t.Fatalf("not well-formed XML: %v", err)
		}
	}
}

// trickySpec has characters every format must escape.
var trickySpec = Spec{
	Executable: "/Users/Jane Doe/bin/keep&alive",
	Args:       []string{"--schedule", "Mon-Fri 08:00-16:00", "--active", "--while", `a"b'c\d$e%f<g>`},
	LogPath:    "/Users/Jane Doe/Library/Logs/keepalive/service.log",
}

// --- launchd ---

func TestPlistGolden(t *testing.T) {
	got := plist(trickySpec)
	golden(t, "launchd.plist", got)
	wellFormedXML(t, got)
	if plutil, err := exec.LookPath("plutil"); err == nil {
		path := filepath.Join(t.TempDir(), "x.plist")
		os.WriteFile(path, got, 0o644)
		if out, err := exec.Command(plutil, "-lint", path).CombinedOutput(); err != nil {
			t.Fatalf("plutil -lint: %v\n%s", err, out)
		}
		out, err := exec.Command(plutil, "-extract", "ProgramArguments.6", "raw", path).Output()
		if err != nil || strings.TrimSpace(string(out)) != `a"b'c\d$e%f<g>` {
			t.Fatalf("plutil sees argument %q (%v)", out, err)
		}
	}
}

func TestPlistWithoutLog(t *testing.T) {
	got := string(plist(Spec{Executable: "/usr/local/bin/keepalive"}))
	if strings.Contains(got, "StandardOutPath") {
		t.Fatal("log keys without a LogPath")
	}
}

func newLaunchd(t *testing.T, f *fakeRunner) *launchd {
	return &launchd{run: f, uid: 501, dir: filepath.Join(t.TempDir(), "LaunchAgents"), sleep: func(time.Duration) {}}
}

const target = "gui/501/" + Label

func TestLaunchdInstallFresh(t *testing.T) {
	f := &fakeRunner{respond: func(cmd []string) (string, error) {
		if cmd[1] == "print" {
			return "", failed("Could not find service")
		}
		return "", nil
	}}
	m := newLaunchd(t, f)
	spec := trickySpec
	spec.LogPath = filepath.Join(t.TempDir(), "logs", "service.log")
	if err := m.Install(spec); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.dir, Label+".plist")
	assertCalls(t, f, "launchctl print "+target, "launchctl bootstrap gui/501 "+path)
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, plist(spec)) {
		t.Fatalf("plist on disk differs (%v)", err)
	}
	if fi, err := os.Stat(filepath.Dir(spec.LogPath)); err != nil || !fi.IsDir() {
		t.Fatalf("log directory not created: %v", err)
	}
}

func TestLaunchdInstallReplacesLoadedJob(t *testing.T) {
	prints := 0
	f := &fakeRunner{respond: func(cmd []string) (string, error) {
		if cmd[1] == "print" {
			prints++
			if prints <= 2 { // loaded, still loaded right after bootout, then gone
				return "state = running", nil
			}
			return "", failed("Could not find service")
		}
		return "", nil
	}}
	m := newLaunchd(t, f)
	if err := m.Install(Spec{Executable: "/opt/homebrew/bin/keepalive", Args: []string{"-a"}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.dir, Label+".plist")
	assertCalls(t, f,
		"launchctl print "+target,
		"launchctl bootout "+target,
		"launchctl print "+target,
		"launchctl print "+target,
		"launchctl bootstrap gui/501 "+path)
}

func TestLaunchdBootstrapErrorCarriesStderrAndHint(t *testing.T) {
	f := &fakeRunner{respond: func(cmd []string) (string, error) {
		switch cmd[1] {
		case "print":
			return "", failed("Could not find service")
		case "bootstrap":
			return "", failed("Bootstrap failed: 5: Input/output error")
		}
		return "", nil
	}}
	err := newLaunchd(t, f).Install(Spec{Executable: "/usr/local/bin/keepalive"})
	if err == nil || !strings.Contains(err.Error(), "Input/output error") {
		t.Fatalf("error %v lacks stderr", err)
	}
	if h := Hint(err); !strings.Contains(h, "plutil -lint") {
		t.Fatalf("hint = %q", h)
	}
}

func TestLaunchdUninstall(t *testing.T) {
	f := &fakeRunner{}
	m := newLaunchd(t, f)
	path := filepath.Join(m.dir, Label+".plist")
	os.MkdirAll(m.dir, 0o755)
	os.WriteFile(path, plist(Spec{Executable: "/x"}), 0o644)
	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, f, "launchctl print "+target, "launchctl bootout "+target)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("plist not removed: %v", err)
	}

	f2 := &fakeRunner{respond: func([]string) (string, error) { return "", failed("Could not find service") }}
	if err := newLaunchd(t, f2).Uninstall(); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("uninstall with nothing installed = %v", err)
	}
	assertCalls(t, f2, "launchctl print "+target)
}

const printRunning = `gui/501/io.github.stigoleg.keepalive = {
	active count = 1
	path = /Users/u/Library/LaunchAgents/io.github.stigoleg.keepalive.plist
	type = LaunchAgent
	state = running

	program = /opt/homebrew/bin/keepalive
	arguments = {
		/opt/homebrew/bin/keepalive
		--plain
	}

	dynamic endpoints = {
		"x" = {
			state = waiting
			pid = 1
		}
	}

	pid = 12345
	last exit code = (never exited)
}
`

const printExited = `gui/501/io.github.stigoleg.keepalive = {
	path = /Users/u/Library/LaunchAgents/io.github.stigoleg.keepalive.plist
	state = not running
	runs = 3
	last exit code = 2
}
`

const printRestarting = `gui/501/io.github.stigoleg.keepalive = {
	state = running
	runs = 7
	pid = 777
	last exit code = 1
}
`

func TestLaunchdRestarts(t *testing.T) {
	for out, want := range map[string]int{
		printRunning:    0, // never exited
		printExited:     2, // runs = 3, last exit code 2
		printRestarting: 6,
		"gui/501/x = {\n\truns = 4\n\tlast exit code = 0\n}\n": 0, // ended normally
	} {
		m := newLaunchd(t, &fakeRunner{respond: func([]string) (string, error) { return out, nil }})
		if st, err := m.Status(); err != nil || st.Restarts != want {
			t.Errorf("restarts = %d, %v; want %d for\n%s", st.Restarts, err, want, out)
		}
	}
}

func TestLaunchdStatus(t *testing.T) {
	tests := map[string]struct {
		file      bool
		out       string
		err       error
		installed bool
		running   bool
		detail    string
	}{
		"running":       {true, printRunning, nil, true, true, "running (pid 12345)"},
		"exited":        {true, printExited, nil, true, false, "loaded, not running (last exit code 2)"},
		"restarting":    {true, printRestarting, nil, true, true, "running (pid 777)"},
		"not loaded":    {true, "", failed("Could not find service"), true, false, "installed but not loaded"},
		"nothing":       {false, "", failed("Could not find service"), false, false, "not installed"},
		"plist missing": {false, printRunning, nil, true, true, "plist file is missing"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := newLaunchd(t, &fakeRunner{respond: func([]string) (string, error) { return tt.out, tt.err }})
			path := filepath.Join(m.dir, Label+".plist")
			if tt.file {
				os.MkdirAll(m.dir, 0o755)
				os.WriteFile(path, nil, 0o644)
			}
			st, err := m.Status()
			if err != nil {
				t.Fatal(err)
			}
			if st.Installed != tt.installed || st.Running != tt.running || !strings.Contains(st.Detail, tt.detail) || st.Path != path {
				t.Fatalf("status = %+v", st)
			}
		})
	}
}

// --- systemd / XDG autostart ---

func TestUnitGolden(t *testing.T) {
	golden(t, "keepalive.service", unit(trickySpec))
}

func TestDesktopGolden(t *testing.T) {
	golden(t, "keepalive.desktop", desktopEntry(trickySpec))
}

func TestSystemdQuote(t *testing.T) {
	for in, want := range map[string]string{
		"--plain":             "--plain",
		"/usr/bin/keepalive":  "/usr/bin/keepalive",
		"Mon-Fri 08:00-16:00": `"Mon-Fri 08:00-16:00"`,
		"":                    `""`,
		`a"b`:                 `"a\"b"`,
		`back\slash`:          `"back\\slash"`,
		"50%":                 "50%%",
		"$HOME":               "$$HOME",
		"it's":                `"it's"`,
		";":                   `";"`,
		"two\nlines":          `"two\nlines"`,
	} {
		if got := systemdQuote(in); got != want {
			t.Errorf("systemdQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestDesktopQuote(t *testing.T) {
	for in, want := range map[string]string{
		"--plain":             "--plain",
		"Mon-Fri 08:00-16:00": `"Mon-Fri 08:00-16:00"`,
		"":                    `""`,
		`a"b`:                 `"a\\"b"`,
		`back\slash`:          `"back\\\\slash"`,
		"$HOME":               `"\\$HOME"`,
		"`cmd`":               "\"\\\\`cmd\\\\`\"",
		"50%":                 "50%%",
		"a b%":                `"a b%%"`,
		"two\nlines":          `"two\nlines"`,
	} {
		if got := desktopQuote(in); got != want {
			t.Errorf("desktopQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func newSystemd(t *testing.T, f *fakeRunner) *systemd {
	return &systemd{run: f, configHome: filepath.Join(t.TempDir(), "config")}
}

func systemdUp(active string) func([]string) (string, error) {
	return func(cmd []string) (string, error) {
		switch cmd[2] {
		case "is-active":
			if active != "active" {
				return active + "\n", failed("")
			}
			return "active\n", nil
		case "is-enabled":
			return "enabled\n", nil
		case "show":
			return "4\n", nil
		}
		return "", nil
	}
}

func TestSystemdInstall(t *testing.T) {
	f := &fakeRunner{respond: systemdUp("inactive")}
	m := newSystemd(t, f)
	desktop := filepath.Join(m.configHome, "autostart", "keepalive.desktop")
	os.MkdirAll(filepath.Dir(desktop), 0o755)
	os.WriteFile(desktop, []byte("old"), 0o644)

	spec := Spec{Executable: "/usr/bin/keepalive", Args: []string{"-a"}}
	if err := m.Install(spec); err != nil {
		t.Fatal(err)
	}
	if m.Name() != "systemd --user" {
		t.Errorf("Name = %q", m.Name())
	}
	assertCalls(t, f,
		"systemctl --user show-environment",
		"systemctl --user is-active keepalive.service",
		"systemctl --user daemon-reload",
		"systemctl --user enable --now keepalive.service")
	data, err := os.ReadFile(filepath.Join(m.configHome, "systemd", "user", "keepalive.service"))
	if err != nil || !bytes.Equal(data, unit(spec)) {
		t.Fatalf("unit on disk differs (%v)", err)
	}
	if _, err := os.Stat(desktop); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("leftover autostart entry not removed")
	}
}

func TestSystemdReinstallRestartsActiveUnit(t *testing.T) {
	f := &fakeRunner{respond: systemdUp("active")}
	if err := newSystemd(t, f).Install(Spec{Executable: "/usr/bin/keepalive"}); err != nil {
		t.Fatal(err)
	}
	if got := f.commands(); got[len(got)-1] != "systemctl --user restart keepalive.service" {
		t.Fatalf("commands = %q, want a restart last", got)
	}
}

func TestSystemdEnableErrorCarriesStderrAndHint(t *testing.T) {
	f := &fakeRunner{respond: func(cmd []string) (string, error) {
		if cmd[2] == "enable" {
			return "", failed("Failed to enable unit: Unit file keepalive.service does not exist.")
		}
		return systemdUp("inactive")(cmd)
	}}
	err := newSystemd(t, f).Install(Spec{Executable: "/usr/bin/keepalive"})
	if err == nil || !strings.Contains(err.Error(), "does not exist") || !strings.Contains(Hint(err), "journalctl --user") {
		t.Fatalf("error %v, hint %q", err, Hint(err))
	}
}

func TestAutostartFallback(t *testing.T) {
	f := &fakeRunner{respond: func(cmd []string) (string, error) {
		return "", failed("Failed to connect to bus: No medium found")
	}}
	m := newSystemd(t, f)
	spec := Spec{Executable: "/usr/bin/keepalive", Args: []string{"-a"}}
	if err := m.Install(spec); err != nil {
		t.Fatal(err)
	}
	if m.Name() != "XDG autostart" {
		t.Errorf("Name = %q", m.Name())
	}
	assertCalls(t, f, "systemctl --user show-environment")
	path := filepath.Join(m.configHome, "autostart", "keepalive.desktop")
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, desktopEntry(spec)) {
		t.Fatalf("desktop entry on disk differs (%v)", err)
	}
	st, err := m.Status()
	if err != nil || !st.Installed || st.Running || st.Detail != "autostart entry (starts at next login)" || st.Path != path {
		t.Fatalf("status = %+v, %v", st, err)
	}
	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("desktop entry not removed")
	}
	if err := m.Uninstall(); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("second uninstall = %v", err)
	}
}

func TestSystemdUninstallAndStatus(t *testing.T) {
	f := &fakeRunner{respond: systemdUp("active")}
	m := newSystemd(t, f)
	if err := m.Install(Spec{Executable: "/usr/bin/keepalive"}); err != nil {
		t.Fatal(err)
	}
	f.calls = nil
	st, err := m.Status()
	if err != nil || !st.Installed || !st.Running || st.Detail != "enabled, active" || !strings.HasSuffix(st.Path, "keepalive.service") ||
		st.Restarts != 4 {
		t.Fatalf("status = %+v, %v", st, err)
	}
	assertCalls(t, f,
		"systemctl --user is-active keepalive.service",
		"systemctl --user is-enabled keepalive.service",
		"systemctl --user show --property=NRestarts --value keepalive.service")
	f.calls = nil
	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, f, "systemctl --user disable --now keepalive.service", "systemctl --user daemon-reload")
	if _, err := os.Stat(st.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unit not removed")
	}
	if st, err := m.Status(); err != nil || st.Installed || st.Detail != "not installed" {
		t.Fatalf("status after uninstall = %+v, %v", st, err)
	}
}

// --- Task Scheduler ---

var windowsSpec = Spec{
	Executable: `C:\Program Files\keepalive\keepalivew.exe`,
	Args:       []string{"--schedule", "Mon-Fri 08:00-16:00", "--while", `say "hi" & <bye>`, `C:\dir\`},
}

func TestTaskXMLGolden(t *testing.T) {
	text := taskXML(windowsSpec, `DESKTOP-1\Jørn`)
	golden(t, "task.xml", []byte(text))
	wellFormedXML(t, []byte(text))
}

func TestTaskXMLIsUTF16LEWithBOM(t *testing.T) {
	text := taskXML(windowsSpec, `DESKTOP-1\Jørn 🙂`)
	data := encodeUTF16LE(text)
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xFE {
		t.Fatalf("no UTF-16LE BOM: % x", data[:4])
	}
	if len(data)%2 != 0 {
		t.Fatal("odd length")
	}
	u := make([]uint16, (len(data)-2)/2)
	for i := range u {
		u[i] = uint16(data[2+2*i]) | uint16(data[3+2*i])<<8
	}
	if got := string(utf16.Decode(u)); got != text {
		t.Fatal("UTF-16 does not round-trip")
	}
	if !strings.HasPrefix(text, `<?xml version="1.0" encoding="UTF-16"?>`) {
		t.Fatalf("declaration: %.50s", text)
	}
}

func TestEscapeArg(t *testing.T) {
	for in, want := range map[string]string{
		"":                    `""`,
		"--plain":             "--plain",
		"Mon-Fri 08:00-16:00": `"Mon-Fri 08:00-16:00"`,
		`a"b`:                 `a\"b`,
		`C:\dir\`:             `C:\dir\`,
		`C:\my dir\`:          `"C:\my dir\\"`,
		`say "hi"`:            `"say \"hi\""`,
		`back\\"q`:            `back\\\\\"q`,
	} {
		if got := escapeArg(in); got != want {
			t.Errorf("escapeArg(%q) = %s, want %s", in, got, want)
		}
	}
}

func newSchtasks(f *fakeRunner) *schtasks {
	return &schtasks{run: f, user: `PC\u`, tempDir: os.TempDir()}
}

const queryRunning = `
Folder: \
HostName:                             PC
TaskName:                             \keepalive
Next Run Time:                        N/A
Status:                               Running
Logon Mode:                           Interactive only
Last Run Time:                        10/4/2026 8:00:01 AM
Last Result:                          267009
`

func TestSchtasksInstall(t *testing.T) {
	var xmlPath string
	var xmlData []byte
	f := &fakeRunner{respond: func(cmd []string) (string, error) {
		switch cmd[1] {
		case "/Query":
			return "", failed("ERROR: The system cannot find the file specified.")
		case "/Create":
			xmlPath = cmd[5]
			xmlData, _ = os.ReadFile(xmlPath)
		}
		return "", nil
	}}
	if err := newSchtasks(f).Install(windowsSpec); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, f,
		"schtasks /Query /TN keepalive /FO LIST /V",
		"schtasks /Create /TN keepalive /XML "+xmlPath+" /F",
		"schtasks /Run /TN keepalive")
	if !bytes.Equal(xmlData, encodeUTF16LE(taskXML(windowsSpec, `PC\u`))) {
		t.Fatal("task file is not the UTF-16 task XML")
	}
	if _, err := os.Stat(xmlPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("temporary task file left behind")
	}
}

func TestSchtasksReinstallEndsRunningTask(t *testing.T) {
	f := &fakeRunner{respond: func(cmd []string) (string, error) {
		if cmd[1] == "/Query" {
			return queryRunning, nil
		}
		return "", nil
	}}
	if err := newSchtasks(f).Install(windowsSpec); err != nil {
		t.Fatal(err)
	}
	if got := f.commands(); len(got) != 4 || got[1] != "schtasks /End /TN keepalive" {
		t.Fatalf("commands = %q", got)
	}
}

func TestSchtasksCreateErrorCarriesStderrAndHint(t *testing.T) {
	f := &fakeRunner{respond: func(cmd []string) (string, error) {
		switch cmd[1] {
		case "/Query":
			return "", failed("ERROR: The system cannot find the file specified.")
		case "/Create":
			return "", failed("ERROR: Access is denied.")
		}
		return "", nil
	}}
	err := newSchtasks(f).Install(windowsSpec)
	if err == nil || !strings.Contains(err.Error(), "Access is denied") || !strings.Contains(Hint(err), "schtasks /Query") {
		t.Fatalf("error %v, hint %q", err, Hint(err))
	}
}

func TestSchtasksStatusAndUninstall(t *testing.T) {
	f := &fakeRunner{respond: func(cmd []string) (string, error) {
		if cmd[1] == "/Query" {
			return queryRunning, nil
		}
		return "", nil
	}}
	m := newSchtasks(f)
	st, err := m.Status()
	if err != nil || !st.Installed || !st.Running || st.Path != "keepalive" || !strings.Contains(st.Detail, "Running") || !strings.Contains(st.Detail, "267009") {
		t.Fatalf("status = %+v, %v", st, err)
	}
	f.calls = nil
	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, f,
		"schtasks /Query /TN keepalive /FO LIST /V",
		"schtasks /End /TN keepalive",
		"schtasks /Delete /TN keepalive /F")

	gone := newSchtasks(&fakeRunner{respond: func([]string) (string, error) {
		return "", failed("ERROR: The system cannot find the file specified.")
	}})
	if st, err := gone.Status(); err != nil || st.Installed || st.Detail != "not installed" {
		t.Fatalf("status of a missing task = %+v, %v", st, err)
	}
	if err := gone.Uninstall(); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("uninstall of a missing task = %v", err)
	}
}

// --- shared ---

func TestSpecValidation(t *testing.T) {
	f := &fakeRunner{}
	for _, m := range []Manager{newLaunchd(t, f), newSystemd(t, f)} {
		for _, spec := range []Spec{{}, {Executable: "keepalive"}, {Executable: "/bin/keep\x00alive"}, {Executable: "/bin/k", Args: []string{"a\x00"}}} {
			if err := m.Install(spec); err == nil {
				t.Errorf("%s accepted %+v", m.Name(), spec)
			}
		}
	}
	if err := newSchtasks(f).Install(Spec{Executable: "/usr/bin/keepalive"}); err == nil {
		t.Error("Task Scheduler accepted a unix path")
	}
	if len(f.calls) != 0 {
		t.Fatalf("invalid specs ran commands: %q", f.commands())
	}
}

func TestResolveExecutable(t *testing.T) {
	cellar := "/opt/homebrew/Cellar/keepalive/2.0.0/bin/keepalive"
	links := map[string]string{"/opt/homebrew/bin/keepalive": cellar}
	env := func(exe, lookPath string) exeEnv {
		return exeEnv{
			executable: func() (string, error) { return exe, nil },
			lookPath: func(string) (string, error) {
				if lookPath == "" {
					return "", exec.ErrNotFound
				}
				return lookPath, nil
			},
			evalSymlinks: func(p string) (string, error) {
				if r, ok := links[p]; ok {
					return r, nil
				}
				return p, nil
			},
			exists:  func(string) bool { return false },
			tempDir: "/var/folders/xy/T",
			goos:    "darwin",
		}
	}
	tests := []struct {
		name, exe, lookPath, want, err string
	}{
		{"homebrew symlink wins", cellar, "/opt/homebrew/bin/keepalive", "/opt/homebrew/bin/keepalive", ""},
		{"other keepalive on PATH", "/Users/u/go/bin/keepalive", "/usr/local/bin/keepalive", "/Users/u/go/bin/keepalive", ""},
		{"not on PATH", "/Users/u/go/bin/keepalive", "", "/Users/u/go/bin/keepalive", ""},
		{"go run", "/var/folders/xy/T/go-build123/b001/exe/keepalive", "", "", "temporary"},
		{"go build cache", "/Users/u/Library/Caches/go-build/ab/cd-d/keepalive", "", "", "temporary"},
		{"temp dir", "/var/folders/xy/T/keepalive", "", "", "temporary"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, warn, err := resolveExecutable(env(tt.exe, tt.lookPath))
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) || !strings.Contains(Hint(err), "install") {
					t.Fatalf("err = %v (hint %q), want %q", err, Hint(err), tt.err)
				}
				return
			}
			if err != nil || got != tt.want || warn != "" {
				t.Fatalf("= %q, %q, %v; want %q", got, warn, err, tt.want)
			}
		})
	}

	win := env("/c/Program Files/keepalive/keepalive.exe", "")
	win.goos = "windows"
	got, warn, err := resolveExecutable(win)
	if err != nil || got != "/c/Program Files/keepalive/keepalive.exe" || !strings.Contains(warn, "console window") {
		t.Fatalf("windows without keepalivew: %q, %q, %v", got, warn, err)
	}
	win.exists = func(p string) bool { return p == "/c/Program Files/keepalive/keepalivew.exe" }
	got, warn, err = resolveExecutable(win)
	if err != nil || got != "/c/Program Files/keepalive/keepalivew.exe" || warn != "" {
		t.Fatalf("windows with keepalivew: %q, %q, %v", got, warn, err)
	}
}

func TestCommandErrorAndTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	_, err := execRunner{timeout: CommandTimeout}.Run("sh", "-c", "echo out; echo boom >&2; exit 3")
	var ce *CommandError
	if !errors.As(err, &ce) || ce.Stderr != "boom" || !strings.Contains(err.Error(), "exit status 3") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
	if !strings.HasPrefix(err.Error(), "sh -c ") {
		t.Errorf("error does not name the command: %v", err)
	}
	start := time.Now()
	_, err = execRunner{timeout: 100 * time.Millisecond}.Run("sleep", "5")
	if err == nil || !strings.Contains(err.Error(), "timed out after 100ms") || time.Since(start) > 4*time.Second {
		t.Fatalf("timeout: %v after %s", err, time.Since(start))
	}
	out, err := execRunner{timeout: CommandTimeout}.Run("sh", "-c", "echo hello")
	if err != nil || string(out) != "hello\n" {
		t.Fatalf("stdout = %q, %v", out, err)
	}
}
