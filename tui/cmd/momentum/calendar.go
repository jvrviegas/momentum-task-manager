package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/backend"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// calendarPollInterval is how often momentum calendar connect asks whether the
// browser is done, as the macOS app does. Tests shorten it.
var calendarPollInterval = 2 * time.Second

// calendarInput is where momentum calendar disconnect reads its answer. Tests
// replace it.
var calendarInput io.Reader = os.Stdin

const googleNotOffered = "Google Calendar isn’t available on this server. Ask whoever runs the server to turn it on."

// googleCalendarAPI is what momentum calendar asks of the server (ADR 0003).
type googleCalendarAPI interface {
	GoogleCalendarStatus(context.Context) (api.GoogleCalendarStatus, error)
	ConnectGoogleCalendar(context.Context) (api.GoogleCalendarLink, error)
	DisconnectGoogleCalendar(context.Context) (api.GoogleCalendarDisconnect, error)
}

// calendarCommand runs momentum calendar connect, status or disconnect for the
// server in [backend]. The connection belongs to the account, so these ask the
// server and never touch config.toml.
func calendarCommand(command string, settings config.Config, configPath string, stdout, stderr io.Writer) int {
	if !settings.Backend.Enabled() {
		fmt.Fprintf(stderr, "momentum: %s needs a [backend] url in %s\n", command, configPath)
		return 1
	}
	credentials, err := backend.SavedCredentials(settings.Backend.URL)
	if err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	server := api.NewClient(settings.Backend.URL, credentials.Token)
	switch command {
	case "calendar connect":
		return connectCalendar(ctx, server, settings, configPath, stdout, stderr)
	case "calendar disconnect":
		return disconnectCalendar(ctx, server, stdout, stderr)
	}
	return calendarStatus(ctx, server, settings, configPath, stdout, stderr)
}

// connectCalendar prints the one-time link, opens it, and waits until the
// account is connected, the link expires, or ctx is done. A connection that
// already works is left alone, as the macOS app offers Connect… only without
// one; a connection Google refuses is replaced.
func connectCalendar(ctx context.Context, server googleCalendarAPI, settings config.Config, configPath string, stdout, stderr io.Writer) int {
	status, err := server.GoogleCalendarStatus(ctx)
	if err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}
	if !status.Offered {
		fmt.Fprintln(stderr, "momentum:", googleNotOffered)
		return 1
	}
	if status.Connected && !status.NeedsReconnect {
		fmt.Fprintf(stdout, "Google Calendar is already connected as %s. To use another account, run momentum calendar disconnect first.\n", status.Email)
		fmt.Fprint(stdout, countOnThisMachine(settings, configPath))
		return 0
	}
	link, err := server.ConnectGoogleCalendar(ctx)
	if err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}
	verb := "connect"
	if status.NeedsReconnect {
		verb = "reconnect"
	}
	fmt.Fprintf(stdout, "To %s Google Calendar, open\n%s\n", verb, link.URL)
	if err := openBrowser(link.URL); err != nil {
		fmt.Fprintln(stdout, "Could not open a browser; open the address above.")
	}
	fmt.Fprintln(stdout, "Waiting for you to finish in the browser (Ctrl+C cancels)…")
	for {
		select {
		case <-ctx.Done():
		case <-time.After(calendarPollInterval):
		}
		current, err := server.GoogleCalendarStatus(ctx)
		if ctx.Err() != nil {
			fmt.Fprintln(stderr, "momentum: stopped waiting; nothing was connected.")
			return 1
		}
		if err != nil {
			fmt.Fprintln(stderr, "momentum:", err)
			return 1
		}
		if current.Connected && !current.NeedsReconnect {
			fmt.Fprintf(stdout, "Connected Google Calendar as %s.\n", current.Email)
			fmt.Fprint(stdout, countOnThisMachine(settings, configPath))
			return 0
		}
		if !time.Now().Before(link.ExpiresAt) {
			fmt.Fprintln(stderr, "momentum: the link expired. Run momentum calendar connect to try again.")
			return 1
		}
	}
}

// calendarStatus says where the account's Google Calendar stands and whether
// this machine's planner counts it. Any answer the server gives is a success.
func calendarStatus(ctx context.Context, server googleCalendarAPI, settings config.Config, configPath string, stdout, stderr io.Writer) int {
	status, err := server.GoogleCalendarStatus(ctx)
	if err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}
	switch {
	case !status.Offered:
		fmt.Fprintln(stdout, googleNotOffered)
	case !status.Connected:
		fmt.Fprintln(stdout, "Google Calendar isn’t connected. Run momentum calendar connect to connect it.")
	case status.NeedsReconnect:
		fmt.Fprintf(stdout, "Google Calendar needs reconnecting. Google no longer accepts the saved connection to %s. Run momentum calendar connect to reconnect it.\n", status.Email)
	default:
		fmt.Fprintf(stdout, "Google Calendar connected as %s; read-only\n", status.Email)
		if hint := countOnThisMachine(settings, configPath); hint != "" {
			fmt.Fprint(stdout, hint)
		} else {
			fmt.Fprintln(stdout, "This machine counts it in the planner.")
		}
	}
	return 0
}

// disconnectCalendar asks, then disconnects. Only y or yes disconnects, so an
// answer that cannot be read, such as no input at all, changes nothing.
func disconnectCalendar(ctx context.Context, server googleCalendarAPI, stdout, stderr io.Writer) int {
	status, err := server.GoogleCalendarStatus(ctx)
	if err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}
	if !status.Offered {
		fmt.Fprintln(stderr, "momentum:", googleNotOffered)
		return 1
	}
	if !status.Connected {
		fmt.Fprintln(stdout, "Google Calendar isn’t connected; nothing to disconnect.")
		return 0
	}
	fmt.Fprintf(stdout, "Momentum will stop reading %s and delete its saved access, for every Mac and the terminal app on your account. The planner counts no meetings until you connect again.\n", status.Email)
	fmt.Fprint(stdout, "Disconnect Google Calendar? [y/N] ")
	answer, _ := bufio.NewReader(calendarInput).ReadString('\n')
	if answer = strings.ToLower(strings.TrimSpace(answer)); answer != "y" && answer != "yes" {
		fmt.Fprintln(stdout, "Nothing was disconnected.")
		return 1
	}
	disconnected, err := server.DisconnectGoogleCalendar(ctx)
	if err != nil {
		fmt.Fprintln(stderr, "momentum: unable to disconnect Google Calendar:", err)
		return 1
	}
	fmt.Fprintln(stdout, "Disconnected Google Calendar.")
	if !disconnected.Revoked {
		fmt.Fprintln(stderr, "momentum: couldn’t reach Google to revoke its access, so it may still show in your Google account’s settings. Remove it there.")
	}
	return 0
}

// countOnThisMachine says how to make this machine's planner count Google
// Calendar, or is empty when it already does. The source belongs to the machine
// (D6) and the terminal app never edits config.toml, so it names the key and
// leaves the editing to the person; a snippet would repeat an existing
// [calendar] table, which TOML refuses.
func countOnThisMachine(settings config.Config, configPath string) string {
	if settings.Calendar.Source == config.CalendarSourceGoogle {
		return ""
	}
	return fmt.Sprintf("To count it on this machine, set source = \"google\" under [calendar] in %s.\n", configPath)
}
