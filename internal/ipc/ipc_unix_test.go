//go:build unix

package ipc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const helperEnv = "KEEPALIVE_IPC_TEST_SERVER"

// helperServer runs in a re-executed test binary: it serves a fake controller
// until it is killed.
func helperServer() {
	srv, err := Listen(ServerInfo{Version: "helper", Origin: OriginTerminal})
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	fmt.Println("ready")
	srv.Serve(context.Background(), newFake())
	os.Exit(0)
}

func TestLockReleasedWhenServerIsKilled(t *testing.T) {
	dir := runtimeDir(t)
	noLeaks(t)
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), helperEnv+"=1", EnvRuntimeDir+"="+dir)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "ready" {
		t.Fatalf("helper: %q, %v", line, err)
	}
	child := cmd.Process.Pid

	_, err = Listen(testInfo)
	var ar *AlreadyRunningError
	if !errors.As(err, &ar) || ar.PID != child {
		t.Fatalf("Listen while helper runs = %v, want AlreadyRunningError with pid %d", err, child)
	}
	st, err := dial(t).Status(ctxTimeout(t))
	if err != nil || st.PID != child || st.Version != "helper" || st.Origin != OriginTerminal {
		t.Fatalf("status from helper = %+v, %v", st, err)
	}

	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	if _, err := os.Stat(filepath.Join(dir, SocketName)); err != nil {
		t.Fatalf("expected the killed server to leave its socket behind: %v", err)
	}
	if _, err := Dial(context.Background()); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Dial after kill = %v, want ErrNotRunning", err)
	}
	serve(t, newFake())
	if st, err := dial(t).Status(ctxTimeout(t)); err != nil || st.PID != 4242 {
		t.Fatalf("status after takeover = %+v, %v", st, err)
	}
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

// listenAndStatus starts a server and checks a client finds it.
func listenAndStatus(t *testing.T) *Server {
	t.Helper()
	srv := serve(t, newFake())
	if _, err := dial(t).Status(ctxTimeout(t)); err != nil {
		t.Fatalf("status: %v", err)
	}
	return srv
}

func TestPermissions(t *testing.T) {
	dir := runtimeDir(t) // private, from MkdirTemp
	listenAndStatus(t)
	for path, want := range map[string]os.FileMode{
		dir:                            0o700,
		filepath.Join(dir, SocketName): 0o600,
		filepath.Join(dir, LockName):   0o600,
	} {
		if got := mode(t, path); got != want {
			t.Errorf("%s mode %o, want %o", filepath.Base(path), got, want)
		}
	}
}

func TestRuntimeDirWeCreateIsPrivate(t *testing.T) {
	base := runtimeDir(t)
	dir := filepath.Join(base, "a", "rt")
	t.Setenv(EnvRuntimeDir, dir)
	listenAndStatus(t)
	if got := mode(t, dir); got != 0o700 {
		t.Fatalf("created runtime dir mode %o, want 700", got)
	}
	if _, err := os.Stat(filepath.Join(dir, SocketName)); err != nil {
		t.Fatalf("socket not directly in the directory keepalive created: %v", err)
	}
}

// TestSharedRuntimeDirIsNotChanged: KEEPALIVE_RUNTIME_DIR=$HOME (or /tmp)
// must not chmod that directory; keepalive uses a private subdirectory.
func TestSharedRuntimeDirIsNotChanged(t *testing.T) {
	dir := runtimeDir(t)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	listenAndStatus(t)
	if got := mode(t, dir); got != 0o755 {
		t.Fatalf("existing runtime dir changed to mode %o", got)
	}
	sub := filepath.Join(dir, fmt.Sprintf("keepalive-%d", os.Getuid()))
	if got := mode(t, sub); got != 0o700 {
		t.Fatalf("%s mode %o, want 700", sub, got)
	}
	for _, name := range []string{SocketName, LockName} {
		if _, err := os.Stat(filepath.Join(sub, name)); err != nil {
			t.Errorf("%s not in the private subdirectory: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s left in the shared directory", name)
		}
	}
	want, err := filepath.EvalSymlinks(sub) // macOS's temp dir is under the /var symlink
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Dir(); err != nil || got != want {
		t.Fatalf("Dir() = %q, %v; want %q", got, err, want)
	}
}

func TestSharedRuntimeDirWithAnOpenSubdirIsRefused(t *testing.T) {
	dir := runtimeDir(t)
	sub := filepath.Join(dir, fmt.Sprintf("keepalive-%d", os.Getuid()))
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if srv, err := Listen(testInfo); err == nil {
		srv.Close()
		t.Fatal("Listen used a subdirectory others can read")
	} else if !strings.Contains(err.Error(), "chmod 700") {
		t.Fatalf("error %q does not say how to fix it", err)
	}
	if got := mode(t, sub); got != 0o755 {
		t.Fatalf("subdirectory changed to mode %o", got)
	}
}

// TestRuntimeDirFollowsSymlink: KEEPALIVE_RUNTIME_DIR=/tmp on macOS names a
// symlink to the shared /private/tmp. The server and every client resolve it
// to the same private subdirectory of the real directory.
func TestRuntimeDirFollowsSymlink(t *testing.T) {
	base := runtimeDir(t)
	shared := filepath.Join(base, "s") // short: macOS's temp dir is long
	link := filepath.Join(base, "l")
	if err := os.Mkdir(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o1777); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvRuntimeDir, link)
	listenAndStatus(t)
	real, err := filepath.EvalSymlinks(shared)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(real, fmt.Sprintf("keepalive-%d", os.Getuid()))
	if got, err := Dir(); err != nil || got != want {
		t.Fatalf("Dir() = %q, %v; want %q", got, err, want)
	}
	if _, err := os.Stat(filepath.Join(want, SocketName)); err != nil {
		t.Fatalf("socket not in the private subdirectory of the link's target: %v", err)
	}
	if got := mode(t, shared); got != 0o777 {
		t.Fatalf("shared directory changed to mode %o", got)
	}
}

// TestRuntimeDirNotYetCreatedBehindSymlink: a runtime directory that does not
// exist yet resolves through its existing parents, so it is the same path
// before and after the server creates it.
func TestRuntimeDirNotYetCreatedBehindSymlink(t *testing.T) {
	base := runtimeDir(t)
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(base, "link")); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvRuntimeDir, filepath.Join(base, "link", "rt"))
	before, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	listenAndStatus(t)
	after, err := Dir()
	if err != nil || after != before {
		t.Fatalf("Dir() = %q before the server created it, %q (%v) after", before, after, err)
	}
	real, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(real, "rt"); after != want {
		t.Fatalf("Dir() = %q, want %q", after, want)
	}
}

// TestPrivateSubdirSymlinkIsRefused: the directory keepalive names itself
// inside a shared one is never followed, so another account cannot point it
// elsewhere.
func TestPrivateSubdirSymlinkIsRefused(t *testing.T) {
	dir := runtimeDir(t)
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, fmt.Sprintf("keepalive-%d", os.Getuid()))); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if srv, err := Listen(testInfo); err == nil {
		srv.Close()
		t.Fatal("Listen followed a symlinked private subdirectory")
	} else if !strings.Contains(err.Error(), "symlinks are refused") {
		t.Fatalf("error %q does not say why", err)
	}
}
