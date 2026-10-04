package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

func writeFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func env(vars map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := vars[k]
		return v, ok
	}
}

// flags builds a flag set shaped like the CLI's session flags.
func flags(t *testing.T, args ...string) *pflag.FlagSet {
	t.Helper()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	var d DurationValue
	fs.VarP(&d, "duration", "d", "")
	fs.IntP("battery", "b", 0, "")
	fs.BoolP("active", "a", false, "")
	fs.Duration("active-idle", 0, "")
	fs.Duration("active-interval", 0, "")
	fs.Bool("active-keys", false, "")
	fs.String("schedule", "", "")
	fs.Bool("keep-display", true, "")
	fs.Bool("notify", false, "")
	fs.BoolP("log", "l", false, "")
	fs.String("log-file", "", "")
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return fs
}

func TestDefaults(t *testing.T) {
	r, err := Resolve(Options{Env: env(nil)})
	if err != nil {
		t.Fatal(err)
	}
	want := Config{ActiveIdle: 2 * time.Minute, ActiveInterval: 30 * time.Second, KeepDisplay: true}
	if r.Config != want {
		t.Fatalf("defaults = %+v, want %+v", r.Config, want)
	}
	for _, k := range Keys {
		if r.Sources[k] != SourceDefault {
			t.Errorf("source[%s] = %s, want default", k, r.Sources[k])
		}
	}
}

func TestMissingFileIsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.toml")
	r, err := Resolve(Options{Path: path, Env: env(nil)})
	if err != nil {
		t.Fatal(err)
	}
	if r.FileFound || r.Path != path {
		t.Fatalf("FileFound=%v Path=%q", r.FileFound, r.Path)
	}
}

func TestMissingExplicitFileIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.toml")
	if _, err := Resolve(Options{Path: path, PathExplicit: true, Env: env(nil)}); err == nil {
		t.Fatal("missing --config file accepted")
	}
}

func TestFileLayer(t *testing.T) {
	path := writeFile(t, `
duration = "2h30m"
battery = 20
active = true
active_idle = "90s"
active_interval = "45s"
active_keys = true
schedule = "mon-fri 09:00-17:00"
keep_display = false
notify = true
log = true
log_file = "/tmp/ka.log"
`)
	r, err := Resolve(Options{Path: path, Env: env(nil)})
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		Duration: 150 * time.Minute, Battery: 20, Active: true,
		ActiveIdle: 90 * time.Second, ActiveInterval: 45 * time.Second, ActiveKeys: true,
		Schedule: "mon-fri 09:00-17:00", KeepDisplay: false, Notify: true, Log: true, LogFile: "/tmp/ka.log",
	}
	if r.Config != want {
		t.Fatalf("config = %+v\nwant     %+v", r.Config, want)
	}
	for _, k := range Keys {
		if r.Sources[k] != SourceFile {
			t.Errorf("source[%s] = %s, want file", k, r.Sources[k])
		}
	}
}

func TestFileDurationAsMinutes(t *testing.T) {
	r, err := Resolve(Options{Path: writeFile(t, "duration = 150\n"), Env: env(nil)})
	if err != nil {
		t.Fatal(err)
	}
	if r.Duration != 150*time.Minute {
		t.Fatalf("duration = %v", r.Duration)
	}
}

func TestUnknownKeyIsNamed(t *testing.T) {
	_, err := Resolve(Options{Path: writeFile(t, "batery = 20\n"), Env: env(nil)})
	if err == nil || !strings.Contains(err.Error(), `"batery"`) {
		t.Fatalf("err = %v, want it to name the unknown key", err)
	}
}

func TestFileTypeErrors(t *testing.T) {
	for _, body := range []string{
		`battery = "20"`,
		`active = "yes"`,
		`active_idle = 90`,
		`duration = true`,
		`[active]`,
		`battery = 0`,
		`battery = 101`,
		`duration = "30s"`,
		`active_idle = "5s"`,
		`active_interval = "1s"`,
		`schedule = ""`,
		`schedule = "Mnday 08:00-16:00"`,
		`not toml at all`,
	} {
		if _, err := Resolve(Options{Path: writeFile(t, body+"\n"), Env: env(nil)}); err == nil {
			t.Errorf("%q accepted", body)
		}
	}
}

