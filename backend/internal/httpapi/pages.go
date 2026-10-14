package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"

	"github.com/jvrviegas/momentum-task-manager/backend/internal/auth"
)

// The browser half of sign-in. /device asks for the code the device shows and
// sends the browser to a provider, which sends it back to the callback.
const (
	devicePath   = "/device"
	callbackPath = "/auth/callback/"
	csrfCookie   = "momentum_csrf"
	stateCookie  = "momentum_state"
	// cookieSeconds matches how long a device code lasts.
	cookieSeconds = 600
	maxForm       = 64 << 10
)

var providerLabels = map[string]string{auth.GoogleName: "Google", auth.GitHubName: "GitHub"}

type providerButton struct{ Name, Label string }

type devicePageData struct {
	Code, CSRF, Error string
	Providers         []providerButton
}

type resultPageData struct{ Title, Message string }

// devicePage asks for the code. It sets a CSRF cookie that the form must
// match, so another site cannot submit a code on the person's behalf.
func (s *server) devicePage(w http.ResponseWriter, r *http.Request) {
	s.renderDevicePage(w, http.StatusOK, r.URL.Query().Get("code"), "")
}

func (s *server) renderDevicePage(w http.ResponseWriter, status int, code, message string) {
	csrf := randomToken()
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookie, Value: csrf, Path: devicePath, MaxAge: cookieSeconds,
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode,
	})
	data := devicePageData{Code: code, CSRF: csrf, Error: message}
	for _, name := range s.auth.Providers() {
		label := providerLabels[name]
		if label == "" {
			label = name
		}
		data.Providers = append(data.Providers, providerButton{Name: name, Label: label})
	}
	render(w, status, devicePageTemplate, data)
}

// deviceSubmit starts the provider sign-in for the code, and keeps its state
// in a cookie that the callback must match.
func (s *server) deviceSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxForm)
	if err := r.ParseForm(); err != nil {
		render(w, http.StatusBadRequest, resultPageTemplate, resultPageData{"Sign-in failed", "The form could not be read. Open the link from your device again."})
		return
	}
	cookie, err := r.Cookie(csrfCookie)
	if err != nil || !sameSecret(cookie.Value, r.PostForm.Get("csrf")) {
		render(w, http.StatusForbidden, resultPageTemplate, resultPageData{"This page expired", "Open the link from your device again."})
		return
	}
	code := r.PostForm.Get("code")
	redirect, state, err := s.auth.BeginSignIn(r.Context(), code, r.PostForm.Get("provider"))
	switch {
	case errors.Is(err, auth.ErrInvalidCode):
		s.renderDevicePage(w, http.StatusBadRequest, code, err.Error())
		return
	case err != nil:
		s.logFailure(r, err)
		render(w, http.StatusInternalServerError, resultPageTemplate, resultPageData{"Sign-in failed", "The server failed. Try again."})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: state, Path: callbackPath, MaxAge: cookieSeconds,
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// callback is where the provider sends the browser back. The state must
// match the cookie, so only the browser that started the sign-in can finish
// it.
func (s *server) callback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Path: callbackPath, MaxAge: -1, HttpOnly: true, Secure: s.secure})
	if query.Get("error") != "" {
		render(w, http.StatusBadRequest, resultPageTemplate, resultPageData{"Sign-in cancelled", "Nothing was approved. Start again on your device if you want to sign in."})
		return
	}
	state := query.Get("state")
	cookie, err := r.Cookie(stateCookie)
	if err != nil || !sameSecret(cookie.Value, state) {
		render(w, http.StatusForbidden, resultPageTemplate, resultPageData{"Sign-in failed", "This sign-in did not start in this browser. Start again on your device."})
		return
	}
	result, err := s.auth.CompleteSignIn(r.Context(), r.PathValue("provider"), query.Get("code"), state)
	switch {
	case errors.Is(err, auth.ErrSignInExpired):
		render(w, http.StatusBadRequest, resultPageTemplate, resultPageData{"Sign-in failed", err.Error()})
	case err != nil:
		s.logFailure(r, err)
		render(w, http.StatusBadGateway, resultPageTemplate, resultPageData{"Sign-in failed", "Signing in with the provider failed. Start again on your device."})
	case !result.Approved:
		render(w, http.StatusForbidden, resultPageTemplate, resultPageData{"Not allowed", result.Email + " may not use this server. Ask its owner to allow the address."})
	default:
		render(w, http.StatusOK, resultPageTemplate, resultPageData{"Signed in", "You are signed in as " + result.Email + ". Return to Momentum on your device; you can close this window."})
	}
}

