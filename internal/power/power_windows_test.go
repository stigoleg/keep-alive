package power

import (
	"testing"
	"unsafe"
)

// REASON_CONTEXT is {ULONG Version; DWORD Flags; union{...} Reason}. The
// union's Detailed member is {HMODULE; ULONG; ULONG; LPWSTR*}, so on 64-bit
// targets the struct is 32 bytes with the union at offset 8, and on 32-bit
// targets 24 bytes with the union at offset 8.
func TestReasonContextLayout(t *testing.T) {
	var rc reasonContext
	want := uintptr(32)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		want = 24
	}
	if got := unsafe.Sizeof(rc); got != want {
		t.Errorf("sizeof(REASON_CONTEXT) = %d, want %d", got, want)
	}
	if got := unsafe.Offsetof(rc.Flags); got != 4 {
		t.Errorf("offsetof(Flags) = %d, want 4", got)
	}
	if got := unsafe.Offsetof(rc.SimpleReasonString); got != 8 {
		t.Errorf("offsetof(Reason) = %d, want 8", got)
	}
}

func TestPowerRequestRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("holds a real power request")
	}
	for _, display := range []bool{false, true} {
		h, err := New().Acquire(t.Context(), Options{KeepDisplay: display, Reason: "test"})
		if err != nil {
			t.Fatalf("Acquire(display=%v): %v", display, err)
		}
		want := "PowerRequest(SystemRequired)"
		if display {
			want = "PowerRequest(SystemRequired, DisplayRequired)"
		}
		if got := h.Describe(); got != want {
			t.Errorf("Describe() = %q, want %q", got, want)
		}
		if err := h.Release(); err != nil {
			t.Fatalf("Release: %v", err)
		}
		if err := h.Release(); err != nil {
			t.Fatalf("second Release: %v", err)
		}
	}
}
