// Command keepalive keeps the system awake. See internal/cli.
package main

import (
	"os"

	"github.com/stigoleg/keep-alive/v2/internal/cli"
)

// version is injected at build time by GoReleaser via
// `-ldflags "-X main.version=<tag>"`. Without it the module version from the
// build info is used (go install), else "dev".
var version = "dev"

func main() {
	os.Exit(cli.Main(version))
}
