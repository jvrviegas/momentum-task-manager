package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/calendar"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// fakeGoogle is Google as the calendar service needs it. It connects whoever
// reaches it with the code "calendar-code".
type fakeGoogle struct {
	grant calendar.Grant

	mu      sync.Mutex
	events  []api.GoogleCalendarEvent
	revoked []string
	window  [2]time.Time
}

func (f *fakeGoogle) AuthCodeURL(state, _, _ string) string {
	return "https://google.test/authorize?state=" + url.QueryEscape(state)
}

func (f *fakeGoogle) Exchange(_ context.Context, code, _, _ string) (calendar.Grant, error) {
	if code != "calendar-code" {
		return calendar.Grant{}, errors.New("fake Google: wrong code")
	}
	return f.grant, nil
}

func (f *fakeGoogle) AccessToken(context.Context, string) (string, error) { return "access", nil }

func (f *fakeGoogle) Revoke(_ context.Context, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revoked = append(f.revoked, token)
	return nil
}

func (f *fakeGoogle) Events(_ context.Context, _ string, start, end time.Time) ([]api.GoogleCalendarEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.window = [2]time.Time{start, end}
	return f.events, nil
}

func (f *fakeGoogle) revokedTokens() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.revoked...)
}

// startConnect asks for the link as the app would and returns its path.
func (h *harness) startConnect(token string) string {
	h.t.Helper()
	response, body := h.postJSON("/v1/calendar/google/connect", token, nil)
	var link api.GoogleCalendarLink
	if err := json.Unmarshal([]byte(body), &link); err != nil || response.StatusCode != http.StatusOK ||
		response.Header.Get("Cache-Control") != "no-store" || !strings.HasPrefix(link.URL, "http://momentum.test"+calendar.ConnectPath) {
		h.t.Fatalf("connect: %d %s", response.StatusCode, body)
	}
	return strings.TrimPrefix(link.URL, "http://momentum.test")
}

// continueToGoogle opens the link in browser, submits its form, and returns
// the state Google will send back.
func (h *harness) continueToGoogle(browser *http.Client, path string) string {
	h.t.Helper()
	response, page := h.do(browser, http.MethodGet, path, "", nil, "")
	match := csrfField.FindStringSubmatch(page)
	if response.StatusCode != http.StatusOK || match == nil {
		h.t.Fatalf("connect page: %d %s", response.StatusCode, page)
	}
	response, page = h.do(browser, http.MethodPost, path, "", strings.NewReader(url.Values{"csrf": {match[1]}}.Encode()), "application/x-www-form-urlencoded")
	location, _ := url.Parse(response.Header.Get("Location"))
	if response.StatusCode != http.StatusSeeOther || location == nil || location.Host != "google.test" {
		h.t.Fatalf("continue: %d %s %s", response.StatusCode, response.Header.Get("Location"), page)
	}
	return location.Query().Get("state")
}

func (h *harness) calendarCallback(browser *http.Client, query string) (int, string) {
	h.t.Helper()
	response, page := h.do(browser, http.MethodGet, calendar.CallbackPath+"?"+query, "", nil, "")
	return response.StatusCode, page
}

func (h *harness) status(token string) api.GoogleCalendarStatus {
	h.t.Helper()
	response, body := h.do(http.DefaultClient, http.MethodGet, "/v1/calendar/google", token, nil, "")
	var status api.GoogleCalendarStatus
	if err := json.Unmarshal([]byte(body), &status); err != nil || response.StatusCode != http.StatusOK {
		h.t.Fatalf("status: %d %s", response.StatusCode, body)
	}
	return status
}

