package proc

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	stillActive = 259   // STILL_ACTIVE, the exit code of a running process
	waitTimeout = 0x102 // WAIT_TIMEOUT from WaitForSingleObject
)

// alive opens the process and checks that it has not exited. An exit code
// of STILL_ACTIVE is ambiguous (a process may exit with 259), so it is
// confirmed by the process handle not being signaled yet.
func alive(pid int) (bool, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	switch {
	case errors.Is(err, windows.ERROR_INVALID_PARAMETER):
		return false, nil // no such process
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return true, nil // exists, but we may not look at it
	case err != nil:
		return false, fmt.Errorf("proc: opening process %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)

	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false, fmt.Errorf("proc: checking process %d: %w", pid, err)
	}
	if code != stillActive {
		return false, nil
	}
	ev, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		return false, fmt.Errorf("proc: checking process %d: %w", pid, err)
	}
	return ev == waitTimeout, nil
}

// findByName walks a Toolhelp32 process snapshot. Executable names there are
// complete, so no prefix matching is needed.
func findByName(query string) ([]Process, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("proc: listing processes: %w", err)
	}
	defer windows.CloseHandle(snap)

	var out []Process
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		name := windows.UTF16ToString(e.ExeFile[:])
		if e.ProcessID == 0 || !nameMatches(query, name) {
			continue
		}
		out = append(out, Process{PID: int(e.ProcessID), Name: name, Path: imagePath(e.ProcessID)})
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("proc: listing processes: %w", err)
	}
	return out, nil
}

// imagePath returns the full executable path of pid, or "" if it cannot be
// read.
func imagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}
