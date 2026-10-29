package diagnostics

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// googleServer is a server that can also say where the account's Google
// Calendar stands.
type googleServer struct {
	fakeServer
	status api.GoogleCalendarStatus
	err    error
	asked  int
}

func (g *googleServer) GoogleCalendarStatus(context.Context) (api.GoogleCalendarStatus, error) {
	g.asked++
	return g.status, g.err
}

func googleSettings(enabled bool) config.Config {
	settings := config.Defaults()
	settings.Calendar.Source = config.CalendarSourceGoogle
	settings.Calendar.Enabled = enabled
	return settings
}

// calendarCheck runs the doctor and returns its "Calendar source" check and
// the whole report. mac is the macOS app's doctor, which has no terminal.
func calendarCheck(t *testing.T, server interface {
	Version(context.Context) (api.Version, error)
	Session(context.Context) (api.Session, error)
}, settings config.Config, mac bool, credentialsErr error) (Check, Report) {
	t.Helper()
	report := Run(context.Background(), DoctorOptions{
		Backend: &BackendOptions{URL: "https://momentum.example", Server: server, Zone: "Europe/Lisbon", CredentialsErr: credentialsErr},
		Config:  settings, StateDir: t.TempDir(), Terminal: "xterm", NoTerminal: mac,
	})
	for _, check := range report.Checks {
		if check.Name == "Calendar source" {
			return check, report
		}
	}
	t.Fatalf("no Calendar source check in %+v", report.Checks)
	return Check{}, report
}

func TestGoogleCalendarPassesWhenItIsConnected(t *testing.T) {
	server := &googleServer{status: api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "joao@gmail.com"}}
	for _, mac := range []bool{true, false} {
		check, report := calendarCheck(t, server, googleSettings(true), mac, nil)
		if !check.OK || check.Skipped || check.Detail != "Google Calendar connected as joao@gmail.com; read-only" || !report.OK() {
			t.Errorf("mac %v: check = %+v, ready %v", mac, check, report.OK())
		}
	}
}

func TestGoogleCalendarFailsWithItsReasonAndWhereToFixItOnTheMac(t *testing.T) {
	for _, test := range []struct {
		name          string
		status        api.GoogleCalendarStatus
		mac, terminal string
	}{
		{"no connection", api.GoogleCalendarStatus{Offered: true},
			"Google Calendar isn’t connected. Connect it in Settings ▸ Calendar.", "Google Calendar isn’t connected. Run momentum calendar connect to connect it."},
		{"a connection Google refuses", api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "joao@gmail.com", NeedsReconnect: true},
			"Google Calendar needs reconnecting. Reconnect it in Settings ▸ Calendar.", "Google Calendar needs reconnecting. Run momentum calendar connect to reconnect it."},
		{"a server that offers nothing", api.GoogleCalendarStatus{},
			"Google Calendar isn’t available on this server. Ask whoever runs the server to turn it on.",
			"Google Calendar isn’t available on this server. Ask whoever runs the server to turn it on."},
	} {
		for mac, want := range map[bool]string{true: test.mac, false: test.terminal} {
			check, report := calendarCheck(t, &googleServer{status: test.status}, googleSettings(true), mac, nil)
			if check.OK || check.Skipped || check.Detail != want || report.OK() {
				t.Errorf("%s, mac %v: check = %+v, want a Problem saying %q", test.name, mac, check, want)
			}
		}
	}
}

// A Google source counts whatever [calendar] enabled says, and needs no
// files (ADR 0003, D9).
func TestGoogleCalendarIsCheckedWhateverEnabledSays(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		server := &googleServer{status: api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "joao@gmail.com"}}
		check, _ := calendarCheck(t, server, googleSettings(enabled), true, nil)
		if !check.OK || server.asked != 1 {
			t.Errorf("enabled %v: check = %+v, %d questions", enabled, check, server.asked)
		}
	}
}

func TestGoogleCalendarIsSkippedWithoutAServerOrASignIn(t *testing.T) {
	status := api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "joao@gmail.com"}
	unauthorized := &api.Error{Code: api.CodeUnauthorized, Message: "sign in again"}
	for _, test := range []struct {
		name           string
		server         *googleServer
		credentialsErr error
		want           string
	}{
		{"the server does not answer", &googleServer{fakeServer: fakeServer{versionErr: errors.New("connection refused")}, status: status}, nil, "needs the server"},
		{"the sign-in is refused", &googleServer{fakeServer: fakeServer{sessionErr: unauthorized}, status: status}, nil, "needs a sign-in"},
		{"there is no saved sign-in", &googleServer{status: status}, errors.New("no saved sign-in"), "needs a sign-in"},
	} {
		check, report := calendarCheck(t, test.server, googleSettings(true), true, test.credentialsErr)
		if !check.Skipped || check.Detail != test.want || test.server.asked != 0 {
			t.Errorf("%s: check = %+v, %d questions; want Skipped with %q and the server not asked", test.name, check, test.server.asked, test.want)
		}
		// The check that failed is the one that fails the report, not this.
		if report.OK() {
			t.Errorf("%s: the report is ready", test.name)
		}
	}

	// No [backend] server at all.
	report := Run(context.Background(), DoctorOptions{Config: googleSettings(true), StateDir: t.TempDir(), Terminal: "xterm", NoTerminal: true})
	for _, check := range report.Checks {
		if check.Name == "Calendar source" && (!check.Skipped || check.Detail != "needs the server") {
			t.Errorf("without a [backend] url: %+v", check)
		}
	}
}

func TestAGoogleCheckThatCannotBeAnsweredIsAProblemWithoutSecrets(t *testing.T) {
	server := &googleServer{err: errors.New("the server answered 500 token=super-secret")}
	check, _ := calendarCheck(t, server, googleSettings(true), true, nil)
	if check.OK || check.Skipped || !strings.HasPrefix(check.Detail, "Unable to check Google Calendar: ") || strings.Contains(check.Detail, "super-secret") {
		t.Errorf("check = %+v", check)
	}
}

func TestAServerClientThatCannotSayWhereGoogleStandsIsSkipped(t *testing.T) {
	check, _ := calendarCheck(t, &fakeServer{}, googleSettings(true), true, nil)
	if !check.Skipped || check.OK && check.Detail == "" {
		t.Errorf("check = %+v", check)
	}
}

func TestTheFilesSourceIsCheckedAsBeforeAndNeverAsksGoogle(t *testing.T) {
	server := &googleServer{status: api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "joao@gmail.com"}}
	check, _ := calendarCheck(t, server, config.Defaults(), true, nil)
	if !check.Skipped || check.Detail != "disabled; task-only planning remains available" || server.asked != 0 {
		t.Errorf("check = %+v, %d questions", check, server.asked)
	}
}

// What the report prints holds the account's address and nothing else of
// Google's: no link, token or event title.
func TestTheReportHoldsNoLinkTokenOrEventTitle(t *testing.T) {
	server := &googleServer{status: api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "joao@gmail.com"}}
	_, report := calendarCheck(t, server, googleSettings(true), false, nil)
	var output bytes.Buffer
	report.Print(&output)
	for _, secret := range []string{"mtk_", "/calendar/connect/", "refresh", "access_token"} {
		if strings.Contains(output.String(), secret) {
			t.Errorf("the report holds %q:\n%s", secret, output.String())
		}
	}
	if !strings.Contains(output.String(), "[ok] Calendar source") {
		t.Errorf("the report lacks the check:\n%s", output.String())
	}
}
