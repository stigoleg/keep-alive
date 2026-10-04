package cli

import (
	"runtime/debug"
	"strings"
)

// ResolveVersion returns the version to report: the -X main.version value when
// set, else the main module version from the build info (go install), else
// "dev".
func ResolveVersion(v string) string {
	if v != "" && v != "dev" {
		return strings.TrimPrefix(v, "v")
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if mv := bi.Main.Version; mv != "" && mv != "(devel)" {
			return strings.TrimPrefix(mv, "v")
		}
	}
	return "dev"
}
