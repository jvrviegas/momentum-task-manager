package calendar

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/storetest"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/viewer"
)

const (
	testClientID     = "client-id"
	testRedirectURL  = "https://momentum.example" + CallbackPath
	testRefreshToken = "refresh-secret-1"
	testAccessToken  = "access-secret-"
	testCode         = "google-code"
)

// fakeGoogle is Google on httptest: OpenID discovery, the token endpoint
// (the code exchange with PKCE, and the refresh), the revoke endpoint, and the
// Calendar API. It records what it was asked, and its fields change what it
// answers.
type fakeGoogle struct {
	t   *testing.T
	URL string
	key *rsa.PrivateKey

	mu sync.Mutex
	// What the browser carried to Google, as Google would remember it.
	challenge, nonce string
	// What the code exchange gives.
	scope          string
	noRefreshToken bool
	subject, email string
	// What each endpoint answers.
	refreshFails  bool
	refreshStatus int
	revokeStatus  int
	revokeBody    string
	apiStatus     int
	pages         [][]map[string]any
	// What was asked.
	tokenRequests []url.Values
	revoked       []string
	eventRequests []url.Values
	eventAuth     []string
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "key"}},
		(&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeGoogle{
		t: t, key: key, scope: "openid https://www.googleapis.com/auth/userinfo.email " + Scope,
		subject: "1157", email: "alice@gmail.com",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": f.URL, "authorization_endpoint": f.URL + "/authorize", "token_endpoint": f.URL + "/token",
			"jwks_uri": f.URL + "/jwks", "revocation_endpoint": f.URL + "/revoke",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "key", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = r.ParseForm()
		f.tokenRequests = append(f.tokenRequests, r.PostForm)
		w.Header().Set("Content-Type", "application/json")
		switch r.PostForm.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
			if r.PostForm.Get("code") != testCode || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge ||
				r.PostForm.Get("redirect_uri") != testRedirectURL {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			idToken, err := jwt.Signed(signer).Claims(map[string]any{
				"iss": f.URL, "aud": testClientID, "sub": f.subject, "email": f.email, "email_verified": true,
				"nonce": f.nonce, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
			}).Serialize()
			if err != nil {
				t.Error(err)
			}
			answer := map[string]any{"access_token": testAccessToken + "1", "token_type": "Bearer", "expires_in": 3600, "id_token": idToken, "scope": f.scope}
			if !f.noRefreshToken {
				answer["refresh_token"] = testRefreshToken
			}
			_ = json.NewEncoder(w).Encode(answer)
		case "refresh_token":
			if f.refreshStatus != 0 {
				w.WriteHeader(f.refreshStatus)
				return
			}
			if f.refreshFails || r.PostForm.Get("refresh_token") != testRefreshToken {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": testAccessToken + strconv.Itoa(len(f.tokenRequests)), "token_type": "Bearer", "expires_in": 3600,
			})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	mux.HandleFunc("POST /revoke", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = r.ParseForm()
		f.revoked = append(f.revoked, r.PostForm.Get("token"))
		if f.revokeStatus != 0 {
			w.WriteHeader(f.revokeStatus)
			_, _ = w.Write([]byte(f.revokeBody))
		}
	})
	mux.HandleFunc("GET /calendar/v3/calendars/primary/events", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.eventRequests = append(f.eventRequests, r.URL.Query())
		f.eventAuth = append(f.eventAuth, r.Header.Get("Authorization"))
		if f.apiStatus != 0 {
			w.WriteHeader(f.apiStatus)
			return
		}
		page := 0
		if token := r.URL.Query().Get("pageToken"); token != "" {
			page, _ = strconv.Atoi(token)
		}
		answer := map[string]any{"items": []map[string]any{}}
		if page < len(f.pages) {
			answer["items"] = f.pages[page]
		}
		if page+1 < len(f.pages) {
			answer["nextPageToken"] = strconv.Itoa(page + 1)
		}
		_ = json.NewEncoder(w).Encode(answer)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	f.URL = server.URL
	return f
}

// visit is the browser reaching Google's authorization endpoint: Google
// remembers the PKCE challenge and the nonce, and the URL it was sent to.
func (f *fakeGoogle) visit(authURL string) url.Values {
	f.t.Helper()
	parsed, err := url.Parse(authURL)
	if err != nil || !strings.HasPrefix(authURL, f.URL+"/authorize?") {
		f.t.Fatalf("authorization URL %q, %v; want one at %s/authorize", authURL, err, f.URL)
	}
	query := parsed.Query()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.challenge, f.nonce = query.Get("code_challenge"), query.Get("nonce")
	return query
}

func (f *fakeGoogle) set(change func(*fakeGoogle)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	change(f)
}

// testClock is a clock the tests move.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// harness is a Service over a fresh database and the fake Google, with two
// users who may each act through their own context.
type harness struct {
	t         *testing.T
	client    *ent.Client
	google    *fakeGoogle
	service   *Service
	clock     *testClock
	key       []byte
	logs      *bytes.Buffer
	asAlice   context.Context
	asBob     context.Context
	alice     uuid.UUID
	bob       uuid.UUID
	anonymous context.Context
	// query is what the last begin sent Google's authorization endpoint.
	query url.Values
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	client := storetest.Client(t)
	ctx := t.Context()
	alice := client.User.Create().SetEmail("alice@example.com").SaveX(ctx)
	bob := client.User.Create().SetEmail("bob@example.com").SaveX(ctx)
	google := newFakeGoogle(t)
	provider, err := NewGoogle(ctx, google.URL, testClientID, "client-secret", testRedirectURL, google.URL+"/calendar/v3")
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{
		t: t, client: client, google: google, clock: &testClock{now: time.Now()}, key: bytes.Repeat([]byte{7}, 32),
		logs: &bytes.Buffer{}, asAlice: viewer.NewContext(ctx, alice.ID), asBob: viewer.NewContext(ctx, bob.ID),
		alice: alice.ID, bob: bob.ID, anonymous: ctx,
	}
	h.service = New(client, Config{
		PublicURL: "https://momentum.example", Key: h.key, Google: provider,
		Logger: slog.New(slog.NewJSONHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	h.service.now = h.clock.Now
	return h
}

// connect runs the whole flow for Alice, as the app and her browser would,
// and returns the link's token and the state.
func (h *harness) connect() (token, state string) {
	h.t.Helper()
	token, state = h.begin(h.asAlice)
	h.complete(state)
	return token, state
}

// begin runs Start, Lookup and Begin for the user ctx acts for, and Google's
// authorization endpoint.
func (h *harness) begin(ctx context.Context) (token, state string) {
	h.t.Helper()
	link, err := h.service.Start(ctx)
	if err != nil {
		h.t.Fatalf("Start: %v", err)
	}
	token = strings.TrimPrefix(link.URL, "https://momentum.example"+ConnectPath)
	if _, err := h.service.Lookup(h.anonymous, token); err != nil {
		h.t.Fatalf("Lookup: %v", err)
	}
	redirect, state, err := h.service.Begin(h.anonymous, token)
	if err != nil {
		h.t.Fatalf("Begin: %v", err)
	}
	h.query = h.google.visit(redirect)
	return token, state
}

func (h *harness) complete(state string) string {
	h.t.Helper()
	email, err := h.service.Complete(h.anonymous, testCode, state)
	if err != nil {
		h.t.Fatalf("Complete: %v", err)
	}
	return email
}
