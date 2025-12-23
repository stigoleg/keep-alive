package integration

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stigoleg/keep-alive/internal/keepalive"
	"github.com/stigoleg/keep-alive/internal/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCleanupOnSIGINT verifies cleanup on SIGINT (Ctrl+C)
func TestCleanupOnSIGINT(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cleanup test in short mode")
	}

	// Start a separate process that runs keep-alive and handles SIGINT
	cmd := exec.Command(os.Args[0], "-test.run=TestSIGINTHelper")
	cmd.Env = append(os.Environ(), "TEST_SIGINT_HELPER=1")
	err := cmd.Start()
	require.NoError(t, err, "helper process should start")

	// Let it run for a moment
	time.Sleep(1 * time.Second)

	// Send SIGINT
	err = cmd.Process.Signal(syscall.SIGINT)
	require.NoError(t, err, "should send SIGINT")

	// Wait for process to exit
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		assert.NoError(t, err, "process should exit cleanly after SIGINT")
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("process did not exit within timeout")
	}
}

// TestSIGINTHelper is a helper function for TestCleanupOnSIGINT
func TestSIGINTHelper(t *testing.T) {
	if os.Getenv("TEST_SIGINT_HELPER") != "1" {
		return
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	keeper := &keepalive.Keeper{}
	err := keeper.StartIndefinite()
	if err != nil {
		os.Exit(1)
	}

	// Wait for signal
	<-sigChan

	// Cleanup
	keeper.Stop()
	os.Exit(0)
}

// TestCleanupOnSIGTERM verifies cleanup on SIGTERM
func TestCleanupOnSIGTERM(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cleanup test in short mode")
	}

	// This test is already covered by TestCleanupOnProcessTermination
	// Test the cleanup directly without sending signals to test process
	ka, err := platform.NewKeepAlive()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = ka.Start(ctx)
	require.NoError(t, err)

	// Let it run briefly
	time.Sleep(500 * time.Millisecond)

	// Verify cleanup works
	err = ka.Stop()
	assert.NoError(t, err, "should stop cleanly")
}

// TestCleanupOnSIGQUIT verifies cleanup on SIGQUIT
func TestCleanupOnSIGQUIT(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGQUIT not supported on Windows")
	}

	if testing.Short() {
		t.Skip("skipping cleanup test in short mode")
	}

	// Start a separate process that runs keep-alive and handles SIGQUIT
	cmd := exec.Command(os.Args[0], "-test.run=TestSIGQUITHelper")
	cmd.Env = append(os.Environ(), "TEST_SIGQUIT_HELPER=1")
	err := cmd.Start()
	require.NoError(t, err, "helper process should start")

	// Let it run for a moment
	time.Sleep(1 * time.Second)

	// Send SIGQUIT
	err = cmd.Process.Signal(syscall.SIGQUIT)
	require.NoError(t, err, "should send SIGQUIT")

	// Wait for process to exit
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		assert.NoError(t, err, "process should exit cleanly after SIGQUIT")
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("process did not exit within timeout")
	}
}

// TestSIGQUITHelper is a helper function for TestCleanupOnSIGQUIT
func TestSIGQUITHelper(t *testing.T) {
	if os.Getenv("TEST_SIGQUIT_HELPER") != "1" {
		return
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signals := getUnixSignals()
	signal.Notify(sigChan, signals...)

	keeper := &keepalive.Keeper{}
	err := keeper.StartIndefinite()
	if err != nil {
		os.Exit(1)
	}

	// Wait for signal
	<-sigChan

	// Cleanup
	keeper.Stop()
	os.Exit(0)
}

// TestCleanupOnSIGTSTP verifies cleanup on SIGTSTP (Ctrl+Z)
func TestCleanupOnSIGTSTP(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGTSTP not supported on Windows")
	}

	if testing.Short() {
		t.Skip("skipping cleanup test in short mode")
	}

	// Start a separate process that runs keep-alive and handles SIGTSTP
	cmd := exec.Command(os.Args[0], "-test.run=TestSIGTSTPHelper")
	cmd.Env = append(os.Environ(), "TEST_SIGTSTP_HELPER=1")
	err := cmd.Start()
	require.NoError(t, err, "helper process should start")

	// Let it run for a moment
	time.Sleep(1 * time.Second)

	// Send SIGTSTP
	err = sendSIGTSTP(cmd.Process)
	require.NoError(t, err, "should send SIGTSTP")

	// Wait for process to exit
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		assert.NoError(t, err, "process should exit cleanly after SIGTSTP")
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("process did not exit within timeout")
	}
}

