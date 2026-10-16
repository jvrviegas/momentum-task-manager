package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/auth"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/logging"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/ent/task"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/storetest"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/store/viewer"
	"github.com/jvrviegas/momentum-task-manager/backend/internal/tasks"
	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// fakeProvider signs in as identity whoever reaches it.
type fakeProvider struct{ identity auth.Identity }

func (f *fakeProvider) Name() string { return auth.GoogleName }

func (f *fakeProvider) AuthCodeURL(state, _, _ string) string {
	return "https://provider.test/authorize?state=" + url.QueryEscape(state)
}

func (f *fakeProvider) Identify(_ context.Context, code, _, _ string) (auth.Identity, error) {
	if code != "provider-code" {
		return auth.Identity{}, errors.New("fake provider: wrong code")
	}
	return f.identity, nil
}

type harness struct {
	t        *testing.T
	url      string
	logs     *bytes.Buffer
	provider *fakeProvider
	client   *ent.Client
	server   *server
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	client := storetest.Client(t)
	provider := &fakeProvider{}
	service := auth.New(client, auth.Config{
		PublicURL: "http://momentum.test", AllowedEmails: []string{"alice@example.com", "bob@example.com"},
		Providers: []auth.Provider{provider},
	})
	var logs bytes.Buffer
	logger := logging.New(&logs)
	userTasks := tasks.New(client)
	httpServer := httptest.NewServer(New(logger, "v1.2.3", service, userTasks))
	t.Cleanup(httpServer.Close)
	return &harness{t: t, url: httpServer.URL, logs: &logs, provider: provider, client: client,
		server: &server{logger: logger, auth: service, tasks: userTasks}}
}

