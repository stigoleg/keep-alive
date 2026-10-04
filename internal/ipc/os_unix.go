//go:build unix

package ipc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

func platformBase(getenv func(string) string, cacheDir func() (string, error)) (string, error) {
	if d := getenv("XDG_RUNTIME_DIR"); d != "" && filepath.IsAbs(d) {
		return d, nil
	}
	return cacheDir()
}

// fallbackDir is used when the regular path is too long for sun_path. It is
// deliberately /tmp, not os.TempDir: macOS's per-user TMPDIR is long too.
func fallbackDir() string { return "/tmp/keepalive-" + strconv.Itoa(os.Getuid()) }

func secureDir(dir string, fi fs.FileInfo) error {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("owned by uid %d, not %d", st.Uid, os.Getuid())
	}
	if fi.Mode().Perm() != 0o700 {
		return os.Chmod(dir, 0o700)
	}
	return nil
}

func restrictSocket(path string) error { return os.Chmod(path, 0o600) }

var errLocked = errors.New("lock held by another process")

func acquireLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, errLocked
		}
		return nil, err
	}
	if err := f.Chmod(0o600); err != nil {
		releaseLock(f)
		return nil, err
	}
	return f, nil
}

// releaseLock unlocks explicitly; closing the descriptor would do it too.
func releaseLock(f *os.File) error {
	unlockErr := unix.Flock(int(f.Fd()), unix.LOCK_UN)
	return errors.Join(unlockErr, f.Close())
}

func notRunning(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ENOENT) || errors.Is(err, fs.ErrNotExist)
}
