package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

func (s *server) listTasks(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	var endedAfter time.Time
	if value := query.Get(api.QueryEndedAfter); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			writeError(w, &api.Error{Code: api.CodeInvalid, Message: "ended_after must be an RFC 3339 time", Field: api.QueryEndedAfter})
			return
		}
		endedAfter = parsed
	}
	found, err := s.tasks.List(r.Context(), query.Get(api.QueryStatus), endedAfter)
	s.respond(w, r, http.StatusOK, api.TaskList{Tasks: found}, err)
}

func (s *server) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.tasks.Projects(r.Context())
	s.respond(w, r, http.StatusOK, api.ProjectList{Projects: projects}, err)
}

func (s *server) listTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.tasks.Tags(r.Context())
	s.respond(w, r, http.StatusOK, api.TagList{Tags: tags}, err)
}

func (s *server) createTask(w http.ResponseWriter, r *http.Request) {
	var input api.CreateTaskRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	created, err := s.tasks.Create(r.Context(), r.Header.Get(api.IdempotencyKeyHeader), input)
	s.respond(w, r, http.StatusCreated, created, err)
}

func (s *server) modifyTask(w http.ResponseWriter, r *http.Request) {
	id, ok := taskID(w, r)
	if !ok {
		return
	}
	var input api.ModifyTaskRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	modified, err := s.tasks.Modify(r.Context(), r.Header.Get(api.IdempotencyKeyHeader), id, input)
	s.respond(w, r, http.StatusOK, modified, err)
}

// taskAction answers a route that applies action to the task in its path.
func (s *server) taskAction(action func(ctx context.Context, key string, id uuid.UUID) (api.Task, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := taskID(w, r)
		if !ok {
			return
		}
		changed, err := action(r.Context(), r.Header.Get(api.IdempotencyKeyHeader), id)
		s.respond(w, r, http.StatusOK, changed, err)
	}
}

func (s *server) undo(w http.ResponseWriter, r *http.Request) {
	undone, err := s.tasks.Undo(r.Context(), r.Header.Get(api.IdempotencyKeyHeader))
	s.respond(w, r, http.StatusOK, undone, err)
}

func (s *server) previewProjectMigration(w http.ResponseWriter, r *http.Request) {
	var input api.ProjectMigration
	if !decodeJSON(w, r, &input) {
		return
	}
	preview, err := s.tasks.PreviewProjectMigration(r.Context(), input)
	s.respond(w, r, http.StatusOK, preview, err)
}

func (s *server) applyProjectMigration(w http.ResponseWriter, r *http.Request) {
	var input api.ApplyProjectMigrationRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := s.tasks.ApplyProjectMigration(r.Context(), r.Header.Get(api.IdempotencyKeyHeader), input)
	s.respond(w, r, http.StatusOK, result, err)
}

func (s *server) setTimeZone(w http.ResponseWriter, r *http.Request) {
	var input api.TimeZone
	if !decodeJSON(w, r, &input) {
		return
	}
	zone, err := s.tasks.SetTimeZone(r.Context(), input.TimeZone)
	s.respond(w, r, http.StatusOK, zone, err)
}

// respond answers with body, or with err: an api.Error as it is, and any
// other error as CodeInternal.
func (s *server) respond(w http.ResponseWriter, r *http.Request, status int, body any, err error) {
	var apiErr *api.Error
	switch {
	case errors.As(err, &apiErr):
		writeError(w, apiErr)
	case err != nil:
		s.internalError(w, r, err)
	default:
		writeJSON(w, status, body)
	}
}

// taskID reads the task's UUID from the path. One that does not parse names
// no task.
func taskID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(api.PathUUID))
	if err != nil {
		writeError(w, &api.Error{Code: api.CodeNotFound, Message: "no such task"})
		return uuid.Nil, false
	}
	return id, true
}

// decodeJSON reads the request body into v, or answers CodeInvalid, naming
// the top-level field of a type error when the decoder reports it.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(v)
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &typeErr) && typeErr.Field != "":
		field, _, _ := strings.Cut(typeErr.Field, ".")
		writeError(w, &api.Error{Code: api.CodeInvalid, Message: field + " has the wrong type", Field: field})
	case err != nil:
		writeError(w, &api.Error{Code: api.CodeInvalid, Message: "the request is not valid JSON"})
	}
	return err == nil
}
