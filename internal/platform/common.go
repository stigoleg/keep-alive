//go:build darwin || linux || windows

package platform

import "github.com/stigoleg/keep-alive/internal/util"

// hasCommand checks if a command is available in the system PATH.
// This is a convenience wrapper around util.HasCommand.
func hasCommand(name string) bool {
	return util.HasCommand(name)
}
