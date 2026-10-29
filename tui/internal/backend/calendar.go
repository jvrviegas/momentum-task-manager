package backend

import (
	"context"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/calendar"
)

// googleSource is the Source of an event that came from Google Calendar.
const googleSource = "google"

// GoogleCalendarStatus answers whether the server offers Google Calendar, and
// whether this account has connected it (ADR 0003).
func (c *Client) GoogleCalendarStatus(ctx context.Context) (api.GoogleCalendarStatus, error) {
	return c.server.GoogleCalendarStatus(ctx)
}

// ConnectGoogleCalendar asks for the one-time link that connects Google
// Calendar to this account in a browser.
func (c *Client) ConnectGoogleCalendar(ctx context.Context) (api.GoogleCalendarLink, error) {
	return c.server.ConnectGoogleCalendar(ctx)
}

// DisconnectGoogleCalendar disconnects Google Calendar from this account.
func (c *Client) DisconnectGoogleCalendar(ctx context.Context) (api.GoogleCalendarDisconnect, error) {
	return c.server.DisconnectGoogleCalendar(ctx)
}

// GoogleCalendarEvents returns the primary Google calendar's events that
// overlap start to end, as the planner reads them from files. A calendar the
// server cannot read comes back as an *api.Error with CodeCalendarUnavailable,
// whose message is the reason to show.
func (c *Client) GoogleCalendarEvents(ctx context.Context, start, end time.Time) ([]calendar.Event, error) {
	found, err := c.server.GoogleCalendarEvents(ctx, start, end)
	if err != nil {
		return nil, err
	}
	events := make([]calendar.Event, len(found))
	for i, event := range found {
		events[i] = calendar.Event{
			Summary: event.Title, Start: event.Start, End: event.End,
			AllDay: event.AllDay, Transparent: event.Transparent, Source: googleSource,
		}
	}
	return events, nil
}
