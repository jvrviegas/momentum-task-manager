// Command momentum is a keyboard-first terminal frontend for Taskwarrior.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvrviegas/momentum/internal/app"
	"github.com/jvrviegas/momentum/internal/config"
	"github.com/jvrviegas/momentum/internal/diagnostics"
	"github.com/jvrviegas/momentum/internal/taskwarrior"
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
		fmt.Fprintf(stderr, "momentum: unsupported argument %q; use inbox, today, or doctor\n", remaining[0])
		return 2
	}

	settings, resolvedPath, err := config.LoadWithOptions(config.LoadOptions{PathOverride: *configPath})
	if err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}
	client := taskwarrior.NewClient("task")
	if command == "doctor" {
		report := diagnostics.Run(context.Background(), diagnostics.DoctorOptions{
			Client: client, Binary: client.Binary, Config: settings, ConfigPath: resolvedPath,
		})
		report.Print(stdout)
		if report.OK() {
			return 0
		}
		return 1
	}
	if err := validateTaskwarrior(context.Background(), client); err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}

	home, _ := os.UserHomeDir()
	logger, closer, _, err := diagnostics.NewLogger(*debug, diagnostics.LogOptions{HomeDir: home, XDGStateHome: os.Getenv("XDG_STATE_HOME")})
	if err != nil {
		fmt.Fprintln(stderr, "momentum: initialize debug log:", err)
		return 1
	}
	defer closer.Close()
	client.OnCommand = func(kind string, result taskwarrior.CommandResult, commandErr error) {
		diagnostics.LogCommand(logger, kind, result, commandErr)
	}
	logger.Debug("starting Momentum", "command", command, "config", resolvedPath)

	initial := app.ViewAuto
	switch command {
	case "inbox":
		initial = app.ViewInbox
	case "today":
		initial = app.ViewToday
	case "":
		// automatic view selection happens after the first export
	default:
		fmt.Fprintf(stderr, "momentum: unknown command %q\n", command)
		return 2
	}
	darkBackground := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	model := newAppModel(client, settings, resolvedPath, initial, &darkBackground)
	program := tea.NewProgram(model)
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(stderr, "momentum:", err)
		return 1
	}
	return 0
}

func newAppModel(client taskwarrior.Client, settings config.Config, resolvedPath string, initial app.ViewName, darkBackground *bool) *app.Model {
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

func extractCommand(args []string) (string, []string, error) {
	var command string
	flags := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "inbox", "today", "doctor":
			if command != "" {
				return "", nil, fmt.Errorf("multiple commands: %s and %s", command, arg)
			}
			command = arg
		case "--config", "-config":
			flags = append(flags, arg)
			if index+1 >= len(args) {
				return "", nil, errors.New("-config requires a path")
			}
			index++
			flags = append(flags, args[index])
		case "--debug", "-debug", "--version", "-version", "--help", "-help", "-h":
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
	fmt.Fprintln(w, "Momentum — a keyboard-first Taskwarrior frontend")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  momentum [inbox|today]")
	fmt.Fprintln(w, "  momentum doctor")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  --config PATH   use another config file")
	fmt.Fprintln(w, "  --debug         enable local diagnostic logging")
	fmt.Fprintln(w, "  --version       print version")
	fmt.Fprintln(w, "  --help          show this help")
}

var versionRE = regexp.MustCompile(`(?i)\b([0-9]+)(?:\.[0-9]+)+\b`)

func validateTaskwarrior(ctx context.Context, client *taskwarrior.CommandClient) error {
	return validateTaskwarriorWithLookPath(ctx, client, exec.LookPath)
}

func validateTaskwarriorWithLookPath(ctx context.Context, client *taskwarrior.CommandClient, lookPath func(string) (string, error)) error {
	if client == nil {
		return errors.New("Taskwarrior is not configured")
	}
	path, err := lookPath(client.Binary)
	if err != nil {
		return errors.New("Taskwarrior executable not found; install Taskwarrior 3.x or run `momentum doctor`")
	}
	version, err := client.Version(ctx)
	if err != nil {
		return fmt.Errorf("cannot run Taskwarrior at %s: %w", path, err)
	}
	match := versionRE.FindStringSubmatch(version)
	if len(match) < 2 {
		return fmt.Errorf("could not determine Taskwarrior version from %q", taskwarrior.Redact(version))
	}
	major, err := strconv.Atoi(match[1])
	if err != nil || major < 3 {
		return fmt.Errorf("Taskwarrior 3.x is required; found %s", taskwarrior.Redact(version))
	}
	return nil
}
