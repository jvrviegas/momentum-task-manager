package bridge

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/calendar"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// The Google Calendar methods of Client, which only the Google tests give
// answers to; everywhere else the server has none to give.
func (f *fakeClient) GoogleCalendarStatus(context.Context) (api.GoogleCalendarStatus, error) {
	return api.GoogleCalendarStatus{}, nil
}

func (f *fakeClient) ConnectGoogleCalendar(context.Context) (api.GoogleCalendarLink, error) {
	return api.GoogleCalendarLink{}, errors.New("no Google Calendar")
}

func (f *fakeClient) DisconnectGoogleCalendar(context.Context) (api.GoogleCalendarDisconnect, error) {
	return api.GoogleCalendarDisconnect{Revoked: true}, nil
}

func (f *fakeClient) GoogleCalendarEvents(context.Context, time.Time, time.Time) ([]calendar.Event, error) {
	return nil, &api.Error{Code: api.CodeCalendarUnavailable, Message: "Google Calendar isn’t connected"}
}

// googleClient is Google Calendar as the server answers for one account.
type googleClient struct {
	planClient

	mu     sync.Mutex
	events []calendar.Event
	err    error
	calls  int
	window [2]time.Time
	status api.GoogleCalendarStatus
	link   api.GoogleCalendarLink
	left   api.GoogleCalendarDisconnect
}

func (g *googleClient) GoogleCalendarEvents(_ context.Context, start, end time.Time) ([]calendar.Event, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	g.window = [2]time.Time{start, end}
	if g.err != nil {
		return nil, g.err
	}
	return append([]calendar.Event(nil), g.events...), nil
}

func (g *googleClient) fetches() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func (g *googleClient) GoogleCalendarStatus(context.Context) (api.GoogleCalendarStatus, error) {
	return g.status, g.err
}

func (g *googleClient) ConnectGoogleCalendar(context.Context) (api.GoogleCalendarLink, error) {
	return g.link, g.err
}

func (g *googleClient) DisconnectGoogleCalendar(context.Context) (api.GoogleCalendarDisconnect, error) {
	return g.left, g.err
}

// googleEvents are the events of macEvents as Google answers them for the
// same Wednesday, which the file tests read too.
func googleEvents() []calendar.Event {
	at := func(hhmm int) time.Time { return time.Date(2026, 10, 14, hhmm/100, hhmm%100, 0, 0, time.Local) }
	return []calendar.Event{
		{Summary: "Team stand-up", Start: at(930), End: at(945), Source: "google"},
		{Summary: "Design review", Start: at(1100), End: at(1200), Source: "google"},
		{Summary: "Lunch with Rui", Start: at(1130), End: at(1215), Source: "google"},
		{Summary: "Retro", Start: at(1630), End: at(1730), Source: "google"},
		{Summary: "Early call", Start: at(700), End: at(800), Source: "google"},
	}
}

func googleSettings() config.Config {
	settings := config.Defaults()
	settings.Calendar.Source = config.CalendarSourceGoogle
	return settings
}

