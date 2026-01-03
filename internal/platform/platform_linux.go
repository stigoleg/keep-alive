//go:build linux

package platform

import (
	"context"

	"github.com/stigoleg/keep-alive/internal/platform/linux"
)

// linuxKeepAlive wraps the linux.KeepAlive implementation.
type linuxKeepAlive struct {
	impl *linux.KeepAlive
}

// NewKeepAlive creates a new platform-specific keep-alive instance.
func NewKeepAlive() (KeepAlive, error) {
	impl, err := linux.NewKeepAlive()
	if err != nil {
		return nil, err
	}
	return &linuxKeepAlive{impl: impl}, nil
}

func (k *linuxKeepAlive) Start(ctx context.Context) error {
	return k.impl.Start(ctx)
}

func (k *linuxKeepAlive) Stop() error {
	return k.impl.Stop()
}

func (k *linuxKeepAlive) SetSimulateActivity(simulate bool) {
	k.impl.SetSimulateActivity(simulate)
}

// GetDependencyMessage returns the formatted dependency message if dependencies are missing.
func GetDependencyMessage() string {
	return linux.GetDependencyMessage()
}

// CheckActivitySimulationCapability checks if the platform can simulate user activity.
// On Linux, this checks for uinput access or availability of ydotool/xdotool.
func CheckActivitySimulationCapability() SimulationCapability {
	linuxCap := linux.CheckActivitySimulationCapability()
	return SimulationCapability{
		CanSimulate:  linuxCap.CanSimulate,
		ErrorMessage: linuxCap.ErrorMessage,
		Instructions: linuxCap.Instructions,
		CanPrompt:    linuxCap.CanPrompt,
	}
}

// PromptActivitySimulationPermission is a no-op on Linux.
func PromptActivitySimulationPermission() {
	linux.PromptActivitySimulationPermission()
}
