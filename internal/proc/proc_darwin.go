package proc

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const (
	// commLimit is the length of p_comm (MAXCOMLEN).
	commLimit = 16
	// sZomb is the p_stat value of a zombie (SZOMB in <sys/proc.h>).
	sZomb = 5
)

// zombie asks the kernel for the process state. Errors report false.
func zombie(pid int) bool {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	return err == nil && kp.Proc.P_pid == int32(pid) && kp.Proc.P_stat == sZomb
}

// findByName lists processes with sysctl kern.proc.all. p_comm is cut to 16
// bytes, so candidates are confirmed against the full executable path where
// pidPath can resolve it; otherwise a cut name matches as a prefix.
func findByName(query string) ([]Process, error) {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, fmt.Errorf("proc: listing processes: %w", err)
	}
	var out []Process
	for i := range kps {
		p := &kps[i].Proc
		if p.P_stat == sZomb {
			continue
		}
		pid := int(p.P_pid)
		comm := unix.ByteSliceToString(p.P_comm[:])
		exact := nameMatches(query, comm)
		cut := truncatedMatches(query, comm, commLimit)
		if !exact && !cut {
			continue
		}
		path := pidPath(pid)
		name := comm
		if path != "" {
			name = filepath.Base(path)
			if !exact && !nameMatches(query, name) {
				continue // the full name shows the prefix match was wrong
			}
		}
		out = append(out, Process{PID: pid, Name: name, Path: path})
	}
	return out, nil
}
