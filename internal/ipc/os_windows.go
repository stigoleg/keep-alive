package ipc

import (
	"errors"
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

func platformBase(getenv func(string) string, cacheDir func() (string, error)) (string, error) {
	if d := getenv("LOCALAPPDATA"); d != "" {
		return d, nil
	}
	return cacheDir() // %LocalAppData% as well
}

func fallbackDir() string { return "" }

// secureDir relies on the per-user ACL that %LOCALAPPDATA% children inherit.
func secureDir(string, fs.FileInfo, bool) error { return nil }

// isPrivate is true: Windows has no mode bits to judge by.
func isPrivate(fs.FileInfo) bool { return true }

func privateSubdir() string { return "keepalive" }

func restrictSocket(string) error { return nil }

var errLocked = errors.New("lock held by another process")

// lockRegion is one byte at offset 4 GiB, past the pid text, so other
// processes can still read the pid while the lock is held.
func lockRegion() *windows.Overlapped { return &windows.Overlapped{OffsetHigh: 1} }

func acquireLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, lockRegion())
	if err != nil {
		f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, errLocked
		}
		return nil, err
	}
	return f, nil
}

func releaseLock(f *os.File) error {
	unlockErr := windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, lockRegion())
	return errors.Join(unlockErr, f.Close())
}

func notRunning(err error) bool {
	return errors.Is(err, windows.WSAECONNREFUSED) || errors.Is(err, fs.ErrNotExist)
}
