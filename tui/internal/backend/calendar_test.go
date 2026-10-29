package backend

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// calendarServer answers the four Google Calendar routes with what a test
// sets, and records each request.
type calendarServer struct {
	mu       sync.Mutex
	requests []request
	auth     []string
	status   int
	answer   string
}

func newCalendarServer(t *testing.T, status int, answer string) (*Client, *calendarServer) {
	t.Helper()
	fake := &calendarServer{status: status, answer: answer}
	mux := http.NewServeMux()
	for _, route := range []string{
		api.RouteConnectGoogleCalendar, api.RouteGoogleCalendarStatus, api.RouteDisconnectGoogleCalendar, api.RouteGoogleCalendarEvents,
	} {
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			fake.mu.Lock()
			defer fake.mu.Unlock()
			fake.requests = append(fake.requests, request{r.Pattern, r.URL.RawQuery, string(body)})
			fake.auth = append(fake.auth, r.Header.Get("Authorization"))
			w.WriteHeader(fake.status)
			io.WriteString(w, fake.answer)
		})
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return New(server.URL, "mtk_device", "Europe/Lisbon"), fake
}

func TestGoogleCalendarEventsComeAsPlannerEvents(t *testing.T) {
	client, fake := newCalendarServer(t, http.StatusOK, `{"events":[
		{"title":"Standup","start":"2026-10-14T09:00:00+01:00","end":"2026-10-14T09:30:00+01:00","allDay":false,"transparent":false},
		{"title":"Offsite","start":"2026-10-14T00:00:00+01:00","end":"2026-10-15T00:00:00+01:00","allDay":true,"transparent":true}]}`)
	west := time.FixedZone("WEST", 3600)
	start := time.Date(2026, 10, 14, 0, 0, 0, 0, west)

	events, err := client.GoogleCalendarEvents(t.Context(), start, start.AddDate(0, 0, 1))
	if err != nil || len(events) != 2 {
		t.Fatalf("GoogleCalendarEvents = %+v, %v", events, err)
	}
	standup, offsite := events[0], events[1]
	if standup.Summary != "Standup" || !standup.Start.Equal(start.Add(9*time.Hour)) || !standup.End.Equal(start.Add(9*time.Hour+30*time.Minute)) ||
		standup.AllDay || standup.Transparent || standup.Source != "google" {
		t.Errorf("standup = %+v", standup)
	}
	if offsite.Summary != "Offsite" || !offsite.Start.Equal(start) || !offsite.End.Equal(start.AddDate(0, 0, 1)) || !offsite.AllDay || !offsite.Transparent {
		t.Errorf("offsite = %+v", offsite)
	}
	// The window goes out in the device's offset, which all-day events follow.
	if len(fake.requests) != 1 || fake.requests[0].pattern != api.RouteGoogleCalendarEvents ||
		fake.requests[0].query != "end=2026-10-15T00%3A00%3A00%2B01%3A00&start=2026-10-14T00%3A00%3A00%2B01%3A00" || fake.auth[0] != "Bearer mtk_device" {
		t.Errorf("requests = %+v, auth %v", fake.requests, fake.auth)
	}
}

func TestAnEmptyDayIsAnEmptyList(t *testing.T) {
	client, _ := newCalendarServer(t, http.StatusOK, `{"events":[]}`)
	events, err := client.GoogleCalendarEvents(t.Context(), time.Now(), time.Now().Add(time.Hour))
	if err != nil || events == nil || len(events) != 0 {
		t.Errorf("GoogleCalendarEvents = %v, %v; want an empty list", events, err)
	}
}

func TestAnUnavailableCalendarComesBackWithTheServersReason(t *testing.T) {
	client, fake := newCalendarServer(t, http.StatusFailedDependency, `{"code":"calendar_unavailable","message":"Google Calendar needs reconnecting"}`)
	events, err := client.GoogleCalendarEvents(t.Context(), time.Now(), time.Now().Add(time.Hour))
	var failure *api.Error
	if !errors.As(err, &failure) || failure.Code != api.CodeCalendarUnavailable || err.Error() != "Google Calendar needs reconnecting" || events != nil {
		t.Fatalf("GoogleCalendarEvents = %v, %v; want calendar_unavailable with the reason", events, err)
	}
	if len(fake.requests) != 1 {
		t.Errorf("%d requests, want 1: an unavailable calendar is the server's answer, not a lost connection", len(fake.requests))
	}
}

func TestTheOtherCalendarRoutesPassThrough(t *testing.T) {
	ctx := t.Context()
	client, fake := newCalendarServer(t, http.StatusOK, `{"offered":true,"connected":true,"email":"alice@gmail.com","needsReconnect":false}`)
	if status, err := client.GoogleCalendarStatus(ctx); err != nil || status != (api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "alice@gmail.com"}) {
		t.Errorf("status = %+v, %v", status, err)
	}
	fake.mu.Lock()
	fake.answer = `{"url":"https://m.example/calendar/connect/abc","expiresAt":"2026-10-14T09:30:00Z"}`
	fake.mu.Unlock()
	if link, err := client.ConnectGoogleCalendar(ctx); err != nil || link.URL != "https://m.example/calendar/connect/abc" {
		t.Errorf("connect = %+v, %v", link, err)
	}
	fake.mu.Lock()
	fake.answer = `{"revoked":false}`
	fake.mu.Unlock()
	if answer, err := client.DisconnectGoogleCalendar(ctx); err != nil || answer.Revoked {
		t.Errorf("disconnect = %+v, %v", answer, err)
	}
	want := []string{api.RouteGoogleCalendarStatus, api.RouteConnectGoogleCalendar, api.RouteDisconnectGoogleCalendar}
	for index, pattern := range want {
		if fake.requests[index].pattern != pattern {
			t.Errorf("request %d = %s, want %s", index, fake.requests[index].pattern, pattern)
		}
	}
}
