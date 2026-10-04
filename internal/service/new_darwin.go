package service

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func newManager() (Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("service: %w", err)
	}
	return &launchd{
		run:   execRunner{timeout: CommandTimeout},
		uid:   os.Getuid(),
		dir:   filepath.Join(home, "Library", "LaunchAgents"),
		sleep: time.Sleep,
	}, nil
}
