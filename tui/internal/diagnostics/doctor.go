package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// Check is one safe doctor result.
type Check struct {
	Name    string
	OK      bool
	Skipped bool
	Detail  string
}

// Report is the complete diagnostic result.
type Report struct {
	Checks []Check
}

func (r Report) OK() bool {
	for _, check := range r.Checks {
		if !check.OK && !check.Skipped {
			return false
		}
	}
	return true
}

// DoctorOptions contains injectable checks for deterministic tests. Backend
// is nil when the config names no [backend] server.
type DoctorOptions struct {
	Backend    *BackendOptions
	Config     config.Config
	ConfigPath string
	HomeDir    string
	StateDir   string
	Terminal   string
	// NoTerminal leaves out the Terminal and State directory checks, which
	// belong to the terminal app: the macOS app has neither a terminal nor a log.
	NoTerminal  bool
	MkdirAll    func(string, os.FileMode) error
	WritableDir func(string) error
}

// BackendOptions describe the [backend] server.
type BackendOptions struct {
	URL    string
	Server interface {
		Version(context.Context) (api.Version, error)
		Session(context.Context) (api.Session, error)
	}
	// CredentialsErr is why no saved sign-in for URL could be read, if any.
	CredentialsErr error
	// Zone is the device's IANA time zone, or "" when it is unknown.
	Zone string
}

// Run performs only read-only server calls and local filesystem checks. It
// never changes a task.
func Run(ctx context.Context, options DoctorOptions) Report {
	if ctx == nil {
		ctx = context.Background()
	}
	if options.MkdirAll == nil {
		options.MkdirAll = os.MkdirAll
	}
	if options.WritableDir == nil {
		options.WritableDir = checkWritableDir
	}
	var report Report
	if options.Backend != nil {
		report.Checks = append(report.Checks, backendChecks(ctx, *options.Backend)...)
	} else {
		report.Checks = append(report.Checks, Check{Name: "Backend server",
			Detail: "no [backend] url in " + configPathDetail(options.ConfigPath) + "; add one, then run momentum login"})
	}
	configErr := options.Config.Validate()
	report.Checks = append(report.Checks, Check{Name: "Momentum config", Detail: errorDetail(configErr, configPathDetail(options.ConfigPath)), OK: configErr == nil})

	if options.Config.Calendar.Source == config.CalendarSourceGoogle {
		report.Checks = append(report.Checks, googleCalendarCheck(ctx, options, report.Checks))
	} else if options.Config.Calendar.Enabled {
		home := options.HomeDir
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		paths := options.Config.Calendar.PathsFor(home)
		calendarDetail := "calendar source unavailable"
		calendarOK := len(paths) > 0
		for _, path := range paths {
			if _, err := os.Stat(path); err != nil {
				calendarOK = false
				calendarDetail = "cannot read configured local ICS source"
				break
			}
		}
		if calendarOK {
			calendarDetail = fmt.Sprintf("%d local ICS source(s); read-only", len(paths))
		}
		report.Checks = append(report.Checks, Check{Name: "Calendar source", Detail: calendarDetail, OK: calendarOK})
	} else {
		report.Checks = append(report.Checks, Check{Name: "Calendar source", Detail: "disabled; task-only planning remains available", Skipped: true, OK: true})
	}

	if options.NoTerminal {
		return report
	}
	terminal := options.Terminal
	if terminal == "" {
		terminal = os.Getenv("TERM")
	}
	report.Checks = append(report.Checks, Check{Name: "Terminal", Detail: terminalDetail(terminal), OK: terminal != ""})
	stateDir := options.StateDir
	if stateDir == "" {
		stateDir, _ = ProcessStateDir()
	}
	if stateDir == "" {
		report.Checks = append(report.Checks, Check{Name: "State directory", Detail: "home directory unavailable", OK: false})
	} else {
		mkdirErr := options.MkdirAll(stateDir, 0o700)
		writeErr := error(nil)
		if mkdirErr == nil {
			writeErr = options.WritableDir(stateDir)
		}
		report.Checks = append(report.Checks, Check{Name: "State directory", Detail: errorDetail(firstError(mkdirErr, writeErr), stateDir), OK: mkdirErr == nil && writeErr == nil})
	}
	return report
}

// backendChecks check that the server answers, that the saved sign-in is
// valid there, and that the device's time zone is known.
func backendChecks(ctx context.Context, options BackendOptions) []Check {
	server := Check{Name: "Backend server", OK: true}
	version, err := options.Server.Version(ctx)
	var apiErr *api.Error
	switch {
	case err == nil:
		server.Detail = options.URL + " answers; version " + safeVersion(version.Server)
	case errors.As(err, &apiErr) && apiErr.Code == api.CodeUnauthorized:
		server.Detail = options.URL + " answers"
	default:
		server = Check{Name: "Backend server", Detail: err.Error()}
	}

	signIn := Check{Name: "Sign-in", Detail: errorDetail(options.CredentialsErr, "")}
	switch {
	case options.CredentialsErr != nil:
	case !server.OK:
		signIn = Check{Name: "Sign-in", Detail: "needs the server", Skipped: true, OK: true}
	default:
		session, err := options.Server.Session(ctx)
		signIn = Check{Name: "Sign-in", OK: err == nil,
			Detail: errorDetail(err, session.Email+" until "+session.ExpiresAt.Local().Format(time.DateOnly))}
	}

	zone := Check{Name: "Time zone", Detail: options.Zone, OK: options.Zone != ""}
	if !zone.OK {
		zone.Detail = "unknown; set TZ to an IANA name such as Europe/Lisbon"
	}
	return []Check{server, signIn, zone}
}

// Print writes a concise, secret-free report.
func (r Report) Print(w io.Writer) {
	for _, check := range r.Checks {
		status := "ok"
		if check.Skipped {
			status = "skip"
		} else if !check.OK {
			status = "fail"
		}
		fmt.Fprintf(w, "[%s] %-24s %s\n", status, check.Name, Redact(check.Detail))
	}
	if r.OK() {
		fmt.Fprintln(w, "Doctor: ready")
	} else {
		fmt.Fprintln(w, "Doctor: issues found")
	}
}

func checkWritableDir(path string) error {
	file, err := os.CreateTemp(path, ".momentum-doctor-")
	if err != nil {
		return err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Remove(name)
}

func safeVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "not reported"
	}
	return value
}

func configPathDetail(path string) string {
	if path == "" {
		return "defaults"
	}
	return path
}

func terminalDetail(value string) string {
	if value == "" {
		return "TERM is not set"
	}
	return value
}

func errorDetail(err error, success string) string {
	if err == nil {
		return success
	}
	return Redact(err.Error())
}

// Redact removes common credentials and secrets from diagnostic text.
func Redact(value string) string {
	value = secretAssignmentRE.ReplaceAllString(value, "$1[REDACTED]")
	value = urlCredentialRE.ReplaceAllString(value, "$1[REDACTED]$3")
	return value
}

var (
	secretAssignmentRE = regexp.MustCompile(`(?i)(encryption_secret|password|token|client_secret)\s*[:=]\s*[^\s,;]+`)
	urlCredentialRE    = regexp.MustCompile(`(?i)(https?://)([^/@\s]+)(@)`)
)

func firstError(errors ...error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}
