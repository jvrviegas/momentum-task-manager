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
	"github.com/jvrviegas/momentum-task-manager/tui/internal/taskwarrior"
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

// Error is a user-facing failure. Field names the edit field that caused it.
type Error struct {
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func fieldError(field, message string) *Error { return &Error{Message: message, Field: field} }

// Client is the Taskwarrior surface the bridge needs; *taskwarrior.CommandClient
// implements it.
type Client interface {
	taskwarrior.Client
	Version(context.Context) (string, error)
	ExportCompleted(context.Context, time.Time) ([]domain.Task, error)
	Context(context.Context) (string, error)
	StopRecurrence(context.Context, string) error
	SyncConfigured(context.Context) (bool, error)
	EstimateUDAReadiness(context.Context) (taskwarrior.EstimateUDAState, error)
}

// apiFields names the edit fields that a backend error's fields belong to.
var apiFields = map[string]string{
	"description": "description", "project": "project", "priority": "priority", "due": "due",
	"scheduled": "scheduled", "recurrence": "recurrence", "estimateMinutes": "estimate", "tags": "tags",
}

// Pure operations never call Taskwarrior and may answer out of order.
var pureOps = map[string]bool{"interpret": true, "suggest": true}

const maxReviews = 8

type Server struct {
	client     Client
	config     config.Config
	configPath string
	now        func() time.Time

	mu         sync.Mutex
	projects   domain.ProjectCatalog
	tags       []string
	reviews    map[int64]quickadd.Interpretation
	nextReview int64
}

func NewServer(client Client, settings config.Config, configPath string) *Server {
	return &Server{
		client:     client,
		config:     settings,
		configPath: configPath,
		now:        time.Now,
		projects:   settings.Projects,
		reviews:    map[int64]quickadd.Interpretation{},
	}
}

// Serve answers requests until r is exhausted. Taskwarrior operations run one
// at a time in arrival order; pure operations answer immediately so capture
// highlighting never waits behind a sync.
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
		default:
			bridgeErr = &Error{Message: err.Error()}
		}
		return Response{ID: request.ID, Error: bridgeErr}
	}
	if result == nil {
		result = struct{}{}
	}
	return Response{ID: request.ID, Result: result}
}

func (s *Server) dispatch(ctx context.Context, op string, raw json.RawMessage) (any, error) {
	switch op {
	case "status":
		return s.status(ctx)
	case "load":
		return s.load(ctx)
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
	case "undo":
		return nil, s.client.Undo(ctx)
	case "sync":
		result, err := s.client.Sync(ctx)
		if err != nil {
			return nil, err
		}
		return syncResult{Changed: result.Changed}, nil
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
