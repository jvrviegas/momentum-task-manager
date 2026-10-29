package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/calendar"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// googleClient is a server that can also read the account's Google Calendar.
type googleClient struct {
	fakeClient
	events []calendar.Event
	err    error
	asked  int
	window [2]time.Time
}

func (g *googleClient) GoogleCalendarEvents(_ context.Context, start, end time.Time) ([]calendar.Event, error) {
	g.asked++
	g.window = [2]time.Time{start, end}
	return g.events, g.err
}

var googleDay = time.Date(2026, 10, 14, 8, 0, 0, 0, time.Local)

// meetingTimes are one Wednesday's meetings as hhmm pairs: 120 busy minutes in
// working hours, where the two that overlap count once and the early one not at
// all.
var meetingTimes = []struct {
	title      string
	from, till int
}{
	{"Team stand-up", 930, 945}, {"Design review", 1100, 1200}, {"Lunch with Rui", 1130, 1215},
	{"Retro", 1630, 1730}, {"Early call", 700, 800},
}

func at(hhmm int) time.Time {
	return time.Date(2026, 10, 14, hhmm/100, hhmm%100, 0, 0, time.Local)
}

func googleMeetings() []calendar.Event {
	var events []calendar.Event
	for _, meeting := range meetingTimes {
		events = append(events, calendar.Event{Summary: meeting.title, Start: at(meeting.from), End: at(meeting.till), Source: "google"})
	}
	return events
}

