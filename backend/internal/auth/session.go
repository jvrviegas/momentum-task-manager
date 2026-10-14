package auth

import (
	"context"
	"strings"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	entsession "github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/session"
)

// Authenticate returns the session token belongs to, or ErrUnauthorized when
// it is unknown, expired, or revoked.
func (s *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	if !strings.HasPrefix(token, tokenPrefix) {
		return Session{}, ErrUnauthorized
	}
	found, err := s.client.Session.Query().
		Where(entsession.TokenHash(hashSecret(token)), entsession.RevokedAtIsNil(), entsession.ExpiresAtGT(s.now())).
		WithUser().
		Only(ctx)
	if ent.IsNotFound(err) {
		return Session{}, ErrUnauthorized
	} else if err != nil {
		return Session{}, err
	}
	return Session{ID: found.ID, UserID: found.UserID, Email: found.Edges.User.Email, ExpiresAt: found.ExpiresAt}, nil
}

// Revoke ends a session, so its token stops working.
func (s *Service) Revoke(ctx context.Context, id int) error {
	_, err := s.client.Session.Update().
		Where(entsession.ID(id), entsession.RevokedAtIsNil()).
		SetRevokedAt(s.now()).
		Save(ctx)
	return err
}
