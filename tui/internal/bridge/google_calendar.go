package bridge

import (
	"context"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/calendar"
)

// googleCacheTTL is how long a load reuses the Google events it last read
// (ADR 0003, D7). It keeps the app's one-minute refresh from calling Google
// every minute; opening Plan Today always asks again.
const googleCacheTTL = 5 * time.Minute

// googleCache is today's Google events as they were last read, without their
// titles, and when. The titles only ever reach Plan Today.
type googleCache struct {
	ok     bool
	day    string
	at     time.Time
	events []calendar.Event
}

// googleToday returns today's events from the Google calendar the server
// reads for this account, in the device's own offset. Without fresh, which
// Plan Today sets, a read less than googleCacheTTL old is reused; either way
// the answer is kept for the loads after it. A failure is not kept, so the
// next load asks again, and a reconnect shows at once.
func (s *Server) googleToday(ctx context.Context, now time.Time, fresh bool) ([]calendar.Event, error) {
	day := now.Format(planDateLayout)
	s.mu.Lock()
	cached := s.googleEvents
	s.mu.Unlock()
	if age := now.Sub(cached.at); !fresh && cached.ok && cached.day == day && age >= 0 && age < googleCacheTTL {
		return cached.events, nil
	}
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	events, err := s.client.GoogleCalendarEvents(ctx, start, start.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	untitled := make([]calendar.Event, len(events))
	for i, event := range events {
		event.Summary = ""
		untitled[i] = event
	}
	s.mu.Lock()
	s.googleEvents = googleCache{ok: true, day: day, at: now, events: untitled}
	s.mu.Unlock()
	if !fresh {
		return untitled, nil
	}
	return events, nil
}

// forgetGoogle drops the kept events, for when the account changes.
func (s *Server) forgetGoogle() {
	s.mu.Lock()
	s.googleEvents = googleCache{}
	s.mu.Unlock()
}

type connectCalendarResult struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expiresAt"`
}

// calendarStatus answers whether the server offers Google Calendar and
// whether this account has connected it.
func (s *Server) calendarStatus(ctx context.Context) (*api.GoogleCalendarStatus, error) {
	status, err := s.client.GoogleCalendarStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &status, nil
}

// connectCalendar asks the server for the one-time link that connects Google
// Calendar to this account. The app opens it in the browser and asks
// calendarStatus until the account is connected.
func (s *Server) connectCalendar(ctx context.Context) (*connectCalendarResult, error) {
	link, err := s.client.ConnectGoogleCalendar(ctx)
	if err != nil {
		return nil, err
	}
	return &connectCalendarResult{URL: link.URL, ExpiresAt: stamp(&link.ExpiresAt)}, nil
}

// disconnectCalendar disconnects Google Calendar from this account, and says
// whether the server could revoke the access at Google too.
func (s *Server) disconnectCalendar(ctx context.Context) (*api.GoogleCalendarDisconnect, error) {
	answer, err := s.client.DisconnectGoogleCalendar(ctx)
	if err != nil {
		return nil, err
	}
	s.forgetGoogle()
	return &answer, nil
}
