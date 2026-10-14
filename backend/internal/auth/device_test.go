package auth

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/identity"
	entsession "github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/session"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/storetest"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// fakeProvider signs in whoever it is told to, and remembers what it was
// given.
type fakeProvider struct {
	name     string
	identity Identity
	err      error
	nonce    string
	verifier string
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) AuthCodeURL(state, nonce, verifier string) string {
	f.nonce, f.verifier = nonce, verifier
	return "https://" + f.name + ".test/authorize?state=" + url.QueryEscape(state)
}

func (f *fakeProvider) Identify(_ context.Context, code, nonce, verifier string) (Identity, error) {
	if code != "provider-code" || nonce != f.nonce || verifier != f.verifier {
		return Identity{}, errors.New("fake provider: wrong code, nonce, or verifier")
	}
	return f.identity, f.err
}

// clock is a time the test moves by hand.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time          { return c.now }
func (c *clock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newService(t *testing.T, providers ...Provider) (*Service, *ent.Client, *clock) {
	t.Helper()
	client := storetest.Client(t)
	service := New(client, Config{
		PublicURL:     "https://momentum.example/",
		AllowedEmails: []string{" Alice@Example.com ", "bob@example.com"},
		Providers:     providers,
	})
	now := &clock{now: time.Now().Truncate(time.Second)}
	service.now = now.Now
	return service, client, now
}

func fakeGoogle(email string) *fakeProvider {
	return &fakeProvider{name: GoogleName, identity: Identity{Subject: "google-" + email, Email: email}}
}

// signIn runs the browser half for userCode and returns its outcome.
func signIn(t *testing.T, service *Service, provider *fakeProvider, userCode string) SignIn {
	t.Helper()
	redirect, state, err := service.BeginSignIn(t.Context(), userCode, provider.name)
	if err != nil {
		t.Fatalf("BeginSignIn: %v", err)
	}
	if parsed, _ := url.Parse(redirect); parsed.Query().Get("state") != state {
		t.Fatalf("redirect %q does not carry the state", redirect)
	}
	result, err := service.CompleteSignIn(t.Context(), provider.name, "provider-code", state)
	if err != nil {
		t.Fatalf("CompleteSignIn: %v", err)
	}
	return result
}

func pollCode(t *testing.T, service *Service, deviceCode string) string {
	t.Helper()
	_, err := service.Poll(t.Context(), deviceCode)
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("Poll error %v is not an api.Error", err)
	}
	return apiErr.Code
}

func TestDeviceSignInIssuesOneTokenForTheApprovedAccount(t *testing.T) {
	provider := fakeGoogle("Alice@Example.com")
	service, client, now := newService(t, provider)
	ctx := t.Context()
	start, err := service.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if start.VerificationURL != "https://momentum.example/device" ||
		start.VerificationURLComplete != "https://momentum.example/device?code="+start.UserCode ||
		normalizeUserCode(start.UserCode) != start.UserCode || start.IntervalSeconds != 5 ||
		!start.ExpiresAt.Equal(now.now.Add(10*time.Minute)) {
		t.Fatalf("Start = %+v", start)
	}
	if code := pollCode(t, service, start.DeviceCode); code != api.CodeAuthorizationPending {
		t.Fatalf("poll before sign-in: %s", code)
	}

	// The code may be typed in any case, without its dash.
	typed := "  " + start.UserCode[:4] + start.UserCode[5:] + " "
	if result := signIn(t, service, provider, typed); !result.Approved || result.Email != "alice@example.com" {
		t.Fatalf("sign-in = %+v", result)
	}

	now.Advance(5 * time.Second)
	token, err := service.Poll(ctx, start.DeviceCode)
	if err != nil {
		t.Fatalf("poll after sign-in: %v", err)
	}
	if token.Email != "alice@example.com" || !token.ExpiresAt.Equal(now.now.Add(90*24*time.Hour)) || len(token.Token) < 40 {
		t.Fatalf("token = %+v", token)
	}
	session, err := service.Authenticate(ctx, token.Token)
	if err != nil || session.Email != "alice@example.com" {
		t.Fatalf("Authenticate = %+v, %v", session, err)
	}
	// Only the hash is stored.
	stored := client.Session.GetX(ctx, session.ID)
	if string(stored.TokenHash) == token.Token || string(stored.TokenHash) != string(hashSecret(token.Token)) {
		t.Fatal("the session does not store exactly the token's hash")
	}
	// The token is issued once.
	if code := pollCode(t, service, start.DeviceCode); code != api.CodeExpiredToken {
		t.Fatalf("second poll: %s", code)
	}
	if count := client.Session.Query().CountX(ctx); count != 1 {
		t.Fatalf("%d sessions, want 1", count)
	}
}

