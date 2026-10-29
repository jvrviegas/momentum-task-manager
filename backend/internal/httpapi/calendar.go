package httpapi

import (
	"errors"
	"html/template"
	"net/http"
	"time"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/calendar"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// The browser half of connecting Google Calendar. The link the app opens
// names the Momentum account and asks to continue; Google sends the browser
// back to the callback. Each page keeps its own cookie, so connecting never
// mixes with signing in.
const (
	calendarCSRFCookie  = "momentum_calendar_csrf"
	calendarStateCookie = "momentum_calendar_state"
)

func refuseConnect(w http.ResponseWriter) {
	writeError(w, &api.Error{Code: api.CodeRateLimited, Message: "too many attempts to connect Google Calendar; wait a few minutes and try again"})
}

// calendarFailed answers an error from the calendar service, and reports
// whether there was one.
func (s *server) calendarFailed(w http.ResponseWriter, r *http.Request, err error) bool {
	var failure *api.Error
	switch {
	case err == nil:
		return false
	case errors.As(err, &failure):
		writeError(w, failure)
	default:
		s.internalError(w, r, err)
	}
	return true
}

func (s *server) connectGoogleCalendar(w http.ResponseWriter, r *http.Request) {
	link, err := s.calendar.Start(r.Context())
	if s.calendarFailed(w, r, err) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, link)
}

func (s *server) googleCalendarStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.calendar.Status(r.Context())
	if s.calendarFailed(w, r, err) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, status)
}

func (s *server) disconnectGoogleCalendar(w http.ResponseWriter, r *http.Request) {
	answer, err := s.calendar.Disconnect(r.Context())
	if s.calendarFailed(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, answer)
}

func (s *server) googleCalendarEvents(w http.ResponseWriter, r *http.Request) {
	start, ok := queryTime(w, r, api.QueryStart)
	if !ok {
		return
	}
	end, ok := queryTime(w, r, api.QueryEnd)
	if !ok {
		return
	}
	if !end.After(start) {
		writeError(w, &api.Error{Code: api.CodeInvalid, Message: api.QueryEnd + " must be after " + api.QueryStart, Field: api.QueryEnd})
		return
	}
	events, err := s.calendar.Events(r.Context(), start, end)
	if s.calendarFailed(w, r, err) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, api.GoogleCalendarEvents{Events: events})
}

// queryTime reads the RFC 3339 instant in query parameter name, in the offset
// it was sent in, or answers CodeInvalid.
func queryTime(w http.ResponseWriter, r *http.Request, name string) (time.Time, bool) {
	value, err := time.Parse(time.RFC3339, r.URL.Query().Get(name))
	if err != nil {
		writeError(w, &api.Error{Code: api.CodeInvalid, Message: name + " must be an RFC 3339 time", Field: name})
		return time.Time{}, false
	}
	return value, true
}

type connectPageData struct{ Email, CSRF string }

// connectPage names the Momentum account the link belongs to and asks to
// continue. Whoever opens the link sees whose account their calendar would
// join, so a link made by someone else cannot connect their calendar quietly.
// It sets a CSRF cookie that the form must match.
func (s *server) connectPage(w http.ResponseWriter, r *http.Request) {
	email, err := s.calendar.Lookup(r.Context(), r.PathValue("link"))
	if s.connectFailed(w, r, err, http.StatusInternalServerError) {
		return
	}
	csrf := randomToken()
	http.SetCookie(w, &http.Cookie{
		Name: calendarCSRFCookie, Value: csrf, Path: calendar.ConnectPath, MaxAge: cookieSeconds,
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode,
	})
	render(w, http.StatusOK, connectPageTemplate, connectPageData{Email: email, CSRF: csrf})
}

// connectSubmit starts the Google round trip, and keeps its state in a cookie
// that the callback must match.
func (s *server) connectSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxForm)
	if err := r.ParseForm(); err != nil {
		render(w, http.StatusBadRequest, resultPageTemplate, resultPageData{"Connecting failed", "The form could not be read. Choose Connect… in Momentum again."})
		return
	}
	cookie, err := r.Cookie(calendarCSRFCookie)
	if err != nil || !sameSecret(cookie.Value, r.PostForm.Get("csrf")) {
		render(w, http.StatusForbidden, resultPageTemplate, resultPageData{"This page expired", "Choose Connect… in Momentum again."})
		return
	}
	redirect, state, err := s.calendar.Begin(r.Context(), r.PathValue("link"))
	if s.connectFailed(w, r, err, http.StatusInternalServerError) {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: calendarStateCookie, Value: state, Path: calendar.CallbackPath, MaxAge: cookieSeconds,
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// connectCallback is where Google sends the browser back. The state must
// match the cookie, so only the browser that continued from the link can
// finish connecting.
func (s *server) connectCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	http.SetCookie(w, &http.Cookie{Name: calendarStateCookie, Path: calendar.CallbackPath, MaxAge: -1, HttpOnly: true, Secure: s.secure})
	if query.Get("error") != "" {
		render(w, http.StatusBadRequest, resultPageTemplate, resultPageData{"Google Calendar wasn’t connected", "Nothing was approved. Choose Connect… in Momentum to try again."})
		return
	}
	state := query.Get("state")
	cookie, err := r.Cookie(calendarStateCookie)
	if err != nil || !sameSecret(cookie.Value, state) {
		render(w, http.StatusForbidden, resultPageTemplate, resultPageData{"Connecting failed", "This did not start in this browser. Choose Connect… in Momentum again."})
		return
	}
	email, err := s.calendar.Complete(r.Context(), query.Get("code"), state)
	if s.connectFailed(w, r, err, http.StatusBadGateway) {
		return
	}
	render(w, http.StatusOK, resultPageTemplate, resultPageData{"Google Calendar connected", "Google Calendar is connected as " + email + ". Return to Momentum; you can close this window."})
}

// connectFailed answers a failed step of connecting with a page, and reports
// whether there was one. Anything but an expired link or a refused permission
// is the server's or Google's, so it answers status and logs only the type of
// the error.
func (s *server) connectFailed(w http.ResponseWriter, r *http.Request, err error, status int) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, calendar.ErrLinkExpired):
		render(w, http.StatusBadRequest, resultPageTemplate, resultPageData{"This link expired", err.Error()})
	case errors.Is(err, calendar.ErrNotGranted):
		render(w, http.StatusBadRequest, resultPageTemplate, resultPageData{"Google Calendar wasn’t connected", err.Error()})
	default:
		s.logger.ErrorContext(r.Context(), "connecting Google Calendar failed", "error_type", errorType(err))
		render(w, status, resultPageTemplate, resultPageData{"Connecting failed", "Connecting Google Calendar failed. Choose Connect… in Momentum to try again."})
	}
	return true
}

var connectPageTemplate = template.Must(template.Must(template.New("connect").Parse(layout)).Parse(`{{define "content"}}
<h1>Connect Google Calendar</h1>
<p>Your Momentum server will read today’s meetings in your primary Google calendar, so Plan Today can count them. It never changes your calendar.</p>
<p>You are connecting it to the Momentum account <strong>{{.Email}}</strong>.</p>
<form method="post">
<input type="hidden" name="csrf" value="{{.CSRF}}">
<p class="warning">Only continue if that is your account and you chose Connect… in Momentum yourself. Whoever gets you to open their link could read your calendar from their account.</p>
<button>Continue to Google</button>
</form>
{{end}}`))
