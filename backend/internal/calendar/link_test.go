package calendar

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/seal"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/calendarlink"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/privacy"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

func TestStartMakesALinkThatNamesTheAccountAndExpires(t *testing.T) {
	h := newHarness(t)
	link, err := h.service.Start(h.asAlice)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link.URL, "https://momentum.example"+ConnectPath) || !link.ExpiresAt.Equal(h.clock.Now().Add(10*time.Minute)) {
		t.Fatalf("link = %+v, want a connect URL that expires in ten minutes", link)
	}
	token := strings.TrimPrefix(link.URL, "https://momentum.example"+ConnectPath)
	// The page names the Momentum account the link belongs to, whoever opens it.
	if email, err := h.service.Lookup(h.anonymous, token); err != nil || email != "alice@example.com" {
		t.Fatalf("Lookup = %q, %v; want Alice's address", email, err)
	}
	// The database keeps only a hash of the link.
	if rows := h.client.CalendarLink.Query().AllX(h.asAlice); len(rows) != 1 || bytes.Contains(rows[0].LinkHash, []byte(token)) {
		t.Fatalf("stored links = %+v", rows)
	}

	h.clock.advance(10*time.Minute + time.Second)
	if _, err := h.service.Lookup(h.anonymous, token); !errors.Is(err, ErrLinkExpired) {
		t.Errorf("Lookup of an expired link: %v", err)
	}
	if _, _, err := h.service.Begin(h.anonymous, token); !errors.Is(err, ErrLinkExpired) {
		t.Errorf("Begin of an expired link: %v", err)
	}
}

func TestAnUnknownLinkIsNotFound(t *testing.T) {
	h := newHarness(t)
	if _, err := h.service.Lookup(h.anonymous, "nonsense"); !errors.Is(err, ErrLinkExpired) {
		t.Errorf("Lookup: %v", err)
	}
	if _, _, err := h.service.Begin(h.anonymous, "nonsense"); !errors.Is(err, ErrLinkExpired) {
		t.Errorf("Begin: %v", err)
	}
	if _, err := h.service.Complete(h.anonymous, testCode, "nonsense"); !errors.Is(err, ErrLinkExpired) {
		t.Errorf("Complete: %v", err)
	}
}

func TestStartClearsEveryonesExpiredLinks(t *testing.T) {
	h := newHarness(t)
	for _, as := range []context.Context{h.asAlice, h.asBob} {
		if _, err := h.service.Start(as); err != nil {
			t.Fatal(err)
		}
	}
	h.clock.advance(11 * time.Minute)
	if _, err := h.service.Start(h.asAlice); err != nil {
		t.Fatal(err)
	}
	if rows := h.client.CalendarLink.Query().AllX(privacy.DecisionContext(h.anonymous, privacy.Allow)); len(rows) != 1 {
		t.Errorf("%d links are left, want only the new one", len(rows))
	}
}

func TestAServerThatOffersNothingMakesNoLinks(t *testing.T) {
	h := newHarness(t)
	for name, service := range map[string]*Service{
		"no key":    New(h.client, Config{PublicURL: "https://momentum.example", Google: h.service.google}),
		"no Google": New(h.client, Config{PublicURL: "https://momentum.example", Key: h.key}),
		"a bad key": New(h.client, Config{PublicURL: "https://momentum.example", Key: h.key[:16], Google: h.service.google}),
	} {
		if service.Offered() {
			t.Errorf("%s: Offered() is true", name)
		}
		var failure *api.Error
		if _, err := service.Start(h.asAlice); !errors.As(err, &failure) || failure.Code != api.CodeCalendarUnavailable {
			t.Errorf("%s: Start: %v, want calendar_unavailable", name, err)
		}
		if _, err := service.Lookup(h.anonymous, "x"); !errors.Is(err, ErrLinkExpired) {
			t.Errorf("%s: Lookup: %v", name, err)
		}
		if status, err := service.Status(h.asAlice); err != nil || status != (api.GoogleCalendarStatus{}) {
			t.Errorf("%s: Status = %+v, %v; want nothing offered", name, status, err)
		}
	}
	if _, err := h.service.Start(h.anonymous); err == nil {
		t.Error("Start without a signed-in user succeeded")
	}
}