func TestEnvLayer(t *testing.T) {
	path := writeFile(t, "battery = 20\nactive_idle = \"90s\"\n")
	r, err := Resolve(Options{Path: path, Env: env(map[string]string{
		"KEEPALIVE_BATTERY":         "30",
		"KEEPALIVE_ACTIVE":          "1",
		"KEEPALIVE_ACTIVE_KEYS":     "yes",
		"KEEPALIVE_KEEP_DISPLAY":    "off",
		"KEEPALIVE_DURATION":        "45",
		"KEEPALIVE_ACTIVE_INTERVAL": "", // empty = unset
	})})
	if err != nil {
		t.Fatal(err)
	}
	if r.Battery != 30 || !r.Active || !r.ActiveKeys || r.KeepDisplay || r.Duration != 45*time.Minute {
		t.Fatalf("config = %+v", r.Config)
	}
	if r.ActiveIdle != 90*time.Second || r.Sources["active_idle"] != SourceFile {
		t.Fatalf("file value lost under env: %v from %s", r.ActiveIdle, r.Sources["active_idle"])
	}
	if r.ActiveInterval != 30*time.Second || r.Sources["active_interval"] != SourceDefault {
		t.Fatalf("empty env var not ignored: %v from %s", r.ActiveInterval, r.Sources["active_interval"])
	}
	for _, k := range []string{"battery", "active", "active_keys", "keep_display", "duration"} {
		if r.Sources[k] != SourceEnv {
			t.Errorf("source[%s] = %s, want env", k, r.Sources[k])
		}
	}
}

func TestEnvErrorNamesVariable(t *testing.T) {
	_, err := Resolve(Options{Env: env(map[string]string{"KEEPALIVE_ACTIVE_IDLE": "soon"})})
	if err == nil || !strings.Contains(err.Error(), "KEEPALIVE_ACTIVE_IDLE") {
		t.Fatalf("err = %v", err)
	}
}

