package proc

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestMain doubles as a helper process: with PROC_TEST_HELPER set, the test
// binary exits at once or sleeps instead of running the tests.
func TestMain(m *testing.M) {
	switch os.Getenv("PROC_TEST_HELPER") {
	case "exit":
		os.Exit(0)
	case "sleep":
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func helper(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), "PROC_TEST_HELPER="+mode)
	return cmd
}

// waitDead polls Alive until it reports false or the timeout passes.
func waitDead(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		ok, err := Alive(pid)
		if err != nil {
			t.Fatalf("Alive(%d): %v", pid, err)
		}
		if !ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Alive(%d) still true after the process exited", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAliveSelf(t *testing.T) {
	ok, err := Alive(os.Getpid())
	if err != nil || !ok {
		t.Fatalf("Alive(own pid) = %v, %v; want true", ok, err)
	}
}

func TestAliveExitedChild(t *testing.T) {
	cmd := helper(t, "exit")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	ok, err := Alive(cmd.Process.Pid)
	if err != nil || ok {
		t.Fatalf("Alive(reaped child) = %v, %v; want false", ok, err)
	}
}

// TestAliveUnreapedChild checks that a child that has exited but was not
// waited for (a zombie on unix, a signaled handle on Windows) is not alive.
func TestAliveUnreapedChild(t *testing.T) {
	cmd := helper(t, "exit")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Wait() })
	waitDead(t, cmd.Process.Pid)
}

func TestAliveRunningChild(t *testing.T) {
	cmd := helper(t, "sleep")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	ok, err := Alive(cmd.Process.Pid)
	if err != nil || !ok {
		t.Fatalf("Alive(running child) = %v, %v; want true", ok, err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	waitDead(t, cmd.Process.Pid)
}

func TestAliveBogusPID(t *testing.T) {
	ok, err := Alive(99999999)
	if err != nil || ok {
		t.Fatalf("Alive(99999999) = %v, %v; want false, nil", ok, err)
	}
	for _, pid := range []int{0, -1} {
		if _, err := Alive(pid); err == nil {
			t.Fatalf("Alive(%d) gave no error", pid)
		}
	}
}

func TestFindByNameSelf(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Base(exe) // proc.test, proc.test.exe on Windows
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	for _, name := range []string{base, strings.ToUpper(base), stem} {
		procs, err := FindByName(name)
		if err != nil {
			t.Fatalf("FindByName(%q): %v", name, err)
		}
		i := slices.IndexFunc(procs, func(p Process) bool { return p.PID == os.Getpid() })
		if i < 0 {
			t.Fatalf("FindByName(%q) = %v, does not include this process (%d)", name, procs, os.Getpid())
		}
		if p := procs[i]; !strings.EqualFold(p.Name, base) && !strings.HasPrefix(strings.ToLower(base), strings.ToLower(p.Name)) {
			t.Errorf("FindByName(%q) names this process %q, want %q", name, p.Name, base)
		}
	}
}

func TestFindByNameNoMatch(t *testing.T) {
	procs, err := FindByName("no-such-process-keepalive-test")
	if err != nil || len(procs) != 0 {
		t.Fatalf("FindByName(nonsense) = %v, %v; want none", procs, err)
	}
	if _, err := FindByName(""); err == nil {
		t.Fatal("FindByName(\"\") gave no error")
	}
}

func TestNameMatches(t *testing.T) {
	tests := []struct {
		query, name string
		want        bool
	}{
		{"zoom", "zoom", true},
		{"zoom", "Zoom", true},
		{"ZOOM", "zoom.us", true},
		{"zoom", "zoom.exe", true},
		{"zoom.exe", "zoom.exe", true},
		{"Zoom.EXE", "zoom.exe", true},
		{"zoom.us", "zoom.us", true},
		{"zoom", "zoomer", false},
		{"zoom", "zoom.us.helper", false}, // only the last extension is dropped
		{"zoom.exe", "zoom", false},
		{"zoom.exe", "zoom.us", false},
		{"zoo", "zoom", false},
		{"zoom", "", false},
		{"bashrc", ".bashrc", false},
	}
	for _, tt := range tests {
		if got := nameMatches(tt.query, tt.name); got != tt.want {
			t.Errorf("nameMatches(%q, %q) = %v, want %v", tt.query, tt.name, got, tt.want)
		}
	}
}

func TestTruncatedMatches(t *testing.T) {
	tests := []struct {
		query, comm string
		limit       int
		want        bool
	}{
		{"Google Chrome Helper", "Google Chrome He", 16, true},
		{"google chrome helper", "Google Chrome He", 16, true},
		{"Google Chrome Helper", "Google Chrome He", 15, false}, // not cut at the limit
		{"Google Chrome", "Google Chrome He", 16, false},
		{"Google Chrome Hat", "Google Chrome He", 16, false},
		{"short", "short", 16, false}, // not truncated; nameMatches handles it
		{"gnome-shell-calendar", "gnome-shell-cal", 15, true},
	}
	for _, tt := range tests {
		if got := truncatedMatches(tt.query, tt.comm, tt.limit); got != tt.want {
			t.Errorf("truncatedMatches(%q, %q, %d) = %v, want %v", tt.query, tt.comm, tt.limit, got, tt.want)
		}
	}
}
