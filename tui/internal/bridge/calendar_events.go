package bridge

import (
	"time"

	"github.com/jvrviegas/momentum-task-manager/tui/internal/calendar"
)

// loadParams carries, for load and plan, the app's own calendar events when its
// source is the Mac's Calendar, which only the app can read. Without them the
// bridge reads the [calendar] files.
type loadParams struct {
	Calendar *calendarInput `json:"calendar"`
}

// calendarInput stands in for the calendar files. Error says why the app
// could not read the calendars, such as "no access", which makes the calendar
// unavailable with that reason. It stands in for [calendar] enabled, which a
// choice of the Mac's Calendar neither reads nor changes.
type calendarInput struct {
	Error  string      `json:"error,omitempty"`
	Events []eventWire `json:"events"`
}

// eventWire is one event as the Mac's Calendar has it. Free is an event the
// calendar shows as free rather than busy. The app sends Title only with plan.
type eventWire struct {
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	AllDay bool      `json:"allDay"`
	Free   bool      `json:"free"`
	Title  string    `json:"title,omitempty"`
}

func (c calendarInput) events() []calendar.Event {
	events := make([]calendar.Event, len(c.Events))
	for i, event := range c.Events {
		events[i] = calendar.Event{
			Summary: event.Title, Start: event.Start, End: event.End, AllDay: event.AllDay, Transparent: event.Free, Source: "macOS Calendar",
		}
	}
	return events
}
