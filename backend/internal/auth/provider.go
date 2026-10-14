package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Provider is where the browser half of sign-in happens.
type Provider interface {
	// Name is the provider's name in URLs and in identities, such as "google".
	Name() string
	// AuthCodeURL is where to send the browser. The provider sends it back
	// with state; nonce and the PKCE verifier bind the result to this
	// sign-in.
	AuthCodeURL(state, nonce, verifier string) string
	// Identify exchanges the code the provider sent back and returns the
	// account that signed in. The email address is one the provider has
	// verified.
	Identify(ctx context.Context, code, nonce, verifier string) (Identity, error)
}

// Identity is an account at a provider.
type Identity struct {
	// Subject is the provider's stable identifier for the account.
	Subject string
	Email   string
}

const (
	GoogleName = "google"
	GitHubName = "github"
	// GoogleIssuer is Google's OpenID Connect issuer.
	GoogleIssuer = "https://accounts.google.com"
)

// NewGoogle signs in with Google through OpenID Connect, discovering its
// endpoints from issuer. redirectURL is this server's callback for it.
func NewGoogle(ctx context.Context, issuer, clientID, clientSecret, redirectURL string) (Provider, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover %s: %w", issuer, err)
	}
	return &google{
		config: oauth2.Config{
			ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
			Endpoint: provider.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "email"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
	}, nil
}

type google struct {
	config   oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func (g *google) Name() string { return GoogleName }

// AuthCodeURL asks Google to show its account picker, so a person always
// sees which account they are approving a device with.
func (g *google) AuthCodeURL(state, nonce, verifier string) string {
	return g.config.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "select_account"))
}

func (g *google) Identify(ctx context.Context, code, nonce, verifier string) (Identity, error) {
	token, err := g.config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("exchange the Google code: %w", err)
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return Identity{}, errors.New("google returned no ID token")
	}
	idToken, err := g.verifier.Verify(ctx, raw)
	if err != nil {
		return Identity{}, fmt.Errorf("verify the Google ID token: %w", err)
	}
	if idToken.Nonce != nonce {
		return Identity{}, errors.New("the Google ID token is for another sign-in")
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, err
	}
	if claims.Email == "" || !claims.EmailVerified {
		return Identity{}, errors.New("google has not verified this account's email address")
	}
	return Identity{Subject: idToken.Subject, Email: claims.Email}, nil
}

// GitHubEndpoints are where GitHub signs people in and answers API calls.
type GitHubEndpoints struct {
	AuthURL, TokenURL, APIURL string
}

// GitHubDotCom is github.com.
var GitHubDotCom = GitHubEndpoints{
	AuthURL:  "https://github.com/login/oauth/authorize",
	TokenURL: "https://github.com/login/oauth/access_token",
	APIURL:   "https://api.github.com",
}

// NewGitHub signs in with a GitHub OAuth app. GitHub does not offer OpenID
// Connect to OAuth apps, so the account is read from its API: the user's id,
// and their primary email address if GitHub has verified it.
func NewGitHub(endpoints GitHubEndpoints, clientID, clientSecret, redirectURL string) Provider {
	return &github{
		config: oauth2.Config{
			ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
			Endpoint: oauth2.Endpoint{AuthURL: endpoints.AuthURL, TokenURL: endpoints.TokenURL},
			Scopes:   []string{"read:user", "user:email"},
		},
		apiURL: endpoints.APIURL,
	}
}

type github struct {
	config oauth2.Config
	apiURL string
}

func (g *github) Name() string { return GitHubName }

func (g *github) AuthCodeURL(state, _, verifier string) string {
	return g.config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "select_account"))
}

func (g *github) Identify(ctx context.Context, code, _, verifier string) (Identity, error) {
	token, err := g.config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("exchange the GitHub code: %w", err)
	}
	client := g.config.Client(ctx, token)
	var user struct {
		ID int64 `json:"id"`
	}
	if err := g.get(ctx, client, "/user", &user); err != nil {
		return Identity{}, err
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := g.get(ctx, client, "/user/emails", &emails); err != nil {
		return Identity{}, err
	}
	for _, email := range emails {
		if email.Primary && email.Verified && user.ID != 0 {
			return Identity{Subject: strconv.FormatInt(user.ID, 10), Email: email.Email}, nil
		}
	}
	return Identity{}, errors.New("github has no verified primary email address for this account")
}

func (g *github) get(ctx context.Context, client *http.Client, path string, into any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, g.apiURL+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("github %s: %w", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("github %s: %s", path, response.Status)
	}
	return json.NewDecoder(response.Body).Decode(into)
}
