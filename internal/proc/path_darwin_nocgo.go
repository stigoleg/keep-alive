//go:build darwin && !cgo

package proc

// pidPath needs proc_pidpath from libproc, which requires cgo. Without it,
// names are matched on the 16-byte p_comm alone.
func pidPath(int) string { return "" }
