//go:build linux || darwin

package proc

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// alive sends signal 0: success or EPERM means the pid exists. A zombie
// still has a pid but has exited, so it is reported as not alive.
func alive(pid int) (bool, error) {
	err := unix.Kill(pid, 0)
	switch {
	case err == nil, errors.Is(err, unix.EPERM):
		return !zombie(pid), nil
	case errors.Is(err, unix.ESRCH):
		return false, nil
	default:
		return false, fmt.Errorf("proc: checking process %d: %w", pid, err)
	}
}
