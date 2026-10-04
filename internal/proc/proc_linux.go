package proc

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// commLimit is the length /proc/<pid>/comm is cut to (TASK_COMM_LEN - 1).
const commLimit = 15

// zombie reports whether /proc/<pid>/stat shows the process as a zombie or
// dead. Errors (no /proc, the process is gone) report false.
func zombie(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	// The state follows the parenthesised command name, which may itself
	// contain spaces and parentheses.
	i := bytes.LastIndexByte(b, ')')
	if i < 0 || i+2 >= len(b) {
		return false
	}
	state := b[i+2]
	return state == 'Z' || state == 'X'
}

// findByName scans /proc. Each process is matched on its comm (cut to 15
// bytes) and on the base name of /proc/<pid>/exe, which is complete but
// unreadable for other users' processes; then a cut comm matches as a prefix.
func findByName(query string) ([]Process, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("proc: listing processes: %w", err)
	}
	var out []Process
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		dir := "/proc/" + e.Name()
		b, err := os.ReadFile(dir + "/comm")
		if err != nil {
			continue // exited meanwhile
		}
		comm := strings.TrimSuffix(string(b), "\n")
		exe, _ := os.Readlink(dir + "/exe")
		exe = strings.TrimSuffix(exe, " (deleted)") // binary replaced since it started
		exeName := ""
		if exe != "" {
			exeName = filepath.Base(exe)
		}
		if !nameMatches(query, comm) && !nameMatches(query, exeName) &&
			(exe != "" || !truncatedMatches(query, comm, commLimit)) {
			continue
		}
		if zombie(pid) {
			continue
		}
		name := comm
		if exeName != "" {
			name = exeName
		}
		out = append(out, Process{PID: pid, Name: name, Path: exe})
	}
	return out, nil
}
