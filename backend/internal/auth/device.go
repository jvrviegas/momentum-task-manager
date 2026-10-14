package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/deviceauthorization"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/identity"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/user"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

var (
	errExpiredToken = &api.Error{Code: api.CodeExpiredToken, Message: "the sign-in code expired; start signing in again"}
	errAccessDenied = &api.Error{Code: api.CodeAccessDenied, Message: "this account may not use this server"}
	errPending      = &api.Error{Code: api.CodeAuthorizationPending, Message: "finish signing in in the browser"}
	errSlowDown     = &api.Error{Code: api.CodeSlowDown, Message: "polling too often; wait longer between requests"}
)

// Start begins a device sign-in.
func (s *Service) Start(ctx context.Context) (api.DeviceSignIn, error) {
	now := s.now()
	// Expired sign-ins are never needed again.
	if _, err := s.client.DeviceAuthorization.Delete().Where(deviceauthorization.ExpiresAtLT(now)).Exec(ctx); err != nil {
		return api.DeviceSignIn{}, err
	}
	deviceCode, hash := newSecret("")
	userCode := newUserCode()
	expiresAt := now.Add(deviceCodeLifetime)
	err := s.client.DeviceAuthorization.Create().
		SetDeviceCodeHash(hash).SetUserCode(userCode).SetExpiresAt(expiresAt).
		Exec(ctx)
	if err != nil {
		return api.DeviceSignIn{}, err
	}
	verification := s.publicURL + "/device"
	return api.DeviceSignIn{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURL:         verification,
		VerificationURLComplete: verification + "?code=" + userCode,
		ExpiresAt:               expiresAt,
		IntervalSeconds:         int(pollInterval / time.Second),
	}, nil
}

// Poll answers a device polling for its token. Until the person has signed
// in, it returns an *api.Error with one of RFC 8628's codes; then it returns
// the token, once.
func (s *Service) Poll(ctx context.Context, deviceCode string) (api.Token, error) {
	now := s.now()
	row, err := s.client.DeviceAuthorization.Query().
		Where(deviceauthorization.DeviceCodeHash(hashSecret(deviceCode))).
		Only(ctx)
	switch {
	case ent.IsNotFound(err):
		return api.Token{}, errExpiredToken
	case err != nil:
		return api.Token{}, err
	case !now.Before(row.ExpiresAt):
		return api.Token{}, errors.Join(errExpiredToken, s.client.DeviceAuthorization.DeleteOne(row).Exec(ctx))
	}
	switch row.Status {
	case deviceauthorization.StatusDenied:
		return api.Token{}, errors.Join(errAccessDenied, s.client.DeviceAuthorization.DeleteOne(row).Exec(ctx))
	case deviceauthorization.StatusPending:
		tooSoon := row.LastPolledAt != nil && now.Sub(*row.LastPolledAt) < pollInterval
		if err := s.client.DeviceAuthorization.UpdateOne(row).SetLastPolledAt(now).Exec(ctx); err != nil {
			return api.Token{}, err
		}
		if tooSoon {
			return api.Token{}, errSlowDown
		}
		return api.Token{}, errPending
	}
	return s.issue(ctx, row)
}

// issue turns an approved sign-in into a session and its token. Deleting the
// sign-in first means a second poll, even a concurrent one, gets nothing.
func (s *Service) issue(ctx context.Context, row *ent.DeviceAuthorization) (api.Token, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return api.Token{}, err
	}
	defer tx.Rollback()
	if err := tx.DeviceAuthorization.DeleteOneID(row.ID).Exec(ctx); ent.IsNotFound(err) {
		return api.Token{}, errExpiredToken
	} else if err != nil {
		return api.Token{}, err
	}
	token, hash := newSecret(tokenPrefix)
	expiresAt := s.now().Add(sessionLifetime)
	if err := tx.Session.Create().SetUserID(*row.UserID).SetTokenHash(hash).SetExpiresAt(expiresAt).Exec(ctx); err != nil {
		return api.Token{}, err
	}
	account, err := tx.User.Get(ctx, *row.UserID)
	if err != nil {
		return api.Token{}, err
	}
	if err := tx.Commit(); err != nil {
		return api.Token{}, err
	}
	return api.Token{Token: token, ExpiresAt: expiresAt, Email: account.Email}, nil
}

