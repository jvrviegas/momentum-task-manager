// Command momentum-server is Momentum's backend (ADR 0002). It reads its
// configuration from environment variables and stops on SIGINT or SIGTERM.
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

	"github.com/jvrviegas/momentum-task-manager/backend/internal/config"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/httpapi"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/logging"
)

// drain leaves a margin inside the ten seconds Cloud Run waits between
// SIGTERM and SIGKILL.
const drain = 8 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "momentum-server:", err)
		os.Exit(1)
	}
}

func run() error {
	settings, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(settings.Port))
	if err != nil {
		return err
	}
	logger.Info("listening", slog.String("addr", listener.Addr().String()))
	if err := httpapi.Serve(ctx, listener, httpapi.New(logger), drain); err != nil {
		return err
	}
	logger.Info("stopped")
	return nil
}
