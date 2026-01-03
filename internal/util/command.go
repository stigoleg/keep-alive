package util

import "os/exec"

// HasCommand checks if a command is available in the system PATH.
func HasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
