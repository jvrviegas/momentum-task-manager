package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jvrviegas/momentum-task-manager/tui/internal/config"
)

// Credentials are this device's sign-in to one server, which momentum login
// saves. The token is sent only to the server that issued it.
type Credentials struct {
	URL       string    `json:"url"`
	Token     string    `json:"token"`
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// ErrNotSignedIn reports that no credentials are saved for the server.
var ErrNotSignedIn = errors.New("not signed in")

// CredentialsPath returns where credentials are saved: credentials.json in
// Momentum's config directory, whichever config file --config names.
func CredentialsPath(homeDir, xdgConfigHome string) string {
	return filepath.Join(filepath.Dir(config.ResolvePathFor("", homeDir, xdgConfigHome)), "credentials.json")
}

// ReadCredentials returns the credentials saved at path for the server at
// url, unless they expired by now; a zero now accepts any expiry. It refuses
// a file that other users can read or change, because its token signs in as
// its owner.
func ReadCredentials(path, url string, now time.Time) (Credentials, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Credentials{}, fmt.Errorf("%w to %s", ErrNotSignedIn, url)
	}
	if err != nil {
		return Credentials{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Credentials{}, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return Credentials{}, fmt.Errorf("other users can open %s; run chmod 600 on it", path)
	}
	var saved Credentials
	if err := json.NewDecoder(file).Decode(&saved); err != nil || saved.Token == "" {
		return Credentials{}, fmt.Errorf("%s holds no sign-in; sign in again", path)
	}
	if !sameServer(saved.URL, url) {
		return Credentials{}, fmt.Errorf("%w to %s; the saved sign-in is for %s", ErrNotSignedIn, url, saved.URL)
	}
	if !saved.ExpiresAt.IsZero() && !now.Before(saved.ExpiresAt) {
		return Credentials{}, fmt.Errorf("%w to %s; the sign-in expired on %s", ErrNotSignedIn, url, saved.ExpiresAt.Local().Format(time.DateOnly))
	}
	return saved, nil
}

// SaveCredentials saves saved at path, readable only by this user. It
// replaces the file in one step, so a reader sees the old sign-in or the new
// one.
func SaveCredentials(path string, saved Credentials) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".credentials-*.json") // mode 0600
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := json.NewEncoder(file).Encode(saved); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// RemoveCredentials removes the credentials saved at path, if any.
func RemoveCredentials(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func sameServer(left, right string) bool {
	return strings.TrimSuffix(left, "/") == strings.TrimSuffix(right, "/")
}

// SavedCredentials reads the credentials saved in this user's config
// directory for the server at url.
func SavedCredentials(url string) (Credentials, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Credentials{}, err
	}
	return ReadCredentials(CredentialsPath(home, os.Getenv("XDG_CONFIG_HOME")), url, time.Now())
}
