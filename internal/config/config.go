// Package config resolves keepalive settings from defaults, the config file,
// KEEPALIVE_* environment variables and command-line flags, in increasing
// order of precedence.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/pflag"

	"github.com/stigoleg/keep-alive/v2/internal/activity"
)

// Config is the effective configuration.
type Config struct {
	Duration       time.Duration // 0 = none
	Battery        int           // 0 = none
	Active         bool
	ActiveIdle     time.Duration
	ActiveInterval time.Duration
	ActiveKeys     bool
	Schedule       string
	KeepDisplay    bool
	Notify         bool
	Log            bool
	LogFile        string
}

// Defaults returns the built-in configuration.
func Defaults() Config {
	return Config{
		ActiveIdle:     activity.DefaultIdleThreshold,
		ActiveInterval: activity.DefaultInterval,
		KeepDisplay:    true,
	}
}

// Source says which layer a value came from.
type Source string

const (
	SourceDefault Source = "default"
	SourceFile    Source = "file"
	SourceEnv     Source = "env"
	SourceFlag    Source = "flag"
)

// MinDuration is the shortest session duration accepted.
const MinDuration = time.Minute

// Keys lists the config keys in display order.
var Keys = []string{
	"duration", "battery", "active", "active_idle", "active_interval", "active_keys",
	"schedule", "keep_display", "notify", "log", "log_file",
}

// FlagName maps a config key to its command-line flag.
func FlagName(key string) string { return strings.ReplaceAll(key, "_", "-") }

// EnvName maps a config key to its environment variable.
func EnvName(key string) string { return "KEEPALIVE_" + strings.ToUpper(key) }

// Resolved is the effective configuration plus where each value came from.
type Resolved struct {
	Config
	Sources   map[string]Source
	Path      string // config file consulted ("" when none)
	FileFound bool
}

// Options controls Resolve.
type Options struct {
	// Path is the config file to read; "" skips the file layer.
	Path string
	// PathExplicit makes a missing file an error (the user passed --config).
	PathExplicit bool
	// Env looks up environment variables; nil means os.LookupEnv.
	Env func(string) (string, bool)
	// Flags supplies the flag layer: only flags whose Changed is true apply.
	Flags *pflag.FlagSet
}

// Error is a configuration error; the CLI reports it as a usage error.
type Error struct {
	Source string // "--battery", "KEEPALIVE_BATTERY", "config file /x"
	Err    error
}

func (e *Error) Error() string { return e.Source + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Resolve layers defaults, file, env and flags.
func Resolve(o Options) (Resolved, error) {
	r := Resolved{Config: Defaults(), Sources: map[string]Source{}, Path: o.Path}
	for _, k := range Keys {
		r.Sources[k] = SourceDefault
	}

	if o.Path != "" {
		data, err := os.ReadFile(o.Path)
		switch {
		case err == nil:
			r.FileFound = true
			if err := r.applyFile(data); err != nil {
				return r, &Error{Source: "config file " + o.Path, Err: err}
			}
		case errors.Is(err, fs.ErrNotExist) && !o.PathExplicit:
		default:
			return r, &Error{Source: "config file", Err: err}
		}
	}

	lookup := o.Env
	if lookup == nil {
		lookup = os.LookupEnv
	}
	for _, k := range Keys {
		raw, ok := lookup(EnvName(k))
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		if err := setKey(&r.Config, k, raw); err != nil {
			return r, &Error{Source: EnvName(k), Err: err}
		}
		r.Sources[k] = SourceEnv
	}

	if o.Flags != nil {
		for _, k := range Keys {
			f := o.Flags.Lookup(FlagName(k))
			if f == nil || !f.Changed {
				continue
			}
			if err := setKey(&r.Config, k, f.Value.String()); err != nil {
				return r, &Error{Source: "--" + f.Name, Err: err}
			}
			r.Sources[k] = SourceFlag
		}
	}
	return r, nil
}

// applyFile decodes TOML and applies every key, rejecting unknown keys and
// values of the wrong type.
func (r *Resolved) applyFile(data []byte) error {
	var doc map[string]any
	if err := toml.Unmarshal(data, &doc); err != nil {
		return err
	}
	for k, v := range doc {
		kind, ok := keyKinds[k]
		if !ok {
			return fmt.Errorf("unknown key %q (valid keys: %s)", k, strings.Join(Keys, ", "))
		}
		raw, err := fileValue(kind, v)
		if err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
		if err := setKey(&r.Config, k, raw); err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
		r.Sources[k] = SourceFile
	}
	return nil
}

type kind int

const (
	kindSessionDuration kind = iota // minutes or a Go duration
	kindDuration
	kindInt
	kindBool
	kindString
)

var keyKinds = map[string]kind{
	"duration":        kindSessionDuration,
	"battery":         kindInt,
	"active":          kindBool,
	"active_idle":     kindDuration,
	"active_interval": kindDuration,
	"active_keys":     kindBool,
	"schedule":        kindString,
	"keep_display":    kindBool,
	"notify":          kindBool,
	"log":             kindBool,
	"log_file":        kindString,
}

// fileValue turns a decoded TOML value into the raw string setKey parses,
// checking it has the TOML type the key expects.
func fileValue(k kind, v any) (string, error) {
	switch k {
	case kindSessionDuration:
		switch x := v.(type) {
		case string:
			return x, nil
		case int64:
			return strconv.FormatInt(x, 10), nil
		}
		return "", errors.New(`want minutes (150) or a duration string ("2h30m")`)
	case kindDuration:
		if x, ok := v.(string); ok {
			return x, nil
		}
		return "", errors.New(`want a duration string such as "90s"`)
	case kindInt:
		if x, ok := v.(int64); ok {
			return strconv.FormatInt(x, 10), nil
		}
		return "", errors.New("want an integer")
	case kindBool:
		if x, ok := v.(bool); ok {
			return strconv.FormatBool(x), nil
		}
		return "", errors.New("want true or false")
	default:
		if x, ok := v.(string); ok {
			return x, nil
		}
		return "", errors.New("want a string")
	}
}

// setKey parses raw for key k and stores it in c. All layers go through it,
// so validation is identical for file, env and flags.
func setKey(c *Config, k, raw string) error {
	raw = strings.TrimSpace(raw)
	switch k {
	case "duration":
		if raw == "" {
			c.Duration = 0
			return nil
		}
		d, err := ParseDuration(raw)
		if err != nil {
			return err
		}
		c.Duration = d
	case "battery":
		n, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("invalid battery percentage %q", raw)
		}
		if n < 1 || n > 100 {
			return fmt.Errorf("battery threshold must be between 1 and 100 (got %d)", n)
		}
		c.Battery = n
	case "active_idle":
		d, err := parseMin(raw, activity.MinIdleThreshold)
		if err != nil {
			return err
		}
		c.ActiveIdle = d
	case "active_interval":
		d, err := parseMin(raw, activity.MinInterval)
		if err != nil {
			return err
		}
		c.ActiveInterval = d
	case "schedule":
		if raw == "" {
			return errors.New("schedule must not be empty")
		}
		c.Schedule = raw
	case "log_file":
		c.LogFile = expandHome(raw)
	case "active", "active_keys", "keep_display", "notify", "log":
		b, err := parseBool(raw)
		if err != nil {
			return err
		}
		switch k {
		case "active":
			c.Active = b
		case "active_keys":
			c.ActiveKeys = b
		case "keep_display":
			c.KeepDisplay = b
		case "notify":
			c.Notify = b
		case "log":
			c.Log = b
		}
	default:
		return fmt.Errorf("unknown key %q", k)
	}
	return nil
}

