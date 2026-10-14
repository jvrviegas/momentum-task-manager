// Package httpapi serves the routes in core/api over HTTP.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// New returns the server's handler. GET /health reports that the server is
// up; Cloud Run reserves some paths that end in z, so it is not /healthz. Any
// other route answers with an api.Error.
func New(logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, &api.Error{Code: api.CodeNotFound, Message: "no such endpoint"})
	})
	return logRequests(logger, mux)
}

var errorStatus = map[string]int{
	api.CodeInvalid:      http.StatusBadRequest,
	api.CodeUnauthorized: http.StatusUnauthorized,
	api.CodeNotFound:     http.StatusNotFound,
	api.CodeConflict:     http.StatusConflict,
	api.CodeInternal:     http.StatusInternalServerError,
}

func writeError(w http.ResponseWriter, e *api.Error) {
	status, ok := errorStatus[e.Code]
	if !ok {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, e)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// logRequests logs each request once, by its route pattern, never its path,
// query, headers, or body. A panic answers 500 and is logged without its
// value, which could hold task contents.
func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				logger.Error("request panicked", slog.String("route", r.Pattern))
				if recorder.status == 0 {
					writeError(recorder, &api.Error{Code: api.CodeInternal, Message: "the server failed; try again"})
				}
			}
			level := slog.LevelInfo
			if recorder.status >= http.StatusInternalServerError {
				level = slog.LevelError
			}
			logger.Log(r.Context(), level, "request",
				slog.String("method", r.Method),
				slog.String("route", r.Pattern),
				slog.Int("status", recorder.status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()))
		}()
		next.ServeHTTP(recorder, r)
		if recorder.status == 0 {
			recorder.status = http.StatusOK
		}
	})
}

// statusRecorder remembers the status a handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(data)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