func TestPollingTooOftenSlowsDown(t *testing.T) {
	service, _, now := newService(t, fakeGoogle("alice@example.com"))
	start, err := service.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		wait time.Duration
		want string
	}{
		{0, api.CodeAuthorizationPending},
		{time.Second, api.CodeSlowDown},
		{5 * time.Second, api.CodeAuthorizationPending},
	} {
		now.Advance(step.wait)
		if code := pollCode(t, service, start.DeviceCode); code != step.want {
			t.Fatalf("after %v: %s, want %s", step.wait, code, step.want)
		}
	}
}

func TestAccountsOutsideTheAllowlistAreDenied(t *testing.T) {
	provider := fakeGoogle("mallory@example.com")
	service, client, _ := newService(t, provider)
	start, err := service.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result := signIn(t, service, provider, start.UserCode); result.Approved || result.Email != "mallory@example.com" {
		t.Fatalf("sign-in = %+v", result)
	}
	if code := pollCode(t, service, start.DeviceCode); code != api.CodeAccessDenied {
		t.Fatalf("poll: %s", code)
	}
	if users := client.User.Query().CountX(t.Context()); users != 0 {
		t.Fatalf("%d users created, want none", users)
	}
}

func TestExpiredAndUnknownSignInsAreRefused(t *testing.T) {
	provider := fakeGoogle("alice@example.com")
	service, _, now := newService(t, provider)
	ctx := t.Context()
	if code := pollCode(t, service, "no-such-device-code"); code != api.CodeExpiredToken {
		t.Fatalf("unknown device code: %s", code)
	}
	if _, _, err := service.BeginSignIn(ctx, "BCDF-GHJK", GoogleName); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("unknown user code: %v", err)
	}
	start, err := service.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.BeginSignIn(ctx, start.UserCode, "facebook"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("unknown provider: %v", err)
	}
	_, state, err := service.BeginSignIn(ctx, start.UserCode, GoogleName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteSignIn(ctx, GoogleName, "provider-code", "forged-state"); !errors.Is(err, ErrSignInExpired) {
		t.Fatalf("forged state: %v", err)
	}
	if _, err := service.CompleteSignIn(ctx, GitHubName, "provider-code", state); !errors.Is(err, ErrSignInExpired) {
		t.Fatalf("state for another provider: %v", err)
	}

	now.Advance(10 * time.Minute)
	if _, err := service.CompleteSignIn(ctx, GoogleName, "provider-code", state); !errors.Is(err, ErrSignInExpired) {
		t.Fatalf("expired sign-in: %v", err)
	}
	if _, _, err := service.BeginSignIn(ctx, start.UserCode, GoogleName); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expired user code: %v", err)
	}
	if code := pollCode(t, service, start.DeviceCode); code != api.CodeExpiredToken {
		t.Fatalf("expired device code: %s", code)
	}
}

