//go:build !darwin && !linux && !windows

package service

import (
	"errors"
	"fmt"
	"runtime"
)

func newManager() (Manager, error) {
	return nil, fmt.Errorf("service: %w on %s", errors.ErrUnsupported, runtime.GOOS)
}
