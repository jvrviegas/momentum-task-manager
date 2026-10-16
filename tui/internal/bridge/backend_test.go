package bridge

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/api"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/backend"
	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// backendServer is a Momentum server that answers each route with a fixed
// answer and records the requests it receives.
type backendServer struct {
	mu       sync.Mutex
	requests []string
	url      string
}

func newBackendServer(t *testing.T, answers map[string]func(body string) (int, string)) *backendServer {
	t.Helper()
	fake := &backendServer{}
	mux := http.NewServeMux()
	for pattern, answer := range answers {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			data, _ := io.ReadAll(r.Body)
			body := string(data)
			if r.URL.RawQuery != "" {
				body = r.URL.RawQuery
			}
			fake.mu.Lock()
			fake.requests = append(fake.requests, strings.TrimSpace(r.Pattern+" "+body))
			fake.mu.Unlock()
			status, text := answer(body)
			w.WriteHeader(status)
			io.WriteString(w, text)
		})
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	fake.url = server.URL
	return fake
}

func answer(status int, body string) func(string) (int, string) {
	return func(string) (int, string) { return status, body }
}

func newBackendBridge(url string) *Server {
	settings := config.Defaults()
	settings.Backend.URL = url
	server := NewServer(backend.New(url, "mtk_device", "Europe/Lisbon"), settings, "/tmp/config.toml")
	server.now = func() time.Time { return testNow }
	return server
}

func TestBackendModeReachesTheServer(t *testing.T) {
	const id = "6b9a5f9e-3c39-4a51-9d55-0d3c8f7f1a10"
	task := `{"uuid":"` + id + `","description":"Send invoice","status":"pending","project":"work",` +
		`"entry":"2026-10-13T09:00:00Z","modified":"2026-10-13T09:00:00Z","due":"2026-10-14T16:00:00Z","estimateMinutes":30,"urgency":9.5}`
	fake := newBackendServer(t, map[string]func(string) (int, string){
		api.RouteVersion: answer(http.StatusOK, `{"server":"v1.2.3"}`),
		api.RouteListTasks: func(query string) (int, string) {
			if strings.Contains(query, "completed") {
				return http.StatusOK, `{"tasks":[]}`
			}
			return http.StatusOK, `{"tasks":[` + task + `]}`
		},
		api.RouteListProjects: answer(http.StatusOK, `{"projects":["work"]}`),
		api.RouteSetTimeZone:  answer(http.StatusOK, `{"timeZone":"Europe/Lisbon"}`),
		api.RouteCreateTask: func(body string) (int, string) {
			if strings.Contains(body, "Refused") {
				return http.StatusBadRequest, `{"code":"invalid","message":"estimate must be 1 to 1440 minutes","field":"estimateMinutes"}`
			}
			return http.StatusCreated, task
		},
		api.RouteModifyTask:   answer(http.StatusOK, task),
		api.RouteCompleteTask: answer(http.StatusOK, task),
		api.RouteUndo:         answer(http.StatusConflict, `{"code":"conflict","message":"the task changed since; undo is not available"}`),
	})
	before := editFields{Description: "Send invoice", Project: "work", Due: "2026-10-14 17:00", Estimate: "30m"}
	after := before
	after.Priority, after.Estimate = "high", "45m"

	responses := serve(t, newBackendBridge(fake.url),
		request(t, 1, "status", nil),
		request(t, 2, "load", nil),
		request(t, 3, "captureExplicit", interpretParams{Input: "Call Ana @2026-10-20 +calls"}),
		request(t, 4, "captureExplicit", interpretParams{Input: "Refused"}),
		request(t, 5, "modify", modifyParams{UUID: id, Before: before, After: after}),
		request(t, 6, "complete", uuidParams{UUID: id}),
		request(t, 7, "undo", nil),
		request(t, 8, "sync", nil),
	)

	var status statusResult
	json.Unmarshal(responses[1].Result, &status)
	if responses[1].Error != nil || status.TaskwarriorVersion != "" || status.SyncConfigured || status.EstimateUDA != "configured" {
		t.Errorf("status = %s %+v", responses[1].Result, responses[1].Error)
	}
	var load loadResult
	json.Unmarshal(responses[2].Result, &load)
	if responses[2].Error != nil || len(load.Tasks) != 1 || load.Tasks[0].UUID != id || load.Tasks[0].Edit.Estimate != "30m" ||
		load.Context != "" || len(load.Projects) != 1 || load.Projects[0].Value != "work" {
		t.Errorf("load = %s %+v", responses[2].Result, responses[2].Error)
	}
	for _, id := range []int64{3, 5, 6} {
		if responses[id].Error != nil {
			t.Errorf("response %d: %+v", id, responses[id].Error)
		}
	}
	if got := responses[4].Error; got == nil || got.Field != "estimate" || got.Message != "estimate must be 1 to 1440 minutes" {
		t.Errorf("refused capture error = %+v", got)
	}
	if got := responses[7].Error; got == nil || got.Field != "" || got.Message != "the task changed since; undo is not available" {
		t.Errorf("undo error = %+v", got)
	}
	if string(responses[8].Result) != `{"changed":false}` {
		t.Errorf("sync = %s", responses[8].Result)
	}

	due := time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local).UTC().Format(time.RFC3339)
	want := []string{
		"GET /v1/version",
		"GET /v1/tasks status=pending",
		"GET /v1/projects",
		"GET /v1/tasks ended_after=" + url.QueryEscape(time.Date(2026, 9, 14, 0, 0, 0, 0, time.Local).UTC().Format(time.RFC3339)) + "&status=completed",
		`PUT /v1/time-zone {"timeZone":"Europe/Lisbon"}`,
		`POST /v1/tasks {"description":"Call Ana","due":"` + due + `","tags":["calls"]}`,
		`POST /v1/tasks {"description":"Refused"}`,
		`PATCH /v1/tasks/{uuid} {"priority":"H","estimateMinutes":45}`,
		"POST /v1/tasks/{uuid}/complete",
		"POST /v1/undo",
	}
	if !reflect.DeepEqual(fake.requests, want) {
		t.Errorf("requests:\n got %q\nwant %q", fake.requests, want)
	}
}

func TestBackendStatusNeedsASignedInDevice(t *testing.T) {
	fake := newBackendServer(t, map[string]func(string) (int, string){
		api.RouteVersion: answer(http.StatusUnauthorized, `{"code":"unauthorized","message":"sign in again"}`),
	})
	responses := serve(t, newBackendBridge(fake.url), request(t, 1, "status", nil))
	if got := responses[1].Error; got == nil || got.Message != "sign in again" {
		t.Errorf("status error = %+v", got)
	}
}