func TestTheAuthorizationURLAsksForReadOnlyCalendarAccess(t *testing.T) {
	h := newHarness(t)
	_, state := h.begin(h.asAlice)
	for key, want := range map[string]string{
		"client_id": testClientID, "redirect_uri": testRedirectURL, "response_type": "code",
		"scope":       "openid email " + Scope,
		"access_type": "offline", "prompt": "consent",
		"code_challenge_method": "S256", "state": state,
	} {
		if got := h.query.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if h.query.Get("code_challenge") == "" || h.query.Get("nonce") == "" {
		t.Errorf("query %v lacks a challenge or a nonce", h.query)
	}
	// The row keeps the state as a hash only, and the verifier and nonce for
	// the callback.
	link := h.client.CalendarLink.Query().OnlyX(h.asAlice)
	if link.StateHash == nil || bytes.Contains(*link.StateHash, []byte(state)) || link.CodeVerifier == "" || link.Nonce == "" {
		t.Errorf("stored round trip = %+v", link)
	}
}

func TestConnectingStoresASealedTokenAndNamesTheGoogleAccount(t *testing.T) {
	h := newHarness(t)
	_, state := h.begin(h.asAlice)
	if email := h.complete(state); email != "alice@gmail.com" {
		t.Errorf("Complete = %q, want the Google account", email)
	}
	status, err := h.service.Status(h.asAlice)
	if err != nil || status != (api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "alice@gmail.com"}) {
		t.Errorf("Status = %+v, %v", status, err)
	}
	row := h.client.CalendarConnection.Query().OnlyX(h.asAlice)
	if row.Subject != "1157" || row.UserID != h.alice {
		t.Errorf("stored connection = %+v", row)
	}
	// Sealed: not the token itself, and it opens with the key and with nothing else.
	if bytes.Contains(row.SealedToken, []byte(testRefreshToken)) {
		t.Fatal("the stored token is not sealed")
	}
	if opened, err := seal.Open(h.key, row.SealedToken); err != nil || string(opened) != testRefreshToken {
		t.Errorf("Open = %q, %v; want the refresh token", opened, err)
	}
	if _, err := seal.Open(bytes.Repeat([]byte{8}, 32), row.SealedToken); err == nil {
		t.Error("another key opens the stored token")
	}
	// The link is spent.
	if links := h.client.CalendarLink.Query().AllX(h.asAlice); len(links) != 0 {
		t.Errorf("%d links are left after connecting", len(links))
	}
}

func TestALinkWorksOnce(t *testing.T) {
	h := newHarness(t)
	_, state := h.begin(h.asAlice)
	h.complete(state)
	if _, err := h.service.Complete(h.anonymous, testCode, state); !errors.Is(err, ErrLinkExpired) {
		t.Errorf("Complete again: %v, want ErrLinkExpired", err)
	}
}

func TestOnlyTheLatestStateWorks(t *testing.T) {
	h := newHarness(t)
	token, first := h.begin(h.asAlice)
	redirect, second, err := h.service.Begin(h.anonymous, token)
	if err != nil {
		t.Fatal(err)
	}
	h.google.visit(redirect)
	if _, err := h.service.Complete(h.anonymous, testCode, first); !errors.Is(err, ErrLinkExpired) {
		t.Errorf("the replaced state: %v, want ErrLinkExpired", err)
	}
	if email := h.complete(second); email == "" {
		t.Error("the latest state did not connect")
	}
}

func TestTwoCompletionsAtOnceConnectOnce(t *testing.T) {
	h := newHarness(t)
	_, state := h.begin(h.asAlice)
	var wg sync.WaitGroup
	results := make(chan error, 6)
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.service.Complete(h.anonymous, testCode, state)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, ErrLinkExpired) {
			t.Errorf("a completion failed with %v", err)
		}
	}
	if succeeded != 1 {
		t.Errorf("%d completions succeeded, want exactly one", succeeded)
	}
}

func TestAnExpiredStateDoesNotConnect(t *testing.T) {
	h := newHarness(t)
	_, state := h.begin(h.asAlice)
	h.clock.advance(11 * time.Minute)
	if _, err := h.service.Complete(h.anonymous, testCode, state); !errors.Is(err, ErrLinkExpired) {
		t.Errorf("Complete: %v, want ErrLinkExpired", err)
	}
	if status, _ := h.service.Status(h.asAlice); status.Connected {
		t.Error("an expired link connected")
	}
}