func (s *server) logFailure(r *http.Request, err error) {
	s.logger.ErrorContext(r.Context(), "sign-in failed", "error_type", errorType(err))
}

func randomToken() string {
	data := make([]byte, 32)
	_, _ = rand.Read(data)
	return base64.RawURLEncoding.EncodeToString(data)
}

func sameSecret(a, b string) bool {
	return a != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// render writes a page with headers that keep it out of frames, caches, and
// referrers: the URL can carry a user code.
func render(w http.ResponseWriter, status int, page *template.Template, data any) {
	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'")
	header.Set("X-Frame-Options", "DENY")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = page.ExecuteTemplate(w, "layout", data)
}

const layout = `{{define "layout"}}<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Momentum sign-in</title>
<style>
:root{color-scheme:light dark;--bg:#f6f6f4;--card:#fff;--text:#1d1d1b;--muted:#5f5f5a;--accent:#2f5bd3;--on-accent:#fff;--error:#b3261e;--border:#d9d9d4}
@media (prefers-color-scheme:dark){:root{--bg:#141413;--card:#1f1f1d;--text:#ececea;--muted:#a3a39d;--accent:#8aa6ff;--on-accent:#10131c;--error:#ff8a80;--border:#3a3a37}}
body{margin:0;min-height:100vh;display:grid;place-items:center;background:var(--bg);color:var(--text);font:16px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
main{box-sizing:border-box;width:min(26rem,calc(100% - 2rem));background:var(--card);border:1px solid var(--border);border-radius:12px;padding:1.75rem}
h1{font-size:1.25rem;margin:0 0 .75rem}
p{margin:.5rem 0;color:var(--muted)}
label{display:block;font-weight:600;margin:1rem 0 .25rem}
input{box-sizing:border-box;width:100%;font:600 1.4rem ui-monospace,monospace;letter-spacing:.15em;text-align:center;text-transform:uppercase;padding:.5rem;border:1px solid var(--border);border-radius:8px;background:transparent;color:var(--text)}
button{display:block;width:100%;margin-top:.75rem;padding:.65rem;font:inherit;font-weight:600;border-radius:8px;border:1px solid var(--accent);background:var(--accent);color:var(--on-accent);cursor:pointer}
.error{color:var(--error)}
.warning{font-size:.9rem}
</style>
</head>
<body><main>{{template "content" .}}</main></body>
</html>{{end}}`

var (
	devicePageTemplate = template.Must(template.Must(template.New("device").Parse(layout)).Parse(`{{define "content"}}
<h1>Sign in to Momentum</h1>
<p>Enter the code your device shows.</p>
<form method="post" action="/device">
<input type="hidden" name="csrf" value="{{.CSRF}}">
<label for="code">Code</label>
<input id="code" name="code" value="{{.Code}}" autocomplete="off" autocapitalize="characters" spellcheck="false" required>
{{if .Error}}<p class="error" role="alert">{{.Error}}</p>{{end}}
<p class="warning">Only continue if you started signing in on your own device and it shows this code. Whoever gets you to approve their code gets into your account.</p>
{{range .Providers}}<button name="provider" value="{{.Name}}">Continue with {{.Label}}</button>{{else}}<p class="error">No sign-in provider is configured on this server.</p>{{end}}
</form>
{{end}}`))
	resultPageTemplate = template.Must(template.Must(template.New("result").Parse(layout)).Parse(`{{define "content"}}
<h1>{{.Title}}</h1>
<p>{{.Message}}</p>
{{end}}`))
)
