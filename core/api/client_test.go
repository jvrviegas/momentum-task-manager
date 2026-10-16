package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jvrviegas/momentum-task-manager/core/domain"
)

// received is what a test server saw of one request.
type received struct {
	pattern, path, query, body, auth, contentType, key string
}

// recordingServer answers every route in core/api with answer and records
// each request it receives.
func recordingServer(t *testing.T, answer func(w http.ResponseWriter, r *http.Request)) (*Client, func() []received) {
	t.Helper()
	var mu sync.Mutex
	var requests []received
	mux := http.NewServeMux()
	for _, route := range []string{
		RouteVersion, RouteListTasks, RouteCreateTask, RouteModifyTask, RouteDeleteTask, RouteCompleteTask,
		RouteStartTask, RouteStopTask, RouteStopRecurrence, RouteUndo, RouteListProjects, RouteListTags,
		RoutePreviewProjectMigration, RouteApplyProjectMigration, RouteSetTimeZone, RouteSession,
		RouteStartDeviceSignIn, RoutePollDeviceSignIn, RouteSignOut,
	} {
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			requests = append(requests, received{
				pattern: r.Pattern, path: r.URL.EscapedPath(), query: r.URL.RawQuery, body: string(body),
				auth: r.Header.Get("Authorization"), contentType: r.Header.Get("Content-Type"), key: r.Header.Get(IdempotencyKeyHeader),
			})
			mu.Unlock()
			answer(w, r)
		})
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	client := NewClient(server.URL+"/", "mtk_device")
	client.retryDelay = time.Millisecond
	return client, func() []received {
		mu.Lock()
		defer mu.Unlock()
		return append([]received(nil), requests...)
	}
}

func answerJSON(status int, body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}
}

