package backend

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCredentialsPathIsInTheConfigDirectory(t *testing.T) {
	if got := CredentialsPath("/home/ana", ""); got != "/home/ana/.config/momentum/credentials.json" {
		t.Errorf("path = %q", got)
	}
	if got := CredentialsPath("/home/ana", "/xdg"); got != "/xdg/momentum/credentials.json" {
		t.Errorf("XDG path = %q", got)
	}
}

func TestSaveAndRemoveCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xdg", "momentum", "credentials.json")
	saved := Credentials{URL: "https://momentum.example", Token: "mtk_first", Email: "ana@example.com", ExpiresAt: time.Date(2027, 1, 14, 9, 30, 0, 0, time.UTC)}
	if err := SaveCredentials(path, saved); err != nil {
		t.Fatal(err)
	}
	saved.Token = "mtk_second"
	if err := SaveCredentials(path, saved); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadCredentials(path, "https://momentum.example", time.Time{}); err != nil || got != saved {
		t.Errorf("read back %+v, %v", got, err)
	}
	for name, want := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
		if info, err := os.Stat(name); err != nil || info.Mode().Perm() != want {
			t.Errorf("%s: mode %v, %v; want %v", name, info.Mode().Perm(), err, want)
		}
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("left %d files, want only credentials.json", len(entries))
	}
	if err := RemoveCredentials(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("after remove: %v", err)
	}
	if err := RemoveCredentials(path); err != nil {
		t.Errorf("removing nothing: %v", err)
	}
}

func TestReadCredentials(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string, mode os.FileMode) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	saved := `{"url":"https://momentum.example/","token":"mtk_secret","email":"ana@example.com","expiresAt":"2027-01-14T09:30:00Z"}`

	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	got, err := ReadCredentials(write("ok.json", saved, 0o600), "https://momentum.example", now)
	want := Credentials{URL: "https://momentum.example/", Token: "mtk_secret", Email: "ana@example.com", ExpiresAt: time.Date(2027, 1, 14, 9, 30, 0, 0, time.UTC)}
	if err != nil || got != want {
		t.Errorf("credentials = %+v, %v", got, err)
	}

	for _, test := range []struct {
		name, path, want string
		notSignedIn      bool
	}{
		{name: "missing", path: filepath.Join(dir, "missing.json"), want: "not signed in to https://momentum.example", notSignedIn: true},
		{name: "another server", path: write("other.json", strings.Replace(saved, "momentum.example", "other.example", 1), 0o600),
			want: "not signed in to https://momentum.example; the saved sign-in is for https://other.example/", notSignedIn: true},
		{name: "readable by others", path: write("open.json", saved, 0o644), want: "other users can open"},
		{name: "writable by the group", path: write("group.json", saved, 0o620), want: "other users can open"},
		{name: "expired", path: write("expired.json", strings.Replace(saved, "2027-01-14T09:30:00Z", "2026-10-16T12:00:00Z", 1), 0o600),
			want: "not signed in to https://momentum.example; the sign-in expired on 2026-10-16", notSignedIn: true},
		{name: "malformed", path: write("bad.json", `{"token":`, 0o600), want: "holds no sign-in"},
		{name: "no token", path: write("empty.json", `{"url":"https://momentum.example"}`, 0o600), want: "holds no sign-in"},
	} {
		_, err := ReadCredentials(test.path, "https://momentum.example", now)
		if err == nil || !strings.Contains(err.Error(), test.want) || errors.Is(err, ErrNotSignedIn) != test.notSignedIn {
			t.Errorf("%s: error = %v, want %q", test.name, err, test.want)
		}
		if err != nil && strings.Contains(err.Error(), "mtk_secret") {
			t.Errorf("%s: error shows the token: %v", test.name, err)
		}
	}
	if _, err := ReadCredentials(filepath.Join(dir, "expired.json"), "https://momentum.example", time.Time{}); err != nil {
		t.Errorf("zero now: %v", err)
	}
}
