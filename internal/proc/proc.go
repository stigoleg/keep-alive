// Package proc checks whether processes are alive, finds processes by name,
// and watches them so a session can keep the system awake while they run.
//
// # Name matching
//
// A name matches a process when it equals the executable's base name or that
// name without its last extension, ignoring case: "zoom" matches "zoom",
// "Zoom", "zoom.us" and "zoom.exe"; "zoom.exe" matches "zoom.exe" only. A
// path is not accepted; give the executable name.
//
// The kernel truncates process names (16 bytes on macOS, 15 on Linux). Each
// platform resolves the full executable path where it can (proc_pidpath on
// macOS with cgo, /proc/<pid>/exe on Linux) and otherwise accepts a truncated
// name that is a prefix of the query.
package proc

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Process is one running process.
type Process struct {
	PID  int
	Name string // executable base name, possibly truncated by the kernel when Path is empty
	Path string // full executable path, empty when it could not be resolved
}

// Lister looks up processes. System returns the real one; tests inject fakes
// into a Watcher.
type Lister interface {
	Alive(pid int) (bool, error)
	FindByName(name string) ([]Process, error)
}

// System returns the Lister backed by this operating system.
func System() Lister { return system{} }

type system struct{}

func (system) Alive(pid int) (bool, error)               { return Alive(pid) }
func (system) FindByName(name string) ([]Process, error) { return FindByName(name) }

// Alive reports whether a process with this pid exists. On unix a process
// owned by another user (EPERM) counts as alive and a zombie (exited, not yet
// reaped by its parent) does not.
func Alive(pid int) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("proc: invalid process ID %d", pid)
	}
	return alive(pid)
}

// FindByName returns every process whose executable name matches name (see
// the package documentation). No match is an empty result, not an error.
func FindByName(name string) ([]Process, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("proc: empty process name")
	}
	return findByName(name)
}

// nameMatches reports whether query names the executable called name: equal
// to it or to it without its last extension, ignoring case.
func nameMatches(query, name string) bool {
	if name == "" {
		return false
	}
	if strings.EqualFold(query, name) {
		return true
	}
	ext := filepath.Ext(name)
	return ext != "" && ext != name && strings.EqualFold(query, strings.TrimSuffix(name, ext))
}

// truncatedMatches reports whether comm, a kernel process name cut to limit
// bytes, is the start of query.
func truncatedMatches(query, comm string, limit int) bool {
	return len(comm) == limit && len(query) > limit && strings.EqualFold(query[:limit], comm)
}