func TestClientSendsEachRoute(t *testing.T) {
	ctx := t.Context()
	const id = "6b9a5f9e-3c39-4a51-9d55-0d3c8f7f1a10"
	task := `{"uuid":"` + id + `","description":"Write the plan","status":"pending","entry":"2026-10-14T08:30:00Z","modified":"2026-10-14T09:30:00Z","urgency":1.5}`
	wantTask := Task{UUID: id, Description: "Write the plan", Status: StatusPending, Entry: at(8), Modified: at(9), Urgency: 1.5}
	migration := ProjectMigration{Source: "work", Destination: "job"}
	mappings := []ProjectTaskMapping{{UUID: id, OldValue: "work", NewValue: "job"}}
	ended := time.Date(2026, 10, 14, 0, 0, 0, 0, time.FixedZone("WEST", 3600))

	for _, test := range []struct {
		name, answer string
		send         func(*Client) (any, error)
		want         any
		pattern      string
		path, query  string
		body         string
		mutation     bool
	}{
		{name: "version", answer: `{"server":"v1.2.3"}`, send: func(c *Client) (any, error) { return c.Version(ctx) },
			want: Version{Server: "v1.2.3"}, pattern: RouteVersion, path: "/v1/version"},
		{name: "session", answer: `{"email":"alice@example.com","expiresAt":"2026-10-14T09:30:00Z"}`, send: func(c *Client) (any, error) { return c.Session(ctx) },
			want: Session{Email: "alice@example.com", ExpiresAt: at(9)}, pattern: RouteSession, path: "/v1/auth/session"},
		{name: "start sign-in", answer: `{"deviceCode":"dc","userCode":"BCDFGHJK","verificationUrl":"https://m.example/device","verificationUrlComplete":"https://m.example/device?code=BCDFGHJK","expiresAt":"2026-10-14T09:30:00Z","intervalSeconds":5}`,
			send: func(c *Client) (any, error) { return c.StartDeviceSignIn(ctx) },
			want: DeviceSignIn{DeviceCode: "dc", UserCode: "BCDFGHJK", VerificationURL: "https://m.example/device",
				VerificationURLComplete: "https://m.example/device?code=BCDFGHJK", ExpiresAt: at(9), IntervalSeconds: 5},
			pattern: RouteStartDeviceSignIn, path: "/v1/auth/device"},
		{name: "poll sign-in", answer: `{"token":"mtk_new","expiresAt":"2026-10-14T09:30:00Z","email":"alice@example.com"}`,
			send:    func(c *Client) (any, error) { return c.PollDeviceSignIn(ctx, "dc") },
			want:    Token{Token: "mtk_new", ExpiresAt: at(9), Email: "alice@example.com"},
			pattern: RoutePollDeviceSignIn, path: "/v1/auth/token", body: `{"deviceCode":"dc"}`},
		{name: "time zone", answer: `{"timeZone":"Europe/Lisbon"}`, send: func(c *Client) (any, error) { return c.SetTimeZone(ctx, "Europe/Lisbon") },
			want: TimeZone{TimeZone: "Europe/Lisbon"}, pattern: RouteSetTimeZone, path: "/v1/time-zone", body: `{"timeZone":"Europe/Lisbon"}`},
		{name: "pending", answer: `{"tasks":[` + task + `]}`, send: func(c *Client) (any, error) { return c.ListTasks(ctx, StatusPending, ended) },
			want: []Task{wantTask}, pattern: RouteListTasks, path: "/v1/tasks", query: "status=pending"},
		{name: "completed", answer: `{"tasks":[]}`, send: func(c *Client) (any, error) { return c.ListTasks(ctx, StatusCompleted, ended) },
			want: []Task{}, pattern: RouteListTasks, path: "/v1/tasks", query: "ended_after=2026-10-13T23%3A00%3A00Z&status=completed"},
		{name: "create", answer: task, send: func(c *Client) (any, error) {
			return c.CreateTask(ctx, CreateTaskRequest{Description: "Write the plan", Tags: []string{"next"}})
		}, want: wantTask, pattern: RouteCreateTask, path: "/v1/tasks", body: `{"description":"Write the plan","tags":["next"]}`, mutation: true},
		{name: "modify", answer: task, send: func(c *Client) (any, error) {
			return c.ModifyTask(ctx, id, ModifyTaskRequest{Project: Change[string]{Kind: domain.Clear}})
		}, want: wantTask, pattern: RouteModifyTask, path: "/v1/tasks/" + id, body: `{"project":null}`, mutation: true},
		{name: "delete", answer: task, send: func(c *Client) (any, error) { return c.DeleteTask(ctx, id) },
			want: wantTask, pattern: RouteDeleteTask, path: "/v1/tasks/" + id, mutation: true},
		{name: "complete", answer: task, send: func(c *Client) (any, error) { return c.CompleteTask(ctx, id) },
			want: wantTask, pattern: RouteCompleteTask, path: "/v1/tasks/" + id + "/complete", mutation: true},
		{name: "start", answer: task, send: func(c *Client) (any, error) { return c.StartTask(ctx, id) },
			want: wantTask, pattern: RouteStartTask, path: "/v1/tasks/" + id + "/start", mutation: true},
		{name: "stop", answer: task, send: func(c *Client) (any, error) { return c.StopTask(ctx, id) },
			want: wantTask, pattern: RouteStopTask, path: "/v1/tasks/" + id + "/stop", mutation: true},
		{name: "stop recurrence", answer: task, send: func(c *Client) (any, error) { return c.StopRecurrence(ctx, id) },
			want: wantTask, pattern: RouteStopRecurrence, path: "/v1/tasks/" + id + "/stop-recurrence", mutation: true},
		{name: "undo", answer: task, send: func(c *Client) (any, error) { return c.Undo(ctx) },
			want: wantTask, pattern: RouteUndo, path: "/v1/undo", mutation: true},
		{name: "projects", answer: `{"projects":["home","work"]}`, send: func(c *Client) (any, error) { return c.Projects(ctx) },
			want: []string{"home", "work"}, pattern: RouteListProjects, path: "/v1/projects"},
		{name: "tags", answer: `{"tags":["next"]}`, send: func(c *Client) (any, error) { return c.Tags(ctx) },
			want: []string{"next"}, pattern: RouteListTags, path: "/v1/tags"},
		{name: "preview", answer: `{"tasks":[{"uuid":"` + id + `","oldValue":"work","newValue":"job"}]}`, send: func(c *Client) (any, error) {
			return c.PreviewProjectMigration(ctx, migration)
		}, want: ProjectMigrationPreview{Tasks: mappings}, pattern: RoutePreviewProjectMigration, path: "/v1/project-migrations/preview",
			body: `{"source":"work","destination":"job","includeSubprojects":false}`},
		{name: "apply", answer: `{"changed":1}`, send: func(c *Client) (any, error) {
			return c.ApplyProjectMigration(ctx, ApplyProjectMigrationRequest{ProjectMigration: migration, Tasks: mappings})
		}, want: ProjectMigrationResult{Changed: 1}, pattern: RouteApplyProjectMigration, path: "/v1/project-migrations/apply",
			body: `{"source":"work","destination":"job","includeSubprojects":false,"tasks":[{"uuid":"` + id + `","oldValue":"work","newValue":"job"}]}`, mutation: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, requests := recordingServer(t, answerJSON(http.StatusOK, test.answer))
			got, err := test.send(client)
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("answer = %#v, %v; want %#v", got, err, test.want)
			}
			seen := requests()
			if len(seen) != 1 {
				t.Fatalf("%d requests, want 1", len(seen))
			}
			request := seen[0]
			if request.pattern != test.pattern || request.path != test.path || request.query != test.query || request.body != test.body {
				t.Errorf("request = %+v, want %s %s?%s with %s", request, test.pattern, test.path, test.query, test.body)
			}
			if request.auth != "Bearer mtk_device" {
				t.Errorf("Authorization = %q", request.auth)
			}
			if wantType := map[bool]string{true: "application/json"}[test.body != ""]; request.contentType != wantType {
				t.Errorf("Content-Type = %q, want %q", request.contentType, wantType)
			}
			if (request.key != "") != test.mutation {
				t.Errorf("Idempotency-Key = %q, mutation %v", request.key, test.mutation)
			}
		})
	}
}

