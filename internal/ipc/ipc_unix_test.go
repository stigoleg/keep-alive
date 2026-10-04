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

func TestPermissions(t *testing.T) {
	dir := runtimeDir(t)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	srv, err := Listen(testInfo)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	for path, want := range map[string]os.FileMode{
		dir:                            0o700,
		filepath.Join(dir, SocketName): 0o600,
		filepath.Join(dir, LockName):   0o600,
	} {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s mode %o, want %o", filepath.Base(path), got, want)
		}
	}
}

func TestRuntimeDirRejectsSymlink(t *testing.T) {
	base := runtimeDir(t)
	target := filepath.Join(base, "target")
	link := filepath.Join(base, "link")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvRuntimeDir, link)
	if srv, err := Listen(testInfo); err == nil {
		srv.Close()
		t.Fatal("Listen accepted a symlinked runtime directory")
	}
}
