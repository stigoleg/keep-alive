package ipc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// maxSocketPath is sizeof(sun_path); the path plus its NUL must fit.
const maxSocketPath = len(syscall.RawSockaddrUnix{}.Path)

// Dir returns the directory holding the socket and the lock file. It does not
// create it.
func Dir() (string, error) {
	dir, err := resolveDir(os.Getenv, os.UserCacheDir)
	if err != nil {
		return "", err
	}
	return privateDir(dir)
}

// privateDir is dir, or a private keepalive-<uid> subdirectory when dir
// already exists and other users may read it (KEEPALIVE_RUNTIME_DIR=$HOME,
// /tmp): a directory keepalive did not create is never chmodded.
func privateDir(dir string) (string, error) {
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() || isPrivate(fi) {
		return dir, nil // ensureDir creates it or reports the problem
	}
	sub := filepath.Join(dir, privateSubdir())
	if !fits(sub) {
		return "", fmt.Errorf("ipc: %s is shared with other users and %s would make the socket path too long (max %d bytes); set %s to a private directory",
			dir, privateSubdir(), maxSocketPath-1, EnvRuntimeDir)
	}
	return sub, nil
}

// SocketPath returns the control socket's path.
func SocketPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, SocketName), nil
}

func resolveDir(getenv func(string) string, cacheDir func() (string, error)) (string, error) {
	if dir := getenv(EnvRuntimeDir); dir != "" {
		if !fits(dir) {
			return "", fmt.Errorf("ipc: %s=%s: socket path too long (max %d bytes)", EnvRuntimeDir, dir, maxSocketPath-1)
		}
		return dir, nil
	}
	base, err := platformBase(getenv, cacheDir)
	if err != nil {
		return "", fmt.Errorf("ipc: no directory for the control socket: %w", err)
	}
	dir := filepath.Join(base, "keepalive")
	if fits(dir) {
		return dir, nil
	}
	if fb := fallbackDir(); fb != "" {
		return fb, nil
	}
	return "", fmt.Errorf("ipc: socket path in %s too long (max %d bytes); set %s", dir, maxSocketPath-1, EnvRuntimeDir)
}

func fits(dir string) bool { return len(filepath.Join(dir, SocketName)) < maxSocketPath }

// ensureDir creates dir (0700) and refuses anything but a real, private
// directory owned by this user, so another account cannot pre-create it.
// Only a directory it creates itself is chmodded.
func ensureDir(dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return fmt.Errorf("ipc: %w", err)
	}
	created := true
	if err := os.Mkdir(dir, 0o700); errors.Is(err, fs.ErrExist) {
		created = false
	} else if err != nil {
		return fmt.Errorf("ipc: %w", err)
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("ipc: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("ipc: %s is not a directory (symlinks are refused)", dir)
	}
	if err := secureDir(dir, fi, created); err != nil {
		return fmt.Errorf("ipc: %s: %w", dir, err)
	}
	return nil
}
