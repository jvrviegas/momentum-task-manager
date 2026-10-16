package backend

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// signInServer starts sign-ins with device code "dc" and answers each poll
// with the next of polls, recording the bodies it receives.
func signInServer(t *testing.T, polls ...string) (*api.Client, string, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var bodies []string
	mux := http.NewServeMux()
	mux.HandleFunc(api.RouteStartDeviceSignIn, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"deviceCode":"dc","userCode":"BCDFGHJK","verificationUrl":"https://m.example/device","verificationUrlComplete":"https://m.example/device?code=BCDFGHJK","expiresAt":"2026-10-16T12:10:00Z","intervalSeconds":5}`)
	})
	mux.HandleFunc(api.RoutePollDeviceSignIn, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		bodies = append(bodies, string(body))
		answer := polls[0]
		polls = polls[1:]
		if !strings.Contains(answer, `"token"`) {
			w.WriteHeader(http.StatusBadRequest)
		}
		io.WriteString(w, answer)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return api.NewClient(server.URL, ""), server.URL, &bodies
}

func pollError(code string) string { return `{"code":"` + code + `","message":"from the server"}` }

func TestSignInPollsUntilThePersonSignsIn(t *testing.T) {
	server, url, bodies := signInServer(t,
		pollError(api.CodeAuthorizationPending), pollError(api.CodeSlowDown), pollError(api.CodeAuthorizationPending),
		`{"token":"mtk_new","expiresAt":"2027-01-14T12:00:00Z","email":"ana@example.com"}`)
	var shown api.DeviceSignIn
	var waits []time.Duration
	got, err := signIn(t.Context(), server, url, func(start api.DeviceSignIn) { shown = start },
		func(_ context.Context, delay time.Duration) error { waits = append(waits, delay); return nil })
	want := Credentials{URL: url, Token: "mtk_new", Email: "ana@example.com", ExpiresAt: time.Date(2027, 1, 14, 12, 0, 0, 0, time.UTC)}
	if err != nil || got != want {
		t.Fatalf("credentials %+v, %v; want %+v", got, err, want)
	}
	if shown.UserCode != "BCDFGHJK" || shown.VerificationURLComplete != "https://m.example/device?code=BCDFGHJK" {
		t.Errorf("shown %+v", shown)
	}
	if wantWaits := []time.Duration{5 * time.Second, 5 * time.Second, 10 * time.Second, 10 * time.Second}; !reflect.DeepEqual(waits, wantWaits) {
		t.Errorf("waits %v, want %v", waits, wantWaits)
	}
	if len(*bodies) != 4 || (*bodies)[0] != `{"deviceCode":"dc"}` {
		t.Errorf("poll bodies %q", *bodies)
	}
}

func TestSignInStops(t *testing.T) {
	noWait := func(context.Context, time.Duration) error { return nil }
	for _, test := range []struct {
		name, poll, want string
	}{
		{name: "refused", poll: pollError(api.CodeAccessDenied), want: "sign-in refused: from the server"},
		{name: "expired", poll: pollError(api.CodeExpiredToken), want: "the code expired before sign-in finished; run momentum login again"},
		{name: "another error", poll: pollError(api.CodeInvalid), want: "from the server"},
	} {
		server, url, _ := signInServer(t, test.poll)
		if _, err := signIn(t.Context(), server, url, func(api.DeviceSignIn) {}, noWait); err == nil || err.Error() != test.want {
			t.Errorf("%s: error %v, want %q", test.name, err, test.want)
		}
	}

	server, url, bodies := signInServer(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := signIn(ctx, server, url, func(api.DeviceSignIn) {}, wait); err == nil {
		t.Error("a cancelled sign-in started")
	}
	shown := false
	if _, err := signIn(t.Context(), server, url, func(api.DeviceSignIn) { shown = true },
		func(context.Context, time.Duration) error { return context.Canceled }); !errors.Is(err, context.Canceled) || !shown || len(*bodies) != 0 {
		t.Errorf("cancelled while waiting: error %v, shown %v, polls %d", err, shown, len(*bodies))
	}
}
