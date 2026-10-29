package calendar

import (
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// eventsPage is one page of the Calendar API's events.list answer, as far as
// planning reads it.
type eventsPage struct {
	Items         []googleEvent `json:"items"`
	NextPageToken string        `json:"nextPageToken"`
}

type googleEvent struct {
	Status       string     `json:"status"`
	Summary      string     `json:"summary"`
	Transparency string     `json:"transparency"`
	Start        googleTime `json:"start"`
	End          googleTime `json:"end"`
	Attendees    []struct {
		Self           bool   `json:"self"`
		ResponseStatus string `json:"responseStatus"`
	} `json:"attendees"`
}

// googleTime is a moment, or with Date set an all-day event's date.
type googleTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
}

// event returns the event as planning needs it, or false for one that does
// not count: a cancelled one, which the API leaves out already, or one the
// person declined (D4). An all-day event starts at midnight of its first day
// and ends at midnight after its last, in loc, the offset of the window the
// app asked for.
func (e googleEvent) event(loc *time.Location) (api.GoogleCalendarEvent, bool) {
	if e.Status == "cancelled" {
		return api.GoogleCalendarEvent{}, false
	}
	for _, attendee := range e.Attendees {
		if attendee.Self && attendee.ResponseStatus == "declined" {
			return api.GoogleCalendarEvent{}, false
		}
	}
	start, allDay, ok := e.Start.moment(loc)
	if !ok {
		return api.GoogleCalendarEvent{}, false
	}
	end, _, ok := e.End.moment(loc)
	if !ok {
		return api.GoogleCalendarEvent{}, false
	}
	return api.GoogleCalendarEvent{Title: e.Summary, Start: start, End: end, AllDay: allDay, Transparent: e.Transparency == "transparent"}, true
}

func (t googleTime) moment(loc *time.Location) (moment time.Time, allDay, ok bool) {
	if t.DateTime != "" {
		moment, err := time.Parse(time.RFC3339, t.DateTime)
		return moment, false, err == nil
	}
	day, err := time.ParseInLocation(time.DateOnly, t.Date, loc)
	return day, true, err == nil
}
