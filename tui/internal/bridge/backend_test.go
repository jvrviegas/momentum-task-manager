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
	server := NewServer(func(string) Client { return backend.New(url, "mtk_device", "Europe/Lisbon") }, settings, "/tmp/config.toml")
	server.now = func() time.Time { return testNow }
	return server
}

func TestTheBridgeReachesTheServer(t *testing.T) {
	const id = "6b9a5f9e-3c39-4a51-9d55-0d3c8f7f1a10"
	task := `{"uuid":"` + id + `","description":"Send invoice","status":"pending","project":"work",` +
		`"entry":"2026-10-13T09:00:00Z","modified":"2026-10-13T09:00:00Z","due":"2026-10-14T16:00:00Z","estimateMinutes":30,"urgency":9.5}`
	fake := newBackendServer(t, map[string]func(string) (int, string){
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
	)

	var status statusResult
	json.Unmarshal(responses[1].Result, &status)
	if responses[1].Error != nil || status.MutationDelaySeconds != 15 || status.RefreshIntervalSeconds != 60 || status.BackendURL != fake.url {
		t.Errorf("status = %s %+v", responses[1].Result, responses[1].Error)
	}
	var load loadResult
	json.Unmarshal(responses[2].Result, &load)
	if responses[2].Error != nil || len(load.Tasks) != 1 || load.Tasks[0].UUID != id || load.Tasks[0].Edit.Estimate != "30m" ||
		len(load.Projects) != 1 || load.Projects[0].Value != "work" {
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

	due := time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local).UTC().Format(time.RFC3339)
	want := []string{
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

// newSigningInBridge returns a bridge whose server starts
// sign-ins with device code "dc", answers each poll with the next of polls,
// lists tasks only for a token, and revokes any token but "mtk_old", which it
// no longer accepts. It records each request's route, body, and token.
func newSigningInBridge(t *testing.T, polls ...string) (*Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var requests []string
	refuse := func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"code":"unauthorized","message":"sign in again"}`)
	}
	mux := http.NewServeMux()
	handle := func(pattern string, answer func(w http.ResponseWriter, auth string)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			auth := r.Header.Get("Authorization")
			mu.Lock()
			requests = append(requests, strings.TrimSpace(r.Pattern+" "+string(body)+" "+auth))
			mu.Unlock()
			answer(w, auth)
		})
	}
	handle(api.RouteStartDeviceSignIn, func(w http.ResponseWriter, _ string) {
		io.WriteString(w, `{"deviceCode":"dc","userCode":"BCDFGHJK","verificationUrl":"https://m.example/device",`+
			`"verificationUrlComplete":"https://m.example/device?code=BCDFGHJK","expiresAt":"2026-10-16T12:10:00.5+01:00","intervalSeconds":5}`)
	})
	handle(api.RoutePollDeviceSignIn, func(w http.ResponseWriter, _ string) {
		answer := polls[0]
		polls = polls[1:]
		if !strings.Contains(answer, `"token"`) {
			w.WriteHeader(http.StatusBadRequest)
		}
		io.WriteString(w, answer)
	})
	handle(api.RouteListTasks, func(w http.ResponseWriter, auth string) {
		if auth == "" {
			refuse(w)
			return
		}
		io.WriteString(w, `{"tasks":[]}`)
	})
	handle(api.RouteListProjects, func(w http.ResponseWriter, _ string) { io.WriteString(w, `{"projects":[]}`) })
	handle(api.RouteSignOut, func(w http.ResponseWriter, auth string) {
		if auth == "Bearer mtk_old" {
			refuse(w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	fake := httptest.NewServer(mux)
	t.Cleanup(fake.Close)

	settings := config.Defaults()
	settings.Backend.URL = fake.URL
	server := NewServer(func(token string) Client { return backend.New(fake.URL, token, "") }, settings, "/tmp/config.toml")
	server.now = func() time.Time { return testNow }
	return server, &requests
}

func pollError(code string) string { return `{"code":"` + code + `","message":"from the server"}` }

func TestBackendSignInThroughTheBridge(t *testing.T) {
	server, requests := newSigningInBridge(t, pollError(api.CodeAuthorizationPending), pollError(api.CodeSlowDown),
		`{"token":"mtk_new","expiresAt":"2027-01-14T12:00:00+00:00","email":"ana@example.com"}`)
	responses := serve(t, server,
		request(t, 1, "status", nil),
		request(t, 2, "load", nil),
		request(t, 3, "signInStart", nil),
		request(t, 4, "signInPoll", signInPollParams{DeviceCode: "dc"}),
		request(t, 5, "signInPoll", signInPollParams{DeviceCode: "dc"}),
		request(t, 6, "signInPoll", signInPollParams{DeviceCode: "dc"}),
		request(t, 7, "setToken", tokenParams{Token: "mtk_new"}),
		request(t, 8, "load", nil),
		request(t, 9, "revoke", tokenParams{Token: "mtk_old"}),
		request(t, 10, "revoke", tokenParams{Token: "mtk_new"}),
		request(t, 11, "setToken", tokenParams{}),
		request(t, 12, "load", nil),
	)

	var status statusResult
	json.Unmarshal(responses[1].Result, &status)
	if status.BackendURL == "" {
		t.Errorf("status = %s %+v", responses[1].Result, responses[1].Error)
	}
	for _, id := range []int64{2, 12} {
		if got := responses[id].Error; got == nil || got.Code != CodeUnauthorized || got.Message != "sign in again" {
			t.Errorf("load %d without a token: %+v", id, got)
		}
	}
	var start signInStartResult
	json.Unmarshal(responses[3].Result, &start)
	wantStart := signInStartResult{DeviceCode: "dc", UserCode: "BCDFGHJK", VerificationURL: "https://m.example/device",
		VerificationURLComplete: "https://m.example/device?code=BCDFGHJK", ExpiresAt: "2026-10-16T11:10:00Z", IntervalSeconds: 5}
	if start != wantStart {
		t.Errorf("start = %+v", start)
	}
	for id, want := range map[int64]string{4: `{"state":"pending"}`, 5: `{"state":"slowDown"}`,
		6: `{"state":"signedIn","token":"mtk_new","email":"ana@example.com","expiresAt":"2027-01-14T12:00:00Z"}`} {
		if string(responses[id].Result) != want || responses[id].Error != nil {
			t.Errorf("poll %d = %s %+v; want %s", id, responses[id].Result, responses[id].Error, want)
		}
	}
	for _, id := range []int64{7, 8, 9, 10, 11} {
		if responses[id].Error != nil {
			t.Errorf("response %d: %+v", id, responses[id].Error)
		}
	}

	want := []string{
		"GET /v1/tasks",
		api.RouteStartDeviceSignIn,
		`POST /v1/auth/token {"deviceCode":"dc"}`,
		`POST /v1/auth/token {"deviceCode":"dc"}`,
		`POST /v1/auth/token {"deviceCode":"dc"}`,
		"GET /v1/tasks  Bearer mtk_new",
		"GET /v1/projects  Bearer mtk_new",
		"GET /v1/tasks  Bearer mtk_new",
		"POST /v1/auth/sign-out  Bearer mtk_old",
		"POST /v1/auth/sign-out  Bearer mtk_new",
		"GET /v1/tasks",
	}
	if !reflect.DeepEqual(*requests, want) {
		t.Errorf("requests:\n got %q\nwant %q", *requests, want)
	}
}

func TestBackendSignInFailures(t *testing.T) {
	server, _ := newSigningInBridge(t, pollError(api.CodeAccessDenied), pollError(api.CodeExpiredToken), pollError(api.CodeInvalid))
	responses := serve(t, server,
		request(t, 1, "signInPoll", signInPollParams{DeviceCode: "dc"}),
		request(t, 2, "signInPoll", signInPollParams{DeviceCode: "dc"}),
		request(t, 3, "signInPoll", signInPollParams{DeviceCode: "dc"}),
	)
	for id, want := range map[int64]string{
		1: "sign-in refused: from the server",
		2: "the code expired before sign-in finished; sign in again",
		3: "from the server",
	} {
		if got := responses[id].Error; got == nil || got.Message != want || got.Code != "" || got.Field != "" {
			t.Errorf("poll %d error = %+v; want %q", id, got, want)
		}
	}
}

func TestBackendErrorsSayTheServerIsUnreachable(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	fake := newBackendServer(t, map[string]func(string) (int, string){
		api.RouteSetTimeZone:  answer(http.StatusOK, `{"timeZone":"Europe/Lisbon"}`),
		api.RouteCompleteTask: answer(http.StatusConflict, `{"code":"conflict","message":"only pending tasks can change"}`),
	})
	responses := serve(t, newBackendBridge(closed.URL), request(t, 1, "load", nil))
	if got := responses[1].Error; got == nil || got.Code != CodeUnreachable || !strings.HasPrefix(got.Message, api.ErrUnreachable.Error()) {
		t.Errorf("load from a stopped server: %+v", got)
	}
	responses = serve(t, newBackendBridge(fake.url), request(t, 1, "complete", uuidParams{UUID: "6b9a5f9e-3c39-4a51-9d55-0d3c8f7f1a10"}))
	if got := responses[1].Error; got == nil || got.Code != "" || got.Message != "only pending tasks can change" {
		t.Errorf("a refused change: %+v", got)
	}
}

func TestServerOperationsNeedABackendURL(t *testing.T) {
	server := NewServer(func(string) Client { return &fakeClient{} }, config.Defaults(), "/tmp/config.toml")
	responses := serve(t, server,
		request(t, 1, "status", nil),
		request(t, 2, "interpret", interpretParams{Input: "Call Ana tomorrow"}),
		request(t, 3, "load", nil),
		request(t, 4, "complete", uuidParams{UUID: "a"}),
		request(t, 5, "setToken", tokenParams{Token: "mtk_new"}),
		request(t, 6, "signInStart", nil),
		request(t, 7, "signInPoll", signInPollParams{DeviceCode: "dc"}),
		request(t, 8, "revoke", tokenParams{Token: "mtk_new"}),
	)
	var status statusResult
	json.Unmarshal(responses[1].Result, &status)
	if responses[1].Error != nil || status.BackendURL != "" || status.ConfigPath != "/tmp/config.toml" {
		t.Errorf("status = %s %+v", responses[1].Result, responses[1].Error)
	}
	if responses[2].Error != nil {
		t.Errorf("interpret: %+v", responses[2].Error)
	}
	for id := int64(3); id <= 8; id++ {
		if got := responses[id].Error; got == nil || got.Message != "Momentum needs a server: add a [backend] url to /tmp/config.toml, then sign in" {
			t.Errorf("response %d: %+v", id, got)
		}
	}
}
