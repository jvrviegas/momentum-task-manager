package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
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

func TestProjectMigrationOverHTTP(t *testing.T) {
	h := newHarness(t)
	alice := h.signIn("alice@example.com").Token
	bob := h.signIn("bob@example.com").Token
	var created []api.Task
	for _, project := range []string{"secret-project", "secret-project.child"} {
		status, body := h.send(http.MethodPost, "/v1/tasks", alice, uuid.NewString(), api.CreateTaskRequest{Description: "Secret plans", Project: project})
		if status != http.StatusCreated {
			t.Fatalf("create = %d %s", status, body)
		}
		created = append(created, decodeBody[api.Task](t, body))
	}
	migration := api.ProjectMigration{Source: "secret-project", Destination: "secret-destination", IncludeSubprojects: true}

	// A preview changes nothing, so it needs no key.
	status, body := h.send(http.MethodPost, "/v1/project-migrations/preview", alice, "", migration)
	// Both tasks were entered in one second, so either can come first.
	preview := decodeBody[api.ProjectMigrationPreview](t, body)
	if status != http.StatusOK || len(preview.Tasks) != 2 || !slices.Contains(preview.Tasks, api.ProjectTaskMapping{
		UUID: created[1].UUID, OldValue: "secret-project.child", NewValue: "secret-destination.child",
	}) {
		t.Fatalf("preview = %d %s", status, body)
	}
	status, body = h.send(http.MethodPost, "/v1/project-migrations/preview", alice, "", `{"source":"","destination":"secret-destination"}`)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "source")
	status, body = h.send(http.MethodPost, "/v1/project-migrations/preview", alice, "", `{"source":`)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "")
	status, body = h.send(http.MethodPost, "/v1/project-migrations/preview", alice, "", `{"includeSubprojects":"yes"}`)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "includeSubprojects")

	apply := api.ApplyProjectMigrationRequest{ProjectMigration: migration, Tasks: preview.Tasks}
	status, body = h.send(http.MethodPost, "/v1/project-migrations/apply", bob, uuid.NewString(), apply)
	wantAPIError(t, status, body, http.StatusConflict, api.CodeConflict, "")
	status, body = h.send(http.MethodPost, "/v1/project-migrations/apply", alice, "", apply)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "")
	status, first := h.send(http.MethodPost, "/v1/project-migrations/apply", alice, "secret-migration-key", apply)
	if status != http.StatusOK || first != "{\"changed\":2}\n" {
		t.Fatalf("apply = %d %s", status, first)
	}
	if status, again := h.send(http.MethodPost, "/v1/project-migrations/apply", alice, "secret-migration-key", apply); status != http.StatusOK || again != first {
		t.Errorf("retried apply = %d %s, want %s", status, again, first)
	}
	status, body = h.send(http.MethodPost, "/v1/project-migrations/apply", alice, uuid.NewString(), apply)
	wantAPIError(t, status, body, http.StatusConflict, api.CodeConflict, "")
	status, body = h.send(http.MethodPost, "/v1/undo", alice, uuid.NewString(), nil)
	wantAPIError(t, status, body, http.StatusConflict, api.CodeConflict, "")

	status, body = h.send(http.MethodGet, "/v1/projects", alice, "", nil)
	if status != http.StatusOK || body != "{\"projects\":[\"secret-destination\",\"secret-destination.child\"]}\n" {
		t.Errorf("projects = %d %s", status, body)
	}
	status, body = h.send(http.MethodPost, "/v1/project-migrations/preview", bob, "", api.ProjectMigration{Source: "secret-destination", Destination: "x"})
	if status != http.StatusOK || body != "{\"tasks\":[]}\n" {
		t.Errorf("Bob's preview = %d %s", status, body)
	}
	for _, path := range []string{"/v1/project-migrations/preview", "/v1/project-migrations/apply"} {
		status, body = h.send(http.MethodPost, path, "mtk_unknown", uuid.NewString(), apply)
		wantAPIError(t, status, body, http.StatusUnauthorized, api.CodeUnauthorized, "")
	}

	for _, secret := range []string{"Secret plans", "secret-project", "secret-destination", "secret-migration-key", created[0].UUID, created[1].UUID} {
		if strings.Contains(h.logs.String(), secret) {
			t.Errorf("log contains %q:\n%s", secret, h.logs.String())
		}
	}
}