func TestGoogleGivesTheSameBusyTimeAndBlocksAsTheSameEventsInAFile(t *testing.T) {
	fromFile := planWith(t, newPlanServer(&planClient{}, calendarSettings(t, testNow.Add(-time.Hour))), nil).Calendar

	// Google stands in for the files, whatever [calendar] enabled says.
	client := &googleClient{events: googleEvents()}
	fromGoogle := planWith(t, newPlanServer(client, googleSettings()), nil).Calendar

	if !reflect.DeepEqual(fromGoogle, fromFile) {
		t.Fatalf("calendar:\n google %+v\n   file %+v", fromGoogle, fromFile)
	}
	if fromGoogle.BusyMinutes != 120 || !fromGoogle.Enabled || !fromGoogle.Available || fromGoogle.Stale {
		t.Fatalf("calendar = %+v", fromGoogle)
	}
	if summary := loadWith(t, newPlanServer(&googleClient{events: googleEvents()}, googleSettings()), nil); summary.CalendarMinutes != 120 || !summary.CalendarAvailable {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestTheServerIsAskedForTodayInTheDevicesOwnOffset(t *testing.T) {
	client := &googleClient{events: googleEvents()}
	planWith(t, newPlanServer(client, googleSettings()), nil)
	day := time.Date(testNow.Year(), testNow.Month(), testNow.Day(), 0, 0, 0, 0, testNow.Location())
	if !client.window[0].Equal(day) || !client.window[1].Equal(day.AddDate(0, 0, 1)) {
		t.Errorf("window = %v, want %v to %v", client.window, day, day.AddDate(0, 0, 1))
	}
	if client.window[0].Location() != testNow.Location() {
		t.Errorf("the window is in %v, want the device's own zone, which all-day events follow", client.window[0].Location())
	}
}

func TestHoursAndEventChoicesApplyToGoogleEventsToo(t *testing.T) {
	at := func(h int) time.Time { return time.Date(2026, 10, 14, h, 0, 0, 0, time.Local) }
	events := append(googleEvents(),
		calendar.Event{Start: at(13), End: at(14), Transparent: true, Source: "google"},
		calendar.Event{Start: time.Date(2026, 10, 14, 0, 0, 0, 0, time.Local), End: time.Date(2026, 10, 15, 0, 0, 0, 0, time.Local), AllDay: true, Source: "google"})
	for name, test := range map[string]struct {
		change func(*config.CalendarConfig)
		busy   int
	}{
		"by default":               {func(*config.CalendarConfig) {}, 120},
		"with free events counted": {func(c *config.CalendarConfig) { c.IncludeTransparent = true }, 180},
		"with all-day events":      {func(c *config.CalendarConfig) { c.IncludeAllDay = true }, 480},
		"with a short day":         {func(c *config.CalendarConfig) { c.WorkingStart, c.WorkingEnd = "11:00", "12:00" }, 60},
	} {
		settings := googleSettings()
		test.change(&settings.Calendar)
		if busy := planWith(t, newPlanServer(&googleClient{events: events}, settings), nil).Calendar; busy.BusyMinutes != test.busy {
			t.Errorf("%s: busy = %d, want %d", name, busy.BusyMinutes, test.busy)
		}
	}
}

func TestAGoogleCalendarThatCannotBeReadMakesTheCalendarUnavailable(t *testing.T) {
	for name, err := range map[string]error{
		"the server says so":        &api.Error{Code: api.CodeCalendarUnavailable, Message: "Google Calendar needs reconnecting"},
		"the server is unreachable": api.ErrUnreachable,
	} {
		client := &googleClient{err: err}
		plan := planWith(t, newPlanServer(client, googleSettings()), nil)
		busy := plan.Calendar
		if !busy.Enabled || busy.Available || busy.BusyMinutes != 0 || busy.Blocks == nil || len(busy.Blocks) != 0 || busy.Error != err.Error() {
			t.Errorf("%s: calendar = %+v, want unavailable with %q", name, busy, err)
		}
		if plan.CapacityMinutes != 480 {
			t.Errorf("%s: planning should go on with tasks: %+v", name, plan)
		}
		if summary := loadWith(t, newPlanServer(&googleClient{err: err}, googleSettings()), nil); summary.CalendarAvailable || summary.CalendarError != err.Error() || summary.CalendarMinutes != 0 {
			t.Errorf("%s: summary = %+v", name, summary)
		}
	}
}

func TestOpeningPlanTodayAlwaysAsksAndALoadReusesTheAnswerForFiveMinutes(t *testing.T) {
	client := &googleClient{events: googleEvents()}
	server := newPlanServer(client, googleSettings())
	now := testNow
	server.now = func() time.Time { return now }

	loadWith(t, server, nil)
	loadWith(t, server, nil)
	if client.fetches() != 1 {
		t.Fatalf("two loads asked Google %d times, want once", client.fetches())
	}
	now = testNow.Add(4*time.Minute + 59*time.Second)
	if summary := loadWith(t, server, nil); client.fetches() != 1 || summary.CalendarMinutes != 120 {
		t.Fatalf("just inside five minutes: %d fetches, %+v", client.fetches(), summary)
	}
	now = testNow.Add(5 * time.Minute)
	loadWith(t, server, nil)
	if client.fetches() != 2 {
		t.Fatalf("after five minutes: %d fetches, want 2", client.fetches())
	}

	// A new meeting shows in Plan Today at once, and the loads after it.
	client.mu.Lock()
	client.events = append(client.events, calendar.Event{Summary: "New", Start: time.Date(2026, 10, 14, 15, 0, 0, 0, time.Local), End: time.Date(2026, 10, 14, 16, 0, 0, 0, time.Local), Source: "google"})
	client.mu.Unlock()
	if busy := planWith(t, server, nil).Calendar; client.fetches() != 3 || busy.BusyMinutes != 180 {
		t.Fatalf("Plan Today: %d fetches, %+v; want a new one and the new meeting", client.fetches(), busy)
	}
	if summary := loadWith(t, server, nil); client.fetches() != 3 || summary.CalendarMinutes != 180 {
		t.Fatalf("the load after it: %d fetches, %+v; want it to reuse that answer", client.fetches(), summary)
	}
}

func TestAnUnavailableAnswerIsNotKept(t *testing.T) {
	client := &googleClient{err: &api.Error{Code: api.CodeCalendarUnavailable, Message: "Google Calendar needs reconnecting"}}
	server := newPlanServer(client, googleSettings())
	loadWith(t, server, nil)
	client.mu.Lock()
	client.err, client.events = nil, googleEvents()
	client.mu.Unlock()
	if summary := loadWith(t, server, nil); client.fetches() != 2 || summary.CalendarMinutes != 120 {
		t.Fatalf("after reconnecting: %d fetches, %+v; want it asked again", client.fetches(), summary)
	}
}

func TestChangingTheHoursOrEventChoicesNeedsNoNewFetch(t *testing.T) {
	client := &googleClient{events: googleEvents()}
	server := newPlanServer(client, googleSettings())
	loadWith(t, server, nil)
	server.config.Calendar.WorkingStart, server.config.Calendar.WorkingEnd = "11:00", "12:00"
	if summary := loadWith(t, server, nil); client.fetches() != 1 || summary.CalendarMinutes != 60 {
		t.Fatalf("after changing the hours: %d fetches, %+v; want the cached events counted at once", client.fetches(), summary)
	}
}

func TestACachedAnswerCarriesNoTitles(t *testing.T) {
	client := &googleClient{events: googleEvents()}
	server := newPlanServer(client, googleSettings())
	// Plan Today fetches with titles and shows them.
	blocks := planWith(t, server, nil).Calendar.Blocks
	if len(blocks) != 3 || strings.Join(blocks[1].Titles, "|") != "Design review|Lunch with Rui" {
		t.Fatalf("blocks = %+v", blocks)
	}
	// What it left for the sidebar holds none.
	server.mu.Lock()
	cached := server.googleEvents
	server.mu.Unlock()
	if len(cached.events) == 0 {
		t.Fatal("Plan Today did not leave its answer for the next load")
	}
	for _, event := range cached.events {
		if event.Summary != "" {
			t.Errorf("a cached event keeps its title %q", event.Summary)
		}
	}
}

// The titles are for Plan Today alone: a load's whole answer, which the app
// keeps in the sidebar, holds none, and Plan Today's holds those of the
// meetings inside the working hours, as its blocks do (an early call before
// them is in no block).
func TestALoadNeverCarriesATitleAndPlanTodayDoes(t *testing.T) {
	server := newPlanServer(&googleClient{events: googleEvents()}, googleSettings())
	responses := serve(t, server, request(t, 1, "load", nil), request(t, 2, "plan", nil))
	for _, event := range googleEvents() {
		if strings.Contains(string(responses[1].Result), event.Summary) {
			t.Errorf("a load carries the title %q", event.Summary)
		}
		if inHours := event.Summary != "Early call"; inHours && !strings.Contains(string(responses[2].Result), event.Summary) {
			t.Errorf("Plan Today lacks the title %q", event.Summary)
		}
	}
}

// Disconnecting ends the calendar at once: the meetings a load kept for the
// next five minutes must not outlive the connection.
func TestDisconnectingForgetsTheKeptEventsSoTheNextLoadSaysSo(t *testing.T) {
	client := &googleClient{events: googleEvents()}
	server := newPlanServer(client, googleSettings())
	if summary := loadWith(t, server, nil); summary.CalendarMinutes != 120 {
		t.Fatalf("before disconnecting: %+v", summary)
	}
	serve(t, server, request(t, 1, "disconnectCalendar", nil))
	// The server now has no calendar to read.
	client.mu.Lock()
	client.err = &api.Error{Code: api.CodeCalendarUnavailable, Message: "Google Calendar isn’t connected"}
	client.mu.Unlock()

	summary := loadWith(t, server, nil)
	if summary.CalendarAvailable || summary.CalendarMinutes != 0 || summary.CalendarError != "Google Calendar isn’t connected" {
		t.Fatalf("after disconnecting: %+v, want the reason and no meetings", summary)
	}
}

func TestSigningInAsSomeoneElseForgetsTheCachedAnswer(t *testing.T) {
	client := &googleClient{events: googleEvents()}
	server := newPlanServer(client, googleSettings())
	loadWith(t, server, nil)
	serve(t, server, request(t, 1, "setToken", tokenParams{Token: "mtk_other"}))
	loadWith(t, server, nil)
	if client.fetches() != 2 {
		t.Errorf("%d fetches, want the second account's calendar asked for again", client.fetches())
	}
}

func TestTheMacsEventsCountOverAGoogleSource(t *testing.T) {
	client := &googleClient{events: googleEvents()[:1]}
	server := newPlanServer(client, googleSettings())
	busy := planWith(t, server, &calendarInput{Events: macEvents()}).Calendar
	if busy.BusyMinutes != 120 || !busy.Available || client.fetches() != 0 {
		t.Fatalf("calendar = %+v, %d fetches; want the Mac's events and Google never asked", busy, client.fetches())
	}
	if busy := planWith(t, server, &calendarInput{Error: "no access"}).Calendar; busy.Error != "no access" || client.fetches() != 0 {
		t.Fatalf("a Mac that says no: %+v, %d fetches", busy, client.fetches())
	}
}

func TestGoogleNeedsNeitherTheFilesSwitchNorTheirPaths(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone.ics")
	settings := googleSettings()
	settings.Calendar.Enabled = true
	settings.Calendar.Paths = []string{missing}
	busy := planWith(t, newPlanServer(&googleClient{events: googleEvents()}, settings), nil).Calendar
	if !busy.Available || busy.Error != "" || busy.BusyMinutes != 120 {
		t.Fatalf("Google leaves the files alone: %+v", busy)
	}
}

func TestTheFilesStayTheSourceUnlessGoogleIsChosen(t *testing.T) {
	client := &googleClient{events: googleEvents()}
	settings := config.Defaults() // enabled is off and the source is files
	if busy := planWith(t, newPlanServer(client, settings), nil).Calendar; busy.Enabled || client.fetches() != 0 {
		t.Fatalf("calendar = %+v, %d fetches; want it off and Google not asked", busy, client.fetches())
	}
}

func TestTheCalendarOperationsReachTheServer(t *testing.T) {
	expires := time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC)
	client := &googleClient{
		status: api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "alice@gmail.com", NeedsReconnect: true},
		link:   api.GoogleCalendarLink{URL: "https://m.example/calendar/connect/abc", ExpiresAt: expires},
		left:   api.GoogleCalendarDisconnect{Revoked: false},
	}
	server := newPlanServer(client, config.Defaults())
	responses := serve(t, server,
		request(t, 1, "calendarStatus", nil), request(t, 2, "connectCalendar", nil), request(t, 3, "disconnectCalendar", nil))

	if got := result[api.GoogleCalendarStatus](t, responses[1]); got != client.status {
		t.Errorf("calendarStatus = %+v", got)
	}
	link := result[connectCalendarResult](t, responses[2])
	if link.URL != "https://m.example/calendar/connect/abc" || link.ExpiresAt != expires.Format(time.RFC3339) {
		t.Errorf("connectCalendar = %+v", link)
	}
	if got := result[api.GoogleCalendarDisconnect](t, responses[3]); got.Revoked {
		t.Errorf("disconnectCalendar = %+v, want the not-revoked answer kept", got)
	}
}

