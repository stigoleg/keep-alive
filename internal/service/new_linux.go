package service

import (
	"fmt"
	"os"
	"path/filepath"
)

func newManager() (Manager, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(dir) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("service: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return &systemd{run: execRunner{timeout: CommandTimeout}, configHome: dir}, nil
}
