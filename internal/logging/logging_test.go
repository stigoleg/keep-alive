package logging

import (
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDisabledDiscardsEverything(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path, closeFn, err := Setup(Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	if path != "" {
		t.Fatalf("path = %q, want empty when disabled", path)
	}
	slog.Error("nothing")
	log.Printf("legacy line")
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("disabled logging wrote files: %v", entries)
	}
}

func TestEnabledWritesDebugAndLegacyLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "keepalive.log")
	got, closeFn, err := Setup(Options{Enabled: true, Debug: true, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("path = %q, want %q", got, path)
	}
	slog.Debug("debug line")
	log.Printf("legacy line")
	if err := closeFn(); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"debug line", "legacy line"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("log missing %q:\n%s", want, body)
		}
	}
	if runtime.GOOS != "windows" {
		assertMode(t, path, 0o600)
		assertMode(t, filepath.Dir(path), 0o700)
	}
}

func TestInfoLevelDropsDebug(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keepalive.log")
	_, closeFn, err := Setup(Options{Enabled: true, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	slog.Debug("hidden")
	slog.Info("shown")
	closeFn()
	body, _ := os.ReadFile(path)
	if strings.Contains(string(body), "hidden") || !strings.Contains(string(body), "shown") {
		t.Fatalf("unexpected log body:\n%s", body)
	}
}

func TestRotatesLargeFileAtOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keepalive.log")
	big := make([]byte, MaxSize+1)
	if err := os.WriteFile(path, big, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".1", []byte("older"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, closeFn, err := Setup(Options{Enabled: true, Path: path})
	if err != nil {
		t.Fatal(err)
	}
	closeFn()
	if info, err := os.Stat(path + ".1"); err != nil || info.Size() != int64(len(big)) {
		t.Fatalf("rotated file: %v, %v", info, err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() >= MaxSize {
		t.Fatalf("new log file not fresh: %v, %v", info, err)
	}
}

func TestDefaultPathUsesCacheDir(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Skipf("no cache dir: %v", err)
	}
	cache, _ := os.UserCacheDir()
	if want := filepath.Join(cache, "keepalive", "keepalive.log"); path != want {
		t.Fatalf("DefaultPath() = %q, want %q", path, want)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %o, want %o", path, got, want)
	}
}
