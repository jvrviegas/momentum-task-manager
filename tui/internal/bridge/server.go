package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/core/domain"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/quickadd"
)

// Request is one line read from the frontend.
type Request struct {
	ID     int64           `json:"id"`
	Op     string          `json:"op"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response answers exactly one Request. Exactly one of Result or Error is set.
type Response struct {
	ID     int64  `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  *Error `json:"error,omitempty"`
}

// Error is a user-facing failure. Field names the edit field that caused it,
// and Code what the failure says about the server.
type Error struct {
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
	Code    string `json:"code,omitempty"`
}

// Codes of server errors.
const (
	// CodeUnreachable: the server gave no answer of its own (api.ErrUnreachable).
	CodeUnreachable = "unreachable"
	// CodeUnauthorized: the server refused the token, or there is none.
	CodeUnauthorized = "unauthorized"
)

func (e *Error) Error() string { return e.Message }

func fieldError(field, message string) *Error { return &Error{Message: message, Field: field} }

// Client is the Momentum server as the bridge reaches it; *backend.Client
// implements it.
type Client interface {
	ExportPending(context.Context) ([]domain.Task, error)
	ExportCompleted(context.Context, time.Time) ([]domain.Task, error)
	Projects(context.Context) ([]string, error)
	Add(context.Context, domain.NewTask) error
	Modify(context.Context, string, domain.TaskDiff) error
	Complete(context.Context, string) error
	Delete(context.Context, string) error
	Start(context.Context, string) error
	Stop(context.Context, string) error
	StopRecurrence(context.Context, string) error
	Undo(context.Context) error
	ApplyProjectMigration(context.Context, domain.ProjectCatalogPlan, []domain.ProjectTaskMapping) error
}

// apiFields names the edit fields that a backend error's fields belong to.
var apiFields = map[string]string{
	"description": "description", "project": "project", "priority": "priority", "due": "due",
	"scheduled": "scheduled", "recurrence": "recurrence", "estimateMinutes": "estimate", "tags": "tags",
}

// Pure operations never reach the server and may answer out of order.
var pureOps = map[string]bool{"interpret": true, "suggest": true}

const maxReviews = 8

type Server struct {
	client Client
	// connect returns a client for the [backend] server signed in with a
	// token.
	connect    func(token string) Client
	config     config.Config
	configPath string
	now        func() time.Time

	// What reloadConfig keeps: the version of config.toml the settings came
	// from, why a newer one was not applied, the [backend] settings the
	// client was made with, and whether the file now asks for others.
	configState     fileState
	configError     string
	startBackend    config.BackendConfig
	restartRequired bool

	mu         sync.Mutex
	projects   domain.ProjectCatalog
	tags       []string
	reviews    map[int64]quickadd.Interpretation
	nextReview int64
}

// NewServer returns a server that reaches the [backend] server in settings
// through connect(token), where token is the one the app last sent with
// setToken; until then it has none. Without a [backend] url, only status and
// the pure operations answer.
func NewServer(connect func(token string) Client, settings config.Config, configPath string) *Server {
	server := &Server{
		connect:    connect,
		config:     settings,
		configPath: configPath,
		now:        time.Now,
		projects:   settings.Projects,
		reviews:    map[int64]quickadd.Interpretation{},

		configState:  statFile(configPath),
		startBackend: settings.Backend,
	}
	if settings.Backend.Enabled() {
		server.client = connect("")
	}
	return server
}

// Serve answers requests until r is exhausted. Server operations run one at
// a time in arrival order; pure operations answer immediately so capture
// highlighting never waits behind a slow request.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	var writeMu sync.Mutex
	encoder := json.NewEncoder(w)
	respond := func(response Response) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = encoder.Encode(response)
	}

	var wg sync.WaitGroup
	queue := make(chan Request, 64)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for request := range queue {
			respond(s.handle(ctx, request))
		}
	}()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var request Request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			respond(Response{Error: &Error{Message: "invalid request: " + err.Error()}})
			continue
		}
		if pureOps[request.Op] {
			wg.Add(1)
			go func() {
				defer wg.Done()
				respond(s.handle(ctx, request))
			}()
			continue
		}
		queue <- request
	}
	close(queue)
	wg.Wait()
	return scanner.Err()
}

func (s *Server) handle(ctx context.Context, request Request) Response {
	result, err := s.dispatch(ctx, request.Op, request.Params)
	if err != nil {
		var bridgeErr *Error
		var apiErr *api.Error
		switch {
		case errors.As(err, &bridgeErr):
		case errors.As(err, &apiErr):
			bridgeErr = &Error{Message: apiErr.Message, Field: apiFields[apiErr.Field]}
			if apiErr.Code == api.CodeUnauthorized {
				bridgeErr.Code = CodeUnauthorized
			}
		default:
			bridgeErr = &Error{Message: err.Error()}
			if errors.Is(err, api.ErrUnreachable) {
				bridgeErr.Code = CodeUnreachable
			}
		}
		return Response{ID: request.ID, Error: bridgeErr}
	}
	if result == nil {
		result = struct{}{}
	}
	return Response{ID: request.ID, Result: result}
}

func (s *Server) dispatch(ctx context.Context, op string, raw json.RawMessage) (any, error) {
	if localOps[op] || op == "load" || op == "plan" {
		s.reloadConfig()
	}
	if !localOps[op] && !pureOps[op] {
		if _, err := s.backendURL(); err != nil {
			return nil, err
		}
	}
	switch op {
	case "status":
		return s.status(ctx)
	case "settings":
		return s.settings(ctx)
	case "savePlanning":
		var p savePlanningParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return s.savePlanning(ctx, p)
	case "saveCalendar":
		var p saveCalendarParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return s.saveCalendar(ctx, p)
	case "projects":
		return s.projectList(ctx)
	case "previewMigration":
		var p projectEditParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return s.previewMigration(ctx, p)
	case "saveProject":
		var p projectEditParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return s.saveProject(ctx, p)
	case "load":
		var p loadParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return s.load(ctx, p.Calendar)
	case "interpret":
		var p interpretParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return s.interpret(p)
	case "suggest":
		var p suggestParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return s.suggest(p), nil
	case "capture":
		var p interpretParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return s.capture(ctx, p)
	case "captureReview":
		var p reviewParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return nil, s.captureReview(ctx, p)
	case "captureExplicit":
		var p interpretParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return nil, s.captureExplicit(ctx, p)
	case "modify":
		var p modifyParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return s.modify(ctx, p)
	case "complete", "start", "stop", "delete", "stopRecurrence":
		var p uuidParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return nil, s.taskAction(ctx, op, p.UUID)
	case "plan":
		var p loadParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return s.plan(ctx, p.Calendar)
	case "applyPlan":
		var p applyPlanParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return s.applyPlan(ctx, p)
	case "undo":
		return nil, s.client.Undo(ctx)
	case "setToken":
		var p tokenParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return nil, s.setToken(p.Token)
	case "signInStart":
		return s.signInStart(ctx)
	case "signInPoll":
		var p signInPollParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return s.signInPoll(ctx, p.DeviceCode)
	case "revoke":
		var p tokenParams
		if err := decode(raw, &p); err != nil {
			return nil, err
		}
		return nil, s.revoke(ctx, p.Token)
	default:
		return nil, fmt.Errorf("unknown operation %q", op)
	}
}

func decode(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("invalid parameters: %w", err)
	}
	return nil
}
