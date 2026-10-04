//go:build !windows

package service

import "syscall"

func sysProcAttr() *syscall.SysProcAttr { return nil }
