//go:build windows

package cli

import (
	"os"
	"os/exec"
)

// runSignals are handled by `keepalive run`.
func runSignals() []os.Signal { return []os.Signal{os.Interrupt} }

// commandSignals never forwards on Windows: Ctrl+C reaches the command
// through the console they share, so keepalive just waits for it.
type commandSignals struct{}

func newCommandSignals(*exec.Cmd) *commandSignals { return &commandSignals{} }

func (*commandSignals) forward(*os.Process, os.Signal) bool { return false }

func childExitCode(ps *os.ProcessState) int { return ps.ExitCode() }

func signalExitCode(os.Signal) int { return 130 } // like cmd.exe after Ctrl+C
