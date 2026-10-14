// Package auth signs people in with the device authorization flow (RFC 8628)
// and resolves their tokens to sessions (backend plan D1, D10). The browser
// half signs in with a Provider; the server issues one opaque token per
// device and stores only its hash.
package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
)

const (
	// deviceCodeLifetime is how long a person has to finish signing in.
	deviceCodeLifetime = 10 * time.Minute
	// pollInterval is how often a device may poll for its token.
	pollInterval = 5 * time.Second
	// sessionLifetime is how long a token works before the device must sign
	// in again.
	sessionLifetime = 90 * 24 * time.Hour
)

var (
	// ErrUnauthorized: the token is missing, unknown, expired, or revoked.
	ErrUnauthorized = errors.New("sign in again")
	// ErrInvalidCode: no pending sign-in has that user code.
	ErrInvalidCode = errors.New("that code is not valid or has expired; check the code on your device")
	// ErrSignInExpired: the browser came back from a provider for a sign-in
	// that has expired, was already completed, or was never started here.
	ErrSignInExpired = errors.New("this sign-in has expired; start again on your device")
)

// Config configures a Service.
type Config struct {
	// PublicURL is the server's address as browsers reach it, without a
	// trailing slash.
	PublicURL string
	// AllowedEmails may sign in (D10); nobody else can.
	AllowedEmails []string
	Providers     []Provider
}

// Service runs device sign-in and resolves tokens.
type Service struct {
	client    *ent.Client
	publicURL string
	allowed   map[string]bool
	providers map[string]Provider
	names     []string
	now       func() time.Time
}

// New returns a Service that keeps its state through client.
func New(client *ent.Client, config Config) *Service {
	service := &Service{
		client:    client,
		publicURL: strings.TrimSuffix(config.PublicURL, "/"),
		allowed:   map[string]bool{},
		providers: map[string]Provider{},
		now:       time.Now,
	}
	for _, email := range config.AllowedEmails {
		service.allowed[normalizeEmail(email)] = true
	}
	for _, provider := range config.Providers {
		service.providers[provider.Name()] = provider
		service.names = append(service.names, provider.Name())
	}
	return service
}

// PublicURL is the server's address as browsers reach it.
func (s *Service) PublicURL() string { return s.publicURL }

// Providers returns the names of the configured providers, in their order.
func (s *Service) Providers() []string { return s.names }

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// Session is a signed-in device.
type Session struct {
	ID        int
	UserID    uuid.UUID
	Email     string
	ExpiresAt time.Time
}

type sessionKey struct{}

// NewContext returns ctx carrying session.
func NewContext(ctx context.Context, session Session) context.Context {
	return context.WithValue(ctx, sessionKey{}, session)
}

// FromContext returns the session ctx carries, if any.
func FromContext(ctx context.Context) (Session, bool) {
	session, ok := ctx.Value(sessionKey{}).(Session)
	return session, ok
}
