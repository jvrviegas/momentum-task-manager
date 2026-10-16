package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// backendSetup writes a config that selects a test server, which answers the
// version and session routes, and returns the config's path. With signedIn,
// it also saves a sign-in to that server.
func backendSetup(t *testing.T, signedIn bool) (string, *[]string) {
	t.Helper()
	var auth []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/version", func(w http.ResponseWriter, r *http.Request) {
		auth = append(auth, r.Header.Get("Authorization"))
		w.Write([]byte(`{"server":"v1.2.3"}`))
	})
	mux.HandleFunc("GET /v1/auth/session", func(w http.ResponseWriter, r *http.Request) {
		auth = append(auth, r.Header.Get("Authorization"))
		w.Write([]byte(`{"email":"ana@example.com","expiresAt":"2027-01-14T12:00:00Z"}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TZ", "Europe/Lisbon")
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte("[backend]\nurl = \""+server.URL+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if signedIn {
		credentials := filepath.Join(root, "xdg", "momentum", "credentials.json")
		os.MkdirAll(filepath.Dir(credentials), 0o700)
		saved := `{"url":"` + server.URL + `","token":"mtk_saved","email":"ana@example.com","expiresAt":"2027-01-14T12:00:00Z"}`
		if err := os.WriteFile(credentials, []byte(saved), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return path, &auth
}

func TestDoctorChecksTheBackendWithTheSavedSignIn(t *testing.T) {
	path, auth := backendSetup(t, true)
	t.Setenv("PATH", t.TempDir()) // no Taskwarrior
	var out, errOut bytes.Buffer
	code := run([]string{"--config", path, "doctor"}, &out, &errOut)
	for _, want := range []string{"[ok] Backend server", "answers; version v1.2.3", "[ok] Sign-in                  ana@example.com until 2027-01-14",
		"[ok] Time zone                Europe/Lisbon", "Doctor: ready"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("doctor output lacks %q:\n%s", want, out.String())
		}
	}
	if code != 0 || strings.Contains(out.String(), "Taskwarrior") || len(*auth) != 2 || (*auth)[0] != "Bearer mtk_saved" || (*auth)[1] != "Bearer mtk_saved" {
		t.Errorf("code %d, auth %q, output:\n%s%s", code, *auth, out.String(), errOut.String())
	}
}

func TestBackendModeNeedsASavedSignIn(t *testing.T) {
	path, auth := backendSetup(t, false)
	var out, errOut bytes.Buffer
	if code := run([]string{"--config", path}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "momentum: not signed in to http://127.0.0.1:") {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
	out.Reset()
	if code := run([]string{"--config", path, "doctor"}, &out, &errOut); code != 1 || !strings.Contains(out.String(), "[fail] Sign-in                  not signed in to") {
		t.Errorf("doctor code %d:\n%s", code, out.String())
	}
	if len(*auth) != 1 || (*auth)[0] != "" {
		t.Errorf("auth %q, want one version check without a token", *auth)
	}
}
