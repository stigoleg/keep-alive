//go:build !unix && !windows

package ipc

import (
	"errors"
	"io/fs"
	"os"
)

func platformBase(_ func(string) string, cacheDir func() (string, error)) (string, error) {
	return cacheDir()
}

func fallbackDir() string { return "" }

func secureDir(string, fs.FileInfo) error { return nil }

func restrictSocket(string) error { return nil }

var errLocked = errors.New("lock held by another process")

func acquireLock(string) (*os.File, error) { return nil, errors.ErrUnsupported }

func releaseLock(f *os.File) error { return f.Close() }

func notRunning(err error) bool { return errors.Is(err, fs.ErrNotExist) }
