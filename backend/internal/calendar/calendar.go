// Package calendar reads a person's Google Calendar for Plan Today (ADR
// 0003). The server keeps one connection per account, with the refresh token
// sealed under CALENDAR_TOKEN_KEY, and answers with the day's events. It
// never stores or logs an event or a title.
package calendar

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/seal"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/calendarconnection"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// The browser half of connecting lives at these paths, which Google's
// redirect URI must include.
const (
	ConnectPath  = "/calendar/connect/"
	CallbackPath = "/calendar/callback/google"

	// linkLifetime is how long a person has to finish connecting.
	linkLifetime = 10 * time.Minute
)

// The reasons CodeCalendarUnavailable gives, which the apps show after
// "Calendar unavailable:".
const (
	reasonNotOffered   = "Google Calendar isn’t available on this server"
	reasonNotConnected = "Google Calendar isn’t connected"
	reasonReconnect    = "Google Calendar needs reconnecting"
	reasonUnreachable  = "Google Calendar can’t be reached right now"
	reasonRefused      = "Google refused to share the calendar"
)

// Config configures a Service.
type Config struct {
	// PublicURL is the server's address as browsers reach it.
	PublicURL string
	// Key seals the refresh token (D5): 32 bytes. Without it, or without
	// Google, the server offers no Google Calendar.
	Key    []byte
	Google *Google
	Logger *slog.Logger
}

// Service connects Google Calendar and reads the day's events.
type Service struct {
	client    *ent.Client
	publicURL string
	key       []byte
	google    *Google
	logger    *slog.Logger
	now       func() time.Time
}

// New returns a Service that keeps its state through client.
func New(client *ent.Client, config Config) *Service {
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Service{
		client: client, publicURL: strings.TrimSuffix(config.PublicURL, "/"),
		key: config.Key, google: config.Google, logger: logger, now: time.Now,
	}
}

// Offered reports whether this server can connect Google Calendar at all.
func (s *Service) Offered() bool { return s.google != nil && len(s.key) == seal.KeySize }

func unavailable(reason string) error {
	return &api.Error{Code: api.CodeCalendarUnavailable, Message: reason}
}

// Status answers RouteGoogleCalendarStatus for the signed-in user in ctx.
func (s *Service) Status(ctx context.Context) (api.GoogleCalendarStatus, error) {
	if !s.Offered() {
		return api.GoogleCalendarStatus{}, nil
	}
	row, err := s.client.CalendarConnection.Query().Only(ctx)
	switch {
	case ent.IsNotFound(err):
		return api.GoogleCalendarStatus{Offered: true}, nil
	case err != nil:
		return api.GoogleCalendarStatus{}, err
	}
	return api.GoogleCalendarStatus{
		Offered: true, Connected: true, Email: row.Email, NeedsReconnect: row.Status == calendarconnection.StatusNeedsReconnect,
	}, nil
}

// Disconnect asks Google to revoke the signed-in user's token and deletes the
// connection. It deletes it even when the token cannot be revoked, because
// Google cannot be reached or the key no longer opens it, and says so.
func (s *Service) Disconnect(ctx context.Context) (api.GoogleCalendarDisconnect, error) {
	row, err := s.client.CalendarConnection.Query().Only(ctx)
	switch {
	case ent.IsNotFound(err):
		return api.GoogleCalendarDisconnect{Revoked: true}, nil
	case err != nil:
		return api.GoogleCalendarDisconnect{}, err
	}
	revoked := false
	if token, err := seal.Open(s.key, row.SealedToken); err == nil && s.google != nil {
		if err := s.google.Revoke(ctx, string(token)); err != nil {
			s.logger.WarnContext(ctx, "revoking the Google token failed", slog.String("error_type", errorType(err)))
		} else {
			revoked = true
		}
	}
	if err := s.client.CalendarConnection.DeleteOne(row).Exec(ctx); err != nil {
		return api.GoogleCalendarDisconnect{}, err
	}
	return api.GoogleCalendarDisconnect{Revoked: revoked}, nil
}

// Events answers RouteGoogleCalendarEvents for the signed-in user in ctx.
// Every way the calendar cannot be read is a CodeCalendarUnavailable with a
// reason, so planning goes on with tasks alone (Plan Today D6).
func (s *Service) Events(ctx context.Context, start, end time.Time) ([]api.GoogleCalendarEvent, error) {
	if !s.Offered() {
		return nil, unavailable(reasonNotOffered)
	}
	row, err := s.client.CalendarConnection.Query().Only(ctx)
	switch {
	case ent.IsNotFound(err):
		return nil, unavailable(reasonNotConnected)
	case err != nil:
		return nil, err
	case row.Status == calendarconnection.StatusNeedsReconnect:
		return nil, unavailable(reasonReconnect)
	}
	// A key that no longer opens the token was rotated or lost, which means
	// reconnecting (D5).
	token, err := seal.Open(s.key, row.SealedToken)
	if err != nil {
		return nil, s.needsReconnecting(ctx, row)
	}
	access, err := s.google.AccessToken(ctx, string(token))
	if errors.Is(err, ErrRefused) {
		return nil, s.needsReconnecting(ctx, row)
	} else if err != nil {
		return nil, s.cannotRead(ctx, err)
	}
	events, err := s.google.Events(ctx, access, start, end)
	var failure *StatusError
	switch {
	case errors.As(err, &failure) && failure.Status == 401:
		return nil, s.needsReconnecting(ctx, row)
	case err != nil:
		return nil, s.cannotRead(ctx, err)
	}
	return events, nil
}

// needsReconnecting keeps the connection as "needs reconnecting" until the
// person reconnects or disconnects, and returns the answer for it.
func (s *Service) needsReconnecting(ctx context.Context, row *ent.CalendarConnection) error {
	if err := s.client.CalendarConnection.UpdateOne(row).SetStatus(calendarconnection.StatusNeedsReconnect).Exec(ctx); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "google calendar needs reconnecting")
	return unavailable(reasonReconnect)
}

// cannotRead answers for a Google failure that leaves the connection as it is:
// Google refused the read, or could not be reached. The log names the type of
// the error, never its text.
func (s *Service) cannotRead(ctx context.Context, err error) error {
	s.logger.WarnContext(ctx, "reading the Google calendar failed", slog.String("error_type", errorType(err)))
	var failure *StatusError
	if errors.As(err, &failure) && failure.Status >= 400 && failure.Status < 500 && failure.Status != 429 {
		return unavailable(reasonRefused)
	}
	return unavailable(reasonUnreachable)
}

// errorType names the type of the innermost error err wraps.
func errorType(err error) string {
	for {
		inner := errors.Unwrap(err)
		if inner == nil {
			return fmt.Sprintf("%T", err)
		}
		err = inner
	}
}

// newSecret returns 32 random bytes as URL-safe text and the hash to store in
// its place.
func newSecret() (string, []byte) {
	data := make([]byte, 32)
	_, _ = rand.Read(data)
	secret := base64.RawURLEncoding.EncodeToString(data)
	return secret, hashSecret(secret)
}

// hashSecret hashes a high-entropy secret for storage and lookup; a plain
// SHA-256 is enough because the secrets are random.
func hashSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}
