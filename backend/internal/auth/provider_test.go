package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/oauth2"
)

// browser remembers what an authorization URL carried, as the provider would:
// the PKCE challenge, which a token request's verifier must match, and the
// nonce to put in the ID token.
type browser struct{ challenge, nonce string }

func (p *browser) remember(t *testing.T, authURL string) url.Values {
	t.Helper()
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("code_challenge_method") != "S256" || query.Get("prompt") != "select_account" {
		t.Fatalf("authorization URL %q lacks S256 PKCE or the account picker", authURL)
	}
	p.challenge, p.nonce = query.Get("code_challenge"), query.Get("nonce")
	return query
}

func (p *browser) matches(verifier string) bool {
	sum := sha256.Sum256([]byte(verifier))
	return verifier != "" && base64.RawURLEncoding.EncodeToString(sum[:]) == p.challenge
}

// fakeIssuer is an OpenID Connect provider that signs the ID token claims
// returns for the nonce the browser carried, for the code "google-code".
func fakeIssuer(t *testing.T, check *browser, claims func(issuer, nonce string) map[string]any) string {
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
	var issuer string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token",
			"jwks_uri": issuer + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "key", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("code") != "google-code" || !check.matches(r.FormValue("code_verifier")) {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		idToken, err := jwt.Signed(signer).Claims(claims(issuer, check.nonce)).Serialize()
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "token_type": "Bearer", "id_token": idToken})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	issuer = server.URL
	return issuer
}

func googleClaims(overrides map[string]any) func(issuer, nonce string) map[string]any {
	return func(issuer, nonce string) map[string]any {
		claims := map[string]any{
			"iss": issuer, "aud": "client-id", "sub": "1111", "email": "alice@example.com", "email_verified": true,
			"nonce": nonce, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
		}
		for key, value := range overrides {
			claims[key] = value
		}
		return claims
	}
}

func TestGoogleIdentifiesTheVerifiedAccount(t *testing.T) {
	for _, test := range []struct {
		name      string
		overrides map[string]any
		want      string // the error, or "" for alice
	}{
		{"verified", nil, ""},
		{"another sign-in's nonce", map[string]any{"nonce": "other"}, "another sign-in"},
		{"unverified email", map[string]any{"email_verified": false}, "not verified"},
		{"another audience", map[string]any{"aud": "someone-else"}, "verify the Google ID token"},
		{"expired", map[string]any{"exp": time.Now().Add(-time.Minute).Unix()}, "verify the Google ID token"},
	} {
		t.Run(test.name, func(t *testing.T) {
			check := &browser{}
			issuer := fakeIssuer(t, check, googleClaims(test.overrides))
			provider, err := NewGoogle(t.Context(), issuer, "client-id", "secret", "https://momentum.example/auth/callback/google")
			if err != nil {
				t.Fatal(err)
			}
			verifier := oauth2.GenerateVerifier()
			query := check.remember(t, provider.AuthCodeURL("state", "nonce-1", verifier))
			if query.Get("nonce") != "nonce-1" || query.Get("state") != "state" || query.Get("scope") != "openid email" ||
				query.Get("redirect_uri") != "https://momentum.example/auth/callback/google" {
				t.Fatalf("authorization URL query %v", query)
			}

			identity, err := provider.Identify(t.Context(), "google-code", "nonce-1", verifier)
			switch {
			case test.want == "" && (err != nil || identity != Identity{Subject: "1111", Email: "alice@example.com"}):
				t.Fatalf("Identify = %+v, %v", identity, err)
			case test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)):
				t.Fatalf("Identify = %+v, %v; want an error with %q", identity, err, test.want)
			}
		})
	}
}

func TestGoogleRequiresThePKCEVerifier(t *testing.T) {
	check := &browser{}
	issuer := fakeIssuer(t, check, googleClaims(nil))
	provider, err := NewGoogle(t.Context(), issuer, "client-id", "secret", "https://momentum.example/auth/callback/google")
	if err != nil {
		t.Fatal(err)
	}
	check.remember(t, provider.AuthCodeURL("state", "nonce-1", oauth2.GenerateVerifier()))
	if _, err := provider.Identify(t.Context(), "google-code", "nonce-1", oauth2.GenerateVerifier()); err == nil {
		t.Fatal("Identify succeeded with another sign-in's verifier")
	}
}

// fakeGitHub answers GitHub's token, user, and email endpoints.
func fakeGitHub(t *testing.T, check *browser, emails []map[string]any) GitHubEndpoints {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("code") != "github-code" || !check.matches(r.FormValue("code_verifier")) {
			http.Error(w, `{"error":"bad_verification_code"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "gho_token", "token_type": "bearer", "scope": "read:user,user:email"})
	})
	authorized := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer gho_token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("GET /user", authorized(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 12345, "login": "alice"})
	}))
	mux.HandleFunc("GET /user/emails", authorized(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(emails)
	}))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return GitHubEndpoints{AuthURL: server.URL + "/login/oauth/authorize", TokenURL: server.URL + "/login/oauth/access_token", APIURL: server.URL}
}

func TestGitHubIdentifiesThePrimaryVerifiedEmail(t *testing.T) {
	for _, test := range []struct {
		name   string
		emails []map[string]any
		want   string // the email, or "" for an error
	}{
		{"primary and verified", []map[string]any{
			{"email": "old@example.com", "primary": false, "verified": true},
			{"email": "alice@example.com", "primary": true, "verified": true},
		}, "alice@example.com"},
		{"primary but unverified", []map[string]any{
			{"email": "alice@example.com", "primary": true, "verified": false},
			{"email": "other@example.com", "primary": false, "verified": true},
		}, ""},
		{"no emails", nil, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			check := &browser{}
			provider := NewGitHub(fakeGitHub(t, check, test.emails), "client-id", "secret", "https://momentum.example/auth/callback/github")
			verifier := oauth2.GenerateVerifier()
			query := check.remember(t, provider.AuthCodeURL("state", "", verifier))
			if query.Get("scope") != "read:user user:email" || query.Get("state") != "state" {
				t.Fatalf("authorization URL query %v", query)
			}
			identity, err := provider.Identify(t.Context(), "github-code", "", verifier)
			switch {
			case test.want != "" && (err != nil || identity != Identity{Subject: "12345", Email: test.want}):
				t.Fatalf("Identify = %+v, %v", identity, err)
			case test.want == "" && err == nil:
				t.Fatalf("Identify = %+v, want an error", identity)
			}
		})
	}
	check := &browser{}
	provider := NewGitHub(fakeGitHub(t, check, nil), "client-id", "secret", "https://momentum.example/auth/callback/github")
	check.remember(t, provider.AuthCodeURL("state", "", oauth2.GenerateVerifier()))
	if _, err := provider.Identify(t.Context(), "github-code", "", oauth2.GenerateVerifier()); err == nil {
		t.Fatal("Identify succeeded with another sign-in's verifier")
	}
}