func TestConnectingGoogleCalendarOverHTTP(t *testing.T) {
	h := newHarness(t)
	h.google.events = []api.GoogleCalendarEvent{{Title: "Secret board meeting", Start: time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)}}
	token := h.signIn("alice@example.com")
	if status := h.status(token.Token); status != (api.GoogleCalendarStatus{Offered: true}) {
		t.Fatalf("status before connecting = %+v", status)
	}

	path := h.startConnect(token.Token)
	browser := h.browser()
	response, page := h.do(browser, http.MethodGet, path, "", nil, "")
	for header, want := range map[string]string{
		"X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer", "Cache-Control": "no-store",
		"Content-Security-Policy": "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'",
	} {
		if response.Header.Get(header) != want {
			t.Errorf("%s = %q, want %q", header, response.Header.Get(header), want)
		}
	}
	// The page names whose Momentum account the calendar would join.
	if !strings.Contains(page, "<strong>alice@example.com</strong>") || !strings.Contains(page, "Continue to Google") ||
		!strings.Contains(page, "Only continue if that is your account") {
		t.Errorf("connect page:\n%s", page)
	}

	state := h.continueToGoogle(browser, path)
	status, page := h.calendarCallback(browser, "code=calendar-code&state="+url.QueryEscape(state))
	if status != http.StatusOK || !strings.Contains(page, "Google Calendar is connected as alice@gmail.com") {
		t.Fatalf("callback: %d %s", status, page)
	}
	if got := h.status(token.Token); got != (api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "alice@gmail.com"}) {
		t.Fatalf("status after connecting = %+v", got)
	}

	// Events come back for the window as it was sent, in its own offset.
	query := url.Values{"start": {"2026-10-14T00:00:00+01:00"}, "end": {"2026-10-15T00:00:00+01:00"}}
	response, body := h.do(http.DefaultClient, http.MethodGet, "/v1/calendar/google/events?"+query.Encode(), token.Token, nil, "")
	var events api.GoogleCalendarEvents
	if err := json.Unmarshal([]byte(body), &events); err != nil || response.StatusCode != http.StatusOK || len(events.Events) != 1 ||
		events.Events[0].Title != "Secret board meeting" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("events: %d %s", response.StatusCode, body)
	}
	if _, offset := h.google.window[0].Zone(); offset != 3600 {
		t.Errorf("the window reached Google at offset %d, want +01:00", offset)
	}

	response, body = h.do(http.DefaultClient, http.MethodDelete, "/v1/calendar/google", token.Token, nil, "")
	var disconnected api.GoogleCalendarDisconnect
	if err := json.Unmarshal([]byte(body), &disconnected); err != nil || response.StatusCode != http.StatusOK || !disconnected.Revoked {
		t.Fatalf("disconnect: %d %s", response.StatusCode, body)
	}
	if revoked := h.google.revokedTokens(); len(revoked) != 1 || revoked[0] != "refresh-secret" {
		t.Errorf("revoked %v, want the refresh token once", revoked)
	}
	if got := h.status(token.Token); got != (api.GoogleCalendarStatus{Offered: true}) {
		t.Errorf("status after disconnecting = %+v", got)
	}

	// None of the link, the state, the code, the token or a title reaches the log.
	for name, secret := range map[string]string{
		"the link": strings.TrimPrefix(path, calendar.ConnectPath), "the state": state, "the code": "calendar-code",
		"the refresh token": "refresh-secret", "an event title": "Secret board meeting", "the Google address": "alice@gmail.com",
	} {
		if strings.Contains(h.logs.String(), secret) {
			t.Errorf("the log holds %s:\n%s", name, h.logs.String())
		}
	}
}

func TestCalendarRoutesNeedASignedInUser(t *testing.T) {
	h := newHarness(t)
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/v1/calendar/google/connect"},
		{http.MethodGet, "/v1/calendar/google"},
		{http.MethodDelete, "/v1/calendar/google"},
		{http.MethodGet, "/v1/calendar/google/events?start=2026-10-14T00:00:00Z&end=2026-10-15T00:00:00Z"},
	} {
		for _, token := range []string{"", "mtk_unknown"} {
			response, body := h.do(http.DefaultClient, route.method, route.path, token, nil, "")
			var failure api.Error
			if err := json.Unmarshal([]byte(body), &failure); err != nil || response.StatusCode != http.StatusUnauthorized || failure.Code != api.CodeUnauthorized {
				t.Errorf("%s %s with token %q: %d %s", route.method, route.path, token, response.StatusCode, body)
			}
		}
	}
}

