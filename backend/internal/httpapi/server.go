// Package httpapi serves the routes in core/api over HTTP.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/auth"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/tasks"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// New returns the server's handler, which reports version as the server's.
// GET /health reports that the server is up; Cloud Run reserves some paths
// that end in z, so it is not /healthz. Sign-in has JSON routes for the apps
// and pages for the browser; the other /v1 routes need a signed-in user. Any
// other route answers with an api.Error. Starting a sign-in, submitting the
// code, and the providers' callbacks share one limit per client, and polling
// has another; proxyHops is how many proxies in front of the server append
// the client's address to X-Forwarded-For.
func New(logger *slog.Logger, version string, signIn *auth.Service, userTasks *tasks.Service, proxyHops int) http.Handler {
	s := &server{logger: logger, auth: signIn, tasks: userTasks, secure: strings.HasPrefix(signIn.PublicURL(), "https://"), proxyHops: proxyHops}
	signIns, polls := newLimiter(signInLimit, limitWindow), newLimiter(pollLimit, limitWindow)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc(api.RouteVersion, s.authenticated(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, api.Version{Server: version})
	}))
	mux.HandleFunc(api.RouteStartDeviceSignIn, s.limited(signIns, refuseSignIn, s.startDeviceSignIn))
	mux.HandleFunc(api.RoutePollDeviceSignIn, s.limited(polls, refusePoll, s.pollDeviceSignIn))
	mux.HandleFunc(api.RouteSession, s.authenticated(s.session))
	mux.HandleFunc(api.RouteSignOut, s.authenticated(s.signOut))
	mux.HandleFunc(api.RouteListTasks, s.authenticated(s.listTasks))
	mux.HandleFunc(api.RouteCreateTask, s.authenticated(s.createTask))
	mux.HandleFunc(api.RouteModifyTask, s.authenticated(s.modifyTask))
	mux.HandleFunc(api.RouteDeleteTask, s.authenticated(s.taskAction(userTasks.Delete)))
	mux.HandleFunc(api.RouteCompleteTask, s.authenticated(s.taskAction(userTasks.Complete)))
	mux.HandleFunc(api.RouteStartTask, s.authenticated(s.taskAction(userTasks.Start)))
	mux.HandleFunc(api.RouteStopTask, s.authenticated(s.taskAction(userTasks.Stop)))
	mux.HandleFunc(api.RouteStopRecurrence, s.authenticated(s.taskAction(userTasks.StopRecurrence)))
	mux.HandleFunc(api.RouteUndo, s.authenticated(s.undo))
	mux.HandleFunc(api.RouteListProjects, s.authenticated(s.listProjects))
	mux.HandleFunc(api.RouteListTags, s.authenticated(s.listTags))
	mux.HandleFunc(api.RoutePreviewProjectMigration, s.authenticated(s.previewProjectMigration))
	mux.HandleFunc(api.RouteApplyProjectMigration, s.authenticated(s.applyProjectMigration))
	mux.HandleFunc(api.RouteSetTimeZone, s.authenticated(s.setTimeZone))
	mux.HandleFunc(api.RoutePreviewImport, s.authenticated(s.previewImport))
	mux.HandleFunc(api.RouteApplyImport, s.authenticated(s.applyImport))
	mux.HandleFunc("GET "+devicePath, s.devicePage)
	mux.HandleFunc("POST "+devicePath, s.limited(signIns, refusePage, s.deviceSubmit))
	mux.HandleFunc("GET "+callbackPath+"{provider}", s.limited(signIns, refusePage, s.callback))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, &api.Error{Code: api.CodeNotFound, Message: "no such endpoint"})
	})
	return logRequests(logger, mux)
}

type server struct {
	logger *slog.Logger
	auth   *auth.Service
	tasks  *tasks.Service
	// secure marks cookies Secure when browsers reach the server over HTTPS.
	secure bool
	// proxyHops is how many trusted proxies append to X-Forwarded-For.
	proxyHops int
}

var errorStatus = map[string]int{
	api.CodeInvalid:              http.StatusBadRequest,
	api.CodeUnauthorized:         http.StatusUnauthorized,
	api.CodeNotFound:             http.StatusNotFound,
	api.CodeConflict:             http.StatusConflict,
	api.CodeInternal:             http.StatusInternalServerError,
	api.CodeRateLimited:          http.StatusTooManyRequests,
	api.CodeAuthorizationPending: http.StatusBadRequest,
	api.CodeSlowDown:             http.StatusBadRequest,
	api.CodeAccessDenied:         http.StatusBadRequest,
	api.CodeExpiredToken:         http.StatusBadRequest,
}

// internalError answers 500 and logs the type of the error, never its text,
// which could hold user data.
func (s *server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.logger.ErrorContext(r.Context(), "request failed", slog.String("error_type", errorType(err)))
	writeError(w, &api.Error{Code: api.CodeInternal, Message: "the server failed; try again"})
}

// errorType names the type of the innermost error err wraps.
func errorType(err error) string {
	for {
		inner := errors.Unwrap(err)
		if inner == nil {
			return fmt.Sprintf("%T", err)
		}
		err = inner
	}
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
