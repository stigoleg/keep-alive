package ipc

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// maxSocketPath is sizeof(sun_path); the path plus its NUL must fit.
const maxSocketPath = len(syscall.RawSockaddrUnix{}.Path)

// Dir returns the directory holding the socket and the lock file. It does not
// create it.
func Dir() (string, error) { return resolveDir(os.Getenv, os.UserCacheDir) }

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

// ensureDir creates dir (0700) and refuses anything but a real directory
// owned by this user, so another account cannot pre-create it.
func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("ipc: %w", err)
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("ipc: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("ipc: %s is not a directory (symlinks are refused)", dir)
	}
	if err := secureDir(dir, fi); err != nil {
		return fmt.Errorf("ipc: %s: %w", dir, err)
	}
	return nil
}
