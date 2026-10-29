package diagnostics

import (
	"context"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// googleStatuser is what a server client offers when it can say where this
// account's Google Calendar stands (ADR 0003). It is asked for apart from
// BackendOptions.Server, so a client that cannot say is skipped, not broken.
type googleStatuser interface {
	GoogleCalendarStatus(context.Context) (api.GoogleCalendarStatus, error)
}

// googleCalendarCheck is the "Calendar source" check when [calendar] source
// is "google", which counts whatever [calendar] enabled says (D9). It needs the
// server and a sign-in, as the Sign-in check needs the server, and is skipped
// when either did not pass, since their own checks say why. Where a fix is in
// the macOS app's Settings, the detail names it; the terminal app has no such
// tab. earlier are the checks already made.
func googleCalendarCheck(ctx context.Context, options DoctorOptions, earlier []Check) Check {
	const name = "Calendar source"
	skip := func(why string) Check { return Check{Name: name, Detail: why, Skipped: true, OK: true} }
	if options.Backend == nil || !passed(earlier, "Backend server") {
		return skip("needs the server")
	}
	if !passed(earlier, "Sign-in") {
		return skip("needs a sign-in")
	}
	server, ok := options.Backend.Server.(googleStatuser)
	if !ok {
		return skip("this server client cannot say where Google Calendar stands")
	}
	status, err := server.GoogleCalendarStatus(ctx)
	if err != nil {
		return Check{Name: name, Detail: "Unable to check Google Calendar: " + Redact(err.Error())}
	}
	problem := func(reason, fix string) Check {
		if options.NoTerminal && fix != "" {
			reason += ". " + fix
		}
		return Check{Name: name, Detail: reason}
	}
	switch {
	case !status.Offered:
		return Check{Name: name, Detail: "Google Calendar isn’t available on this server. Ask whoever runs the server to turn it on."}
	case !status.Connected:
		return problem("Google Calendar isn’t connected", "Connect it in Settings ▸ Calendar.")
	case status.NeedsReconnect:
		return problem("Google Calendar needs reconnecting", "Reconnect it in Settings ▸ Calendar.")
	}
	return Check{Name: name, Detail: "Google Calendar connected as " + status.Email + "; read-only", OK: true}
}

// passed reports whether the check named name was made and passed.
func passed(checks []Check, name string) bool {
	for _, check := range checks {
		if check.Name == name {
			return check.OK && !check.Skipped
		}
	}
	return false
}
