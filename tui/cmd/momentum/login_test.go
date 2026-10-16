package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jvrviegas/momentum-task-manager/core/api"
)

// accountServer is a test server for momentum login and logout. It answers
// sign-out with signOutStatus and records the tokens sent to sign out.
type accountServer struct {
	url, config, credentials string
	signOutStatus            int
	signedOut                []string
}

func newAccountServer(t *testing.T) *accountServer {
	t.Helper()
	account := &accountServer{signOutStatus: http.StatusNoContent}
	mux := http.NewServeMux()
	mux.HandleFunc(api.RouteStartDeviceSignIn, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"deviceCode":"dc","userCode":"BCDFGHJK","verificationUrl":"`+account.url+`/device","verificationUrlComplete":"`+account.url+`/device?code=BCDFGHJK","expiresAt":"2026-10-16T12:10:00Z","intervalSeconds":1}`)
	})
	mux.HandleFunc(api.RoutePollDeviceSignIn, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"token":"mtk_new","expiresAt":"2027-01-14T12:00:00Z","email":"ana@example.com"}`)
	})
	mux.HandleFunc(api.RouteSignOut, func(w http.ResponseWriter, r *http.Request) {
		account.signedOut = append(account.signedOut, r.Header.Get("Authorization"))
		w.WriteHeader(account.signOutStatus)
		switch account.signOutStatus {
		case http.StatusUnauthorized:
			io.WriteString(w, `{"code":"unauthorized","message":"sign in again"}`)
		case http.StatusInternalServerError:
			io.WriteString(w, `{"code":"internal","message":"the server failed; try again"}`)
		}
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	account.url = server.URL

	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	account.config = filepath.Join(root, "config.toml")
	account.credentials = filepath.Join(root, "xdg", "momentum", "credentials.json")
	if err := os.WriteFile(account.config, []byte("[backend]\nurl = \""+server.URL+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return account
}

func (a *accountServer) save(t *testing.T, token string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(a.credentials), 0o700)
	saved := `{"url":"` + a.url + `","token":"` + token + `","email":"ana@example.com","expiresAt":"2026-12-01T12:00:00Z"}`
	if err := os.WriteFile(a.credentials, []byte(saved), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoginSavesTheSignInAndRevokesTheOneItReplaces(t *testing.T) {
	account := newAccountServer(t)
	account.save(t, "mtk_old")
	var opened []string
	defer func(original func(string) error) { openBrowser = original }(openBrowser)
	openBrowser = func(url string) error { opened = append(opened, url); return nil }

	var out, errOut bytes.Buffer
	if code := run([]string{"login", "--config", account.config}, &out, &errOut); code != 0 {
		t.Fatalf("code %d, stderr %q", code, errOut.String())
	}
	for _, want := range []string{"enter the code BCDFGHJK at\n" + account.url + "/device\n", "Signed in to " + account.url + " as ana@example.com until 2027-01-14."} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
	if len(opened) != 1 || opened[0] != account.url+"/device?code=BCDFGHJK" {
		t.Errorf("opened %q", opened)
	}
	saved, err := os.ReadFile(account.credentials)
	info, _ := os.Stat(account.credentials)
	if err != nil || !strings.Contains(string(saved), `"token":"mtk_new"`) || info.Mode().Perm() != 0o600 {
		t.Errorf("saved %s (%v), mode %v", saved, err, info.Mode().Perm())
	}
	if len(account.signedOut) != 1 || account.signedOut[0] != "Bearer mtk_old" || errOut.Len() != 0 {
		t.Errorf("signed out %q, stderr %q", account.signedOut, errOut.String())
	}
}

func TestLoginShowsTheAddressWhenNoBrowserOpens(t *testing.T) {
	account := newAccountServer(t)
	defer func(original func(string) error) { openBrowser = original }(openBrowser)
	openBrowser = func(string) error { return errors.New("no browser") }

	var out, errOut bytes.Buffer
	if code := run([]string{"login", "--config", account.config}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "Could not open a browser; open the address above.") {
		t.Errorf("code %d, output:\n%s%s", code, out.String(), errOut.String())
	}
	if len(account.signedOut) != 0 {
		t.Errorf("signed out %q with nothing saved before", account.signedOut)
	}
}

func TestLogoutRevokesAndRemovesTheSignIn(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		warning string
	}{
		{name: "revoked", status: http.StatusNoContent},
		{name: "already revoked", status: http.StatusUnauthorized},
		{name: "not revoked", status: http.StatusInternalServerError,
			warning: "momentum: could not revoke the sign-in on the server, so its token stays valid until 2026-12-01: the server failed; try again\n"},
	} {
		account := newAccountServer(t)
		account.signOutStatus = test.status
		account.save(t, "mtk_saved")
		var out, errOut bytes.Buffer
		code := run([]string{"logout", "--config", account.config}, &out, &errOut)
		if code != 0 || out.String() != "Signed out of "+account.url+".\n" || errOut.String() != test.warning {
			t.Errorf("%s: code %d, stdout %q, stderr %q", test.name, code, out.String(), errOut.String())
		}
		if _, err := os.Stat(account.credentials); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: credentials left: %v", test.name, err)
		}
		if len(account.signedOut) != 1 || account.signedOut[0] != "Bearer mtk_saved" {
			t.Errorf("%s: signed out %q", test.name, account.signedOut)
		}
	}
}

func TestLogoutWithoutASignIn(t *testing.T) {
	account := newAccountServer(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"logout", "--config", account.config}, &out, &errOut); code != 0 || out.String() != "momentum: not signed in to "+account.url+"\n" {
		t.Errorf("code %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
	if len(account.signedOut) != 0 {
		t.Errorf("signed out %q", account.signedOut)
	}
}

func TestLoginAndLogoutNeedABackend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"login", "logout"} {
		var out, errOut bytes.Buffer
		if code := run([]string{command, "--config", path}, &out, &errOut); code != 1 || errOut.String() != "momentum: "+command+" needs a [backend] url in "+path+"\n" {
			t.Errorf("%s: code %d, stderr %q", command, code, errOut.String())
		}
	}
}