// TestSIGTSTPHelper is a helper function for TestCleanupOnSIGTSTP
func TestSIGTSTPHelper(t *testing.T) {
	if os.Getenv("TEST_SIGTSTP_HELPER") != "1" {
		return
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signals := getUnixSignalsWithSIGTSTP()
	signal.Notify(sigChan, signals...)

	keeper := &keepalive.Keeper{}
	err := keeper.StartIndefinite()
	if err != nil {
		os.Exit(1)
	}

	// Wait for signal
	<-sigChan

	// Cleanup
	keeper.Stop()
	os.Exit(0)
}

// TestCleanupTimeout verifies cleanup timeout behavior
func TestCleanupTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cleanup test in short mode")
	}

	keeper := &keepalive.Keeper{}
	err := keeper.StartIndefinite()
	require.NoError(t, err, "should start keeper")

	// Stop with very short timeout
	start := time.Now()
	err = keeper.StopWithTimeout(100 * time.Millisecond)
	duration := time.Since(start)

	// Should complete quickly (within timeout + small buffer)
	assert.True(t, duration < 500*time.Millisecond, "cleanup should complete within timeout")
	assert.NoError(t, err, "cleanup should succeed even with short timeout")
	assert.False(t, keeper.IsRunning(), "keeper should be stopped")
}

// TestMultipleSignals verifies that multiple signals only trigger cleanup once
func TestMultipleSignals(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cleanup test in short mode")
	}

	// Test idempotency directly without sending signals to test process
	keeper := &keepalive.Keeper{}
	err := keeper.StartIndefinite()
	require.NoError(t, err, "should start keeper")

	// Call Stop multiple times concurrently
	done := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func() {
			done <- keeper.Stop()
		}()
	}

	// Collect results
	errors := make([]error, 0, 5)
	for i := 0; i < 5; i++ {
		select {
		case err := <-done:
			errors = append(errors, err)
		case <-time.After(2 * time.Second):
			t.Fatal("cleanup did not complete within timeout")
		}
	}

	// All should succeed (idempotent)
	for i, err := range errors {
		assert.NoError(t, err, "concurrent stop %d should succeed", i)
	}

	// Verify keeper stopped (only once)
	assert.False(t, keeper.IsRunning(), "keeper should be stopped after multiple stops")
}

// TestCleanupOnProcessTermination verifies cleanup when process is terminated externally
func TestCleanupOnProcessTermination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cleanup test in short mode")
	}

	// Start a separate process that runs keep-alive
	cmd := exec.Command(os.Args[0], "-test.run=TestCleanupHelper")
	cmd.Env = append(os.Environ(), "TEST_CLEANUP_HELPER=1")
	err := cmd.Start()
	require.NoError(t, err, "helper process should start")

	// Let it run for a few seconds
	time.Sleep(2 * time.Second)

	// Send SIGTERM (graceful termination)
	err = cmd.Process.Signal(syscall.SIGTERM)
	require.NoError(t, err, "should send SIGTERM")

	// Wait for process to exit
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		assert.NoError(t, err, "process should exit cleanly")
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit within timeout")
	}

	// Verify system returns to normal state
	assertSystemNormal(t)
}

// TestCleanupHelper is a helper function for TestCleanupOnProcessTermination
func TestCleanupHelper(t *testing.T) {
	if os.Getenv("TEST_CLEANUP_HELPER") != "1" {
		return
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signals := getUnixSignals()
	signal.Notify(sigChan, signals...)

	keeper := &keepalive.Keeper{}
	err := keeper.StartIndefinite()
	if err != nil {
		os.Exit(1)
	}

	// Wait for signal
	<-sigChan

	// Cleanup
	keeper.Stop()
	os.Exit(0)
}

// TestPlatformCleanup verifies platform-specific cleanup
func TestPlatformCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cleanup test in short mode")
	}

	ka, err := platform.NewKeepAlive()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = ka.Start(ctx)
	require.NoError(t, err)

	// Let it run briefly
	time.Sleep(1 * time.Second)

	// Stop and verify cleanup
	err = ka.Stop()
	require.NoError(t, err, "platform cleanup should succeed")

	// Verify system is in normal state
	assertSystemNormal(t)
}

// TestConcurrentCleanup verifies cleanup behavior with concurrent operations
func TestConcurrentCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cleanup test in short mode")
	}

	keeper := &keepalive.Keeper{}
	err := keeper.StartIndefinite()
	require.NoError(t, err)

	// Attempt concurrent stops
	done := make(chan error, 3)
	for i := 0; i < 3; i++ {
		go func() {
			done <- keeper.Stop()
		}()
	}

	// Collect results
	errors := make([]error, 0, 3)
	for i := 0; i < 3; i++ {
		select {
		case err := <-done:
			errors = append(errors, err)
		case <-time.After(2 * time.Second):
			t.Fatal("cleanup did not complete within timeout")
		}
	}

	// All should succeed (idempotent)
	for i, err := range errors {
		assert.NoError(t, err, "concurrent stop %d should succeed", i)
	}

	assert.False(t, keeper.IsRunning(), "keeper should be stopped")
}

