package backend

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// slowDown is how much longer to wait after api.CodeSlowDown (RFC 8628).
const slowDown = 5 * time.Second

// SignIn signs this device in to the server at url with the device flow
// (D1). It calls show with the code the person enters in the browser, then
// polls until they finish, refuse, or the code expires, or ctx ends.
func SignIn(ctx context.Context, url string, show func(api.DeviceSignIn)) (Credentials, error) {
	return signIn(ctx, api.NewClient(url, ""), url, show, wait)
}

func signIn(ctx context.Context, server *api.Client, url string, show func(api.DeviceSignIn), wait func(context.Context, time.Duration) error) (Credentials, error) {
	start, err := server.StartDeviceSignIn(ctx)
	if err != nil {
		return Credentials{}, err
	}
	show(start)
	interval := time.Duration(start.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = slowDown
	}
	for {
		if err := wait(ctx, interval); err != nil {
			return Credentials{}, err
		}
		token, err := server.PollDeviceSignIn(ctx, start.DeviceCode)
		var failure *api.Error
		switch {
		case err == nil:
			return Credentials{URL: url, Token: token.Token, Email: token.Email, ExpiresAt: token.ExpiresAt}, nil
		case !errors.As(err, &failure):
			return Credentials{}, err
		case failure.Code == api.CodeAuthorizationPending:
		case failure.Code == api.CodeSlowDown:
			interval += slowDown
		case failure.Code == api.CodeAccessDenied:
			return Credentials{}, fmt.Errorf("sign-in refused: %s", failure.Message)
		case failure.Code == api.CodeExpiredToken:
			return Credentials{}, errors.New("the code expired before sign-in finished; run momentum login again")
		default:
			return Credentials{}, err
		}
	}
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