func TestClientWithoutATokenSendsNoAuthorization(t *testing.T) {
	client, requests := recordingServer(t, answerJSON(http.StatusOK, `{"server":"v1.2.3"}`))
	client.token = ""
	if _, err := client.Version(t.Context()); err != nil || requests()[0].auth != "" {
		t.Errorf("error %v, Authorization %q", err, requests()[0].auth)
	}
}

func TestClientSignsOutWithItsToken(t *testing.T) {
	client, requests := recordingServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	if err := client.SignOut(t.Context()); err != nil {
		t.Fatal(err)
	}
	if seen := requests(); len(seen) != 1 || seen[0].pattern != RouteSignOut || seen[0].auth != "Bearer mtk_device" || seen[0].body != "" || seen[0].key != "" {
		t.Errorf("requests = %+v", seen)
	}
}

func TestClientUsesANewKeyPerMutation(t *testing.T) {
	client, requests := recordingServer(t, answerJSON(http.StatusOK, `{"uuid":"u"}`))
	for range 2 {
		if _, err := client.CompleteTask(t.Context(), "u"); err != nil {
			t.Fatal(err)
		}
	}
	if seen := requests(); seen[0].key == seen[1].key || len(seen[0].key) < 16 {
		t.Errorf("keys %q and %q, want two different random keys", seen[0].key, seen[1].key)
	}
}

func TestClientRetriesWithTheSameKey(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	client, requests := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		attempt := attempts
		mu.Unlock()
		switch attempt {
		case 1:
			answerJSON(http.StatusServiceUnavailable, "Service Unavailable")(w, r)
		case 2:
			<-r.Context().Done() // the attempt times out
		default:
			answerJSON(http.StatusCreated, `{"uuid":"u","description":"Call Ana"}`)(w, r)
		}
	})
	client.attemptTimeout = 100 * time.Millisecond
	created, err := client.CreateTask(t.Context(), CreateTaskRequest{Description: "Call Ana"})
	if err != nil || created.UUID != "u" {
		t.Fatalf("create = %+v, %v", created, err)
	}
	seen := requests()
	if len(seen) != 3 {
		t.Fatalf("%d attempts, want 3", len(seen))
	}
	for _, request := range seen[1:] {
		if request.key != seen[0].key || request.body != seen[0].body {
			t.Errorf("retry %+v differs from the first %+v", request, seen[0])
		}
	}
}