func TestTheCalendarOperationsReportTheServersErrors(t *testing.T) {
	for name, test := range map[string]struct {
		err  error
		code string
	}{
		"a refused token":         {&api.Error{Code: api.CodeUnauthorized, Message: "sign in again"}, CodeUnauthorized},
		"an unreachable server":   {api.ErrUnreachable, CodeUnreachable},
		"a server without Google": {&api.Error{Code: api.CodeCalendarUnavailable, Message: "Google Calendar isn’t available on this server"}, ""},
	} {
		server := newPlanServer(&googleClient{err: test.err}, config.Defaults())
		for _, op := range []string{"calendarStatus", "connectCalendar", "disconnectCalendar"} {
			response := serve(t, server, request(t, 1, op, nil))[1]
			if response.Error == nil || response.Error.Code != test.code || response.Error.Message != test.err.Error() {
				t.Errorf("%s, %s: error = %+v, want %q with code %q", name, op, response.Error, test.err, test.code)
			}
		}
	}
}

func TestTheCalendarOperationsNeedAServerAndSignIn(t *testing.T) {
	server := NewServer(func(string) Client { return &googleClient{} }, config.Defaults(), "/tmp/config.toml")
	for _, op := range []string{"calendarStatus", "connectCalendar", "disconnectCalendar"} {
		if response := serve(t, server, request(t, 1, op, nil))[1]; response.Error == nil || !strings.Contains(response.Error.Message, "needs a server") {
			t.Errorf("%s without a [backend] url: %+v", op, response.Error)
		}
	}
}
