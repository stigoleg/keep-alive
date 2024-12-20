//go:build !windows
package platform

func setWindowsKeepAlive() error {
	return nil
}

func stopWindowsKeepAlive() error {
	return nil
}
