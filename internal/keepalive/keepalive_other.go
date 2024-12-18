//go:build !windows

package keepalive

// For non-Windows platforms, these are no-ops.
func setWindowsKeepAlive() error {
	return nil
}

func stopWindowsKeepAlive() error {
	return nil
}
