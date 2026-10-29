package api

import "time"

// Google Calendar is read through the server (ADR 0003): one connection per
// account, whichever device made it. Every route needs the token, and none
// needs an IdempotencyKeyHeader: a second RouteConnectGoogleCalendar only
// makes another link, and the older one expires.
//
// To connect, the app asks RouteConnectGoogleCalendar for a link, opens
// GoogleCalendarLink.URL in the browser, and polls RouteGoogleCalendarStatus
// until Connected, or until ExpiresAt passes. The person finishes in the
// browser, on pages that are not part of this contract.
//
// RouteDisconnectGoogleCalendar asks Google to revoke the token and deletes
// the connection. It answers with GoogleCalendarDisconnect, and it deletes the
// connection even when Google could not be reached to revoke it.
//
// RouteGoogleCalendarEvents answers with the events of the primary calendar
// that overlap the window from QueryStart to QueryEnd, both RFC 3339
// instants, and leaves out the ones the person declined. A repeating event
// arrives as the occurrences in the window, with cancelled ones left out and
// moved ones at their new time. With no connection, or a connection Google
// has refused, it answers CodeCalendarUnavailable with HTTP 424, since the
// client retries a 502, 503 or 504 as an unreachable server. A missing or
// invalid instant answers CodeInvalid, with Field naming it.
const (
	RouteConnectGoogleCalendar    = "POST /v1/calendar/google/connect"
	RouteGoogleCalendarStatus     = "GET /v1/calendar/google"
	RouteDisconnectGoogleCalendar = "DELETE /v1/calendar/google"
	RouteGoogleCalendarEvents     = "GET /v1/calendar/google/events"
)

// Query parameters of RouteGoogleCalendarEvents.
const (
	QueryStart = "start"
	QueryEnd   = "end"
)

// GoogleCalendarLink answers RouteConnectGoogleCalendar. The link works once,
// in the browser that opens it, until ExpiresAt.
type GoogleCalendarLink struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// GoogleCalendarStatus answers RouteGoogleCalendarStatus. Offered is false
// when the server has no Google client or no key to keep the token with, and
// then nothing else is set. Email names the Google account that was
// connected. NeedsReconnect is true while Google refuses the saved
// connection: the person reconnects, or disconnects.
type GoogleCalendarStatus struct {
	Offered        bool   `json:"offered"`
	Connected      bool   `json:"connected"`
	Email          string `json:"email,omitempty"`
	NeedsReconnect bool   `json:"needsReconnect"`
}

// GoogleCalendarDisconnect answers RouteDisconnectGoogleCalendar. Revoked is
// false only when Google could not be asked to revoke the token, which is
// then deleted here but may still show in the Google account's settings;
// with no connection there is nothing to revoke and it is true.
type GoogleCalendarDisconnect struct {
	Revoked bool `json:"revoked"`
}

// GoogleCalendarEvents answers RouteGoogleCalendarEvents.
type GoogleCalendarEvents struct {
	Events []GoogleCalendarEvent `json:"events"`
}

// GoogleCalendarEvent is one event as planning needs it. An AllDay event
// starts at midnight of its first day and ends at midnight after its last,
// in the UTC offset of the window's start. Transparent is Google's "show as
// available".
type GoogleCalendarEvent struct {
	Title       string    `json:"title"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	AllDay      bool      `json:"allDay"`
	Transparent bool      `json:"transparent"`
}
