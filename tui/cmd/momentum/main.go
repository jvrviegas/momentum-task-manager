// Command momentum is a keyboard-first terminal task manager that keeps its
// tasks on a Momentum server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/app"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/backend"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/diagnostics"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/taskwarrior"
)

// Version is replaced by release builds; local builds remain explicit.
var Version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	command, flagArgs, err := extractCommand(args)
	if err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 2
	}
	flags := flag.NewFlagSet("momentum", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "path to Momentum config.toml")
	debug := flags.Bool("debug", false, "write local diagnostic logs")
	showVersion := flags.Bool("version", false, "print version")
	showHelp := flags.Bool("help", false, "show help")
	dryRun := flags.Bool("dry-run", false, "with import, report without importing")
	flags.Usage = func() { printUsage(stderr) }
	if err := flags.Parse(flagArgs); err != nil {
		return 2
	}
	if *showHelp {
		printUsage(stdout)
		return 0
	}
	if *showVersion {
		fmt.Fprintln(stdout, Version)
		return 0
	}
	if remaining := flags.Args(); len(remaining) > 0 {
		fmt.Fprintf(stderr, "momentum: unsupported argument %q; use inbox, today, completed, doctor, login, logout, import, or calendar\n", remaining[0])
		return 2
	}
	if command == "calendar" {
		fmt.Fprintln(stderr, "momentum: calendar needs connect, status or disconnect")
		return 2
	}
	if *dryRun && command != "import" {
		fmt.Fprintln(stderr, "momentum: --dry-run applies only to import")
		return 2
	}

	settings, resolvedPath, err := config.LoadWithOptions(config.LoadOptions{PathOverride: *configPath})
	if err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}
	if command == "login" || command == "logout" {
		return account(command, settings, resolvedPath, stdout, stderr)
	}
	if strings.HasPrefix(command, "calendar ") {
		return calendarCommand(command, settings, resolvedPath, stdout, stderr)
	}
	if command == "import" {
		return importTasks(context.Background(), settings, resolvedPath, taskwarrior.NewClient("task"), *dryRun, stdout, stderr)
	}
	if command == "doctor" {
		options := diagnostics.DoctorOptions{Config: settings, ConfigPath: resolvedPath}
		if settings.Backend.Enabled() {
			options.Backend = backendDoctor(settings.Backend.URL)
		}
		report := diagnostics.Run(context.Background(), options)
		report.Print(stdout)
		if report.OK() {
			return 0
		}
		return 1
	}
	if !settings.Backend.Enabled() {
		fmt.Fprint(stderr, needsServer(resolvedPath))
		return 1
	}
	credentials, err := backend.SavedCredentials(settings.Backend.URL)
	if err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}
	storage := backend.New(settings.Backend.URL, credentials.Token, backend.DeviceZone())

	home, _ := os.UserHomeDir()
	logger, closer, _, err := diagnostics.NewLogger(*debug, diagnostics.LogOptions{HomeDir: home, XDGStateHome: os.Getenv("XDG_STATE_HOME")})
	if err != nil {
		fmt.Fprintln(stderr, "momentum: initialize debug log:", err)
		return 1
	}
	defer closer.Close()
	logger.Debug("starting Momentum", "command", command, "config", resolvedPath)

	initial := app.ViewAuto
	switch command {
	case "inbox":
		initial = app.ViewInbox
	case "today":
		initial = app.ViewToday
	case "completed":
		initial = app.ViewCompleted
	case "":
		// automatic view selection happens after the first export
	default:
		fmt.Fprintf(stderr, "momentum: unknown command %q\n", command)
		return 2
	}
	darkBackground := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	model := newAppModel(storage, settings, resolvedPath, initial, &darkBackground)
	program := tea.NewProgram(model)
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}
	return 0
}

func newAppModel(client app.Client, settings config.Config, resolvedPath string, initial app.ViewName, darkBackground *bool) *app.Model {
	store := config.NewProjectCatalogStore(resolvedPath)
	migrationClient, _ := client.(app.ProjectMigrationClient)
	coordinator := app.NewProjectMigrationCoordinator(store, migrationClient)
	return app.NewModel(app.ModelOptions{
		Client:               client,
		Config:               settings,
		InitialView:          initial,
		DarkBackground:       darkBackground,
		ProjectStore:         store,
		ProjectConfigPath:    resolvedPath,
		MigrationCoordinator: coordinator,
	})
}

// needsServer is the setup steps for a config with no [backend] server.
func needsServer(configPath string) string {
	return fmt.Sprintf(`momentum: Momentum keeps tasks on a Momentum server. Add its address to %s:

  [backend]
  url = "https://momentum.example.com"

then run momentum login. To bring over Taskwarrior tasks, run momentum import.
`, configPath)
}

// backendDoctor describes the [backend] server to momentum doctor.
func backendDoctor(url string) *diagnostics.BackendOptions {
	credentials, err := backend.SavedCredentials(url)
	return &diagnostics.BackendOptions{
		URL: url, Server: api.NewClient(url, credentials.Token), CredentialsErr: err, Zone: backend.DeviceZone(),
	}
}

func extractCommand(args []string) (string, []string, error) {
	var command string
	flags := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "inbox", "today", "completed", "doctor", "login", "logout", "import", "calendar":
			if command != "" {
				return "", nil, fmt.Errorf("multiple commands: %s and %s", command, arg)
			}
			command = arg
		case "connect", "status", "disconnect":
			// Only calendar takes a command of its own; anywhere else these
			// are arguments that run rejects.
			if command == "calendar" {
				command += " " + arg
			} else {
				flags = append(flags, arg)
			}
		case "--config", "-config":
			flags = append(flags, arg)
			if index+1 >= len(args) {
				return "", nil, errors.New("-config requires a path")
			}
			index++
			flags = append(flags, args[index])
		case "--debug", "-debug", "--version", "-version", "--help", "-help", "-h", "--dry-run", "-dry-run":
			flags = append(flags, arg)
		default:
			if strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "-config=") {
				flags = append(flags, arg)
			} else {
				flags = append(flags, arg)
			}
		}
	}
	return command, flags, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Momentum — a keyboard-first task manager")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  momentum [inbox|today|completed]")
	fmt.Fprintln(w, "  momentum doctor")
	fmt.Fprintln(w, "  momentum login")
	fmt.Fprintln(w, "  momentum logout")
	fmt.Fprintln(w, "  momentum import [--dry-run]")
	fmt.Fprintln(w, "  momentum calendar connect|status|disconnect")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  --config PATH   use another config file")
	fmt.Fprintln(w, "  --debug         enable local diagnostic logging")
	fmt.Fprintln(w, "  --version       print version")
	fmt.Fprintln(w, "  --help          show this help")
	fmt.Fprintln(w, "  --dry-run       with import, report what it would do and import nothing")
}
