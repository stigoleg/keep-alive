//go:build !linux && !darwin && !windows

package proc

import "errors"

func alive(int) (bool, error) { return false, errors.ErrUnsupported }

func findByName(string) ([]Process, error) { return nil, errors.ErrUnsupported }
