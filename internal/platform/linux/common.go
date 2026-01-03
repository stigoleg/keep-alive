//go:build linux

package linux

import (
	"bytes"
	"log"
	"os/exec"
	"strings"

	"github.com/stigoleg/keep-alive/internal/util"
)

// hasCommand checks if a command is available in the system PATH.
// This is a convenience wrapper around util.HasCommand.
func hasCommand(name string) bool {
	return util.HasCommand(name)
}

// runVerbose executes a command and returns the combined output (stdout+stderr) and any error.
func runVerbose(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return strings.TrimSpace(buf.String()), err
}

// runBestEffort executes a command and logs any errors but does not return them.
func runBestEffort(name string, args ...string) {
	if out, err := runVerbose(name, args...); err != nil {
		log.Printf("linux: best-effort command %s %s failed: %v (output: %q)", name, strings.Join(args, " "), err, out)
	}
}
