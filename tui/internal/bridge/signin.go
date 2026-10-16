package bridge

import (
	"context"
	"errors"
	"fmt"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// The macOS app signs in with the device flow (backend plan D1) through these
// operations and keeps the token in the Keychain. It sends the token with
// setToken whenever the bridge starts, so the bridge never stores it.

// backendURL returns the [backend] server, which sign-in needs.
func (s *Server) backendURL() (string, error) {
	if !s.config.Backend.Enabled() || s.connect == nil {
		return "", fmt.Errorf("sign-in needs a [backend] url in %s", s.configPath)
	}
	return s.config.Backend.URL, nil
}

// setToken signs every later request in with token; an empty token signs
// them out.
func (s *Server) setToken(token string) error {
	if _, err := s.backendURL(); err != nil {
		return err
	}
	s.client = s.connect(token)
	return nil
}

func (s *Server) signInStart(ctx context.Context) (*signInStartResult, error) {
	url, err := s.backendURL()
	if err != nil {
		return nil, err
	}
	start, err := api.NewClient(url, "").StartDeviceSignIn(ctx)
	if err != nil {
		return nil, err
	}
	return &signInStartResult{
		DeviceCode:              start.DeviceCode,
		UserCode:                start.UserCode,
		VerificationURL:         start.VerificationURL,
		VerificationURLComplete: start.VerificationURLComplete,
		ExpiresAt:               stamp(&start.ExpiresAt),
		IntervalSeconds:         start.IntervalSeconds,
	}, nil
}

// signInPoll asks the server once whether the sign-in of deviceCode has
// finished. A refused sign-in or an expired code is an error.
func (s *Server) signInPoll(ctx context.Context, deviceCode string) (*signInPollResult, error) {
	url, err := s.backendURL()
	if err != nil {
		return nil, err
	}
	token, err := api.NewClient(url, "").PollDeviceSignIn(ctx, deviceCode)
	var failure *api.Error
	switch {
	case err == nil:
		return &signInPollResult{State: "signedIn", Token: token.Token, Email: token.Email, ExpiresAt: stamp(&token.ExpiresAt)}, nil
	case !errors.As(err, &failure):
		return nil, err
	case failure.Code == api.CodeAuthorizationPending:
		return &signInPollResult{State: "pending"}, nil
	case failure.Code == api.CodeSlowDown:
		return &signInPollResult{State: "slowDown"}, nil
	case failure.Code == api.CodeAccessDenied:
		return nil, fmt.Errorf("sign-in refused: %s", failure.Message)
	case failure.Code == api.CodeExpiredToken:
		return nil, errors.New("the code expired before sign-in finished; sign in again")
	default:
		return nil, err
	}
}

// revoke ends the sign-in of token on the server. One the server no longer
// accepts has already ended.
func (s *Server) revoke(ctx context.Context, token string) error {
	url, err := s.backendURL()
	if err != nil {
		return err
	}
	err = api.NewClient(url, token).SignOut(ctx)
	var failure *api.Error
	if errors.As(err, &failure) && failure.Code == api.CodeUnauthorized {
		return nil
	}
	return err
}
