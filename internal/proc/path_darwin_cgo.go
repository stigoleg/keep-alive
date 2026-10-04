//go:build darwin && cgo

package proc

/*
#include <libproc.h>
*/
import "C"

import "unsafe"

// pidPath returns the full executable path of pid, or "" when the kernel
// will not say (the process exited, or it belongs to a protected process).
func pidPath(pid int) string {
	var buf [C.PROC_PIDPATHINFO_MAXSIZE]byte
	n := C.proc_pidpath(C.int(pid), unsafe.Pointer(&buf[0]), C.uint32_t(len(buf)))
	if n <= 0 {
		return ""
	}
	return string(buf[:n])
}
