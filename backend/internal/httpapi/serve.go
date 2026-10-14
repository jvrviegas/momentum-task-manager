package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// Serve answers requests on listener until ctx ends, then stops accepting
// new ones and waits up to drain for those in flight.
func Serve(ctx context.Context, listener net.Listener, handler http.Handler, drain time.Duration) error {
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), drain)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		return err
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