func TestVersionAndTimeZoneOverHTTP(t *testing.T) {
	h := newHarness(t)
	alice := h.signIn("alice@example.com").Token

	status, body := h.send(http.MethodGet, "/v1/version", alice, "", nil)
	if status != http.StatusOK || decodeBody[api.Version](t, body) != (api.Version{Server: "v1.2.3"}) {
		t.Errorf("version = %d %s", status, body)
	}

	status, body = h.send(http.MethodPut, "/v1/time-zone", alice, "", api.TimeZone{TimeZone: "Pacific/Auckland"})
	if status != http.StatusOK || decodeBody[api.TimeZone](t, body) != (api.TimeZone{TimeZone: "Pacific/Auckland"}) {
		t.Errorf("set time zone = %d %s", status, body)
	}
	if zone := h.client.User.Query().OnlyX(t.Context()).TimeZone; zone != "Pacific/Auckland" {
		t.Errorf("stored zone = %q", zone)
	}
	status, body = h.send(http.MethodPut, "/v1/time-zone", alice, "", api.TimeZone{TimeZone: "Mars/Olympus_Mons"})
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "timeZone")
	status, body = h.send(http.MethodPut, "/v1/time-zone", alice, "", `{"timeZone":7}`)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "timeZone")

	for _, route := range []struct{ method, path string }{{http.MethodGet, "/v1/version"}, {http.MethodPut, "/v1/time-zone"}} {
		status, body := h.send(route.method, route.path, "mtk_unknown", "", api.TimeZone{TimeZone: "UTC"})
		wantAPIError(t, status, body, http.StatusUnauthorized, api.CodeUnauthorized, "")
	}
	if strings.Contains(h.logs.String(), "Auckland") {
		t.Errorf("log holds the time zone:\n%s", h.logs.String())
	}
}

func TestImportOverHTTP(t *testing.T) {
	h := newHarness(t)
	alice := h.signIn("alice@example.com").Token
	bob := h.signIn("bob@example.com").Token
	entry := time.Date(2026, 7, 3, 8, 0, 0, 0, time.UTC)
	id := uuid.NewString()
	request := api.ImportRequest{Tasks: []api.Task{{UUID: id, Description: "Secret plans", Status: api.StatusCompleted,
		Project: "secret-project", Entry: entry, Modified: entry, End: &entry,
		Extra: map[string]json.RawMessage{"secret-field": json.RawMessage(`"secret-value"`)}}}}

	// A preview changes nothing, so it needs no key.
	status, body := h.send(http.MethodPost, "/v1/import/preview", alice, "", request)
	if status != http.StatusOK || body != `{"created":["`+id+`"],"unchanged":[],"different":[]}`+"\n" {
		t.Fatalf("preview = %d %s", status, body)
	}
	status, body = h.send(http.MethodPost, "/v1/import/apply", alice, "", request)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "")
	status, first := h.send(http.MethodPost, "/v1/import/apply", alice, "secret-import-key", request)
	if status != http.StatusOK || first != `{"created":["`+id+`"],"unchanged":[],"different":[]}`+"\n" {
		t.Fatalf("apply = %d %s", status, first)
	}
	if status, again := h.send(http.MethodPost, "/v1/import/apply", alice, "secret-import-key", request); status != http.StatusOK || again != first {
		t.Errorf("retried apply = %d %s, want %s", status, again, first)
	}
	status, body = h.send(http.MethodPost, "/v1/import/apply", alice, uuid.NewString(), request)
	if status != http.StatusOK || body != `{"created":[],"unchanged":["`+id+`"],"different":[]}`+"\n" {
		t.Errorf("second import = %d %s", status, body)
	}
	status, body = h.send(http.MethodGet, "/v1/tasks?status=completed&ended_after=2026-07-01T00:00:00Z", alice, "", nil)
	if list := decodeBody[api.TaskList](t, body); status != http.StatusOK || len(list.Tasks) != 1 || list.Tasks[0].UUID != id ||
		string(list.Tasks[0].Extra["secret-field"]) != `"secret-value"` {
		t.Errorf("completed list = %d %s", status, body)
	}
	status, body = h.send(http.MethodPost, "/v1/undo", alice, uuid.NewString(), nil)
	wantAPIError(t, status, body, http.StatusConflict, api.CodeConflict, "")

	status, body = h.send(http.MethodPost, "/v1/import/preview", alice, "", `{"tasks":[]}`)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "tasks")
	status, body = h.send(http.MethodPost, "/v1/import/preview", alice, "", `{"tasks":`)
	wantAPIError(t, status, body, http.StatusBadRequest, api.CodeInvalid, "")
	status, body = h.send(http.MethodPost, "/v1/import/apply", bob, uuid.NewString(), request)
	wantAPIError(t, status, body, http.StatusConflict, api.CodeConflict, "")
	for _, path := range []string{"/v1/import/preview", "/v1/import/apply"} {
		status, body = h.send(http.MethodPost, path, "mtk_unknown", uuid.NewString(), request)
		wantAPIError(t, status, body, http.StatusUnauthorized, api.CodeUnauthorized, "")
	}

	for _, secret := range []string{"Secret plans", "secret-project", "secret-field", "secret-value", "secret-import-key", id} {
		if strings.Contains(h.logs.String(), secret) {
			t.Errorf("log contains %q:\n%s", secret, h.logs.String())
		}
	}
}
