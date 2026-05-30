//go:build linux

package platform

import (
	"strings"
	"testing"
)

func TestLinuxActivitySimulationStatusUsesRealBackends(t *testing.T) {
	tests := []struct {
		name      string
		caps      linuxCapabilities
		hasUinput bool
		want      string
	}{
		{
			name:      "uinput preferred",
			caps:      linuxCapabilities{displayServer: displayServerWayland, ydotoolAvailable: true},
			hasUinput: true,
			want:      "uinput",
		},
		{
			name: "ydotool works on wayland",
			caps: linuxCapabilities{
				displayServer:    displayServerWayland,
				ydotoolAvailable: true,
			},
			want: "ydotool",
		},
		{
			name: "xdotool works only on x11",
			caps: linuxCapabilities{
				displayServer:    displayServerX11,
				xdotoolAvailable: true,
			},
			want: "xdotool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := linuxActivitySimulationStatus(tt.caps, tt.hasUinput)
			if !status.Available {
				t.Fatalf("status.Available = false, want true: %s", status.Message)
			}
			if status.Method != tt.want {
				t.Fatalf("status.Method = %q, want %q", status.Method, tt.want)
			}
		})
	}
}

func TestLinuxActivitySimulationStatusRejectsSoftFallback(t *testing.T) {
	status := linuxActivitySimulationStatus(linuxCapabilities{
		displayServer:     displayServerWayland,
		gdbusAvailable:    true,
		dbusSendAvailable: true,
		wtypeAvailable:    true,
	}, false)

	if status.Available {
		t.Fatalf("status.Available = true, want false")
	}
	if !strings.Contains(status.Message, "no real Linux mouse input backend") {
		t.Fatalf("status.Message = %q, want real backend warning", status.Message)
	}
}