func TestClientReportsAServerThatGivesNoAnswer(t *testing.T) {
	for _, test := range []struct {
		name   string
		answer func(http.ResponseWriter, *http.Request)
		want   string
	}{
		{name: "dropped connections", answer: func(w http.ResponseWriter, _ *http.Request) {
			connection, _, _ := w.(http.Hijacker).Hijack()
			connection.Close()
		}, want: "cannot reach the Momentum server: Get "},
		{name: "a gateway", answer: answerJSON(http.StatusBadGateway, "<html>Bad Gateway</html>"),
			want: "cannot reach the Momentum server: the server answered 502 Bad Gateway"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, requests := recordingServer(t, test.answer)
			_, err := client.Projects(t.Context())
			if !errors.Is(err, ErrUnreachable) || !strings.HasPrefix(err.Error(), test.want) {
				t.Errorf("error = %v, want ErrUnreachable starting %q", err, test.want)
			}
			if len(requests()) != 3 {
				t.Errorf("%d attempts, want 3", len(requests()))
			}
		})
	}

	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	client := NewClient(closed.URL, "mtk_device")
	client.retryDelay = time.Millisecond
	if _, err := client.Projects(t.Context()); !errors.Is(err, ErrUnreachable) {
		t.Errorf("closed server: error = %v, want ErrUnreachable", err)
	}
}

func TestClientReturnsTheServersErrorsWithoutRetrying(t *testing.T) {
	for _, test := range []struct {
		status int
		body   string
		want   Error
	}{
		{http.StatusBadRequest, `{"code":"invalid","message":"estimate must be 1 to 1440 minutes","field":"estimateMinutes"}`,
			Error{Code: CodeInvalid, Message: "estimate must be 1 to 1440 minutes", Field: "estimateMinutes"}},
		{http.StatusUnauthorized, `{"code":"unauthorized","message":"sign in again"}`, Error{Code: CodeUnauthorized, Message: "sign in again"}},
		{http.StatusConflict, `{"code":"conflict","message":"the task changed since; undo is not available"}`,
			Error{Code: CodeConflict, Message: "the task changed since; undo is not available"}},
		{http.StatusInternalServerError, `{"code":"internal","message":"the server failed; try again"}`,
			Error{Code: CodeInternal, Message: "the server failed; try again"}},
		{http.StatusNotFound, `<html>Not Found</html>`, Error{Code: CodeInternal, Message: "the server answered 404 Not Found"}},
	} {
		client, requests := recordingServer(t, answerJSON(test.status, test.body))
		_, err := client.Undo(t.Context())
		var failure *Error
		if !errors.As(err, &failure) || *failure != test.want {
			t.Errorf("%d: error = %#v, want %#v", test.status, err, test.want)
		}
		if len(requests()) != 1 {
			t.Errorf("%d: %d attempts, want 1", test.status, len(requests()))
		}
	}
}

func TestClientStopsRetryingWhenTheCallerGivesUp(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	client, requests := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		cancel()
		answerJSON(http.StatusServiceUnavailable, "")(w, r)
	})
	client.retryDelay = time.Hour
	_, err := client.Tags(ctx)
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrUnreachable) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	if len(requests()) != 1 {
		t.Errorf("%d attempts, want 1", len(requests()))
	}
}

func TestClientRefusesAnAnswerItCannotDecode(t *testing.T) {
	client, _ := recordingServer(t, answerJSON(http.StatusOK, `{"tasks":{}}`))
	var syntax *json.UnmarshalTypeError
	if _, err := client.ListTasks(t.Context(), StatusPending, time.Time{}); !errors.As(err, &syntax) || errors.Is(err, ErrUnreachable) {
		t.Errorf("error = %v, want a decode error", err)
	}
}
