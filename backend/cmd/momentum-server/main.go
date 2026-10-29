// Command momentum-server is Momentum's backend (ADR 0002). It reads its
// configuration from environment variables and stops on SIGINT or SIGTERM.
//
//	momentum-server            serve
//	momentum-server migrate    apply pending database migrations and exit
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/auth"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/calendar"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/config"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/httpapi"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/logging"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/tasks"
)

// Version is replaced by release builds; local builds remain explicit.
var Version = "dev"

// drain leaves a margin inside the ten seconds Cloud Run waits between
// SIGTERM and SIGKILL.
const drain = 8 * time.Second

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "momentum-server:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 1 || (len(args) == 1 && args[0] != "migrate") {
		return fmt.Errorf("usage: momentum-server [migrate]")
	}
	settings, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := store.Open(settings.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if len(args) == 1 {
		if err := store.Migrate(ctx, db); err != nil {
			return err
		}
		logger.Info("migrated")
		return nil
	}

	providers, err := signInProviders(ctx, settings)
	if err != nil {
		return err
	}
	if len(providers) == 0 {
		logger.Warn("no sign-in provider is configured; nobody can sign in")
	}
	if len(settings.AllowedEmails) == 0 {
		logger.Warn("ALLOWED_EMAILS is empty; nobody can sign in")
	}
	client := store.NewClient(db)
	signIn := auth.New(client, auth.Config{
		PublicURL: settings.PublicURL, AllowedEmails: settings.AllowedEmails, Providers: providers,
	})
	calendars, err := calendarService(ctx, logger, client, settings)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(settings.Port))
	if err != nil {
		return err
	}
	logger.Info("listening", slog.String("addr", listener.Addr().String()))
	if err := httpapi.Serve(ctx, listener, httpapi.New(logger, Version, signIn, tasks.New(client), calendars, settings.ProxyHops), drain); err != nil {
		return err
	}
	logger.Info("stopped")
	return nil
}

// calendarService returns the Google Calendar service, which offers Google
// Calendar only with a Google OAuth client and CALENDAR_TOKEN_KEY both set
// (ADR 0003, D5). Without them it still answers, saying nothing is offered.
func calendarService(ctx context.Context, logger *slog.Logger, client *ent.Client, settings config.Config) (*calendar.Service, error) {
	service := calendar.Config{PublicURL: settings.PublicURL, Key: settings.CalendarTokenKey, Logger: logger}
	if settings.CalendarTokenKey == nil || !settings.Google.Configured() {
		logger.Info("Google Calendar is not offered; it needs GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET and CALENDAR_TOKEN_KEY")
		return calendar.New(client, service), nil
	}
	google, err := calendar.NewGoogle(ctx, auth.GoogleIssuer, settings.Google.ID, settings.Google.Secret,
		settings.PublicURL+calendar.CallbackPath, calendar.GoogleAPI)
	if err != nil {
		return nil, err
	}
	service.Google = google
	return calendar.New(client, service), nil
}

// signInProviders returns the providers the configuration has credentials
// for, each calling back to PUBLIC_URL/auth/callback/<name>.
func signInProviders(ctx context.Context, settings config.Config) ([]auth.Provider, error) {
	callback := settings.PublicURL + "/auth/callback/"
	var providers []auth.Provider
	if settings.Google.Configured() {
		google, err := auth.NewGoogle(ctx, auth.GoogleIssuer, settings.Google.ID, settings.Google.Secret, callback+auth.GoogleName)
		if err != nil {
			return nil, err
		}
		providers = append(providers, google)
	}
	if settings.GitHub.Configured() {
		providers = append(providers, auth.NewGitHub(auth.GitHubDotCom, settings.GitHub.ID, settings.GitHub.Secret, callback+auth.GitHubName))
	}
	return providers, nil
}