// ParseDuration parses a session duration: a bare integer is minutes, anything
// else a Go duration ("2h30m", "45m"). It must be at least one minute.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	var d time.Duration
	if n, err := strconv.Atoi(s); err == nil {
		d = time.Duration(n) * time.Minute
	} else {
		parsed, perr := time.ParseDuration(s)
		if perr != nil {
			return 0, fmt.Errorf("invalid duration %q: use minutes (150) or a duration such as 2h30m or 45m", s)
		}
		d = parsed
	}
	if d < MinDuration {
		return 0, fmt.Errorf("duration must be at least 1m (got %s)", s)
	}
	return d, nil
}

func parseMin(raw string, min time.Duration) (time.Duration, error) {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: use a duration such as 90s or 2m", raw)
	}
	if d < min {
		return 0, fmt.Errorf("must be at least %s (got %s)", min, raw)
	}
	return d, nil
}

func parseBool(raw string) (bool, error) {
	switch strings.ToLower(raw) {
	case "yes", "on":
		return true, nil
	case "no", "off":
		return false, nil
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid boolean %q: use true or false", raw)
	}
	return b, nil
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// DurationValue is a pflag.Value for --duration that validates while flags
// are parsed.
type DurationValue time.Duration

func (d *DurationValue) Set(s string) error {
	v, err := ParseDuration(s)
	if err != nil {
		return err
	}
	*d = DurationValue(v)
	return nil
}

func (d *DurationValue) String() string {
	if *d == 0 {
		return ""
	}
	return time.Duration(*d).String()
}

func (d *DurationValue) Type() string { return "duration" }

// DefaultPath returns the config file location for this OS:
// $XDG_CONFIG_HOME/keepalive/config.toml when XDG_CONFIG_HOME is set, else
// ~/.config/keepalive/config.toml (macOS, Linux) or
// %APPDATA%\keepalive\config.toml (Windows).
func DefaultPath() (string, error) {
	return pathFor(runtime.GOOS, os.Getenv, os.UserHomeDir)
}

func pathFor(goos string, getenv func(string) string, home func() (string, error)) (string, error) {
	if x := getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "keepalive", "config.toml"), nil
	}
	if goos == "windows" {
		if a := getenv("APPDATA"); a != "" {
			return filepath.Join(a, "keepalive", "config.toml"), nil
		}
		return "", errors.New("cannot locate the config directory: APPDATA is not set")
	}
	h, err := home()
	if err != nil {
		return "", fmt.Errorf("cannot locate the config directory: %w", err)
	}
	return filepath.Join(h, ".config", "keepalive", "config.toml"), nil
}

// ErrExists is returned by Init when the file exists and force is false.
var ErrExists = errors.New("config file already exists")

// Init writes the commented template to path, creating parent directories.
func Init(path string, force bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%w: %s", ErrExists, path)
	}
	if err != nil {
		return err
	}
	if force {
		_ = f.Chmod(0o600)
	}
	if _, err := f.WriteString(Template); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