// browser returns a client that keeps cookies and does not follow redirects.
func (h *harness) browser() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (h *harness) do(client *http.Client, method, path, token string, body io.Reader, contentType string) (*http.Response, string) {
	h.t.Helper()
	request, err := http.NewRequest(method, h.url+path, body)
	if err != nil {
		h.t.Fatal(err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := client.Do(request)
	if err != nil {
		h.t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response, string(data)
}

func (h *harness) postJSON(path, token string, body any) (*http.Response, string) {
	h.t.Helper()
	data, _ := json.Marshal(body)
	return h.do(http.DefaultClient, http.MethodPost, path, token, bytes.NewReader(data), "application/json")
}

func (h *harness) start() api.DeviceSignIn {
	h.t.Helper()
	response, body := h.postJSON("/v1/auth/device", "", nil)
	var start api.DeviceSignIn
	if err := json.Unmarshal([]byte(body), &start); err != nil || response.StatusCode != http.StatusOK ||
		response.Header.Get("Cache-Control") != "no-store" {
		h.t.Fatalf("start: %d %s", response.StatusCode, body)
	}
	return start
}

func (h *harness) poll(deviceCode string) (int, api.Token, api.Error) {
	h.t.Helper()
	response, body := h.postJSON("/v1/auth/token", "", api.PollDeviceSignInRequest{DeviceCode: deviceCode})
	var token api.Token
	var failure api.Error
	if response.StatusCode == http.StatusOK {
		_ = json.Unmarshal([]byte(body), &token)
	} else {
		_ = json.Unmarshal([]byte(body), &failure)
	}
	return response.StatusCode, token, failure
}

var csrfField = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

// approve opens the device page in browser, submits userCode, and returns
// the provider redirect's state.
func (h *harness) approve(browser *http.Client, userCode string) string {
	h.t.Helper()
	response, page := h.do(browser, http.MethodGet, "/device?code="+url.QueryEscape(userCode), "", nil, "")
	match := csrfField.FindStringSubmatch(page)
	if response.StatusCode != http.StatusOK || match == nil || !strings.Contains(page, `value="`+userCode+`"`) {
		h.t.Fatalf("device page: %d %s", response.StatusCode, page)
	}
	form := url.Values{"csrf": {match[1]}, "code": {userCode}, "provider": {auth.GoogleName}}
	response, page = h.do(browser, http.MethodPost, "/device", "", strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
	location, _ := url.Parse(response.Header.Get("Location"))
	if response.StatusCode != http.StatusSeeOther || location == nil || location.Host != "provider.test" {
		h.t.Fatalf("submit: %d %s %s", response.StatusCode, response.Header.Get("Location"), page)
	}
	return location.Query().Get("state")
}

func (h *harness) callback(browser *http.Client, state string) (int, string) {
	h.t.Helper()
	response, page := h.do(browser, http.MethodGet, "/auth/callback/google?code=provider-code&state="+url.QueryEscape(state), "", nil, "")
	return response.StatusCode, page
}

// signIn runs the whole flow for email and returns the device's token.
func (h *harness) signIn(email string) api.Token {
	h.t.Helper()
	h.provider.identity = auth.Identity{Subject: "subject-" + email, Email: email}
	start := h.start()
	browser := h.browser()
	if status, page := h.callback(browser, h.approve(browser, start.UserCode)); status != http.StatusOK || !strings.Contains(page, "signed in as "+email) {
		h.t.Fatalf("callback: %d %s", status, page)
	}
	status, token, failure := h.poll(start.DeviceCode)
	if status != http.StatusOK {
		h.t.Fatalf("poll: %d %+v", status, failure)
	}
	return token
}

func TestDeviceSignInOverHTTP(t *testing.T) {
	h := newHarness(t)
	h.provider.identity = auth.Identity{Subject: "subject-1", Email: "Alice@Example.com"}
	start := h.start()
	if start.VerificationURLComplete != "http://momentum.test/device?code="+start.UserCode {
		t.Fatalf("start = %+v", start)
	}
	if status, _, failure := h.poll(start.DeviceCode); status != http.StatusBadRequest || failure.Code != api.CodeAuthorizationPending {
		t.Fatalf("poll before sign-in: %d %+v", status, failure)
	}

	browser := h.browser()
	response, page := h.do(browser, http.MethodGet, "/device?code="+start.UserCode, "", nil, "")
	for header, want := range map[string]string{
		"X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer", "Cache-Control": "no-store",
		"Content-Security-Policy": "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'",
	} {
		if response.Header.Get(header) != want {
			t.Errorf("%s = %q, want %q", header, response.Header.Get(header), want)
		}
	}
	if !strings.Contains(page, "Continue with Google") || !strings.Contains(page, "Only continue if you started signing in") {
		t.Errorf("device page:\n%s", page)
	}
	if status, page := h.callback(browser, h.approve(browser, start.UserCode)); status != http.StatusOK || !strings.Contains(page, "signed in as alice@example.com") {
		t.Fatalf("callback: %d %s", status, page)
	}

	status, token, failure := h.poll(start.DeviceCode)
	if status != http.StatusOK || token.Email != "alice@example.com" || !strings.HasPrefix(token.Token, "mtk_") {
		t.Fatalf("poll after sign-in: %d %+v %+v", status, token, failure)
	}
	response, body := h.do(http.DefaultClient, http.MethodGet, "/v1/auth/session", token.Token, nil, "")
	var session api.Session
	if err := json.Unmarshal([]byte(body), &session); err != nil || response.StatusCode != http.StatusOK || session.Email != "alice@example.com" {
		t.Fatalf("session: %d %s", response.StatusCode, body)
	}
	if response, body := h.do(http.DefaultClient, http.MethodPost, "/v1/auth/sign-out", token.Token, nil, ""); response.StatusCode != http.StatusNoContent {
		t.Fatalf("sign-out: %d %s", response.StatusCode, body)
	}
	if response, _ := h.do(http.DefaultClient, http.MethodGet, "/v1/auth/session", token.Token, nil, ""); response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("session after sign-out: %d", response.StatusCode)
	}

	// None of the secrets or the address reach the log.
	for _, secret := range []string{token.Token, token.Token[4:], start.DeviceCode, start.UserCode, "alice@example.com", "Alice@Example.com"} {
		if strings.Contains(h.logs.String(), secret) {
			t.Errorf("log contains %q:\n%s", secret, h.logs.String())
		}
	}
}

func TestDeviceFormNeedsItsCSRFCookie(t *testing.T) {
	h := newHarness(t)
	start := h.start()
	page := h.browser()
	_, body := h.do(page, http.MethodGet, "/device", "", nil, "")
	csrf := csrfField.FindStringSubmatch(body)[1]
	// Another site can post the form, but not with the person's cookie.
	for _, client := range []*http.Client{h.browser(), page} {
		form := url.Values{"csrf": {csrf}, "code": {start.UserCode}, "provider": {auth.GoogleName}}
		if client == page {
			form.Set("csrf", "forged")
		}
		response, body := h.do(client, http.MethodPost, "/device", "", strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
		if response.StatusCode != http.StatusForbidden || response.Header.Get("Location") != "" {
			t.Fatalf("submit without the matching cookie: %d %s", response.StatusCode, body)
		}
	}
}

func TestCallbackNeedsTheBrowserThatStarted(t *testing.T) {
	h := newHarness(t)
	h.provider.identity = auth.Identity{Subject: "subject-1", Email: "alice@example.com"}
	start := h.start()
	browser := h.browser()
	state := h.approve(browser, start.UserCode)
	if status, page := h.callback(h.browser(), state); status != http.StatusForbidden || !strings.Contains(page, "did not start in this browser") {
		t.Fatalf("callback from another browser: %d %s", status, page)
	}
	if status, _, failure := h.poll(start.DeviceCode); status != http.StatusBadRequest || failure.Code != api.CodeAuthorizationPending {
		t.Fatalf("poll: %d %+v", status, failure)
	}
	if status, page := h.callback(browser, state); status != http.StatusOK {
		t.Fatalf("callback from the browser that started: %d %s", status, page)
	}
	// The state cookie is gone, so the callback cannot be replayed.
	if status, _ := h.callback(browser, state); status != http.StatusForbidden {
		t.Fatalf("replayed callback: %d", status)
	}
}

func TestSignInPagesExplainFailures(t *testing.T) {
	h := newHarness(t)
	h.provider.identity = auth.Identity{Subject: "subject-1", Email: "mallory@example.com"}
	start := h.start()
	browser := h.browser()

	_, body := h.do(browser, http.MethodGet, "/device", "", nil, "")
	form := url.Values{"csrf": {csrfField.FindStringSubmatch(body)[1]}, "code": {"BCDF-GHJK"}, "provider": {auth.GoogleName}}
	if response, page := h.do(browser, http.MethodPost, "/device", "", strings.NewReader(form.Encode()), "application/x-www-form-urlencoded"); response.StatusCode != http.StatusBadRequest || !strings.Contains(page, "not valid or has expired") {
		t.Fatalf("unknown code: %d %s", response.StatusCode, page)
	}
	if response, page := h.do(browser, http.MethodGet, "/auth/callback/google?error=access_denied&state=x", "", nil, ""); response.StatusCode != http.StatusBadRequest || !strings.Contains(page, "Sign-in cancelled") {
		t.Fatalf("cancelled at the provider: %d %s", response.StatusCode, page)
	}
	if status, page := h.callback(browser, h.approve(browser, start.UserCode)); status != http.StatusForbidden || !strings.Contains(page, "mallory@example.com may not use this server") {
		t.Fatalf("account outside the allowlist: %d %s", status, page)
	}
	if status, _, failure := h.poll(start.DeviceCode); status != http.StatusBadRequest || failure.Code != api.CodeAccessDenied {
		t.Fatalf("poll: %d %+v", status, failure)
	}
}

func TestPollNeedsADeviceCode(t *testing.T) {
	h := newHarness(t)
	response, body := h.do(http.DefaultClient, http.MethodPost, "/v1/auth/token", "", strings.NewReader("{"), "application/json")
	if response.StatusCode != http.StatusBadRequest || !strings.Contains(body, `"code":"invalid"`) {
		t.Fatalf("malformed body: %d %s", response.StatusCode, body)
	}
	if response, body := h.postJSON("/v1/auth/token", "", api.PollDeviceSignInRequest{}); response.StatusCode != http.StatusBadRequest || !strings.Contains(body, `"field":"deviceCode"`) {
		t.Fatalf("no device code: %d %s", response.StatusCode, body)
	}
	if status, _, failure := h.poll("unknown"); status != http.StatusBadRequest || failure.Code != api.CodeExpiredToken {
		t.Fatalf("unknown device code: %d %+v", status, failure)
	}
}

func TestAuthenticatedRoutesRefuseMissingAndUnknownTokens(t *testing.T) {
	h := newHarness(t)
	token := h.signIn("alice@example.com")
	for _, header := range []string{"", "Basic " + token.Token, "Bearer", "Bearer mtk_unknown", "Bearer " + token.Token + "x"} {
		request, _ := http.NewRequest(http.MethodGet, h.url+"/v1/auth/session", nil)
		if header != "" {
			request.Header.Set("Authorization", header)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized || response.Header.Get("WWW-Authenticate") != `Bearer realm="momentum"` {
			t.Errorf("Authorization %q: %d %q", header, response.StatusCode, response.Header.Get("WWW-Authenticate"))
		}
	}
	if response, _ := h.do(http.DefaultClient, http.MethodPost, "/v1/auth/sign-out", "", nil, ""); response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sign-out without a token: %d", response.StatusCode)
	}
}

// Each token reaches only its own user's tasks: the middleware acts for the
// token's user, and the store's privacy rules do the rest.
func TestTokensReachOnlyTheirOwnTasks(t *testing.T) {
	h := newHarness(t)
	alice, bob := h.signIn("alice@example.com"), h.signIn("bob@example.com")
	for _, account := range []api.Token{alice, bob} {
		session, err := h.server.auth.Authenticate(t.Context(), account.Token)
		if err != nil {
			t.Fatal(err)
		}
		ctx := viewer.NewContext(t.Context(), session.UserID)
		h.client.Task.Create().SetUserID(session.UserID).SetDescription(account.Email + "'s task").SetStatus(task.StatusPending).SaveX(ctx)
	}
	list := h.server.authenticated(func(w http.ResponseWriter, r *http.Request) {
		tasks, err := h.client.Task.Query().All(r.Context())
		if err != nil {
			h.server.internalError(w, r, err)
			return
		}
		var descriptions []string
		for _, found := range tasks {
			descriptions = append(descriptions, found.Description)
		}
		writeJSON(w, http.StatusOK, descriptions)
	})
	for token, want := range map[string]string{alice.Token: `["alice@example.com's task"]`, bob.Token: `["bob@example.com's task"]`} {
		request := httptest.NewRequest(http.MethodGet, "/tasks", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		list(response, request)
		if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != want {
			t.Errorf("tasks = %d %s, want %s", response.Code, response.Body.String(), want)
		}
	}
}
