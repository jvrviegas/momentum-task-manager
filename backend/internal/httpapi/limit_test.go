package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/auth"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/calendar"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/logging"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/tasks"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

func TestLimiterCountsEachClientInFixedWindows(t *testing.T) {
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	l := newLimiter(2, time.Minute)
	l.now = func() time.Time { return now }
	for i, want := range []bool{true, true, false, false} {
		if ok, _ := l.allow("a"); ok != want {
			t.Fatalf("request %d from a: %v", i+1, ok)
		}
	}
	if ok, _ := l.allow("b"); !ok {
		t.Fatal("b shares a's count")
	}
	now = now.Add(45 * time.Second)
	if ok, wait := l.allow("a"); ok || wait != 15*time.Second {
		t.Fatalf("a later in the window: %v, wait %v", ok, wait)
	}
	now = now.Add(15 * time.Second)
	if ok, wait := l.allow("a"); !ok || wait != time.Minute {
		t.Fatalf("a in the next window: %v, wait %v", ok, wait)
	}
}

func TestClientAddressTrustsOnlyTheProxiesEntries(t *testing.T) {
	for _, c := range []struct {
		name, remote string
		forwarded    []string
		hops         int
		want         string
	}{
		{"no proxy ignores the header", "198.51.100.4:5555", []string{"203.0.113.7"}, 0, "198.51.100.4"},
		{"one proxy, no header of the client's", "10.0.0.1:443", []string{"203.0.113.7"}, 1, "203.0.113.7"},
		{"one proxy after a forged entry", "10.0.0.1:443", []string{"6.6.6.6, 203.0.113.7"}, 1, "203.0.113.7"},
		{"one proxy, forged header first", "10.0.0.1:443", []string{"6.6.6.6", "203.0.113.7"}, 1, "203.0.113.7"},
		{"a load balancer and Cloud Run", "10.0.0.1:443", []string{"6.6.6.6,203.0.113.7,34.1.2.3"}, 2, "203.0.113.7"},
		{"fewer entries than proxies", "10.0.0.1:443", []string{"203.0.113.7"}, 2, "10.0.0.1"},
		{"no header with a proxy", "10.0.0.1:443", nil, 1, "10.0.0.1"},
		{"IPv6 stands for its /64", "10.0.0.1:443", []string{"2001:db8:1:2:aaaa::1"}, 1, "2001:db8:1:2::/64"},
		{"IPv6 connection", "[2001:db8:1:2::9]:5555", nil, 0, "2001:db8:1:2::/64"},
		{"IPv4 in IPv6", "10.0.0.1:443", []string{"::ffff:203.0.113.7"}, 1, "203.0.113.7"},
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/auth/device", nil)
		request.RemoteAddr = c.remote
		for _, value := range c.forwarded {
			request.Header.Add("X-Forwarded-For", value)
		}
		if got := clientAddress(request, c.hops); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// TestSignInRoutesAreLimitedPerClient reaches the limits without a database:
// each limit is checked before its route does anything.
func TestSignInRoutesAreLimitedPerClient(t *testing.T) {
	var logs bytes.Buffer
	handler := New(logging.New(&logs), "dev", auth.New(nil, auth.Config{PublicURL: "https://momentum.test"}), tasks.New(nil), calendar.New(nil, calendar.Config{}), 1)
	send := func(method, path, client string, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.RemoteAddr = "10.0.0.1:443"
		request.Header.Set("X-Forwarded-For", client)
		if method == http.MethodPost && path == devicePath {
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	alice, bob := "203.0.113.7", "203.0.113.8"

	// Forms without the CSRF cookie are refused without touching sign-in.
	for i := range signInLimit {
		if response := send(http.MethodPost, devicePath, alice, "code=BCDF-GHJK&provider=google"); response.Code != http.StatusForbidden {
			t.Fatalf("form %d: %d", i+1, response.Code)
		}
	}
	page := send(http.MethodPost, devicePath, alice, "")
	retry, _ := strconv.Atoi(page.Header().Get("Retry-After"))
	if page.Code != http.StatusTooManyRequests || !strings.Contains(page.Body.String(), "Too many attempts") ||
		retry < 1 || retry > int(limitWindow/time.Second) || page.Header().Get("Content-Security-Policy") == "" {
		t.Errorf("form over the limit: %d, Retry-After %q, %s", page.Code, page.Header().Get("Retry-After"), page.Body.String())
	}
	// Starting a sign-in and the callbacks share the limit.
	start := send(http.MethodPost, "/v1/auth/device", alice, "")
	var failure api.Error
	json.Unmarshal(start.Body.Bytes(), &failure)
	if start.Code != http.StatusTooManyRequests || failure.Code != api.CodeRateLimited || start.Header().Get("Retry-After") == "" {
		t.Errorf("start over the limit: %d %s", start.Code, start.Body.String())
	}
	if callback := send(http.MethodGet, "/auth/callback/google?code=x&state=y", alice, ""); callback.Code != http.StatusTooManyRequests {
		t.Errorf("callback over the limit: %d", callback.Code)
	}
	// A forged entry in front of the proxy's does not make a new client.
	if forged := send(http.MethodPost, "/v1/auth/device", "6.6.6.6, "+alice, ""); forged.Code != http.StatusTooManyRequests {
		t.Errorf("forged X-Forwarded-For: %d", forged.Code)
	}
	// Another client, the device page, and signed-in routes are not limited.
	if other := send(http.MethodPost, devicePath, bob, ""); other.Code != http.StatusForbidden {
		t.Errorf("another client's form: %d", other.Code)
	}
	if form := send(http.MethodGet, devicePath, alice, ""); form.Code != http.StatusOK {
		t.Errorf("device page: %d", form.Code)
	}
	if tasks := send(http.MethodGet, "/v1/tasks?status=pending", alice, ""); tasks.Code != http.StatusUnauthorized {
		t.Errorf("task list without a token: %d", tasks.Code)
	}

	// Polling has its own, larger limit, and over it answers slow_down.
	for i := range pollLimit {
		if response := send(http.MethodPost, "/v1/auth/token", bob, "not json"); response.Code != http.StatusBadRequest ||
			!strings.Contains(response.Body.String(), api.CodeInvalid) {
			t.Fatalf("poll %d: %d %s", i+1, response.Code, response.Body.String())
		}
	}
	poll := send(http.MethodPost, "/v1/auth/token", bob, "not json")
	json.Unmarshal(poll.Body.Bytes(), &failure)
	if poll.Code != http.StatusBadRequest || failure.Code != api.CodeSlowDown || poll.Header().Get("Retry-After") == "" {
		t.Errorf("poll over the limit: %d %s", poll.Code, poll.Body.String())
	}
	if strings.Contains(logs.String(), alice) || strings.Contains(logs.String(), bob) {
		t.Errorf("the log holds a client address:\n%s", logs.String())
	}
}
