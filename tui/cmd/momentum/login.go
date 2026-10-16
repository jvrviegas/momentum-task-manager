package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/backend"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// openBrowser opens url in the default browser without waiting for it.
// Tests replace it.
var openBrowser = func(url string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	command := exec.CommandContext(context.Background(), name, url)
	if err := command.Start(); err != nil {
		return err
	}
	go command.Wait()
	return nil
}

// account runs momentum login or logout for the server in [backend].
func account(command string, settings config.Config, configPath string, stdout, stderr io.Writer) int {
	if !settings.Backend.Enabled() {
		fmt.Fprintf(stderr, "momentum: %s needs a [backend] url in %s\n", command, configPath)
		return 1
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}
	path := backend.CredentialsPath(home, os.Getenv("XDG_CONFIG_HOME"))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if command == "login" {
		return login(ctx, settings.Backend.URL, path, stdout, stderr)
	}
	return logout(ctx, settings.Backend.URL, path, stdout, stderr)
}

// login signs in with the device flow and saves the sign-in at path. A
// sign-in it replaces is revoked, so its token stops working.
func login(ctx context.Context, url, path string, stdout, stderr io.Writer) int {
	previous, _ := backend.ReadCredentials(path, url, time.Time{})
	credentials, err := backend.SignIn(ctx, url, func(start api.DeviceSignIn) {
		fmt.Fprintf(stdout, "To sign in to %s, enter the code %s at\n%s\n", url, start.UserCode, start.VerificationURL)
		if err := openBrowser(start.VerificationURLComplete); err != nil {
			fmt.Fprintln(stdout, "Could not open a browser; open the address above.")
		}
		fmt.Fprintln(stdout, "Waiting for you to finish signing in (Ctrl+C cancels)…")
	})
	if err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}
	if err := backend.SaveCredentials(path, credentials); err != nil {
		fmt.Fprintln(stderr, "momentum: save the sign-in:", err)
		return 1
	}
	fmt.Fprintf(stdout, "Signed in to %s as %s until %s.\n", url, credentials.Email, credentials.ExpiresAt.Local().Format(time.DateOnly))
	if previous.Token != "" && previous.Token != credentials.Token {
		if err := revoke(ctx, url, previous.Token); err != nil {
			fmt.Fprintf(stderr, "momentum: could not revoke the previous sign-in, which stays valid until %s: %v\n",
				previous.ExpiresAt.Local().Format(time.DateOnly), err)
		}
	}
	return 0
}

// logout revokes the sign-in saved at path and removes it. When the server
// cannot revoke it, the sign-in is still removed from this device.
func logout(ctx context.Context, url, path string, stdout, stderr io.Writer) int {
	saved, err := backend.ReadCredentials(path, url, time.Time{}) // an expired sign-in is removed too
	if errors.Is(err, backend.ErrNotSignedIn) {
		fmt.Fprintln(stdout, "momentum:", err)
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}
	revokeErr := revoke(ctx, url, saved.Token)
	if err := backend.RemoveCredentials(path); err != nil {
		fmt.Fprintln(stderr, "momentum: remove the sign-in:", err)
		return 1
	}
	if revokeErr != nil {
		fmt.Fprintf(stderr, "momentum: could not revoke the sign-in on the server, so its token stays valid until %s: %v\n",
			saved.ExpiresAt.Local().Format(time.DateOnly), revokeErr)
	}
	fmt.Fprintf(stdout, "Signed out of %s.\n", url)
	return 0
}

// revoke ends the sign-in of token. One the server no longer accepts has
// already ended.
func revoke(ctx context.Context, url, token string) error {
	err := api.NewClient(url, token).SignOut(ctx)
	var failure *api.Error
	if errors.As(err, &failure) && failure.Code == api.CodeUnauthorized {
		return nil
	}
	return err
}
