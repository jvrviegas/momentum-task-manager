package diagnostics

import (
	"os"
	"path/filepath"
)

// StateDir resolves the XDG state directory used for logs.
func StateDir(homeDir, xdgStateHome string) string {
	if xdgStateHome != "" {
		return filepath.Join(xdgStateHome, "momentum")
	}
	return filepath.Join(homeDir, ".local", "state", "momentum")
}

// ProcessStateDir resolves against the process environment.
func ProcessStateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return StateDir(home, os.Getenv("XDG_STATE_HOME")), nil
}

func LogPath(homeDir, xdgStateHome string) string {
	return filepath.Join(StateDir(homeDir, xdgStateHome), "momentum.log")
}
