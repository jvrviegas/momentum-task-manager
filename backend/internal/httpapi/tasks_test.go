package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// send makes a request with token and, unless it is empty, the idempotency
// key. A string body is sent as it is; anything else as JSON.
func (h *harness) send(method, path, token, key string, body any) (int, string) {
	h.t.Helper()
	var reader io.Reader
	switch body := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(body)
	default:
		data, err := json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequest(method, h.url+path, reader)
	if err != nil {
		h.t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if key != "" {
		request.Header.Set(api.IdempotencyKeyHeader, key)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		h.t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(data)
}

func decodeBody[T any](t *testing.T, body string) T {
	t.Helper()
	var value T
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return value
}

// wantAPIError checks a failed response's status, code, and field.
func wantAPIError(t *testing.T, status int, body string, wantStatus int, code, field string) {
	t.Helper()
	failure := decodeBody[api.Error](t, body)
	if status != wantStatus || failure.Code != code || failure.Field != field {
		t.Errorf("response = %d %s, want %d %s on %q", status, body, wantStatus, code, field)
	}
}

func TestTaskRoutesOverHTTP(t *testing.T) {
	h := newHarness(t)
	alice := h.signIn("alice@example.com").Token
	bob := h.signIn("bob@example.com").Token
	create := api.CreateTaskRequest{Description: "Secret plans", Project: "secret-project", Tags: []string{"secret-tag"}}

	status, body := h.send(http.MethodPost, "/v1/tasks", alice, "", create)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "")

	status, first := h.send(http.MethodPost, "/v1/tasks", alice, "secret-key-1", create)
	created := decodeBody[api.Task](t, first)
	if status != http.StatusCreated || created.Description != "Secret plans" || created.Status != api.StatusPending {
		t.Fatalf("create = %d %s", status, first)
	}
	if status, again := h.send(http.MethodPost, "/v1/tasks", alice, "secret-key-1", create); status != http.StatusCreated || again != first {
		t.Errorf("retried create = %d %s, want %s", status, again, first)
	}
	status, body = h.send(http.MethodPost, "/v1/tasks", alice, "secret-key-1", api.CreateTaskRequest{Description: "Other"})
	wantAPIError(t, status, body, http.StatusConflict, api.CodeConflict, "")

	status, body = h.send(http.MethodGet, "/v1/tasks?status=pending", alice, "", nil)
	if listed := decodeBody[api.TaskList](t, body); status != http.StatusOK || len(listed.Tasks) != 1 || listed.Tasks[0].UUID != created.UUID {
		t.Errorf("pending = %d %s", status, body)
	}
	status, body = h.send(http.MethodGet, "/v1/projects", alice, "", nil)
	if projects := decodeBody[api.ProjectList](t, body); status != http.StatusOK || len(projects.Projects) != 1 || projects.Projects[0] != "secret-project" {
		t.Errorf("projects = %d %s", status, body)
	}
	status, body = h.send(http.MethodGet, "/v1/tags", alice, "", nil)
	if tags := decodeBody[api.TagList](t, body); status != http.StatusOK || len(tags.Tags) != 1 || tags.Tags[0] != "secret-tag" {
		t.Errorf("tags = %d %s", status, body)
	}

	taskPath := "/v1/tasks/" + created.UUID
	modify := `{"priority":"H","project":null,"tags":{"add":["more"]}}`
	status, body = h.send(http.MethodPatch, taskPath, alice, "secret-key-2", modify)
	if modified := decodeBody[api.Task](t, body); status != http.StatusOK || modified.Priority != "H" || modified.Project != "" || len(modified.Tags) != 2 {
		t.Errorf("modify = %d %s", status, body)
	}
	if status, again := h.send(http.MethodPatch, taskPath, alice, "secret-key-2", modify); status != http.StatusOK || again != body {
		t.Errorf("retried modify = %d %s, want %s", status, again, body)
	}
	status, body = h.send(http.MethodPatch, taskPath, alice, uuid.NewString(), `{"priority":"X"}`)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "priority")
	status, body = h.send(http.MethodPatch, taskPath, alice, uuid.NewString(), `{"tags":{"add":"one"}}`)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "tags")
	status, body = h.send(http.MethodPost, "/v1/tasks", alice, uuid.NewString(), `{"description":"x","estimateMinutes":"an hour"}`)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "estimateMinutes")
	for _, malformed := range []string{`{"estimateMinutes":"an hour"}`, `{"due":"tomorrow"}`, `{"due":`} {
		status, body = h.send(http.MethodPatch, taskPath, alice, uuid.NewString(), malformed)
		wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "")
	}

	for _, step := range []struct {
		action string
		check  func(api.Task) bool
	}{
		{"start", func(task api.Task) bool { return task.Start != nil }},
		{"stop", func(task api.Task) bool { return task.Start == nil }},
		{"complete", func(task api.Task) bool { return task.Status == api.StatusCompleted && task.End != nil }},
	} {
		status, body := h.send(http.MethodPost, taskPath+"/"+step.action, alice, uuid.NewString(), nil)
		if status != http.StatusOK || !step.check(decodeBody[api.Task](t, body)) {
			t.Errorf("%s = %d %s", step.action, status, body)
		}
	}
	status, body = h.send(http.MethodPost, taskPath+"/start", alice, uuid.NewString(), nil)
	wantAPIError(t, status, body, http.StatusConflict, api.CodeConflict, "")

	status, body = h.send(http.MethodPost, "/v1/tasks", alice, uuid.NewString(), api.CreateTaskRequest{Description: "Second"})
	second := decodeBody[api.Task](t, body)
	status, body = h.send(http.MethodDelete, "/v1/tasks/"+second.UUID, alice, uuid.NewString(), nil)
	if deleted := decodeBody[api.Task](t, body); status != http.StatusOK || deleted.Status != api.StatusDeleted {
		t.Errorf("delete = %d %s", status, body)
	}

	endedAfter := url.QueryEscape(time.Now().Add(-time.Hour).Format(time.RFC3339))
	status, body = h.send(http.MethodGet, "/v1/tasks?status=completed&ended_after="+endedAfter, alice, "", nil)
	if listed := decodeBody[api.TaskList](t, body); status != http.StatusOK || len(listed.Tasks) != 1 || listed.Tasks[0].UUID != created.UUID {
		t.Errorf("completed = %d %s", status, body)
	}
	status, body = h.send(http.MethodGet, "/v1/tasks?status=pending", alice, "", nil)
	if status != http.StatusOK || body != "{\"tasks\":[]}\n" {
		t.Errorf("pending after completing = %d %s", status, body)
	}
	status, body = h.send(http.MethodGet, "/v1/tasks?status=waiting", alice, "", nil)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, api.QueryStatus)
	status, body = h.send(http.MethodGet, "/v1/tasks?status=completed&ended_after=yesterday", alice, "", nil)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, api.QueryEndedAfter)

	// Paths that name no task of the user's.
	status, body = h.send(http.MethodPatch, "/v1/tasks/not-a-uuid", alice, uuid.NewString(), `{"priority":"L"}`)
	wantAPIError(t, status, body, http.StatusNotFound, api.CodeNotFound, "")
	status, body = h.send(http.MethodPost, "/v1/tasks/"+uuid.NewString()+"/start", alice, uuid.NewString(), nil)
	wantAPIError(t, status, body, http.StatusNotFound, api.CodeNotFound, "")
	status, body = h.send(http.MethodDelete, taskPath, bob, uuid.NewString(), nil)
	wantAPIError(t, status, body, http.StatusNotFound, api.CodeNotFound, "")
	status, body = h.send(http.MethodGet, "/v1/projects", bob, "", nil)
	if status != http.StatusOK || body != "{\"projects\":[]}\n" {
		t.Errorf("Bob's projects = %d %s", status, body)
	}

	for _, route := range []string{"GET /v1/tasks?status=pending", "POST /v1/tasks", "PATCH " + taskPath, "DELETE " + taskPath,
		"POST " + taskPath + "/complete", "POST " + taskPath + "/start", "POST " + taskPath + "/stop", "GET /v1/projects", "GET /v1/tags"} {
		method, path, _ := strings.Cut(route, " ")
		status, body := h.send(method, path, "mtk_unknown", uuid.NewString(), nil)
		wantAPIError(t, status, body, http.StatusUnauthorized, api.CodeUnauthorized, "")
	}

	for _, route := range []string{"POST /v1/tasks", "PATCH " + taskPath, "DELETE " + taskPath,
		"POST " + taskPath + "/complete", "POST " + taskPath + "/start", "POST " + taskPath + "/stop"} {
		method, path, _ := strings.Cut(route, " ")
		status, body := h.send(method, path, alice, "", `{"description":"No key"}`)
		wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "")
	}

	for _, secret := range []string{"Secret plans", "secret-project", "secret-tag", "secret-key", created.UUID} {
		if strings.Contains(h.logs.String(), secret) {
			t.Errorf("log contains %q:\n%s", secret, h.logs.String())
		}
	}
}

