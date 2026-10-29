package bridge

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// googleDoctor runs the Mac's doctor for a config whose source is Google and
// a server that says status.
func googleDoctor(t *testing.T, client *googleClient) *doctorResult {
	t.Helper()
	server := newConfigFile(t, backendTable+"[calendar]\nsource = \"google\"\n").server()
	server.client = client
	server.DeviceZone = "Europe/Lisbon"
	return doctorOf(t, server)
}

func TestTheMacsDoctorChecksGoogleCalendarAndNamesWhereToFixIt(t *testing.T) {
	for _, test := range []struct {
		name   string
		status api.GoogleCalendarStatus
		state  string
		detail string
		ready  bool
	}{
		{"connected", api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "joao@gmail.com"}, "ok",
			"Google Calendar connected as joao@gmail.com; read-only", true},
		{"not connected", api.GoogleCalendarStatus{Offered: true}, "fail",
			"Google Calendar isn’t connected. Connect it in Settings ▸ Calendar.", false},
		{"needs reconnecting", api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "joao@gmail.com", NeedsReconnect: true}, "fail",
			"Google Calendar needs reconnecting. Reconnect it in Settings ▸ Calendar.", false},
		{"not offered", api.GoogleCalendarStatus{}, "fail",
			"Google Calendar isn’t available on this server. Ask whoever runs the server to turn it on.", false},
	} {
		got := googleDoctor(t, &googleClient{status: test.status})
		check := checkNamed(t, got, "Calendar source")
		if check.Status != test.state || check.Detail != test.detail || got.Ready != test.ready {
			t.Errorf("%s: check = %+v, ready %v; want %s with %q, ready %v", test.name, check, got.Ready, test.state, test.detail, test.ready)
		}
	}
}

func TestTheMacsDoctorLeavesSecretsOutOfAGoogleCheckThatCouldNotAnswer(t *testing.T) {
	got := googleDoctor(t, &googleClient{err: errors.New("the server answered 500 token=mtk_abc123secret")})
	check := checkNamed(t, got, "Calendar source")
	if check.Status != "fail" || !strings.HasPrefix(check.Detail, "Unable to check Google Calendar: ") || !strings.Contains(check.Detail, "[REDACTED]") {
		t.Errorf("check = %+v", check)
	}
	wire, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), "mtk_abc123secret") {
		t.Errorf("the report holds the token: %s", wire)
	}
}

func TestTheMacsDoctorStillChecksTheFilesWhenTheyAreTheSource(t *testing.T) {
	client := &googleClient{status: api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "joao@gmail.com"}}
	server := newConfigFile(t, backendTable+"[calendar]\nenabled = false\n").server()
	server.client = client
	check := checkNamed(t, doctorOf(t, server), "Calendar source")
	if check.Status != "skip" || !strings.Contains(check.Detail, "disabled") {
		t.Errorf("check = %+v", check)
	}
}