func TestFlagLayerWinsOnlyWhenChanged(t *testing.T) {
	path := writeFile(t, "battery = 20\nactive = true\n")
	r, err := Resolve(Options{
		Path:  path,
		Env:   env(map[string]string{"KEEPALIVE_BATTERY": "30", "KEEPALIVE_ACTIVE_IDLE": "3m"}),
		Flags: flags(t, "-b", "40", "--active-idle=4m", "--keep-display=false", "-d", "2h"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Battery != 40 || r.ActiveIdle != 4*time.Minute || r.KeepDisplay || r.Duration != 2*time.Hour {
		t.Fatalf("config = %+v", r.Config)
	}
	if !r.Active || r.Sources["active"] != SourceFile {
		t.Fatalf("unchanged flag overrode the file: active=%v from %s", r.Active, r.Sources["active"])
	}
	for _, k := range []string{"battery", "active_idle", "keep_display", "duration"} {
		if r.Sources[k] != SourceFlag {
			t.Errorf("source[%s] = %s, want flag", k, r.Sources[k])
		}
	}
}

func TestFlagValidation(t *testing.T) {
	for _, args := range [][]string{
		{"-b", "0"},
		{"-b", "101"},
		{"--active-idle", "9s"},
		{"--active-interval", "4s"},
		{"--schedule", ""},
		{"--schedule", "mon-fri 9-17"},
	} {
		if _, err := Resolve(Options{Env: env(nil), Flags: flags(t, args...)}); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
}

func TestParseDuration(t *testing.T) {
	good := map[string]time.Duration{
		"150":    150 * time.Minute,
		"1":      time.Minute,
		"2h30m":  150 * time.Minute,
		"45m":    45 * time.Minute,
		" 90m ":  90 * time.Minute,
		"1h":     time.Hour,
		"60s":    time.Minute,
		"1m0.5s": time.Minute + 500*time.Millisecond,
	}
	for in, want := range good {
		got, err := ParseDuration(in)
		if err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"0", "-5", "30s", "0m", "-1h", "abc", "", "2x", "1.5"} {
		if _, err := ParseDuration(in); err == nil {
			t.Errorf("ParseDuration(%q) accepted", in)
		}
	}
}

func TestDurationValueRoundTrips(t *testing.T) {
	var v DurationValue
	if err := v.Set("150"); err != nil {
		t.Fatal(err)
	}
	got, err := ParseDuration(v.String())
	if err != nil || got != 150*time.Minute {
		t.Fatalf("round trip = %v, %v", got, err)
	}
	if v.Type() != "duration" {
		t.Fatalf("Type() = %q", v.Type())
	}
}

func TestPathFor(t *testing.T) {
	home := func() (string, error) { return "/home/u", nil }
	get := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	tests := []struct {
		goos string
		vars map[string]string
		want string
	}{
		{"linux", nil, filepath.Join("/home/u", ".config", "keepalive", "config.toml")},
		{"darwin", nil, filepath.Join("/home/u", ".config", "keepalive", "config.toml")},
		{"darwin", map[string]string{"XDG_CONFIG_HOME": "/x"}, filepath.Join("/x", "keepalive", "config.toml")},
		{"windows", map[string]string{"APPDATA": `C:\AppData`}, filepath.Join(`C:\AppData`, "keepalive", "config.toml")},
		{"windows", map[string]string{"APPDATA": `C:\AppData`, "XDG_CONFIG_HOME": "/x"}, filepath.Join("/x", "keepalive", "config.toml")},
	}
	for _, tt := range tests {
		got, err := pathFor(tt.goos, get(tt.vars), home)
		if err != nil || got != tt.want {
			t.Errorf("pathFor(%s, %v) = %q, %v; want %q", tt.goos, tt.vars, got, err, tt.want)
		}
	}
	if _, err := pathFor("windows", get(nil), home); err == nil {
		t.Error("windows without APPDATA accepted")
	}
}

func TestInitWritesTemplateAndRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	if err := Init(path, false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file mode = %o, want 600", perm)
	}
	if err := Init(path, false); err == nil {
		t.Fatal("Init overwrote an existing file without force")
	}
	if err := Init(path, true); err != nil {
		t.Fatalf("Init with force: %v", err)
	}

	// The template as written parses to defaults ...
	r, err := Resolve(Options{Path: path, Env: env(nil)})
	if err != nil {
		t.Fatalf("template does not parse: %v", err)
	}
	if r.Config != Defaults() {
		t.Fatalf("template is not all defaults: %+v", r.Config)
	}

	// ... and every documented key parses when uncommented.
	body, _ := os.ReadFile(path)
	uncomment := regexp.MustCompile(`(?m)^# ([a-z_]+ = .*)$`)
	enabled := uncomment.ReplaceAllString(string(body), "$1")
	r, err = Resolve(Options{Path: writeFile(t, enabled), Env: env(nil)})
	if err != nil {
		t.Fatalf("uncommented template does not parse: %v\n%s", err, enabled)
	}
	for _, k := range Keys {
		if r.Sources[k] != SourceFile {
			t.Errorf("template does not document %s", k)
		}
	}
}

func TestShowAnnotatesSources(t *testing.T) {
	path := writeFile(t, "battery = 20\n")
	r, err := Resolve(Options{
		Path:  path,
		Env:   env(map[string]string{"KEEPALIVE_ACTIVE": "true"}),
		Flags: flags(t, "-d", "45m"),
	})
	if err != nil {
		t.Fatal(err)
	}
	out := r.TOML()
	for _, want := range []string{
		`duration = "45m0s"`, "# flag",
		"battery = 20", "# file",
		"active = true", "# env",
		`active_idle = "2m0s"`, "# default",
		path,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("show output missing %q:\n%s", want, out)
		}
	}
	// The output itself is valid config.
	if _, err := Resolve(Options{Path: writeFile(t, out), Env: env(nil)}); err != nil {
		t.Fatalf("show output does not parse: %v\n%s", err, out)
	}
}
