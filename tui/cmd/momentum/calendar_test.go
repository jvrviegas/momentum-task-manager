package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// calendarServer is a fake Momentum server for momentum calendar. It answers
// the status route from statuses, one per request and then the last again, so a
// test can have the browser "finish" after some polls, and records what it was
// asked to do.
type calendarServer struct {
	config, url string
	statuses    []string
	asked       atomic.Int32
	connects    atomic.Int32
	disconnects atomic.Int32
	linkLife    time.Duration
	disconnect  func(http.ResponseWriter)

	mu     sync.Mutex
	tokens []string // the Authorization of every status question
}

const (
	notOffered     = `{"offered":false,"connected":false,"needsReconnect":false}`
	notConnected   = `{"offered":true,"connected":false,"needsReconnect":false}`
	connectedAsAna = `{"offered":true,"connected":true,"email":"ana@gmail.com","needsReconnect":false}`
	needsReconnect = `{"offered":true,"connected":true,"email":"ana@gmail.com","needsReconnect":true}`
	googleConfig   = "[calendar]\nsource = \"google\"\n"
)

// newCalendarServer starts the server, signs a device in to it and writes a
// config.toml for it, with extra after the [backend] table.
func newCalendarServer(t *testing.T, extra string, statuses ...string) *calendarServer {
	t.Helper()
	server := &calendarServer{statuses: statuses, linkLife: time.Minute}
	mux := http.NewServeMux()
	mux.HandleFunc(api.RouteGoogleCalendarStatus, func(w http.ResponseWriter, r *http.Request) {
		server.mu.Lock()
		server.tokens = append(server.tokens, r.Header.Get("Authorization"))
		server.mu.Unlock()
		index := int(server.asked.Add(1)) - 1
		if index >= len(server.statuses) {
			index = len(server.statuses) - 1
		}
		io.WriteString(w, server.statuses[index])
	})
	mux.HandleFunc(api.RouteConnectGoogleCalendar, func(w http.ResponseWriter, r *http.Request) {
		server.connects.Add(1)
		io.WriteString(w, `{"url":"`+server.url+`/calendar/connect/one-time","expiresAt":"`+time.Now().Add(server.linkLife).UTC().Format(time.RFC3339Nano)+`"}`)
	})
	mux.HandleFunc(api.RouteDisconnectGoogleCalendar, func(w http.ResponseWriter, r *http.Request) {
		server.disconnects.Add(1)
		if server.disconnect != nil {
			server.disconnect(w)
			return
		}
		io.WriteString(w, `{"revoked":true}`)
	})
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	server.url = httpServer.URL

	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	server.config = filepath.Join(root, "config.toml")
	if err := os.WriteFile(server.config, []byte("[backend]\nurl = \""+server.url+"\"\n"+extra), 0o600); err != nil {
		t.Fatal(err)
	}
	credentials := filepath.Join(root, "xdg", "momentum", "credentials.json")
	os.MkdirAll(filepath.Dir(credentials), 0o700)
	saved := `{"url":"` + server.url + `","token":"mtk_saved","email":"ana@example.com","expiresAt":"2999-01-01T00:00:00Z"}`
	if err := os.WriteFile(credentials, []byte(saved), 0o600); err != nil {
		t.Fatal(err)
	}

	original, originalPoll := openBrowser, calendarPollInterval
	t.Cleanup(func() { openBrowser, calendarPollInterval = original, originalPoll })
	calendarPollInterval = time.Millisecond
	return server
}

// calendar runs momentum calendar with the answer typed at any question.
func (s *calendarServer) calendar(t *testing.T, typed string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	original := calendarInput
	t.Cleanup(func() { calendarInput = original })
	calendarInput = strings.NewReader(typed)
	var out, errOut bytes.Buffer
	code = run(append([]string{"calendar"}, append(args, "--config", s.config)...), &out, &errOut)
	return code, out.String(), errOut.String()
}

func settingsFor(t *testing.T, path string) config.Config {
	t.Helper()
	settings, _, err := config.LoadWithOptions(config.LoadOptions{PathOverride: path})
	if err != nil {
		t.Fatal(err)
	}
	return settings
}

