package calendar

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// west is the device's offset in these tests, one hour east of UTC.
var west = time.FixedZone("WEST", 3600)

func day() (start, end time.Time) {
	start = time.Date(2026, 10, 14, 0, 0, 0, 0, west)
	return start, start.AddDate(0, 0, 1)
}

func timed(summary, start, end string, extra ...map[string]any) map[string]any {
	item := map[string]any{
		"summary": summary, "status": "confirmed",
		"start": map[string]any{"dateTime": start}, "end": map[string]any{"dateTime": end},
	}
	for _, more := range extra {
		for key, value := range more {
			item[key] = value
		}
	}
	return item
}

func at(h, m int) time.Time { return time.Date(2026, 10, 14, h, m, 0, 0, west) }

// sameEvents compares events by instant, since times decode with their own
// zone values.
func sameEvents(t *testing.T, got, want []api.GoogleCalendarEvent) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
	for index := range want {
		g, w := got[index], want[index]
		if g.Title != w.Title || !g.Start.Equal(w.Start) || !g.End.Equal(w.End) || g.AllDay != w.AllDay || g.Transparent != w.Transparent {
			t.Errorf("event %d = %+v, want %+v", index, g, w)
		}
	}
}

func unavailableWith(t *testing.T, err error, reason string) {
	t.Helper()
	var failure *api.Error
	if !errors.As(err, &failure) || failure.Code != api.CodeCalendarUnavailable || failure.Message != reason {
		t.Fatalf("error = %#v, want calendar_unavailable: %s", err, reason)
	}
}

