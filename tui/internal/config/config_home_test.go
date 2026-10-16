package config

import (
	"os"
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

func TestLoadReadsXDGConfigHomeFromTheProcess(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	path := filepath.Join(xdg, "momentum", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("theme = \"dark\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, resolved, err := LoadWithOptions(LoadOptions{HomeDir: t.TempDir()})
	if err != nil || resolved != path || got.Theme != ThemeDark {
		t.Fatalf("path=%q theme=%q err=%v, want %q", resolved, got.Theme, err, path)
	}
	// An injected environment replaces the process's.
	if _, resolved, _ := LoadWithOptions(LoadOptions{HomeDir: "/tmp/home", Env: map[string]string{}}); resolved != filepath.Join("/tmp/home", ".config", "momentum", "config.toml") {
		t.Fatalf("with Env, path=%q", resolved)
	}
}
