package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

func (f *fakeClient) Version(context.Context) (api.Version, error) {
	return api.Version{Server: "1.4.0"}, nil
}

func (f *fakeClient) Session(context.Context) (api.Session, error) {
	return api.Session{Email: "ana@example.com", ExpiresAt: time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)}, nil
}

// failingServer is a server that answers neither the version nor the session.
type failingServer struct {
	fakeClient
	err error
}

func (f *failingServer) Version(context.Context) (api.Version, error) { return api.Version{}, f.err }
func (f *failingServer) Session(context.Context) (api.Session, error) { return api.Session{}, f.err }

func doctorOf(t *testing.T, server *Server) *doctorResult {
	t.Helper()
	result, err := call(t, server, "doctor", nil)
	if err != nil {
		t.Fatal(err)
	}
	return result.(*doctorResult)
}

func checkNamed(t *testing.T, result *doctorResult, name string) doctorCheckWire {
	t.Helper()
	for _, check := range result.Checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("no check named %q in %+v", name, result.Checks)
	return doctorCheckWire{}
}

func TestDoctorChecksTheServerTheSignInTheZoneTheConfigAndTheCalendar(t *testing.T) {
	server := newConfigFile(t, planning6h).server()
	server.Version, server.DeviceZone = "v1.2.3", "Europe/Lisbon"
	got := doctorOf(t, server)

	var names []string
	for _, check := range got.Checks {
		names = append(names, check.Name)
	}
	if strings.Join(names, "|") != "Backend server|Sign-in|Time zone|Momentum config|Calendar source" {
		t.Fatalf("checks = %v: the Terminal and State directory checks belong to the terminal app", names)
	}
	if check := checkNamed(t, got, "Backend server"); check.Status != "ok" || !strings.Contains(check.Detail, "version 1.4.0") {
		t.Errorf("server = %+v", check)
	}
	if check := checkNamed(t, got, "Sign-in"); check.Status != "ok" || !strings.Contains(check.Detail, "ana@example.com until 2026-12-01") {
		t.Errorf("sign-in = %+v", check)
	}
	if check := checkNamed(t, got, "Time zone"); check.Status != "ok" || check.Detail != "Europe/Lisbon" {
		t.Errorf("zone = %+v", check)
	}
	if check := checkNamed(t, got, "Calendar source"); check.Status != "skip" {
		t.Errorf("calendar = %+v", check)
	}
	if !got.Ready || got.BridgeVersion != "v1.2.3" || got.ConfigPath == "" {
		t.Fatalf("result = %+v", got)
	}
}

func TestDoctorAnswersBeforeAServerIsNamed(t *testing.T) {
	file := newConfigFile(t, "[planning]\ndaily_capacity = \"6h\"\n")
	got := doctorOf(t, file.server())
	check := checkNamed(t, got, "Backend server")
	if check.Status != "fail" || !strings.Contains(check.Detail, "no [backend] url") || got.Ready {
		t.Fatalf("server = %+v ready=%v", check, got.Ready)
	}
	if len(got.Checks) == 0 || got.Checks[0].Name != "Backend server" {
		t.Fatalf("checks = %+v", got.Checks)
	}
}

func TestDoctorLeavesSecretsAndEventTitlesOutOfTheReport(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ics := filepath.Join(home, "work.ics")
	meeting := "BEGIN:VCALENDAR\nBEGIN:VEVENT\nDTSTART:20261014T100000Z\nDTEND:20261014T110000Z\nSUMMARY:Secret merger talks\nEND:VEVENT\nEND:VCALENDAR\n"
	if err := os.WriteFile(ics, []byte(meeting), 0o600); err != nil {
		t.Fatal(err)
	}
	file := newConfigFile(t, backendTable+"[calendar]\nenabled = true\npaths = [\"~/work.ics\"]\n")
	server := file.server()
	server.client = &failingServer{err: errors.New("request failed: token=mtk_abc123secret")}

	got := doctorOf(t, server)
	wire, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"mtk_abc123secret", "Secret merger talks"} {
		if strings.Contains(string(wire), leaked) {
			t.Errorf("the report holds %q: %s", leaked, wire)
		}
	}
	if check := checkNamed(t, got, "Backend server"); check.Status != "fail" || !strings.Contains(check.Detail, "[REDACTED]") {
		t.Errorf("server = %+v", check)
	}
	if check := checkNamed(t, got, "Calendar source"); check.Status != "ok" || !strings.Contains(check.Detail, "1 local ICS source") {
		t.Errorf("calendar = %+v", check)
	}
}

func TestDoctorFailsTheConfigCheckWithTheFilesOwnMessage(t *testing.T) {
	file := newConfigFile(t, planning6h)
	server := file.server()
	file.write(backendTable + "[calendar]\nenabled = true\n")
	if status := statusOf(t, server); status.ConfigError == "" {
		t.Fatal("the file should not load")
	}

	got := doctorOf(t, server)
	check := checkNamed(t, got, "Momentum config")
	if check.Status != "fail" || check.Detail != server.configError || got.Ready {
		t.Fatalf("config = %+v ready=%v", check, got.Ready)
	}
}