func TestEventsComeFromThePrimaryCalendarForTheDay(t *testing.T) {
	h := newHarness(t)
	h.connect()
	h.google.set(func(f *fakeGoogle) {
		f.pages = [][]map[string]any{{
			timed("Standup", "2026-10-14T09:00:00+01:00", "2026-10-14T09:30:00+01:00"),
			{
				"summary": "Offsite", "status": "confirmed", "transparency": "transparent",
				"start": map[string]any{"date": "2026-10-14"}, "end": map[string]any{"date": "2026-10-15"},
			},
		}}
	})
	start, end := day()
	events, err := h.service.Events(h.asAlice, start, end)
	if err != nil {
		t.Fatal(err)
	}
	// An all-day event runs from midnight to midnight in the window's offset,
	// and "transparent" is Google's show-as-available.
	sameEvents(t, events, []api.GoogleCalendarEvent{
		{Title: "Standup", Start: at(9, 0), End: at(9, 30)},
		{Title: "Offsite", Start: start, End: end, AllDay: true, Transparent: true},
	})

	if len(h.google.eventRequests) != 1 {
		t.Fatalf("%d calendar requests, want 1", len(h.google.eventRequests))
	}
	query := h.google.eventRequests[0]
	for key, want := range map[string]string{
		"singleEvents": "true", "showDeleted": "false",
		"timeMin": "2026-10-14T00:00:00+01:00", "timeMax": "2026-10-15T00:00:00+01:00",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	// Only what planning needs is asked for, so no description or other
	// person's address reaches the server.
	fields := query.Get("fields")
	if !strings.Contains(fields, "attendees(self,responseStatus)") || strings.Contains(fields, "description") ||
		strings.Contains(fields, "email") || strings.Contains(fields, "organizer") {
		t.Errorf("fields = %q", fields)
	}
	if auth := h.google.eventAuth[0]; !strings.HasPrefix(auth, "Bearer "+testAccessToken) {
		t.Errorf("Authorization = %q, want the access token the refresh gave", auth)
	}
}

func TestDeclinedMeetingsAreLeftOut(t *testing.T) {
	h := newHarness(t)
	h.connect()
	attendees := func(self, others string) map[string]any {
		return map[string]any{"attendees": []map[string]any{
			{"self": false, "responseStatus": others}, {"self": true, "responseStatus": self},
		}}
	}
	h.google.set(func(f *fakeGoogle) {
		f.pages = [][]map[string]any{{
			timed("Declined", "2026-10-14T09:00:00+01:00", "2026-10-14T10:00:00+01:00", attendees("declined", "accepted")),
			timed("Accepted", "2026-10-14T10:00:00+01:00", "2026-10-14T11:00:00+01:00", attendees("accepted", "accepted")),
			timed("Tentative", "2026-10-14T11:00:00+01:00", "2026-10-14T12:00:00+01:00", attendees("tentative", "accepted")),
			timed("Unanswered", "2026-10-14T12:00:00+01:00", "2026-10-14T13:00:00+01:00", attendees("needsAction", "accepted")),
			timed("Others declined", "2026-10-14T13:00:00+01:00", "2026-10-14T14:00:00+01:00", attendees("accepted", "declined")),
			timed("Mine alone", "2026-10-14T14:00:00+01:00", "2026-10-14T15:00:00+01:00"),
		}}
	})
	events, err := h.service.Events(h.asAlice, at(0, 0), at(0, 0).AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, event := range events {
		titles = append(titles, event.Title)
	}
	if got, want := strings.Join(titles, ","), "Accepted,Tentative,Unanswered,Others declined,Mine alone"; got != want {
		t.Errorf("titles = %s, want %s", got, want)
	}
}

// A weekly meeting on Monday, Tuesday and Wednesday of this window, where
// Tuesday's was cancelled and Wednesday's moved from 09:00 to 14:00. Google
// answers single occurrences when asked for them, so each one is read at its
// own time.
func TestARepeatingMeetingWithACancelledAndAMovedOccurrence(t *testing.T) {
	h := newHarness(t)
	h.connect()
	h.google.set(func(f *fakeGoogle) {
		f.pages = [][]map[string]any{{
			timed("Daily sync", "2026-10-12T09:00:00+01:00", "2026-10-12T09:30:00+01:00"),
			// The cancelled occurrence has no times, and the API leaves it out
			// when showDeleted is false; a server that included it must still
			// not count it.
			{"id": "sync_20261013", "status": "cancelled"},
			timed("Daily sync", "2026-10-14T14:00:00+01:00", "2026-10-14T14:30:00+01:00"),
		}}
	})
	events, err := h.service.Events(h.asAlice, time.Date(2026, 10, 12, 0, 0, 0, 0, west), time.Date(2026, 10, 15, 0, 0, 0, 0, west))
	if err != nil {
		t.Fatal(err)
	}
	sameEvents(t, events, []api.GoogleCalendarEvent{
		{Title: "Daily sync", Start: time.Date(2026, 10, 12, 9, 0, 0, 0, west), End: time.Date(2026, 10, 12, 9, 30, 0, 0, west)},
		{Title: "Daily sync", Start: at(14, 0), End: at(14, 30)},
	})
}

func TestEventsFollowEveryPage(t *testing.T) {
	h := newHarness(t)
	h.connect()
	h.google.set(func(f *fakeGoogle) {
		f.pages = [][]map[string]any{
			{timed("One", "2026-10-14T09:00:00+01:00", "2026-10-14T09:30:00+01:00")},
			{timed("Two", "2026-10-14T10:00:00+01:00", "2026-10-14T10:30:00+01:00")},
		}
	})
	start, end := day()
	events, err := h.service.Events(h.asAlice, start, end)
	if err != nil {
		t.Fatal(err)
	}
	sameEvents(t, events, []api.GoogleCalendarEvent{{Title: "One", Start: at(9, 0), End: at(9, 30)}, {Title: "Two", Start: at(10, 0), End: at(10, 30)}})
	if len(h.google.eventRequests) != 2 || h.google.eventRequests[1].Get("pageToken") != "1" {
		t.Errorf("requests = %v, want a second one with the next page's token", h.google.eventRequests)
	}
}

func TestAnEmptyDayIsAnEmptyListNotAFailure(t *testing.T) {
	h := newHarness(t)
	h.connect()
	start, end := day()
	if events, err := h.service.Events(h.asAlice, start, end); err != nil || events == nil || len(events) != 0 {
		t.Errorf("Events = %v, %v; want an empty list", events, err)
	}
}

func TestGoogleRefusingTheConnectionMarksItNeedsReconnecting(t *testing.T) {
	h := newHarness(t)
	h.connect()
	h.google.set(func(f *fakeGoogle) { f.refreshFails = true })
	start, end := day()
	_, err := h.service.Events(h.asAlice, start, end)
	unavailableWith(t, err, reasonReconnect)
	status, _ := h.service.Status(h.asAlice)
	if !status.Connected || !status.NeedsReconnect {
		t.Fatalf("Status = %+v, want connected and needing a reconnect", status)
	}

	// The mark stays: Google is not asked again until the person reconnects.
	before := len(h.google.tokenRequests)
	_, err = h.service.Events(h.asAlice, start, end)
	unavailableWith(t, err, reasonReconnect)
	if len(h.google.tokenRequests) != before {
		t.Error("Events asked Google again for a connection already marked")
	}

	// Reconnecting clears it.
	h.google.set(func(f *fakeGoogle) { f.refreshFails = false })
	h.connect()
	if _, err := h.service.Events(h.asAlice, start, end); err != nil {
		t.Errorf("Events after reconnecting: %v", err)
	}
}

func TestAKeyThatNoLongerOpensTheTokenMeansReconnecting(t *testing.T) {
	h := newHarness(t)
	h.connect()
	rotated := New(h.client, Config{PublicURL: "https://momentum.example", Key: make([]byte, 32), Google: h.service.google})
	start, end := day()
	_, err := rotated.Events(h.asAlice, start, end)
	unavailableWith(t, err, reasonReconnect)
	if status, _ := rotated.Status(h.asAlice); !status.NeedsReconnect {
		t.Errorf("Status = %+v, want a reconnect to be needed", status)
	}
}

func TestEventsNeedAConnectionAndAnOfferingServer(t *testing.T) {
	h := newHarness(t)
	start, end := day()
	_, err := h.service.Events(h.asAlice, start, end)
	unavailableWith(t, err, reasonNotConnected)
	_, err = New(h.client, Config{PublicURL: "https://momentum.example"}).Events(h.asAlice, start, end)
	unavailableWith(t, err, reasonNotOffered)
}

func TestAGoogleFailureLeavesTheConnectionAsItWas(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*fakeGoogle)
		reason string
		marked bool
	}{
		{"the calendar answers 500", func(f *fakeGoogle) { f.apiStatus = 500 }, reasonUnreachable, false},
		{"the calendar answers 503", func(f *fakeGoogle) { f.apiStatus = 503 }, reasonUnreachable, false},
		{"Google limits the rate", func(f *fakeGoogle) { f.apiStatus = 429 }, reasonUnreachable, false},
		{"the token endpoint answers 500", func(f *fakeGoogle) { f.refreshStatus = 500 }, reasonUnreachable, false},
		{"the calendar refuses with 403", func(f *fakeGoogle) { f.apiStatus = 403 }, reasonRefused, false},
		{"the calendar does not know the access token", func(f *fakeGoogle) { f.apiStatus = 401 }, reasonReconnect, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t)
			h.connect()
			h.google.set(test.change)
			start, end := day()
			_, err := h.service.Events(h.asAlice, start, end)
			unavailableWith(t, err, test.reason)
			if status, _ := h.service.Status(h.asAlice); !status.Connected || status.NeedsReconnect != test.marked {
				t.Errorf("Status = %+v, want connected, needs reconnect %v", status, test.marked)
			}
		})
	}
}
