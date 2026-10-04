package activity

import "math"

// virtualDeskAbsolute maps a point on the Windows virtual desktop to the
// 0..65535 range SendInput expects with MOUSEEVENTF_ABSOLUTE |
// MOUSEEVENTF_VIRTUALDESK, using the integer formula Chrome Remote Desktop
// uses. desk is the virtual screen (SM_XVIRTUALSCREEN..SM_CYVIRTUALSCREEN),
// whose origin is negative when a monitor sits left of or above the primary.
func virtualDeskAbsolute(x, y float64, desk Rect) (int32, int32) {
	return normalizeAxis(x, desk.X, desk.W), normalizeAxis(y, desk.Y, desk.H)
}

func normalizeAxis(v, origin, size float64) int32 {
	span := int64(size) - 1
	if span <= 0 {
		return 0
	}
	n := (int64(math.Round(v)) - int64(origin)) * 65535 / span
	return int32(min(max(n, 0), 65535))
}
