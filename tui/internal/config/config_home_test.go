package config

import (
	"path/filepath"
	"testing"
)

func TestLoadOptionsCanInjectHomeThroughEnvironment(t *testing.T) {
	_, path, err := LoadWithOptions(LoadOptions{Env: map[string]string{"HOME": "/tmp/home", "XDG_CONFIG_HOME": ""}})
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join("/tmp/home", ".config", "momentum", "config.toml") {
		t.Fatalf("path=%q", path)
	}
}
