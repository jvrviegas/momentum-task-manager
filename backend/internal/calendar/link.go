package calendar

import (
	"context"
	"errors"

	"golang.org/x/oauth2"

	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/seal"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/calendarconnection"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/calendarlink"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/privacy"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/viewer"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// ErrLinkExpired: the link, or the state the browser came back with, has
// expired, was already used, or was never made here.
var ErrLinkExpired = errors.New("this link has expired or was already used; choose Connect… in Momentum again")

// Connecting goes in four steps. The app calls Start while signed in. The
// browser, which has no signed-in user, opens the link (Lookup), asks to
// continue (Begin) and comes back from Google (Complete). Each of those looks
// its row up by a hash, which no user's privacy rule can allow, so they step
// past the rules for that lookup alone and then act as the link's user.

// Start makes the one-time link for the signed-in user in ctx.
func (s *Service) Start(ctx context.Context) (api.GoogleCalendarLink, error) {
	if !s.Offered() {
		return api.GoogleCalendarLink{}, unavailable(reasonNotOffered)
	}
	userID, ok := viewer.UserID(ctx)
	if !ok {
		return api.GoogleCalendarLink{}, errors.New("no signed-in user")
	}
	now := s.now()
	// Expired links are never needed again.
	if _, err := s.client.CalendarLink.Delete().Where(calendarlink.ExpiresAtLT(now)).Exec(privacy.DecisionContext(ctx, privacy.Allow)); err != nil {
		return api.GoogleCalendarLink{}, err
	}
	token, hash := newSecret()
	expiresAt := now.Add(linkLifetime)
	if err := s.client.CalendarLink.Create().SetUserID(userID).SetLinkHash(hash).SetExpiresAt(expiresAt).Exec(ctx); err != nil {
		return api.GoogleCalendarLink{}, err
	}
	return api.GoogleCalendarLink{URL: s.publicURL + ConnectPath + token, ExpiresAt: expiresAt}, nil
}

// Lookup returns the email address of the Momentum account the link belongs
// to, which the page names so a person never connects their calendar to
// someone else's account without seeing whose it is.
func (s *Service) Lookup(ctx context.Context, token string) (string, error) {
	if !s.Offered() {
		return "", ErrLinkExpired
	}
	link, err := s.client.CalendarLink.Query().
		Where(calendarlink.LinkHash(hashSecret(token)), calendarlink.ExpiresAtGT(s.now())).
		WithUser().Only(privacy.DecisionContext(ctx, privacy.Allow))
	if ent.IsNotFound(err) {
		return "", ErrLinkExpired
	} else if err != nil {
		return "", err
	}
	return link.Edges.User.Email, nil
}

// Begin starts the Google round trip for the link. It returns where to send
// the browser, and the state Google will send back, which the browser must
// keep. Asking again replaces the state, so only the latest works.
func (s *Service) Begin(ctx context.Context, token string) (redirect, state string, err error) {
	if !s.Offered() {
		return "", "", ErrLinkExpired
	}
	state, stateHash := newSecret()
	nonce, _ := newSecret()
	verifier := oauth2.GenerateVerifier()
	changed, err := s.client.CalendarLink.Update().
		Where(calendarlink.LinkHash(hashSecret(token)), calendarlink.ExpiresAtGT(s.now())).
		SetStateHash(stateHash).SetCodeVerifier(verifier).SetNonce(nonce).
		Save(privacy.DecisionContext(ctx, privacy.Allow))
	if err != nil {
		return "", "", err
	}
	if changed == 0 {
		return "", "", ErrLinkExpired
	}
	return s.google.AuthCodeURL(state, nonce, verifier), state, nil
}

// Complete finishes connecting when Google sends the browser back with code
// and state, and returns the Google account's email address. The state works
// once: the link is deleted before the code is exchanged, so a second try, even
// a concurrent one, gets ErrLinkExpired.
func (s *Service) Complete(ctx context.Context, code, state string) (string, error) {
	if !s.Offered() {
		return "", ErrLinkExpired
	}
	allow := privacy.DecisionContext(ctx, privacy.Allow)
	stateHash := hashSecret(state)
	link, err := s.client.CalendarLink.Query().
		Where(calendarlink.StateHash(stateHash), calendarlink.ExpiresAtGT(s.now())).Only(allow)
	if ent.IsNotFound(err) {
		return "", ErrLinkExpired
	} else if err != nil {
		return "", err
	}
	deleted, err := s.client.CalendarLink.Delete().Where(calendarlink.ID(link.ID), calendarlink.StateHash(stateHash)).Exec(allow)
	if err != nil {
		return "", err
	}
	if deleted == 0 {
		return "", ErrLinkExpired
	}
	grant, err := s.google.Exchange(ctx, code, link.Nonce, link.CodeVerifier)
	if err != nil {
		return "", err
	}
	sealed, err := seal.Seal(s.key, []byte(grant.RefreshToken))
	if err != nil {
		return "", err
	}
	if err := s.save(viewer.NewContext(ctx, link.UserID), link.UserID, grant, sealed); err != nil {
		return "", err
	}
	return grant.Email, nil
}

// save keeps the connection for userID, replacing the one it already has: the
// person reconnecting, perhaps with another Google account. ctx acts for
// userID.
func (s *Service) save(ctx context.Context, userID uuid.UUID, grant Grant, sealed []byte) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	existing, err := tx.CalendarConnection.Query().Only(ctx)
	switch {
	case ent.IsNotFound(err):
		err = tx.CalendarConnection.Create().SetUserID(userID).SetSubject(grant.Subject).SetEmail(grant.Email).SetSealedToken(sealed).Exec(ctx)
	case err == nil:
		err = tx.CalendarConnection.UpdateOne(existing).SetSubject(grant.Subject).SetEmail(grant.Email).SetSealedToken(sealed).
			SetStatus(calendarconnection.StatusConnected).Exec(ctx)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
