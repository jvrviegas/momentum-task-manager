package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

const (
	// Scope is all Momentum asks Google for besides who the account is (ADR
	// 0003, D2): to read events, never to change them.
	Scope = "https://www.googleapis.com/auth/calendar.events.readonly"
	// GoogleAPI is the Google Calendar API's address.
	GoogleAPI = "https://www.googleapis.com/calendar/v3"

	maxPages = 10
	maxBody  = 8 << 20
)

var (
	// ErrNotGranted: Google did not give Momentum lasting access to the
	// calendar, because the person left the permission unticked or Google sent
	// no refresh token.
	ErrNotGranted = errors.New("access to the calendar was not given; choose Connect… in Momentum again and allow it")
	// ErrRefused: Google no longer accepts the saved connection, because the
	// person revoked it or it expired.
	ErrRefused = errors.New("Google refuses the saved connection")
)

// StatusError is an answer from Google's Calendar API that was not a success.
type StatusError struct{ Status int }

func (e *StatusError) Error() string { return fmt.Sprintf("Google answered %d", e.Status) }

// Google is Momentum's Google OAuth client and the Calendar API it reads.
type Google struct {
	config    oauth2.Config
	verifier  *oidc.IDTokenVerifier
	revokeURL string
	apiURL    string
}

// Grant is what Google gave when the person approved the connection.
type Grant struct {
	RefreshToken string
	// Subject is Google's stable identifier for the account.
	Subject string
	Email   string
}

// NewGoogle discovers Google's endpoints from issuer and returns the client
// for the OAuth client clientID. redirectURL is this server's callback, and
// apiURL is the Calendar API's address, GoogleAPI outside tests.
func NewGoogle(ctx context.Context, issuer, clientID, clientSecret, redirectURL, apiURL string) (*Google, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover %s: %w", issuer, err)
	}
	var discovered struct {
		RevocationEndpoint string `json:"revocation_endpoint"`
	}
	if err := provider.Claims(&discovered); err != nil || discovered.RevocationEndpoint == "" {
		return nil, fmt.Errorf("%s names no revocation endpoint", issuer)
	}
	return &Google{
		config: oauth2.Config{
			ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
			Endpoint: provider.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "email", Scope},
		},
		verifier:  provider.Verifier(&oidc.Config{ClientID: clientID}),
		revokeURL: discovered.RevocationEndpoint,
		apiURL:    strings.TrimSuffix(apiURL, "/"),
	}, nil
}

// AuthCodeURL is where to send the browser. Google sends it back with state;
// nonce and the PKCE verifier bind the result to this connect. It asks for
// offline access and the consent screen, so Google gives a refresh token.
func (g *Google) AuthCodeURL(state, nonce, verifier string) string {
	return g.config.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier),
		oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
}

// Exchange trades the code Google sent back for the Grant, or answers
// ErrNotGranted.
func (g *Google) Exchange(ctx context.Context, code, nonce, verifier string) (Grant, error) {
	token, err := g.config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Grant{}, fmt.Errorf("exchange the Google code: %w", err)
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return Grant{}, errors.New("google returned no ID token")
	}
	idToken, err := g.verifier.Verify(ctx, raw)
	if err != nil {
		return Grant{}, fmt.Errorf("verify the Google ID token: %w", err)
	}
	if idToken.Nonce != nonce {
		return Grant{}, errors.New("the Google ID token is for another connect")
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Grant{}, err
	}
	if claims.Email == "" || !claims.EmailVerified {
		return Grant{}, errors.New("google has not verified this account's email address")
	}
	scope, _ := token.Extra("scope").(string)
	if token.RefreshToken == "" || !strings.Contains(" "+scope+" ", " "+Scope+" ") {
		return Grant{}, ErrNotGranted
	}
	return Grant{RefreshToken: token.RefreshToken, Subject: idToken.Subject, Email: claims.Email}, nil
}

// AccessToken trades a refresh token for an access token, or answers
// ErrRefused when Google no longer accepts the connection.
func (g *Google) AccessToken(ctx context.Context, refreshToken string) (string, error) {
	token, err := g.config.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken}).Token()
	var failure *oauth2.RetrieveError
	switch {
	case errors.As(err, &failure) && failure.ErrorCode == "invalid_grant":
		return "", ErrRefused
	case err != nil:
		return "", fmt.Errorf("refresh the Google token: %w", err)
	}
	return token.AccessToken, nil
}

// Revoke asks Google to revoke a refresh token. A token Google already
// reports as invalid counts as revoked.
func (g *Google) Revoke(ctx context.Context, refreshToken string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, g.revokeURL, strings.NewReader(url.Values{"token": {refreshToken}}.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("revoke the Google token: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxBody))
	if response.StatusCode == http.StatusOK || (response.StatusCode == http.StatusBadRequest && strings.Contains(string(body), "invalid_token")) {
		return nil
	}
	return &StatusError{Status: response.StatusCode}
}

// Events reads the primary calendar's events that overlap start to end, as
// the single occurrences of repeating ones, without cancelled events. It asks
// Google for the fields planning needs and nothing else, so descriptions and
// other people's addresses never reach this server.
func (g *Google) Events(ctx context.Context, accessToken string, start, end time.Time) ([]api.GoogleCalendarEvent, error) {
	events := []api.GoogleCalendarEvent{}
	pageToken := ""
	for range maxPages {
		query := url.Values{
			"singleEvents": {"true"}, "showDeleted": {"false"}, "orderBy": {"startTime"}, "maxResults": {"250"},
			"timeMin": {start.Format(time.RFC3339)}, "timeMax": {end.Format(time.RFC3339)},
			"fields": {"nextPageToken,items(status,summary,transparency,start,end,attendees(self,responseStatus))"},
		}
		if pageToken != "" {
			query.Set("pageToken", pageToken)
		}
		page, err := g.eventsPage(ctx, accessToken, query)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			if event, ok := item.event(start.Location()); ok {
				events = append(events, event)
			}
		}
		if pageToken = page.NextPageToken; pageToken == "" {
			break
		}
	}
	return events, nil
}

func (g *Google) eventsPage(ctx context.Context, accessToken string, query url.Values) (eventsPage, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, g.apiURL+"/calendars/primary/events?"+query.Encode(), nil)
	if err != nil {
		return eventsPage{}, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return eventsPage{}, fmt.Errorf("read the Google calendar: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return eventsPage{}, &StatusError{Status: response.StatusCode}
	}
	var page eventsPage
	if err := json.NewDecoder(io.LimitReader(response.Body, maxBody)).Decode(&page); err != nil {
		return eventsPage{}, fmt.Errorf("decode the Google calendar: %w", err)
	}
	return page, nil
}