func hint(config string) string {
	return "To count it on this machine, set source = \"google\" under [calendar] in " + config + ".\n"
}

func TestConnectPrintsTheLinkAndWaitsUntilTheAccountIsConnected(t *testing.T) {
	server := newCalendarServer(t, "", notConnected, notConnected, notConnected, connectedAsAna)
	var opened []string
	openBrowser = func(url string) error { opened = append(opened, url); return nil }

	code, out, errOut := server.calendar(t, "", "connect")
	link := server.url + "/calendar/connect/one-time"
	want := "To connect Google Calendar, open\n" + link + "\n" +
		"Waiting for you to finish in the browser (Ctrl+C cancels)…\n" +
		"Connected Google Calendar as ana@gmail.com.\n" + hint(server.config)
	if code != 0 || out != want || errOut != "" {
		t.Fatalf("code %d, stdout %q, stderr %q; want stdout %q", code, out, errOut, want)
	}
	if len(opened) != 1 || opened[0] != link {
		t.Errorf("opened %q", opened)
	}
	if server.connects.Load() != 1 || server.asked.Load() != 4 {
		t.Errorf("%d links asked for and %d status questions; want 1 and 4", server.connects.Load(), server.asked.Load())
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	for _, token := range server.tokens {
		if token != "Bearer mtk_saved" {
			t.Errorf("asked with %q, not the saved sign-in", token)
		}
	}
}

func TestConnectSaysNothingToSetWhenThisMachineAlreadyCountsGoogle(t *testing.T) {
	server := newCalendarServer(t, googleConfig, notConnected, connectedAsAna)
	openBrowser = func(string) error { return nil }
	code, out, _ := server.calendar(t, "", "connect")
	if code != 0 || !strings.HasSuffix(out, "Connected Google Calendar as ana@gmail.com.\n") {
		t.Errorf("code %d, stdout %q", code, out)
	}
}

func TestConnectShowsTheAddressWhenNoBrowserOpens(t *testing.T) {
	server := newCalendarServer(t, "", notConnected, connectedAsAna)
	openBrowser = func(string) error { return errors.New("no browser") }
	code, out, _ := server.calendar(t, "", "connect")
	if code != 0 || !strings.Contains(out, "Could not open a browser; open the address above.\n") {
		t.Errorf("code %d, stdout %q", code, out)
	}
}

func TestReconnectWaitsUntilGoogleAcceptsTheConnectionAgain(t *testing.T) {
	// A connection Google refuses is still a connection to the server, so the
	// wait is over only when it no longer needs reconnecting.
	server := newCalendarServer(t, "", needsReconnect, needsReconnect, needsReconnect, connectedAsAna)
	openBrowser = func(string) error { return nil }
	code, out, _ := server.calendar(t, "", "connect")
	if code != 0 || !strings.HasPrefix(out, "To reconnect Google Calendar, open\n") || !strings.Contains(out, "Connected Google Calendar as ana@gmail.com.\n") {
		t.Errorf("code %d, stdout %q", code, out)
	}
	if server.asked.Load() != 4 {
		t.Errorf("%d status questions; want 4", server.asked.Load())
	}
}

func TestConnectChangesNothingWhenGoogleAlreadyWorks(t *testing.T) {
	server := newCalendarServer(t, "", connectedAsAna)
	openBrowser = func(string) error { t.Error("opened a browser"); return nil }
	code, out, errOut := server.calendar(t, "", "connect")
	want := "Google Calendar is already connected as ana@gmail.com. To use another account, run momentum calendar disconnect first.\n" + hint(server.config)
	if code != 0 || out != want || errOut != "" || server.connects.Load() != 0 {
		t.Errorf("code %d, stdout %q, stderr %q, %d links asked for", code, out, errOut, server.connects.Load())
	}
}

func TestConnectEndsWhenTheLinkExpires(t *testing.T) {
	server := newCalendarServer(t, "", notConnected)
	server.linkLife = 30 * time.Millisecond
	openBrowser = func(string) error { return nil }
	code, out, errOut := server.calendar(t, "", "connect")
	if code != 1 || errOut != "momentum: the link expired. Run momentum calendar connect to try again.\n" || strings.Contains(out, "Connected") {
		t.Errorf("code %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestConnectStopsWaitingWhenInterrupted(t *testing.T) {
	server := newCalendarServer(t, "", notConnected)
	openBrowser = func(string) error { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	var out, errOut bytes.Buffer
	client := api.NewClient(server.url, "mtk_saved")
	if code := connectCalendar(ctx, client, settingsFor(t, server.config), server.config, &out, &errOut); code != 1 ||
		errOut.String() != "momentum: stopped waiting; nothing was connected.\n" {
		t.Errorf("code %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
}

func TestConnectNeedsAServerThatOffersGoogleCalendar(t *testing.T) {
	server := newCalendarServer(t, "", notOffered)
	openBrowser = func(string) error { t.Error("opened a browser"); return nil }
	code, _, errOut := server.calendar(t, "", "connect")
	if code != 1 || errOut != "momentum: Google Calendar isn’t available on this server. Ask whoever runs the server to turn it on.\n" || server.connects.Load() != 0 {
		t.Errorf("code %d, stderr %q, %d links asked for", code, errOut, server.connects.Load())
	}
}

func TestStatusSaysWhereGoogleCalendarStands(t *testing.T) {
	for _, test := range []struct {
		name, status, extra, want string
	}{
		{"working and counted here", connectedAsAna, googleConfig,
			"Google Calendar connected as ana@gmail.com; read-only\nThis machine counts it in the planner.\n"},
		{"working but not counted here", connectedAsAna, "",
			"Google Calendar connected as ana@gmail.com; read-only\n" + hint("CONFIG")},
		{"not connected", notConnected, "",
			"Google Calendar isn’t connected. Run momentum calendar connect to connect it.\n"},
		{"refused by Google", needsReconnect, "",
			"Google Calendar needs reconnecting. Google no longer accepts the saved connection to ana@gmail.com. Run momentum calendar connect to reconnect it.\n"},
		{"not offered", notOffered, "",
			"Google Calendar isn’t available on this server. Ask whoever runs the server to turn it on.\n"},
	} {
		server := newCalendarServer(t, test.extra, test.status)
		code, out, errOut := server.calendar(t, "", "status")
		if want := strings.ReplaceAll(test.want, "CONFIG", server.config); code != 0 || out != want || errOut != "" {
			t.Errorf("%s: code %d, stdout %q, stderr %q; want %q", test.name, code, out, errOut, want)
		}
		if server.connects.Load() != 0 || server.disconnects.Load() != 0 {
			t.Errorf("%s: status changed something", test.name)
		}
	}
}

func TestDisconnectAsksFirstAndThenDisconnects(t *testing.T) {
	for _, answer := range []string{"y\n", "Yes\n", " y \n"} {
		server := newCalendarServer(t, "", connectedAsAna)
		code, out, errOut := server.calendar(t, answer, "disconnect")
		want := "Momentum will stop reading ana@gmail.com and delete its saved access, for every Mac and the terminal app on your account. The planner counts no meetings until you connect again.\n" +
			"Disconnect Google Calendar? [y/N] Disconnected Google Calendar.\n"
		if code != 0 || out != want || errOut != "" || server.disconnects.Load() != 1 {
			t.Errorf("answer %q: code %d, stdout %q, stderr %q, %d disconnects", answer, code, out, errOut, server.disconnects.Load())
		}
	}
}

func TestDisconnectChangesNothingUnlessTheAnswerIsYes(t *testing.T) {
	for _, answer := range []string{"\n", "n\n", "no\n", "yep\n", ""} {
		server := newCalendarServer(t, "", connectedAsAna)
		code, out, _ := server.calendar(t, answer, "disconnect")
		if code != 1 || !strings.HasSuffix(out, "Disconnect Google Calendar? [y/N] Nothing was disconnected.\n") || server.disconnects.Load() != 0 {
			t.Errorf("answer %q: code %d, stdout %q, %d disconnects", answer, code, out, server.disconnects.Load())
		}
	}
}

func TestDisconnectHasNothingToDoWithoutAConnection(t *testing.T) {
	server := newCalendarServer(t, "", notConnected)
	code, out, errOut := server.calendar(t, "y\n", "disconnect")
	if code != 0 || out != "Google Calendar isn’t connected; nothing to disconnect.\n" || errOut != "" || server.disconnects.Load() != 0 {
		t.Errorf("code %d, stdout %q, stderr %q, %d disconnects", code, out, errOut, server.disconnects.Load())
	}
}

func TestDisconnectNeedsAServerThatOffersGoogleCalendar(t *testing.T) {
	server := newCalendarServer(t, "", notOffered)
	code, _, errOut := server.calendar(t, "y\n", "disconnect")
	if code != 1 || !strings.HasPrefix(errOut, "momentum: Google Calendar isn’t available on this server.") || server.disconnects.Load() != 0 {
		t.Errorf("code %d, stderr %q, %d disconnects", code, errOut, server.disconnects.Load())
	}
}

func TestDisconnectAConnectionThatNeedsReconnecting(t *testing.T) {
	server := newCalendarServer(t, "", needsReconnect)
	code, out, _ := server.calendar(t, "y\n", "disconnect")
	if code != 0 || !strings.Contains(out, "stop reading ana@gmail.com") || server.disconnects.Load() != 1 {
		t.Errorf("code %d, stdout %q, %d disconnects", code, out, server.disconnects.Load())
	}
}

func TestDisconnectSaysWhenGoogleCouldNotRevokeTheAccess(t *testing.T) {
	server := newCalendarServer(t, "", connectedAsAna)
	server.disconnect = func(w http.ResponseWriter) { io.WriteString(w, `{"revoked":false}`) }
	code, out, errOut := server.calendar(t, "y\n", "disconnect")
	want := "momentum: couldn’t reach Google to revoke its access, so it may still show in your Google account’s settings. Remove it there.\n"
	if code != 0 || !strings.HasSuffix(out, "Disconnected Google Calendar.\n") || errOut != want {
		t.Errorf("code %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestDisconnectThatFailsSaysWhy(t *testing.T) {
	server := newCalendarServer(t, "", connectedAsAna)
	server.disconnect = func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"code":"internal","message":"the server failed; try again"}`)
	}
	code, out, errOut := server.calendar(t, "y\n", "disconnect")
	if code != 1 || strings.Contains(out, "Disconnected") || errOut != "momentum: unable to disconnect Google Calendar: the server failed; try again\n" {
		t.Errorf("code %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestCalendarCommandsNeedABackendAndASignIn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"calendar", "status", "--config", path}, &out, &errOut); code != 1 || errOut.String() != "momentum: calendar status needs a [backend] url in "+path+"\n" {
		t.Errorf("no backend: code %d, stderr %q", code, errOut.String())
	}

	server := newCalendarServer(t, "", connectedAsAna)
	os.RemoveAll(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "momentum"))
	if code, _, errOut := server.calendar(t, "", "status"); code != 1 || !strings.Contains(errOut, "not signed in") || server.asked.Load() != 0 {
		t.Errorf("no sign-in: code %d, stderr %q, %d questions", code, errOut, server.asked.Load())
	}
}

func TestCalendarNeedsOneOfItsThreeCommands(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"calendar"}, &out, &errOut); code != 2 || errOut.String() != "momentum: calendar needs connect, status or disconnect\n" {
		t.Errorf("bare calendar: code %d, stderr %q", code, errOut.String())
	}
	errOut.Reset()
	if code := run([]string{"calendar", "sync"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), `unsupported argument "sync"`) {
		t.Errorf("calendar sync: code %d, stderr %q", code, errOut.String())
	}
	errOut.Reset()
	if code := run([]string{"calendar", "connect", "status"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), `unsupported argument "status"`) {
		t.Errorf("two commands: code %d, stderr %q", code, errOut.String())
	}
}

func TestExtractCommandFindsTheCalendarCommandWhereverTheFlagsAre(t *testing.T) {
	for _, args := range [][]string{
		{"calendar", "connect", "--config", "/tmp/c.toml"},
		{"--config", "/tmp/c.toml", "calendar", "connect"},
		{"calendar", "--debug", "connect"},
	} {
		if got, _, err := extractCommand(args); err != nil || got != "calendar connect" {
			t.Errorf("args=%#v got=%q err=%v", args, got, err)
		}
	}
}
