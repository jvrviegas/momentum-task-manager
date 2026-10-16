package diagnostics

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/taskwarrior"
)

func doctorClient(responses ...taskwarrior.CommandResult) (*taskwarrior.CommandClient, *doctorRunner) {
	runner := &doctorRunner{responses: responses}
	return taskwarrior.NewClientWithRunner("task", runner), runner
}

type doctorRunner struct {
	responses []taskwarrior.CommandResult
	calls     [][]string
}

func (runner *doctorRunner) Run(_ context.Context, command string, args ...string) (taskwarrior.CommandResult, error) {
	result := taskwarrior.CommandResult{ExitCode: 0}
	if len(runner.responses) > 0 {
		result = runner.responses[0]
		runner.responses = runner.responses[1:]
	}
	runner.calls = append(runner.calls, append([]string{command}, args...))
	if result.ExitCode != 0 {
		return result, errors.New("command failed")
	}
	return result, nil
}

func TestStateDirUsesXDGOrHomeFallback(t *testing.T) {
	if got := StateDir("/home/user", "/tmp/state"); got != filepath.Join("/tmp/state", "momentum") {
		t.Fatalf("xdg=%q", got)
	}
	if got := StateDir("/home/user", ""); got != filepath.Join("/home/user", ".local", "state", "momentum") {
		t.Fatalf("home=%q", got)
	}
}

func TestDoctorPassesReadOnlyChecks(t *testing.T) {
	client, runner := doctorClient(
		taskwarrior.CommandResult{Stdout: "Taskwarrior 3.5.0"},
		taskwarrior.CommandResult{Stdout: "[]"},
		taskwarrior.CommandResult{Stdout: "https://sync.example"},
		taskwarrior.CommandResult{Stdout: "client"},
		taskwarrior.CommandResult{Stdout: "secret"},
		taskwarrior.CommandResult{Stdout: "duration"},
	)
	stateDir := t.TempDir()
	settings := config.Defaults()
	report := Run(context.Background(), DoctorOptions{
		Client: client, Binary: "task", Config: settings, ConfigPath: filepath.Join(t.TempDir(), "config.toml"),
		StateDir: stateDir, Terminal: "xterm-256color", LookPath: func(string) (string, error) { return "/usr/bin/task", nil },
	})
	if !report.OK() {
		var output bytes.Buffer
		report.Print(&output)
		t.Fatalf("report not ready:\n%s", output.String())
	}
	for _, call := range runner.calls {
		if len(call) > 1 && (call[1] == "add" || call[1] == "done" || call[1] == "modify" || call[1] == "sync") {
			t.Fatalf("doctor attempted mutation: %#v", call)
		}
	}
}

func TestDoctorReportsMissingExecutableAndConfig(t *testing.T) {
	settings := config.Defaults()
	settings.Theme = "invalid"
	report := Run(context.Background(), DoctorOptions{
		Client: nil, Binary: "task", Config: settings, StateDir: t.TempDir(), Terminal: "",
		LookPath: func(string) (string, error) { return "", errors.New("missing") },
	})
	if report.OK() {
		t.Fatal("invalid doctor state reported ready")
	}
	var output bytes.Buffer
	report.Print(&output)
	for _, want := range []string{"Taskwarrior executable", "not found", "Momentum config", "Doctor: issues found"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output missing %q: %s", want, output.String())
		}
	}
}

func TestDoctorDoesNotPrintSyncSecret(t *testing.T) {
	client, _ := doctorClient(
		taskwarrior.CommandResult{Stdout: "Taskwarrior 3.5.0"},
		taskwarrior.CommandResult{Stdout: "[]"},
		taskwarrior.CommandResult{Stdout: "url"},
		taskwarrior.CommandResult{Stdout: "client"},
		taskwarrior.CommandResult{Stdout: "super-secret"},
		taskwarrior.CommandResult{Stdout: "duration"},
	)
	var output bytes.Buffer
	report := Run(context.Background(), DoctorOptions{Client: client, Config: config.Defaults(), StateDir: t.TempDir(), Terminal: "xterm", LookPath: func(string) (string, error) { return "task", nil }})
	report.Print(&output)
	if strings.Contains(output.String(), "super-secret") {
		t.Fatalf("secret leaked: %s", output.String())
	}
}

func TestDoctorReportsEstimateUDASetupWithoutMutating(t *testing.T) {
	client, runner := doctorClient(
		taskwarrior.CommandResult{Stdout: "Taskwarrior 3.5.0"},
		taskwarrior.CommandResult{Stdout: "[]"},
		taskwarrior.CommandResult{Stdout: ""},
	)
	settings := config.Defaults()
	settings.Sync.Enabled = false
	report := Run(context.Background(), DoctorOptions{Client: client, Config: settings, StateDir: t.TempDir(), Terminal: "xterm", LookPath: func(string) (string, error) { return "task", nil }})
	var output bytes.Buffer
	report.Print(&output)
	text := output.String()
	if !strings.Contains(text, "Estimate UDA") || !strings.Contains(text, "uda.estimate.type=duration") || !strings.Contains(text, "uda.estimate.label=Estimate") {
		t.Fatalf("output=%q", text)
	}
	for _, call := range runner.calls {
		if len(call) > 1 && (call[1] == "add" || call[1] == "modify" || call[1] == "sync") {
			t.Fatalf("doctor attempted mutation: %#v", call)
		}
	}
}