func TestAServerThatOffersNothingSaysSo(t *testing.T) {
	h := newHarness(t)
	token := h.signIn("alice@example.com")
	bare := httptest.NewServer(New(h.server.logger, "dev", h.server.auth, h.server.tasks, calendar.New(h.client, calendar.Config{}), 0))
	t.Cleanup(bare.Close)
	do := func(method, path string) (int, string) {
		request, _ := http.NewRequest(method, bare.URL+path, nil)
		request.Header.Set("Authorization", "Bearer "+token.Token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		return response.StatusCode, string(data)
	}
	if status, body := do(http.MethodGet, "/v1/calendar/google"); status != http.StatusOK || strings.TrimSpace(body) != `{"offered":false,"connected":false,"needsReconnect":false}` {
		t.Errorf("status = %d %s", status, body)
	}
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/v1/calendar/google/connect"},
		{http.MethodGet, "/v1/calendar/google/events?start=2026-10-14T00:00:00Z&end=2026-10-15T00:00:00Z"},
	} {
		status, body := do(route.method, route.path)
		var failure api.Error
		if err := json.Unmarshal([]byte(body), &failure); err != nil || status != http.StatusFailedDependency || failure.Code != api.CodeCalendarUnavailable ||
			failure.Message != "Google Calendar isn’t available on this server" {
			t.Errorf("%s %s = %d %s", route.method, route.path, status, body)
		}
	}
}

func TestEventsNeedAValidWindow(t *testing.T) {
	h := newHarness(t)
	token := h.signIn("alice@example.com")
	for name, test := range map[string]struct {
		query url.Values
		field string
	}{
		"no start":       {url.Values{"end": {"2026-10-15T00:00:00Z"}}, "start"},
		"no end":         {url.Values{"start": {"2026-10-14T00:00:00Z"}}, "end"},
		"a bad start":    {url.Values{"start": {"yesterday"}, "end": {"2026-10-15T00:00:00Z"}}, "start"},
		"a date only":    {url.Values{"start": {"2026-10-14"}, "end": {"2026-10-15T00:00:00Z"}}, "start"},
		"end before":     {url.Values{"start": {"2026-10-15T00:00:00Z"}, "end": {"2026-10-14T00:00:00Z"}}, "end"},
		"an empty range": {url.Values{"start": {"2026-10-14T00:00:00Z"}, "end": {"2026-10-14T00:00:00Z"}}, "end"},
	} {
		response, body := h.do(http.DefaultClient, http.MethodGet, "/v1/calendar/google/events?"+test.query.Encode(), token.Token, nil, "")
		var failure api.Error
		if err := json.Unmarshal([]byte(body), &failure); err != nil || response.StatusCode != http.StatusBadRequest || failure.Code != api.CodeInvalid || failure.Field != test.field {
			t.Errorf("%s: %d %s, want invalid on %q", name, response.StatusCode, body, test.field)
		}
	}
	// Valid, but nothing is connected yet: the reason comes back as it is.
	response, body := h.do(http.DefaultClient, http.MethodGet, "/v1/calendar/google/events?start=2026-10-14T00:00:00Z&end=2026-10-15T00:00:00Z", token.Token, nil, "")
	var failure api.Error
	if err := json.Unmarshal([]byte(body), &failure); err != nil || response.StatusCode != http.StatusFailedDependency ||
		failure.Code != api.CodeCalendarUnavailable || failure.Message != "Google Calendar isn’t connected" {
		t.Errorf("events before connecting: %d %s", response.StatusCode, body)
	}
}