func TestStateWorksOnce(t *testing.T) {
	provider := fakeGoogle("alice@example.com")
	service, _, _ := newService(t, provider)
	ctx := t.Context()
	start, err := service.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, state, err := service.BeginSignIn(ctx, start.UserCode, GoogleName)
	if err != nil {
		t.Fatal(err)
	}
	provider.err = errors.New("the provider failed")
	if _, err := service.CompleteSignIn(ctx, GoogleName, "provider-code", state); err == nil {
		t.Fatal("CompleteSignIn succeeded although the provider failed")
	}
	provider.err = nil
	if _, err := service.CompleteSignIn(ctx, GoogleName, "provider-code", state); !errors.Is(err, ErrSignInExpired) {
		t.Fatalf("reused state: %v", err)
	}
	// The person can start the browser half again with the same code.
	if result := signIn(t, service, provider, start.UserCode); !result.Approved {
		t.Fatalf("second attempt = %+v", result)
	}
}

func TestIdentitiesWithTheSameEmailShareAUser(t *testing.T) {
	alice := fakeGoogle("alice@example.com")
	aliceOnGitHub := &fakeProvider{name: GitHubName, identity: Identity{Subject: "12345", Email: "ALICE@example.com"}}
	service, client, now := newService(t, alice, aliceOnGitHub)
	ctx := t.Context()
	var tokens []api.Token
	for _, provider := range []*fakeProvider{alice, aliceOnGitHub, alice} {
		start, err := service.Start(ctx)
		if err != nil {
			t.Fatal(err)
		}
		signIn(t, service, provider, start.UserCode)
		now.Advance(5 * time.Second)
		token, err := service.Poll(ctx, start.DeviceCode)
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}
	if users := client.User.Query().CountX(ctx); users != 1 {
		t.Fatalf("%d users, want 1", users)
	}
	if linked := client.Identity.Query().Where(identity.Provider(GitHubName), identity.Subject("12345")).CountX(ctx); linked != 1 {
		t.Fatalf("%d GitHub identities, want 1", linked)
	}
	// Each device has its own token.
	if tokens[0].Token == tokens[1].Token || tokens[0].Token == tokens[2].Token {
		t.Fatal("devices share a token")
	}
}

func TestRevokedAndExpiredTokensStopWorking(t *testing.T) {
	provider := fakeGoogle("alice@example.com")
	service, client, now := newService(t, provider)
	ctx := t.Context()
	issue := func() api.Token {
		start, err := service.Start(ctx)
		if err != nil {
			t.Fatal(err)
		}
		signIn(t, service, provider, start.UserCode)
		now.Advance(5 * time.Second)
		token, err := service.Poll(ctx, start.DeviceCode)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	laptop, phone := issue(), issue()
	session, err := service.Authenticate(ctx, laptop.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Revoke(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, laptop.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked token: %v", err)
	}
	if _, err := service.Authenticate(ctx, phone.Token); err != nil {
		t.Fatalf("revoking one device signed out another: %v", err)
	}
	if revoked := client.Session.Query().Where(entsession.RevokedAtNotNil()).CountX(ctx); revoked != 1 {
		t.Fatalf("%d revoked sessions, want 1", revoked)
	}
	now.Advance(90 * 24 * time.Hour)
	if _, err := service.Authenticate(ctx, phone.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expired token: %v", err)
	}
	for _, token := range []string{"", "Bearer", "mtk_", "mtk_unknown", phone.Token[4:]} {
		if _, err := service.Authenticate(ctx, token); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("Authenticate(%q): %v", token, err)
		}
	}
}

func TestUserCodes(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		code := newUserCode()
		if normalizeUserCode(code) != code || len(code) != 9 {
			t.Fatalf("newUserCode() = %q", code)
		}
		seen[code] = true
	}
	if len(seen) < 999 {
		t.Fatalf("%d distinct codes in 1000", len(seen))
	}
	for _, text := range []string{"", "BCDF-GHJ", "BCDF-GHJKL", "ABCD-EFGH", "BCDF-GHJ1", "bcdf_ghjk"} {
		if got := normalizeUserCode(text); got != "" {
			t.Errorf("normalizeUserCode(%q) = %q, want none", text, got)
		}
	}
}