func TestUndoOverHTTP(t *testing.T) {
	h := newHarness(t)
	laptop := h.signIn("alice@example.com").Token
	phone := h.signIn("alice@example.com").Token

	status, body := h.send(http.MethodPost, "/v1/tasks", laptop, uuid.NewString(), api.CreateTaskRequest{Description: "Secret undo"})
	created := decodeBody[api.Task](t, body)
	taskPath := "/v1/tasks/" + created.UUID
	if status, body = h.send(http.MethodPatch, taskPath, laptop, uuid.NewString(), `{"priority":"H"}`); status != http.StatusOK {
		t.Fatalf("modify = %d %s", status, body)
	}

	status, body = h.send(http.MethodPost, "/v1/undo", laptop, "", nil)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "")
	status, body = h.send(http.MethodPost, "/v1/undo", phone, uuid.NewString(), nil)
	wantAPIError(t, status, body, http.StatusConflict, api.CodeConflict, "")

	status, first := h.send(http.MethodPost, "/v1/undo", laptop, "secret-undo-key", nil)
	if undone := decodeBody[api.Task](t, first); status != http.StatusOK || undone.UUID != created.UUID || undone.Priority != "" {
		t.Errorf("undo = %d %s", status, first)
	}
	if status, again := h.send(http.MethodPost, "/v1/undo", laptop, "secret-undo-key", nil); status != http.StatusOK || again != first {
		t.Errorf("retried undo = %d %s, want %s", status, again, first)
	}
	status, body = h.send(http.MethodPost, "/v1/undo", laptop, uuid.NewString(), nil)
	wantAPIError(t, status, body, http.StatusConflict, api.CodeConflict, "")

	// Undoing a create removes the task.
	status, body = h.send(http.MethodPost, "/v1/tasks", phone, uuid.NewString(), api.CreateTaskRequest{Description: "Secret typo"})
	typo := decodeBody[api.Task](t, body)
	status, body = h.send(http.MethodPost, "/v1/undo", phone, uuid.NewString(), nil)
	if undone := decodeBody[api.Task](t, body); status != http.StatusOK || undone.UUID != typo.UUID || undone.Status != api.StatusDeleted {
		t.Errorf("undo of a create = %d %s", status, body)
	}
	status, body = h.send(http.MethodGet, "/v1/tasks?status=pending", phone, "", nil)
	if listed := decodeBody[api.TaskList](t, body); status != http.StatusOK || len(listed.Tasks) != 1 || listed.Tasks[0].UUID != created.UUID {
		t.Errorf("pending after undoing a create = %d %s", status, body)
	}

	status, body = h.send(http.MethodPost, "/v1/undo", "mtk_unknown", uuid.NewString(), nil)
	wantAPIError(t, status, body, http.StatusUnauthorized, api.CodeUnauthorized, "")

	for _, secret := range []string{"Secret undo", "Secret typo", "secret-undo-key", created.UUID, typo.UUID} {
		if strings.Contains(h.logs.String(), secret) {
			t.Errorf("log contains %q:\n%s", secret, h.logs.String())
		}
	}
}