// meetingsFile is the same meetings as a calendar file, modified recently so
// that it is not stale.
func meetingsFile(t *testing.T) string {
	t.Helper()
	var ics strings.Builder
	ics.WriteString("BEGIN:VCALENDAR\n")
	for _, meeting := range meetingTimes {
		const stamp = "20060102T150405Z"
		ics.WriteString("BEGIN:VEVENT\nDTSTART:" + at(meeting.from).UTC().Format(stamp) + "\nDTEND:" + at(meeting.till).UTC().Format(stamp) + "\nSUMMARY:" + meeting.title + "\nEND:VEVENT\n")
	}
	ics.WriteString("END:VCALENDAR\n")
	path := filepath.Join(t.TempDir(), "work.ics")
	if err := os.WriteFile(path, []byte(ics.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, googleDay.Add(-time.Hour), googleDay.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	return path
}

func googleSettings(enabled bool) config.Config {
	settings := config.Defaults()
	settings.Calendar.Source = config.CalendarSourceGoogle
	settings.Calendar.Enabled = enabled
	return settings
}

func openPlannerWith(t *testing.T, client Client, settings config.Config) *Model {
	t.Helper()
	model := NewModel(ModelOptions{Client: client, Config: settings, Now: func() time.Time { return googleDay }, Context: context.Background(), Width: 100, Height: 30})
	model.Tasks = []domain.Task{{UUID: "candidate", Description: "candidate", Status: "pending", Estimate: &domain.Estimate{Minutes: 30}}}
	model.Views = domain.BuildViews(model.Tasks, googleDay)
	cmd := model.OpenPlanner()
	if cmd == nil {
		t.Fatal("the planner asked for no calendar")
	}
	model.Update(cmd())
	return model
}

func TestThePlannerReadsGoogleWhateverEnabledSays(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		client := &googleClient{events: googleMeetings()}
		model := openPlannerWith(t, client, googleSettings(enabled))
		state := model.CalendarResult
		if !state.Available || state.Stale || state.Err != "" || state.BusyMinutes != 120 || state.MeetingCount != 5 || client.asked != 1 {
			t.Errorf("enabled %v: state = %+v, asked %d times", enabled, state, client.asked)
		}
		if summary := model.Planner.Summary; summary.CalendarMinutes != 120 || !summary.CalendarAvailable || len(summary.CalendarEvents) != 5 {
			t.Errorf("enabled %v: planner summary = %+v", enabled, summary)
		}
		if first := model.CalendarResult.Events[0]; first.Summary != "Early call" {
			t.Errorf("enabled %v: the strip starts with %+v, want the meetings in time order with their titles", enabled, first)
		}
	}
}

func TestGoogleGivesThePlannerTheSameBusyTimeAndMeetingsAsTheSameEventsInAFile(t *testing.T) {
	asFile := config.Defaults()
	asFile.Calendar.Enabled = true
	asFile.Calendar.Paths = []string{meetingsFile(t)}
	fromFile := CalendarCommand(context.Background(), 1, asFile.Calendar, nil, googleDay)().(CalendarMsg).Result

	client := &googleClient{events: googleMeetings()}
	fromGoogle := CalendarCommand(context.Background(), 1, googleSettings(false).Calendar, client, googleDay)().(CalendarMsg).Result

	if fromFile.BusyMinutes != 120 || !fromFile.Available {
		t.Fatalf("the file gave %+v, so this compares nothing", fromFile)
	}
	if fromGoogle.BusyMinutes != fromFile.BusyMinutes || fromGoogle.MeetingCount != fromFile.MeetingCount ||
		fromGoogle.Available != fromFile.Available || fromGoogle.Stale != fromFile.Stale || len(fromGoogle.Events) != len(fromFile.Events) {
		t.Fatalf("google %+v, file %+v", fromGoogle, fromFile)
	}
	for i, event := range fromGoogle.Events {
		other := fromFile.Events[i]
		if event.Summary != other.Summary || !event.Start.Equal(other.Start) || !event.End.Equal(other.End) || event.AllDay != other.AllDay {
			t.Errorf("event %d: google %+v, file %+v", i, event, other)
		}
	}
	if client.asked != 1 {
		t.Errorf("google was asked %d times", client.asked)
	}
}

func TestTheFilesSourceNeverAsksGoogle(t *testing.T) {
	settings := config.Defaults()
	settings.Calendar.Enabled = true
	settings.Calendar.Paths = []string{meetingsFile(t)}
	client := &googleClient{events: googleMeetings()}
	if model := openPlannerWith(t, client, settings); model.CalendarResult.BusyMinutes != 120 || client.asked != 0 {
		t.Errorf("state = %+v, google asked %d times", model.CalendarResult, client.asked)
	}
}

func TestThePlannerAsksForTodayInTheDevicesOwnZone(t *testing.T) {
	client := &googleClient{events: googleMeetings()}
	openPlannerWith(t, client, googleSettings(true))
	day := time.Date(2026, 10, 14, 0, 0, 0, 0, time.Local)
	if !client.window[0].Equal(day) || !client.window[1].Equal(day.AddDate(0, 0, 1)) || client.window[0].Location() != time.Local {
		t.Errorf("window = %v to %v, want %v to %v in the device's own zone, which all-day events follow", client.window[0], client.window[1], day, day.AddDate(0, 0, 1))
	}
}

func TestAGoogleCalendarThatCannotBeReadLeavesPlanningRunning(t *testing.T) {
	reason := "Google Calendar needs reconnecting"
	client := &googleClient{err: &api.Error{Code: api.CodeCalendarUnavailable, Message: reason}}
	model := openPlannerWith(t, client, googleSettings(true))
	if state := model.CalendarResult; state.Available || state.Err != reason || state.BusyMinutes != 0 {
		t.Errorf("state = %+v", state)
	}
	if !model.Planner.Open || model.Planner.Summary.CalendarError != reason || model.Planner.Summary.CalendarMinutes != 0 || len(model.Planner.Items) != 1 {
		t.Errorf("planner = open %v, summary %+v, %d items; want it open with the reason, no calendar time and the tasks", model.Planner.Open, model.Planner.Summary, len(model.Planner.Items))
	}
	if view := model.Planner.View(); !strings.Contains(view, "Calendar unavailable: "+reason) {
		t.Errorf("the planner does not say why:\n%s", view)
	}
}

func TestAServerThatCannotReadGoogleLeavesPlanningRunning(t *testing.T) {
	model := openPlannerWith(t, &fakeClient{}, googleSettings(true))
	if model.CalendarResult.Available || model.CalendarResult.Err != "Google Calendar needs a Momentum server" || !model.Planner.Open {
		t.Errorf("state = %+v, planner open %v", model.CalendarResult, model.Planner.Open)
	}
}
