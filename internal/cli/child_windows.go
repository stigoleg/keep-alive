//go:build windows

package cli

import "os"

// runSignals are handled by `keepalive run`.
func runSignals() []os.Signal { return []os.Signal{os.Interrupt} }

// forwardSignal never forwards on Windows: Ctrl+C reaches the child through
// the console they share, so keepalive just waits for it.
func forwardSignal(os.Signal) bool { return false }

func childExitCode(ps *os.ProcessState) int { return ps.ExitCode() }

func signalExitCode(os.Signal) int { return 130 } // like cmd.exe after Ctrl+C
