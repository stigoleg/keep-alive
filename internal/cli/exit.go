package cli

import (
	"errors"
	"fmt"
	"io"
)

// Process exit codes.
const (
	ExitOK         = 0 // normal end, including user stop, SIGINT and SIGTERM
	ExitFailure    = 1 // runtime error
	ExitUsage      = 2 // usage or validation error
	ExitNoInstance = 3 // no running instance (phase 5: status/stop)
)

// ExitError carries an exit code and an optional hint. A nil Err with a
// non-zero Code exits silently with that code: `keepalive run` (phase 4)
// returns &ExitError{Code: childCode} to propagate the child's exit status.
type ExitError struct {
	Code int
	Err  error
	Hint string
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit status %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

func usageErr(err error, hint string) error {
	return &ExitError{Code: ExitUsage, Err: err, Hint: hint}
}

func runtimeErr(err error, hint string) error {
	return &ExitError{Code: ExitFailure, Err: err, Hint: hint}
}

// ExitCode maps a command error to the process exit code. Errors that are not
// an *ExitError come from cobra's argument and flag parsing, so they are
// usage errors.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return ExitUsage
}

const (
	ansiRed   = "\x1b[1;31m"
	ansiDim   = "\x1b[2m"
	ansiReset = "\x1b[0m"
)

// printError writes "keepalive: error: …" and an optional "hint: …" line.
func printError(w io.Writer, err error, color bool) {
	var ee *ExitError
	hint := ""
	if errors.As(err, &ee) {
		if ee.Err == nil {
			return
		}
		hint = ee.Hint
	} else {
		hint = "run 'keepalive --help' for usage"
	}
	label, hintLabel, reset := "error:", "hint:", ""
	if color {
		label, hintLabel, reset = ansiRed+label+ansiReset, ansiDim+hintLabel, ansiReset
	}
	fmt.Fprintf(w, "keepalive: %s %s\n", label, err.Error())
	if hint != "" {
		fmt.Fprintf(w, "%s %s%s\n", hintLabel, hint, reset)
	}
}
