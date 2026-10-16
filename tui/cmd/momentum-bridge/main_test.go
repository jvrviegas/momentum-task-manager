package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestBackendConfigTakesTheTokenFromTheApp(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
		mu.Unlock()
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"code":"unauthorized","message":"sign in again"}`))
			return
		}
		w.Write([]byte(`{"tasks":[],"projects":[]}`))
	}))
	defer server.Close()

	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	config := filepath.Join(root, "config.toml")
	if err := os.WriteFile(config, []byte("[backend]\nurl = \""+server.URL+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The TUI's sign-in is not the app's.
	credentials := filepath.Join(root, "xdg", "momentum", "credentials.json")
	os.MkdirAll(filepath.Dir(credentials), 0o700)
	if err := os.WriteFile(credentials, []byte(`{"url":"`+server.URL+`","token":"mtk_saved"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	lines := strings.Join([]string{
		`{"id":1,"op":"status"}`,
		`{"id":2,"op":"load"}`,
		`{"id":3,"op":"setToken","params":{"token":"mtk_app"}}`,
		`{"id":4,"op":"load"}`,
	}, "\n") + "\n"

	var out, errOut bytes.Buffer
	code := run([]string{"-config", config, "-task", filepath.Join(root, "no-task")}, strings.NewReader(lines), &out, &errOut)
	if code != 0 || errOut.Len() != 0 {
		t.Fatalf("code %d, stderr %s", code, errOut.String())
	}
	for _, want := range []string{
		`{"id":1,"result":{"taskwarriorVersion":"",`, `"estimateUDA":"configured"`, `"backendUrl":"` + server.URL + `"`,
		`{"id":2,"error":{"message":"sign in again","code":"unauthorized"}}`,
		`{"id":3,"result":{}}`,
		`{"id":4,"result":{"tasks":[]`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout lacks %s:\n%s", want, out.String())
		}
	}
	want := []string{
		"GET /v1/tasks ",
		"GET /v1/tasks Bearer mtk_app",
		"GET /v1/projects Bearer mtk_app",
		"GET /v1/tasks Bearer mtk_app",
	}
	if !reflect.DeepEqual(requests, want) {
		t.Errorf("requests:\n got %q\nwant %q", requests, want)
	}
}
