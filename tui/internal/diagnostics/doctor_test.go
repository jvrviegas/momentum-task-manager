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
)

func TestStateDirUsesXDGOrHomeFallback(t *testing.T) {
	if got := StateDir("/home/user", "/tmp/state"); got != filepath.Join("/tmp/state", "momentum") {
		t.Fatalf("xdg=%q", got)
	}
	if got := StateDir("/home/user", ""); got != filepath.Join("/home/user", ".local", "state", "momentum") {
		t.Fatalf("home=%q", got)
	}
}

func TestDoctorReportsMissingServerAndConfig(t *testing.T) {
	settings := config.Defaults()
	settings.Theme = "invalid"
	report := Run(context.Background(), DoctorOptions{Config: settings, ConfigPath: "/home/ana/.config/momentum/config.toml", StateDir: t.TempDir(), Terminal: ""})
	if report.OK() {
		t.Fatal("invalid doctor state reported ready")
	}
	var output bytes.Buffer
	report.Print(&output)
	for _, want := range []string{"[fail] Backend server", "no [backend] url in /home/ana/.config/momentum/config.toml", "run momentum login", "Momentum config", "Doctor: issues found"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output missing %q: %s", want, output.String())
		}
	}
}

func TestDoctorDoesNotPrintSecrets(t *testing.T) {
	server := &fakeServer{versionErr: errors.New("refused token=super-secret at https://ana:hunter2@momentum.example")}
	report := Run(context.Background(), DoctorOptions{
		Backend: &BackendOptions{URL: "https://momentum.example", Server: server, Zone: "Europe/Lisbon"},
		Config:  config.Defaults(), StateDir: t.TempDir(), Terminal: "xterm",
	})
	var output bytes.Buffer
	report.Print(&output)
	if strings.Contains(output.String(), "super-secret") || strings.Contains(output.String(), "hunter2") {
		t.Fatalf("secret leaked: %s", output.String())
	}
}

func TestDoctorWritableDirectoryHookIsCalled(t *testing.T) {
	called := ""
	report := Run(context.Background(), DoctorOptions{
		Config: config.Defaults(), StateDir: "/state", Terminal: "xterm",
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

func TestNewLoggerWritesStructuredRecords(t *testing.T) {
	var output bytes.Buffer
	logger, closer, path, err := NewLogger(true, LogOptions{Output: &output})
	if err != nil || path != "" {
		t.Fatalf("path=%q err=%v", path, err)
	}
	LogTransition(logger, "loading", "ready", "initial")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "state transition") || !strings.Contains(output.String(), "from=loading") {
		t.Fatalf("log=%q", output.String())
	}
}

func TestRedactCoversSecretsAndURLCredentials(t *testing.T) {
	for _, value := range []string{"encryption_secret=hidden", "token: hidden", "url https://user:hidden@example.test/path"} {
		if got := Redact(value); strings.Contains(got, "hidden") || !strings.Contains(got, "REDACTED") {
			t.Errorf("Redact(%q) = %q", value, got)
		}
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

func TestDoctorChecksTheServer(t *testing.T) {
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
		})
	}
}
