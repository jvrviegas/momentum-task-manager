package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

// blockingServer serves a handler that waits for release, and returns the
// server's address, a cancel that starts the shutdown, and Serve's result.
func blockingServer(t *testing.T, drain time.Duration) (string, chan struct{}, chan struct{}, context.CancelFunc, chan error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, listener, handler, drain) }()
	return listener.Addr().String(), entered, release, cancel, served
}

func TestServeFinishesRequestsInFlightBeforeReturning(t *testing.T) {
	addr, entered, release, cancel, served := blockingServer(t, 5*time.Second)
	responses := make(chan *http.Response, 1)
	go func() {
		response, err := http.Get("http://" + addr + "/")
		if err != nil {
			t.Error(err)
		}
		responses <- response
	}()
	<-entered
	cancel()

	// The listener closes at once, while the request in flight goes on.
	deadline := time.Now().Add(5 * time.Second)
	for {
		connection, err := net.Dial("tcp", addr)
		if err != nil {
			break
		}
		connection.Close()
		if time.Now().After(deadline) {
			t.Fatal("still accepting connections after shutdown began")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case err := <-served:
		t.Fatalf("Serve returned %v before the request finished", err)
	default:
	}

	close(release)
	if response := <-responses; response == nil || response.StatusCode != http.StatusNoContent {
		t.Fatalf("response = %v", response)
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve = %v, want nil", err)
	}
}

func TestServeStopsWaitingAfterDrain(t *testing.T) {
	addr, entered, release, cancel, served := blockingServer(t, 50*time.Millisecond)
	defer close(release)
	go func() {
		if response, err := http.Get("http://" + addr + "/"); err == nil {
			response.Body.Close()
		}
	}()
	<-entered
	cancel()
	if err := <-served; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Serve = %v, want context.DeadlineExceeded", err)
	}
}
