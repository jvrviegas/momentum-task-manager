// Command momentum-bridge serves Momentum's Taskwarrior operations to the
// native macOS app as newline-delimited JSON over stdin and stdout.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jvrviegas/momentum-task-manager/tui/internal/bridge"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/taskwarrior"
)

var Version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("momentum-bridge", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "path to Momentum config.toml")
	taskBinary := flags.String("task", "task", "Taskwarrior executable")
	showVersion := flags.Bool("version", false, "print version")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, Version)
		return 0
	}
	settings, resolvedPath, err := config.LoadWithOptions(config.LoadOptions{PathOverride: *configPath})
	if err != nil {
		fmt.Fprintln(stderr, "momentum-bridge:", err)
		return 1
	}
	server := bridge.NewServer(taskwarrior.NewClient(*taskBinary), settings, resolvedPath)
	if err := server.Serve(context.Background(), stdin, stdout); err != nil {
		fmt.Fprintln(stderr, "momentum-bridge:", err)
		return 1
	}
	return 0
}
