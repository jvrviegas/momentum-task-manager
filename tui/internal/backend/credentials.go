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

// Credentials are this device's sign-in to one server, which signing in
// saves (T14). The token is sent only to the server that issued it.
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
// url. It refuses a file that other users can read or change, because its
// token signs in as its owner.
func ReadCredentials(path, url string) (Credentials, error) {
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
	if sameServer(saved.URL, url) {
		return saved, nil
	}
	return Credentials{}, fmt.Errorf("%w to %s; the saved sign-in is for %s", ErrNotSignedIn, url, saved.URL)
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
	return ReadCredentials(CredentialsPath(home, os.Getenv("XDG_CONFIG_HOME")), url)
}
