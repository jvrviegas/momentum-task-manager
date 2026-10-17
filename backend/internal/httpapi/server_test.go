package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/auth"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/logging"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/tasks"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

func TestHealthAnswersOK(t *testing.T) {
	var logs bytes.Buffer
	response := httptest.NewRecorder()
	New(logging.New(&logs), "dev", auth.New(nil, auth.Config{}), tasks.New(nil), 0).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))

	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" ||
		response.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("GET /health = %d %q %q", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	if !strings.Contains(logs.String(), `"route":"GET /health","status":200`) {
		t.Fatalf("log:\n%s", logs.String())
	}
}

func TestUnknownRouteAnswersNotFoundError(t *testing.T) {
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/healthz", nil),
		httptest.NewRequest(http.MethodPost, "/health", nil),
		httptest.NewRequest(http.MethodGet, "/v1/nothing", nil),
	} {
		response := httptest.NewRecorder()
		New(logging.New(&bytes.Buffer{}), "dev", auth.New(nil, auth.Config{}), tasks.New(nil), 0).ServeHTTP(response, request)
		var body api.Error
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != http.StatusNotFound || body.Code != api.CodeNotFound {
			t.Errorf("%s %s = %d %s", request.Method, request.URL, response.Code, response.Body.String())
		}
	}
}

func TestRequestLogLeavesOutPathQueryHeadersAndBody(t *testing.T) {
	var logs bytes.Buffer
	request := httptest.NewRequest(http.MethodPost, "/v1/tasks/secret-uuid?q=secret-query", strings.NewReader(`{"description":"secret plans"}`))
	request.Header.Set("Authorization", "Bearer secret-token")
	New(logging.New(&logs), "dev", auth.New(nil, auth.Config{}), tasks.New(nil), 0).ServeHTTP(httptest.NewRecorder(), request)

	if !strings.Contains(logs.String(), `"message":"request","method":"POST","route":"/","status":404`) {
		t.Errorf("log:\n%s", logs.String())
	}
	for _, secret := range []string{"secret-uuid", "secret-query", "secret-token", "secret plans"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("log contains %q:\n%s", secret, logs.String())
		}
	}
}

func TestPanicAnswersInternalErrorWithoutLoggingItsValue(t *testing.T) {
	var logs bytes.Buffer
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/tasks", func(http.ResponseWriter, *http.Request) {
		panic(errors.New(`add "secret plans"`))
	})
	response := httptest.NewRecorder()
	logRequests(logging.New(&logs), mux).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/tasks", nil))

	var body api.Error
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != http.StatusInternalServerError || body.Code != api.CodeInternal {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(logs.String(), `"severity":"ERROR","message":"request panicked","route":"POST /v1/tasks"`) ||
		!strings.Contains(logs.String(), `"severity":"ERROR","message":"request","method":"POST","route":"POST /v1/tasks","status":500`) {
		t.Errorf("log:\n%s", logs.String())
	}
	if strings.Contains(logs.String(), "secret plans") {
		t.Errorf("log contains the panic value:\n%s", logs.String())
	}
}
