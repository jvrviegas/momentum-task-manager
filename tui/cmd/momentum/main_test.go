package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/tui/internal/app"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

func TestNewAppModelUsesTheResolvedConfigPathForEveryPathMode(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name    string
		options config.LoadOptions
	}{
		{name: "explicit", options: config.LoadOptions{PathOverride: filepath.Join(root, "explicit.toml"), Env: map[string]string{}}},
		{name: "xdg", options: config.LoadOptions{HomeDir: filepath.Join(root, "home"), XDGConfigHome: filepath.Join(root, "xdg"), Env: map[string]string{}}},
		{name: "fallback", options: config.LoadOptions{HomeDir: filepath.Join(root, "fallback-home"), Env: map[string]string{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings, resolved, err := config.LoadWithOptions(tc.options)
			if err != nil {
				t.Fatal(err)
			}
			dark := true
			model := newAppModel(nil, settings, resolved, app.ViewInbox, &dark)
			store, ok := model.ProjectStore.(*config.FileProjectCatalogStore)
			if !ok || store.Path != resolved || model.ProjectConfigPath != resolved {
				t.Fatalf("store=%#v path=%q resolved=%q", model.ProjectStore, model.ProjectConfigPath, resolved)
			}
			if _, err := os.Stat(resolved); !os.IsNotExist(err) {
				t.Fatalf("model construction touched %q: %v", resolved, err)
			}
		})
	}
}

func TestVersionCommandDoesNotLoadConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--version"}, &out, &errOut); code != 0 || strings.TrimSpace(out.String()) != Version || errOut.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestHelpCommandPrintsCLIContract(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	for _, want := range []string{"momentum doctor", "momentum login", "momentum logout", "momentum [inbox|today|completed]", "--config PATH", "--debug"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help missing %q", want)
		}
	}
}

func TestExtractCommandSupportsCommandBeforeAndAfterFlags(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{args: []string{"doctor"}, want: "doctor"},
		{args: []string{"--debug", "today", "--config", "/tmp/c.toml"}, want: "today"},
		{args: []string{"inbox", "--config=/tmp/c.toml"}, want: "inbox"},
		{args: []string{"completed"}, want: "completed"},
		{args: []string{"login", "--config", "/tmp/c.toml"}, want: "login"},
		{args: []string{"logout"}, want: "logout"},
		{args: []string{"--config", "inbox", "--help"}, want: ""},
	}
	for _, tc := range cases {
		got, _, err := extractCommand(tc.args)
		if err != nil || got != tc.want {
			t.Errorf("args=%#v got=%q err=%v", tc.args, got, err)
		}
	}
}

func TestExtractCommandRejectsMultipleCommands(t *testing.T) {
	if _, _, err := extractCommand([]string{"inbox", "today"}); err == nil {
		t.Fatal("expected multiple command error")
	}
}

func TestUnknownArgumentExitsBeforeReachingTheServer(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"custom-filter"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "unsupported argument") {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestMissingConfigAndBadConfigAreReported(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--config", "/definitely/missing/config.toml", "--version"}, &out, &errOut); code != 0 {
		t.Fatalf("version should bypass config, code=%d", code)
	}
	if code := run([]string{"--config", "/definitely/missing/config.toml", "--help"}, &out, &errOut); code != 0 {
		t.Fatalf("help should bypass config, code=%d", code)
	}
}

func TestAConfigWithoutAServerPrintsTheSetupSteps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`theme = "dark"`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"--config", path, "today"}, &out, &errOut); code != 1 || out.Len() != 0 {
		t.Fatalf("code=%d out=%q", code, out.String())
	}
	for _, want := range []string{"Add its address to " + path, "[backend]", `url = "https://momentum.example.com"`, "then run momentum login", "momentum import"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("setup steps lack %q:\n%s", want, errOut.String())
		}
	}
}

func TestUsageOutputIsStableEnoughForScripts(t *testing.T) {
	var output bytes.Buffer
	printUsage(&output)
	if !strings.HasPrefix(output.String(), "Momentum") || !strings.Contains(output.String(), "Usage:") {
		t.Fatalf("usage=%q", output.String())
	}
}
