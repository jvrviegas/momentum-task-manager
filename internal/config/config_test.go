package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaults(t *testing.T) {
	got := Defaults()
	if got.RefreshInterval != time.Minute || got.Theme != ThemeAuto || got.Icons != IconsUnicode {
		t.Fatalf("unexpected defaults: %#v", got)
	}
	if !got.Sync.Enabled || got.Sync.Interval != 5*time.Minute || got.Sync.MutationDelay != 15*time.Second || !got.Sync.Startup || !got.Sync.Shutdown {
		t.Fatalf("unexpected sync defaults: %#v", got.Sync)
	}
}

func TestResolvePathOverrideWins(t *testing.T) {
	got := ResolvePathFor("/custom/momentum.toml", "/home/user", "/xdg")
	if got != "/custom/momentum.toml" {
		t.Fatalf("got %q", got)
	}
}

func TestResolvePathUsesXDGConfigHome(t *testing.T) {
	got := ResolvePathFor("", "/home/user", "/tmp/config")
	want := filepath.Join("/tmp/config", "momentum", "config.toml")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolvePathFallsBackToHomeConfig(t *testing.T) {
	got := ResolvePathFor("", "/home/user", "")
	want := filepath.Join("/home/user", ".config", "momentum", "config.toml")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestMissingConfigUsesDefaultsWithoutReadingUserConfig(t *testing.T) {
	got, path, err := LoadWithOptions(LoadOptions{PathOverride: filepath.Join(t.TempDir(), "missing.toml"), HomeDir: filepath.Join(t.TempDir(), "other-home")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "missing.toml") || !reflect.DeepEqual(got, Defaults()) {
		t.Fatalf("path/config mismatch: %q %#v", path, got)
	}
}

func TestLoadsTOMLAndDurations(t *testing.T) {
	path := writeConfig(t, `refresh_interval = "0s"
theme = "light"
icons = "ascii"
[sync]
enabled = false
interval = "7m"
mutation_delay = "2s"
startup = false
shutdown = false
`)
	got, _, err := LoadWithOptions(LoadOptions{PathOverride: path, Env: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if got.RefreshInterval != 0 || got.Theme != ThemeLight || got.Icons != IconsASCII || got.Sync.Enabled || got.Sync.Interval != 7*time.Minute || got.Sync.MutationDelay != 2*time.Second || got.Sync.Startup || got.Sync.Shutdown {
		t.Fatalf("unexpected config: %#v", got)
	}
}

func TestExplicitPathOutranksXDG(t *testing.T) {
	path := writeConfig(t, `theme = "dark"`)
	got, resolved, err := LoadWithOptions(LoadOptions{PathOverride: path, HomeDir: t.TempDir(), XDGConfigHome: t.TempDir(), Env: map[string]string{}})
	if err != nil || resolved != path || got.Theme != ThemeDark {
		t.Fatalf("got config=%#v path=%q err=%v", got, resolved, err)
	}
}

func TestUnknownTopLevelKeyIsRejected(t *testing.T) {
	path := writeConfig(t, `mispelled = true`)
	_, _, err := LoadWithOptions(LoadOptions{PathOverride: path, Env: map[string]string{}})
	if err == nil || !strings.Contains(err.Error(), "mispelled") {
		t.Fatalf("expected actionable unknown-key error, got %v", err)
	}
}

func TestUnknownNestedKeyIsRejected(t *testing.T) {
	path := writeConfig(t, `[sync]
mutaton_delay = "1s"
`)
	_, _, err := LoadWithOptions(LoadOptions{PathOverride: path, Env: map[string]string{}})
	if err == nil || !strings.Contains(err.Error(), "sync.mutaton_delay") {
		t.Fatalf("expected nested unknown-key error, got %v", err)
	}
}

func TestInvalidThemeAndIconValuesAreRejected(t *testing.T) {
	for _, content := range []string{`theme = "blue"`, `icons = "emoji"`} {
		path := writeConfig(t, content)
		_, _, err := LoadWithOptions(LoadOptions{PathOverride: path, Env: map[string]string{}})
		if err == nil || !strings.Contains(err.Error(), "config key") {
			t.Errorf("content %q: expected validation error, got %v", content, err)
		}
	}
}

func TestInvalidDurationsAreRejected(t *testing.T) {
	for _, content := range []string{`refresh_interval = "-1s"`, `[sync]
interval = "0s"`, `[sync]
mutation_delay = "-1s"`} {
		path := writeConfig(t, content)
		_, _, err := LoadWithOptions(LoadOptions{PathOverride: path, Env: map[string]string{}})
		if err == nil || !strings.Contains(err.Error(), "duration") {
			t.Errorf("content %q: expected duration error, got %v", content, err)
		}
	}
}

func TestIconEnvironmentOverrideWins(t *testing.T) {
	path := writeConfig(t, `icons = "unicode"`)
	got, _, err := LoadWithOptions(LoadOptions{PathOverride: path, Env: map[string]string{"MOMENTUM_ICONS": "nerd"}})
	if err != nil || got.Icons != IconsNerd {
		t.Fatalf("got %#v err=%v", got, err)
	}
}

func TestInvalidIconEnvironmentOverrideIsRejected(t *testing.T) {
	path := writeConfig(t, ``)
	_, _, err := LoadWithOptions(LoadOptions{PathOverride: path, Env: map[string]string{"MOMENTUM_ICONS": "bad"}})
	if err == nil || !strings.Contains(err.Error(), "icons") {
		t.Fatalf("expected icon validation error, got %v", err)
	}
}