func TestTheConnectFormNeedsItsCSRFCookie(t *testing.T) {
	h := newHarness(t)
	token := h.signIn("alice@example.com")
	path := h.startConnect(token.Token)
	page := h.browser()
	_, body := h.do(page, http.MethodGet, path, "", nil, "")
	csrf := csrfField.FindStringSubmatch(body)[1]
	// Another site can post the form, but not with the person's cookie.
	for name, test := range map[string]struct {
		client *http.Client
		csrf   string
	}{"another browser": {h.browser(), csrf}, "a forged value": {page, "forged"}, "no value": {page, ""}} {
		response, body := h.do(test.client, http.MethodPost, path, "", strings.NewReader(url.Values{"csrf": {test.csrf}}.Encode()), "application/x-www-form-urlencoded")
		if response.StatusCode != http.StatusForbidden || response.Header.Get("Location") != "" {
			t.Errorf("%s: %d %s", name, response.StatusCode, body)
		}
	}
	// The link was not spent by the refusals.
	if state := h.continueToGoogle(page, path); state == "" {
		t.Error("the link no longer continues after refused submissions")
	}
}

func TestTheCallbackNeedsTheBrowserThatContinued(t *testing.T) {
	h := newHarness(t)
	token := h.signIn("alice@example.com")
	path := h.startConnect(token.Token)
	browser := h.browser()
	state := h.continueToGoogle(browser, path)
	code := "code=calendar-code&state="

	// Another browser has no state cookie; a forged state does not match it.
	if status, page := h.calendarCallback(h.browser(), code+url.QueryEscape(state)); status != http.StatusForbidden || !strings.Contains(page, "did not start in this browser") {
		t.Errorf("another browser: %d %s", status, page)
	}
	if status, _ := h.calendarCallback(browser, code+"forged"); status != http.StatusForbidden {
		t.Errorf("a forged state: %d", status)
	}
	if got := h.status(token.Token); got.Connected {
		t.Fatal("a refused callback connected")
	}

	// The refusal above cleared the cookie, so this browser must continue again.
	state = h.continueToGoogle(browser, path)
	if status, page := h.calendarCallback(browser, code+url.QueryEscape(state)); status != http.StatusOK {
		t.Fatalf("the right browser: %d %s", status, page)
	}
	// The state works once: a replay finds no cookie and no link.
	if status, _ := h.calendarCallback(browser, code+url.QueryEscape(state)); status != http.StatusForbidden {
		t.Errorf("a replay: %d, want 403", status)
	}
}

func TestPagesExplainWhyConnectingFailed(t *testing.T) {
	h := newHarness(t)
	token := h.signIn("alice@example.com")

	// Google sends the person back without approving.
	path := h.startConnect(token.Token)
	browser := h.browser()
	h.continueToGoogle(browser, path)
	if status, page := h.calendarCallback(browser, "error=access_denied"); status != http.StatusBadRequest || !strings.Contains(page, "Google Calendar wasn’t connected") {
		t.Errorf("a refused approval: %d %s", status, page)
	}

	// A link that is unknown, or already used, is said to have expired.
	for name, link := range map[string]string{"unknown": calendar.ConnectPath + "nonsense"} {
		if response, page := h.do(h.browser(), http.MethodGet, link, "", nil, ""); response.StatusCode != http.StatusBadRequest || !strings.Contains(page, "This link expired") {
			t.Errorf("%s link: %d %s", name, response.StatusCode, page)
		}
	}
	used := h.startConnect(token.Token)
	b := h.browser()
	state := h.continueToGoogle(b, used)
	if status, _ := h.calendarCallback(b, "code=calendar-code&state="+url.QueryEscape(state)); status != http.StatusOK {
		t.Fatal("connecting failed")
	}
	if response, page := h.do(h.browser(), http.MethodGet, used, "", nil, ""); response.StatusCode != http.StatusBadRequest || !strings.Contains(page, "This link expired") {
		t.Errorf("a used link: %d %s", response.StatusCode, page)
	}

	// Google refuses the code: the page does not say why, and the log holds only a type.
	h.google.grant = calendar.Grant{}
	path = h.startConnect(token.Token)
	b = h.browser()
	state = h.continueToGoogle(b, path)
	if status, page := h.calendarCallback(b, "code=wrong&state="+url.QueryEscape(state)); status != http.StatusBadGateway || strings.Contains(page, "wrong code") {
		t.Errorf("a refused code: %d %s", status, page)
	}
	if strings.Contains(h.logs.String(), "fake Google: wrong code") {
		t.Errorf("the log holds Google's error text:\n%s", h.logs.String())
	}
}

