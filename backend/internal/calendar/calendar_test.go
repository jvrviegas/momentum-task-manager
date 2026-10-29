package calendar

import (
	"strings"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

func TestStatusBeforeAndAfterConnecting(t *testing.T) {
	h := newHarness(t)
	if status, err := h.service.Status(h.asAlice); err != nil || status != (api.GoogleCalendarStatus{Offered: true}) {
		t.Errorf("Status before connecting = %+v, %v; want offered and nothing else", status, err)
	}
	h.connect()
	if status, err := h.service.Status(h.asAlice); err != nil || status != (api.GoogleCalendarStatus{Offered: true, Connected: true, Email: "alice@gmail.com"}) {
		t.Errorf("Status after connecting = %+v, %v", status, err)
	}
}

func TestDisconnectingRevokesTheTokenAtGoogleAndDeletesTheConnection(t *testing.T) {
	h := newHarness(t)
	h.connect()
	answer, err := h.service.Disconnect(h.asAlice)
	if err != nil || !answer.Revoked {
		t.Fatalf("Disconnect = %+v, %v; want revoked", answer, err)
	}
	if len(h.google.revoked) != 1 || h.google.revoked[0] != testRefreshToken {
		t.Errorf("Google revoked %v, want the refresh token once", h.google.revoked)
	}
	if status, _ := h.service.Status(h.asAlice); status.Connected {
		t.Error("the connection is still there")
	}
	if rows := h.client.CalendarConnection.Query().AllX(h.asAlice); len(rows) != 0 {
		t.Errorf("%d connection rows are left", len(rows))
	}
}

func TestDisconnectingDeletesTheConnectionEvenWhenGoogleCannotRevoke(t *testing.T) {
	for name, change := range map[string]func(*fakeGoogle){
		"Google answers 500": func(f *fakeGoogle) { f.revokeStatus = 500 },
		"Google answers 403": func(f *fakeGoogle) { f.revokeStatus = 403 },
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.connect()
			h.google.set(change)
			answer, err := h.service.Disconnect(h.asAlice)
			if err != nil || answer.Revoked {
				t.Fatalf("Disconnect = %+v, %v; want the connection deleted and not revoked", answer, err)
			}
			if rows := h.client.CalendarConnection.Query().AllX(h.asAlice); len(rows) != 0 {
				t.Errorf("%d connection rows are left", len(rows))
			}
		})
	}
}

func TestATokenGoogleAlreadyCallsInvalidCountsAsRevoked(t *testing.T) {
	h := newHarness(t)
	h.connect()
	h.google.set(func(f *fakeGoogle) { f.revokeStatus, f.revokeBody = 400, `{"error":"invalid_token"}` })
	if answer, err := h.service.Disconnect(h.asAlice); err != nil || !answer.Revoked {
		t.Errorf("Disconnect = %+v, %v; want revoked", answer, err)
	}
}

func TestDisconnectingWithAKeyThatNoLongerOpensTheToken(t *testing.T) {
	h := newHarness(t)
	h.connect()
	rotated := New(h.client, Config{PublicURL: "https://momentum.example", Key: make([]byte, 32), Google: h.service.google})
	answer, err := rotated.Disconnect(h.asAlice)
	if err != nil || answer.Revoked || len(h.google.revoked) != 0 {
		t.Fatalf("Disconnect = %+v, %v, revoked %v; want deleted, not revoked, and Google not asked", answer, err, h.google.revoked)
	}
	if rows := h.client.CalendarConnection.Query().AllX(h.asAlice); len(rows) != 0 {
		t.Errorf("%d connection rows are left", len(rows))
	}
}

func TestDisconnectingWithNoConnectionHasNothingToRevoke(t *testing.T) {
	h := newHarness(t)
	if answer, err := h.service.Disconnect(h.asAlice); err != nil || !answer.Revoked || len(h.google.revoked) != 0 {
		t.Errorf("Disconnect = %+v, %v, revoked %v; want revoked and Google not asked", answer, err, h.google.revoked)
	}
}

func TestUsersOnlySeeAndChangeTheirOwnConnection(t *testing.T) {
	h := newHarness(t)
	h.connect()
	if status, _ := h.service.Status(h.asBob); status != (api.GoogleCalendarStatus{Offered: true}) {
		t.Errorf("Bob's status = %+v, want nothing connected", status)
	}
	start, end := day()
	_, err := h.service.Events(h.asBob, start, end)
	unavailableWith(t, err, reasonNotConnected)
	if answer, err := h.service.Disconnect(h.asBob); err != nil || !answer.Revoked || len(h.google.revoked) != 0 {
		t.Errorf("Bob's Disconnect = %+v, %v, revoked %v", answer, err, h.google.revoked)
	}
	if status, _ := h.service.Status(h.asAlice); !status.Connected {
		t.Error("Bob disconnected Alice")
	}
}

// Nothing the flow logs may hold a token, a code, a link, a state or an event
// title. The run goes through the paths that log: a connection that needs
// reconnecting, and a revoke that fails.
func TestLogsHoldNoSecretsAndNoTitles(t *testing.T) {
	h := newHarness(t)
	token, state := h.begin(h.asAlice)
	h.complete(state)
	h.google.set(func(f *fakeGoogle) {
		f.pages = [][]map[string]any{{timed("Secret board meeting", "2026-10-14T09:00:00+01:00", "2026-10-14T10:00:00+01:00")}}
	})
	start, end := day()
	if _, err := h.service.Events(h.asAlice, start, end); err != nil {
		t.Fatal(err)
	}
	h.google.set(func(f *fakeGoogle) { f.apiStatus = 500 })
	_, _ = h.service.Events(h.asAlice, start, end)
	h.google.set(func(f *fakeGoogle) { f.apiStatus, f.refreshFails = 0, true })
	_, _ = h.service.Events(h.asAlice, start, end)
	h.google.set(func(f *fakeGoogle) { f.revokeStatus = 500 })
	_, _ = h.service.Disconnect(h.asAlice)

	logs := h.logs.String()
	for _, message := range []string{"google calendar needs reconnecting", "reading the Google calendar failed", "revoking the Google token failed"} {
		if !strings.Contains(logs, message) {
			t.Errorf("the logs lack %q, so the run did not reach that path:\n%s", message, logs)
		}
	}
	for name, secret := range map[string]string{
		"the refresh token": testRefreshToken, "an access token": testAccessToken, "the code": testCode,
		"the link": token, "the state": state, "an event title": "Secret board meeting",
		"the Google address": "alice@gmail.com", "the Google subject": "1157",
	} {
		if strings.Contains(logs, secret) {
			t.Errorf("the logs hold %s", name)
		}
	}
}
