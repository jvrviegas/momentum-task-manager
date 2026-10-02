package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/internal/app"
	"github.com/jvrviegas/momentum-task-manager/internal/config"
	"github.com/jvrviegas/momentum-task-manager/internal/taskwarrior"
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

func TestVersionCommandDoesNotLoadConfigOrTaskwarrior(t *testing.T) {
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
	for _, want := range []string{"momentum doctor", "momentum [inbox|today|completed]", "--config PATH", "--debug"} {
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

func TestUnknownArgumentExitsBeforeTaskwarrior(t *testing.T) {
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

func TestValidateTaskwarriorAcceptsVersionThree(t *testing.T) {
	client := taskwarrior.NewClientWithRunner("task", taskwarrior.RunnerFunc(func(context.Context, string, ...string) (taskwarrior.CommandResult, error) {
		return taskwarrior.CommandResult{ExitCode: 0, Stdout: "Taskwarrior 3.5.0"}, nil
	}))
	if err := validateTaskwarriorWithLookPath(context.Background(), client, func(string) (string, error) { return "/usr/bin/task", nil }); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTaskwarriorRejectsVersionTwo(t *testing.T) {
	client := taskwarrior.NewClientWithRunner("task", taskwarrior.RunnerFunc(func(context.Context, string, ...string) (taskwarrior.CommandResult, error) {
		return taskwarrior.CommandResult{ExitCode: 0, Stdout: "Taskwarrior 2.6.2"}, nil
	}))
	if err := validateTaskwarriorWithLookPath(context.Background(), client, func(string) (string, error) { return "/usr/bin/task", nil }); err == nil || !strings.Contains(err.Error(), "3.x") {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateTaskwarriorRejectsMissingExecutable(t *testing.T) {
	client := taskwarrior.NewClientWithRunner("momentum-no-task-binary", taskwarrior.RunnerFunc(func(context.Context, string, ...string) (taskwarrior.CommandResult, error) {
		t.Fatal("runner should not be called when executable is missing")
		return taskwarrior.CommandResult{}, nil
	}))
	if err := validateTaskwarrior(context.Background(), client); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateTaskwarriorRejectsVersionCommandFailure(t *testing.T) {
	client := taskwarrior.NewClientWithRunner("task", taskwarrior.RunnerFunc(func(context.Context, string, ...string) (taskwarrior.CommandResult, error) {
		return taskwarrior.CommandResult{ExitCode: 1, Stderr: "failed"}, errors.New("failed")
	}))
	if err := validateTaskwarriorWithLookPath(context.Background(), client, func(string) (string, error) { return "/usr/bin/task", nil }); err == nil || !strings.Contains(err.Error(), "cannot run") {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateTaskwarriorRejectsUnparseableVersion(t *testing.T) {
	client := taskwarrior.NewClientWithRunner("task", taskwarrior.RunnerFunc(func(context.Context, string, ...string) (taskwarrior.CommandResult, error) {
		return taskwarrior.CommandResult{ExitCode: 0, Stdout: "Taskwarrior development"}, nil
	}))
	if err := validateTaskwarriorWithLookPath(context.Background(), client, func(string) (string, error) { return "/usr/bin/task", nil }); err == nil || !strings.Contains(err.Error(), "determine") {
		t.Fatalf("err=%v", err)
	}
}

func TestUsageOutputIsStableEnoughForScripts(t *testing.T) {
	var output bytes.Buffer
	printUsage(&output)
	if !strings.HasPrefix(output.String(), "Momentum") || !strings.Contains(output.String(), "Usage:") {
		t.Fatalf("usage=%q", output.String())
	}
}