func TestNothingIsConnectedWithoutTheCalendarPermission(t *testing.T) {
	for name, change := range map[string]func(*fakeGoogle){
		"the permission was left unticked": func(f *fakeGoogle) { f.scope = "openid https://www.googleapis.com/auth/userinfo.email" },
		"Google sent no refresh token":     func(f *fakeGoogle) { f.noRefreshToken = true },
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.google.set(change)
			_, state := h.begin(h.asAlice)
			if _, err := h.service.Complete(h.anonymous, testCode, state); !errors.Is(err, ErrNotGranted) {
				t.Fatalf("Complete: %v, want ErrNotGranted", err)
			}
			if status, _ := h.service.Status(h.asAlice); status.Connected {
				t.Error("a connection was saved")
			}
		})
	}
}

func TestACodeForAnotherRoundTripDoesNotConnect(t *testing.T) {
	h := newHarness(t)
	_, state := h.begin(h.asAlice)
	if _, err := h.service.Complete(h.anonymous, "someone-elses-code", state); err == nil || errors.Is(err, ErrLinkExpired) {
		t.Errorf("Complete with a wrong code: %v, want Google's refusal", err)
	}
	if status, _ := h.service.Status(h.asAlice); status.Connected {
		t.Error("a connection was saved")
	}
}

func TestReconnectingReplacesTheConnectionInPlace(t *testing.T) {
	h := newHarness(t)
	h.connect()
	h.google.set(func(f *fakeGoogle) { f.refreshFails = true })
	if _, err := h.service.Events(h.asAlice, h.clock.Now(), h.clock.Now().Add(time.Hour)); err == nil {
		t.Fatal("Events succeeded with a refused connection")
	}
	if status, _ := h.service.Status(h.asAlice); !status.NeedsReconnect {
		t.Fatalf("Status = %+v, want needs reconnect", status)
	}

	// Reconnecting, with another Google account this time.
	h.google.set(func(f *fakeGoogle) { f.refreshFails, f.subject, f.email = false, "2290", "alice@work.example" })
	_, state := h.begin(h.asAlice)
	if email := h.complete(state); email != "alice@work.example" {
		t.Errorf("Complete = %q", email)
	}
	status, err := h.service.Status(h.asAlice)
	if err != nil || status != (api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "alice@work.example"}) {
		t.Errorf("Status = %+v, %v; want connected and no longer needing a reconnect", status, err)
	}
	if rows := h.client.CalendarConnection.Query().AllX(h.asAlice); len(rows) != 1 || rows[0].Subject != "2290" {
		t.Errorf("connections = %+v, want one, replaced in place", rows)
	}
}

func TestTheBypassReachesNothingElse(t *testing.T) {
	h := newHarness(t)
	_, aliceState := h.begin(h.asAlice)
	bobLink, err := h.service.Start(h.asBob)
	if err != nil {
		t.Fatal(err)
	}
	bobToken := strings.TrimPrefix(bobLink.URL, "https://momentum.example"+ConnectPath)

	// Each lookup finds its own link and names its own account.
	if email, err := h.service.Lookup(h.anonymous, bobToken); err != nil || email != "bob@example.com" {
		t.Errorf("Lookup of Bob's link = %q, %v", email, err)
	}
	// Starting Alice's round trip left Bob's link untouched, and connecting
	// Alice saved her row only.
	h.complete(aliceState)
	// A NULL state reads back as a pointer to an empty slice, so check the length.
	row := h.client.CalendarLink.Query().Where(calendarlink.UserID(h.bob)).OnlyX(privacy.DecisionContext(h.anonymous, privacy.Allow))
	if (row.StateHash != nil && len(*row.StateHash) > 0) || row.Nonce != "" || row.CodeVerifier != "" {
		t.Errorf("Bob's link was changed: %d state bytes, nonce %q, verifier %q", len(*row.StateHash), row.Nonce, row.CodeVerifier)
	}
	if status, _ := h.service.Status(h.asBob); status.Connected {
		t.Error("Alice's connect reached Bob's account")
	}
	// The service never leaves the client open: without the bypass, reads
	// still need a signed-in user.
	if _, err := h.client.CalendarLink.Query().All(h.anonymous); !errors.Is(err, privacy.Deny) {
		t.Errorf("a query without a user: %v, want it denied", err)
	}
	if _, err := h.client.CalendarConnection.Query().All(h.anonymous); !errors.Is(err, privacy.Deny) {
		t.Errorf("a connection query without a user: %v, want it denied", err)
	}
}