func TestRecurrenceOverHTTP(t *testing.T) {
	h := newHarness(t)
	alice := h.signIn("alice@example.com").Token
	bob := h.signIn("bob@example.com").Token
	due := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)

	status, body := h.send(http.MethodPost, "/v1/tasks", alice, uuid.NewString(), api.CreateTaskRequest{Description: "Secret series", Due: &due, Recurrence: "weekly"})
	instance := decodeBody[api.Task](t, body)
	if status != http.StatusCreated || instance.Status != api.StatusPending || instance.Parent == "" || instance.Recurrence != "weekly" {
		t.Fatalf("create = %d %s", status, body)
	}
	status, body = h.send(http.MethodPost, "/v1/tasks", alice, uuid.NewString(), api.CreateTaskRequest{Description: "No anchor", Recurrence: "weekly"})
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "due")
	status, body = h.send(http.MethodGet, "/v1/tasks?status=recurring", alice, "", nil)
	if listed := decodeBody[api.TaskList](t, body); status != http.StatusOK || len(listed.Tasks) != 1 || listed.Tasks[0].UUID != instance.Parent {
		t.Errorf("recurring = %d %s", status, body)
	}

	stopPath := "/v1/tasks/" + instance.UUID + "/stop-recurrence"
	status, body = h.send(http.MethodPost, stopPath, alice, "", nil)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "")
	status, first := h.send(http.MethodPost, stopPath, alice, "secret-stop-key", nil)
	if stopped := decodeBody[api.Task](t, first); status != http.StatusOK || stopped.UUID != instance.Parent || stopped.Until == nil {
		t.Errorf("stop = %d %s", status, first)
	}
	if status, again := h.send(http.MethodPost, stopPath, alice, "secret-stop-key", nil); status != http.StatusOK || again != first {
		t.Errorf("retried stop = %d %s, want %s", status, again, first)
	}
	status, body = h.send(http.MethodPost, stopPath, alice, uuid.NewString(), nil)
	wantAPIError(t, status, body, http.StatusConflict, api.CodeConflict, "")
	for _, path := range []string{stopPath, "/v1/tasks/" + uuid.NewString() + "/stop-recurrence", "/v1/tasks/not-a-uuid/stop-recurrence"} {
		status, body = h.send(http.MethodPost, path, bob, uuid.NewString(), nil)
		wantAPIError(t, status, body, http.StatusNotFound, api.CodeNotFound, "")
	}
	status, body = h.send(http.MethodPost, stopPath, "mtk_unknown", uuid.NewString(), nil)
	wantAPIError(t, status, body, http.StatusUnauthorized, api.CodeUnauthorized, "")

	// The stopped series makes no instance after this one.
	if status, body = h.send(http.MethodPost, "/v1/tasks/"+instance.UUID+"/complete", alice, uuid.NewString(), nil); status != http.StatusOK {
		t.Fatalf("complete = %d %s", status, body)
	}
	status, body = h.send(http.MethodGet, "/v1/tasks?status=pending", alice, "", nil)
	if status != http.StatusOK || body != "{\"tasks\":[]}\n" {
		t.Errorf("pending after the last instance = %d %s", status, body)
	}

	for _, secret := range []string{"Secret series", "secret-stop-key", instance.UUID, instance.Parent} {
		if strings.Contains(h.logs.String(), secret) {
			t.Errorf("log contains %q:\n%s", secret, h.logs.String())
		}
	}
}