// BeginSignIn starts the browser half of the pending sign-in with userCode,
// at the provider named providerName. It returns where to send the browser,
// and the state the provider will send back, which the browser must keep.
func (s *Service) BeginSignIn(ctx context.Context, userCode, providerName string) (string, string, error) {
	provider, ok := s.providers[providerName]
	code := normalizeUserCode(userCode)
	if !ok || code == "" {
		return "", "", ErrInvalidCode
	}
	state, stateHash := newSecret("")
	nonce, _ := newSecret("")
	verifier := oauth2.GenerateVerifier()
	changed, err := s.client.DeviceAuthorization.Update().
		Where(deviceauthorization.UserCode(code), deviceauthorization.StatusEQ(deviceauthorization.StatusPending),
			deviceauthorization.ExpiresAtGT(s.now())).
		SetProvider(providerName).SetStateHash(stateHash).SetCodeVerifier(verifier).SetNonce(nonce).
		Save(ctx)
	if err != nil {
		return "", "", err
	}
	if changed == 0 {
		return "", "", ErrInvalidCode
	}
	return provider.AuthCodeURL(state, nonce, verifier), state, nil
}

// SignIn is how the browser half ended.
type SignIn struct {
	Email string
	// Approved is false when Email may not use this server.
	Approved bool
}

// CompleteSignIn finishes the browser half when the provider sends it back
// with code and state, and approves the device if the account may sign in.
func (s *Service) CompleteSignIn(ctx context.Context, providerName, code, state string) (SignIn, error) {
	provider, ok := s.providers[providerName]
	if !ok {
		return SignIn{}, ErrSignInExpired
	}
	stateHash := hashSecret(state)
	row, err := s.client.DeviceAuthorization.Query().
		Where(deviceauthorization.StateHash(stateHash), deviceauthorization.Provider(providerName),
			deviceauthorization.StatusEQ(deviceauthorization.StatusPending), deviceauthorization.ExpiresAtGT(s.now())).
		Only(ctx)
	if ent.IsNotFound(err) {
		return SignIn{}, ErrSignInExpired
	} else if err != nil {
		return SignIn{}, err
	}
	// The state works once.
	cleared, err := s.client.DeviceAuthorization.Update().
		Where(deviceauthorization.ID(row.ID), deviceauthorization.StateHash(stateHash)).
		ClearStateHash().ClearCodeVerifier().ClearNonce().
		Save(ctx)
	if err != nil {
		return SignIn{}, err
	}
	if cleared == 0 {
		return SignIn{}, ErrSignInExpired
	}
	found, err := provider.Identify(ctx, code, row.Nonce, row.CodeVerifier)
	if err != nil {
		return SignIn{}, err
	}
	email := normalizeEmail(found.Email)
	if !s.allowed[email] {
		return SignIn{Email: email}, s.client.DeviceAuthorization.UpdateOneID(row.ID).SetStatus(deviceauthorization.StatusDenied).Exec(ctx)
	}
	userID, err := s.account(ctx, providerName, found.Subject, email)
	if err != nil {
		return SignIn{}, err
	}
	err = s.client.DeviceAuthorization.UpdateOneID(row.ID).SetStatus(deviceauthorization.StatusApproved).SetUserID(userID).Exec(ctx)
	return SignIn{Email: email, Approved: err == nil}, err
}

// account returns the user an identity belongs to. A new identity joins the
// user with the same email address, or a new user.
func (s *Service) account(ctx context.Context, provider, subject, email string) (uuid.UUID, error) {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback()
	linked, err := tx.Identity.Query().Where(identity.Provider(provider), identity.Subject(subject)).Only(ctx)
	if err == nil {
		return linked.UserID, tx.Commit()
	} else if !ent.IsNotFound(err) {
		return uuid.Nil, err
	}
	account, err := tx.User.Query().Where(user.Email(email)).Only(ctx)
	if ent.IsNotFound(err) {
		account, err = tx.User.Create().SetEmail(email).Save(ctx)
	}
	if err != nil {
		return uuid.Nil, err
	}
	if err := tx.Identity.Create().SetUserID(account.ID).SetProvider(provider).SetSubject(subject).Exec(ctx); err != nil {
		return uuid.Nil, err
	}
	return account.ID, tx.Commit()
}