func TestDoctorWritableDirectoryHookIsCalled(t *testing.T) {
	called := ""
	report := Run(context.Background(), DoctorOptions{
		Config: config.Defaults(), StateDir: "/state", Terminal: "xterm",
		LookPath: func(string) (string, error) { return "task", nil },
		MkdirAll: func(path string, _ os.FileMode) error { called = path; return nil },
		WritableDir: func(path string) error {
			if path != "/state" {
				t.Fatalf("path=%q", path)
			}
			return nil
		},
	})
	if called != "/state" || len(report.Checks) == 0 {
		t.Fatalf("called=%q report=%#v", called, report)
	}
}

func TestReportPrintMarksSkippedChecks(t *testing.T) {
	report := Report{Checks: []Check{{Name: "optional", Skipped: true, OK: true, Detail: "not configured"}}}
	var output bytes.Buffer
	report.Print(&output)
	if !strings.Contains(output.String(), "[skip]") || !report.OK() {
		t.Fatalf("output=%q ok=%v", output.String(), report.OK())
	}
}

func TestNewLoggerDebugOffDoesNotCreateFile(t *testing.T) {
	logger, closer, path, err := NewLogger(false, LogOptions{HomeDir: t.TempDir()})
	if err != nil || logger == nil || path != "" {
		t.Fatalf("logger=%v path=%q err=%v", logger, path, err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNewLoggerWritesStructuredRecordsAndRedacts(t *testing.T) {
	var output bytes.Buffer
	logger, closer, path, err := NewLogger(true, LogOptions{Output: &output})
	if err != nil || path != "" {
		t.Fatalf("path=%q err=%v", path, err)
	}
	LogCommand(logger, "sync", taskwarrior.CommandResult{ExitCode: 1}, errors.New("encryption_secret=hidden"))
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "hidden") || !strings.Contains(output.String(), "sync") {
		t.Fatalf("log=%q", output.String())
	}
}

func TestNewLoggerUsesXDGStatePathAndMode(t *testing.T) {
	xdg := t.TempDir()
	_, closer, path, err := NewLogger(true, LogOptions{HomeDir: t.TempDir(), XDGStateHome: xdg})
	if err != nil {
		t.Fatal(err)
	}
	closer.Close()
	if !strings.HasPrefix(path, filepath.Join(xdg, "momentum")) {
		t.Fatalf("path=%q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
}

// fakeServer answers the doctor's backend checks.
type fakeServer struct {
	versionErr, sessionErr error
	sessions               int
}

func (f *fakeServer) Version(context.Context) (api.Version, error) {
	return api.Version{Server: "v1.2.3"}, f.versionErr
}

func (f *fakeServer) Session(context.Context) (api.Session, error) {
	f.sessions++
	return api.Session{Email: "ana@example.com", ExpiresAt: time.Date(2027, 1, 14, 12, 0, 0, 0, time.Local)}, f.sessionErr
}

func TestDoctorChecksTheBackendInsteadOfTaskwarrior(t *testing.T) {
	unauthorized := &api.Error{Code: api.CodeUnauthorized, Message: "sign in again"}
	unreachable := fmt.Errorf("%w: dial tcp: connection refused", api.ErrUnreachable)
	for _, test := range []struct {
		name           string
		server         *fakeServer
		credentialsErr error
		zone           string
		ok             bool
		want           []string
		sessions       int
	}{
		{name: "ready", server: &fakeServer{}, zone: "Europe/Lisbon", ok: true, sessions: 1, want: []string{
			"[ok] Backend server           https://momentum.example answers; version v1.2.3",
			"[ok] Sign-in                  ana@example.com until 2027-01-14",
			"[ok] Time zone                Europe/Lisbon",
		}},
		{name: "not signed in", server: &fakeServer{versionErr: unauthorized}, zone: "Europe/Lisbon",
			credentialsErr: errors.New("not signed in to https://momentum.example"), want: []string{
				"[ok] Backend server           https://momentum.example answers",
				"[fail] Sign-in                  not signed in to https://momentum.example",
			}},
		{name: "a revoked token", server: &fakeServer{versionErr: unauthorized, sessionErr: unauthorized}, zone: "Europe/Lisbon", sessions: 1,
			want: []string{"[fail] Sign-in                  sign in again"}},
		{name: "unreachable", server: &fakeServer{versionErr: unreachable}, zone: "Europe/Lisbon", want: []string{
			"[fail] Backend server           cannot reach the Momentum server: dial tcp: connection refused",
			"[skip] Sign-in                  needs the server",
		}},
		{name: "an unknown zone", server: &fakeServer{}, ok: false, sessions: 1,
			want: []string{"[fail] Time zone                unknown; set TZ to an IANA name such as Europe/Lisbon"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings := config.Defaults()
			settings.Backend.URL = "https://momentum.example"
			report := Run(context.Background(), DoctorOptions{
				Backend: &BackendOptions{URL: settings.Backend.URL, Server: test.server, CredentialsErr: test.credentialsErr, Zone: test.zone},
				Config:  settings, StateDir: t.TempDir(), Terminal: "xterm-256color",
				LookPath: func(string) (string, error) { return "", errors.New("missing") },
			})
			var output bytes.Buffer
			report.Print(&output)
			if report.OK() != test.ok || test.server.sessions != test.sessions {
				t.Errorf("OK %v, %d session checks:\n%s", report.OK(), test.server.sessions, output.String())
			}
			for _, line := range test.want {
				if !strings.Contains(output.String(), line) {
					t.Errorf("report lacks %q:\n%s", line, output.String())
				}
			}
			for _, check := range report.Checks {
				if strings.HasPrefix(check.Name, "Taskwarrior") || check.Name == "Sync settings" || check.Name == "Estimate UDA" {
					t.Errorf("report checks %s:\n%s", check.Name, output.String())
				}
			}
		})
	}
}
