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

func TestBackendConfigServesTheBackend(t *testing.T) {
	var auth string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/version", func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Write([]byte(`{"server":"v1.2.3"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	config := filepath.Join(root, "config.toml")
	if err := os.WriteFile(config, []byte("[backend]\nurl = \""+server.URL+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status := `{"id":1,"op":"status"}` + "\n"

	var out, errOut bytes.Buffer
	if code := run([]string{"-config", config}, strings.NewReader(status), &out, &errOut); code != 1 ||
		!strings.Contains(errOut.String(), "momentum-bridge: not signed in to "+server.URL) || auth != "" {
		t.Errorf("without a sign-in: code %d, stderr %q, auth %q", code, errOut.String(), auth)
	}

	credentials := filepath.Join(root, "xdg", "momentum", "credentials.json")
	os.MkdirAll(filepath.Dir(credentials), 0o700)
	if err := os.WriteFile(credentials, []byte(`{"url":"`+server.URL+`","token":"mtk_saved"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	code := run([]string{"-config", config, "-task", filepath.Join(root, "no-task")}, strings.NewReader(status), &out, &errOut)
	if code != 0 || auth != "Bearer mtk_saved" || !strings.Contains(out.String(), `"taskwarriorVersion":""`) ||
		!strings.Contains(out.String(), `"estimateUDA":"configured"`) {
		t.Errorf("code %d, auth %q, stdout %s, stderr %s", code, auth, out.String(), errOut.String())
	}
}