// The connection belongs to the account, not to a device (ADR 0003, C3), so a
// device signing out, and the same person signing in on another, leaves it.
func TestSigningOutAndInAgainKeepsTheCalendarConnection(t *testing.T) {
	h := newHarness(t)
	first := h.signIn("alice@example.com")
	path := h.startConnect(first.Token)
	browser := h.browser()
	state := h.continueToGoogle(browser, path)
	if status, _ := h.calendarCallback(browser, "code=calendar-code&state="+url.QueryEscape(state)); status != http.StatusOK {
		t.Fatal("connecting failed")
	}

	if response, _ := h.do(http.DefaultClient, http.MethodPost, "/v1/auth/sign-out", first.Token, nil, ""); response.StatusCode != http.StatusNoContent {
		t.Fatalf("sign-out: %d", response.StatusCode)
	}
	if response, _ := h.do(http.DefaultClient, http.MethodGet, "/v1/calendar/google", first.Token, nil, ""); response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("the signed-out device still reaches the calendar: %d", response.StatusCode)
	}

	second := h.signIn("alice@example.com")
	if got := h.status(second.Token); got != (api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "alice@gmail.com"}) {
		t.Errorf("status on the new device = %+v, want the connection kept", got)
	}
	if revoked := h.google.revokedTokens(); len(revoked) != 0 {
		t.Errorf("Google was asked to revoke %v when a device signed out", revoked)
	}
}

func TestEachUserSeesOnlyTheirOwnCalendarConnection(t *testing.T) {
	h := newHarness(t)
	alice, bob := h.signIn("alice@example.com"), h.signIn("bob@example.com")
	path := h.startConnect(alice.Token)
	browser := h.browser()
	state := h.continueToGoogle(browser, path)
	if status, _ := h.calendarCallback(browser, "code=calendar-code&state="+url.QueryEscape(state)); status != http.StatusOK {
		t.Fatal("connecting failed")
	}
	if got := h.status(bob.Token); got != (api.GoogleCalendarStatus{Offered: true}) {
		t.Errorf("Bob's status = %+v", got)
	}
	if got := h.status(alice.Token); !got.Connected {
		t.Errorf("Alice's status = %+v", got)
	}
	// Bob's disconnect leaves Alice's connection alone.
	if response, _ := h.do(http.DefaultClient, http.MethodDelete, "/v1/calendar/google", bob.Token, nil, ""); response.StatusCode != http.StatusOK {
		t.Fatalf("Bob's disconnect: %d", response.StatusCode)
	}
	if got := h.status(alice.Token); !got.Connected || len(h.google.revokedTokens()) != 0 {
		t.Errorf("Alice's status after Bob disconnected = %+v", got)
	}
}

func TestConnectingIsLimitedPerClient(t *testing.T) {
	h := newHarness(t)
	token := h.signIn("alice@example.com")
	for attempt := 1; attempt <= signInLimit+1; attempt++ {
		response, body := h.postJSON("/v1/calendar/google/connect", token.Token, nil)
		if attempt <= signInLimit && response.StatusCode != http.StatusOK {
			t.Fatalf("attempt %d: %d %s", attempt, response.StatusCode, body)
		}
		if attempt > signInLimit {
			var failure api.Error
			if err := json.Unmarshal([]byte(body), &failure); err != nil || response.StatusCode != http.StatusTooManyRequests ||
				failure.Code != api.CodeRateLimited || response.Header.Get("Retry-After") == "" {
				t.Errorf("attempt %d: %d %s, want rate_limited with Retry-After", attempt, response.StatusCode, body)
			}
		}
	}
	// Sign-in has its own limit, so connecting does not use it up.
	if response, _ := h.postJSON("/v1/auth/device", "", nil); response.StatusCode != http.StatusOK {
		t.Errorf("starting a sign-in after the connect limit: %d", response.StatusCode)
	}
}
