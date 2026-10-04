// Package logging configures the process-wide slog logger. Logging is off
// unless enabled; when on it writes to a file, never to the working
// directory.
package logging

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// MaxSize is the size above which the log is rotated when it is opened.
const MaxSize = 5 << 20

// Options controls Setup.
type Options struct {
	Enabled bool   // write a log file at all
	Debug   bool   // debug level instead of info
	Path    string // "" means DefaultPath()
}

// Setup installs the default slog logger. The standard log package is routed
// into it as well (slog.SetDefault does that), so legacy log.Printf calls
// follow the same destination. It returns the log file path ("" when
// disabled) and a func that closes the file.
func Setup(o Options) (string, func() error, error) {
	if !o.Enabled {
		slog.SetDefault(slog.New(slog.DiscardHandler))
		return "", func() error { return nil }, nil
	}
	path := o.Path
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return "", nil, err
		}
		path = p
	}
	f, err := open(path)
	if err != nil {
		return "", nil, err
	}
	level := slog.LevelInfo
	if o.Debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: level})))
	return path, f.Close, nil
}

// DefaultPath is keepalive/keepalive.log in the user cache directory.
func DefaultPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "keepalive", "keepalive.log"), nil
}

// open creates the directory (0700) and file (0600), rotating an oversized
// file to path.1 first and keeping only that one.
func open(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if info, err := os.Stat(path); err == nil && info.Size() > MaxSize {
		if err := os.Rename(path, path+".1"); err != nil {
			return nil, err
		}
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}
