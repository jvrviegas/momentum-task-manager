package config

import (
	"bytes"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
)

// env returns a getenv over DATABASE_URL and values.
func env(values map[string]string) func(string) string {
	return func(key string) string {
		if value, ok := values[key]; ok {
			return value
		}
		if key == "DATABASE_URL" {
			return "postgres://localhost/momentum"
		}
		return ""
	}
}

func TestLoadDefaults(t *testing.T) {
	config, err := Load(env(nil))
	want := Config{Port: 8080, DatabaseURL: "postgres://localhost/momentum", PublicURL: "http://localhost:8080"}
	if err != nil || !reflect.DeepEqual(config, want) {
		t.Fatalf("Load() = %+v, %v; want %+v", config, err, want)
	}
}

func TestLoadReadsEveryVariable(t *testing.T) {
	config, err := Load(env(map[string]string{
		"PORT": "9090", "PUBLIC_URL": "https://momentum.example/", "ALLOWED_EMAILS": " alice@example.com, ,bob@example.com ",
		"GOOGLE_CLIENT_ID": "google-id", "GOOGLE_CLIENT_SECRET": "google-secret",
		"GITHUB_CLIENT_ID": "github-id", "GITHUB_CLIENT_SECRET": "github-secret", "TRUSTED_PROXY_HOPS": "1",
	}))
	want := Config{
		Port: 9090, DatabaseURL: "postgres://localhost/momentum", PublicURL: "https://momentum.example",
		AllowedEmails: []string{"alice@example.com", "bob@example.com"},
		Google:        OAuthClient{"google-id", "google-secret"}, GitHub: OAuthClient{"github-id", "github-secret"},
		ProxyHops: 1,
	}
	if err != nil || !reflect.DeepEqual(config, want) {
		t.Fatalf("Load() = %+v, %v; want %+v", config, err, want)
	}
	if !config.Google.Configured() || (OAuthClient{}).Configured() {
		t.Fatal("Configured() is wrong")
	}
}

func TestLoadReadsTheCalendarTokenKey(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	encoded := base64.StdEncoding.EncodeToString(key)
	// A trailing newline is what `echo` leaves in a Secret Manager value.
	for _, value := range []string{encoded, encoded + "\n", " " + encoded + " "} {
		config, err := Load(env(map[string]string{"CALENDAR_TOKEN_KEY": value}))
		if err != nil || !bytes.Equal(config.CalendarTokenKey, key) {
			t.Errorf("CALENDAR_TOKEN_KEY %q: Load() = %v, %v; want the key", value, config.CalendarTokenKey, err)
		}
	}
	// No key turns Google Calendar off; it is not an error.
	if config, err := Load(env(nil)); err != nil || config.CalendarTokenKey != nil {
		t.Errorf("without a key: Load() = %v, %v; want none", config.CalendarTokenKey, err)
	}
}

func TestLoadRefusesACalendarTokenKeyThatIsNot32BytesOfBase64(t *testing.T) {
	for name, value := range map[string]string{
		"not base64":   "this is not base64!",
		"16 bytes":     base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 16)),
		"31 bytes":     base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 31)),
		"33 bytes":     base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 33)),
		"unpadded":     strings.TrimRight(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)), "="),
		"32 raw bytes": strings.Repeat("k", 32),
	} {
		_, err := Load(env(map[string]string{"CALENDAR_TOKEN_KEY": value}))
		if err == nil {
			t.Errorf("%s: Load() succeeded, want an error", name)
			continue
		}
		if !strings.Contains(err.Error(), "CALENDAR_TOKEN_KEY") || strings.Contains(err.Error(), value) {
			t.Errorf("%s: error %q must name the variable and leave its value out", name, err)
		}
	}
}

func TestLoadDefaultsPublicURLToThePort(t *testing.T) {
	config, err := Load(env(map[string]string{"PORT": "9090"}))
	if err != nil || config.PublicURL != "http://localhost:9090" {
		t.Fatalf("Load() = %+v, %v", config, err)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	for _, values := range []map[string]string{
		{"PORT": "http"}, {"PORT": "0"}, {"PORT": "65536"}, {"PORT": "-1"}, {"PORT": "80 "},
		{"DATABASE_URL": ""},
		{"PUBLIC_URL": "momentum.example"}, {"PUBLIC_URL": "ftp://momentum.example"},
		{"PUBLIC_URL": "https://momentum.example/app"}, {"PUBLIC_URL": "https://momentum.example?x=1"},
		{"GOOGLE_CLIENT_ID": "google-id"}, {"GITHUB_CLIENT_SECRET": "github-secret"},
		{"TRUSTED_PROXY_HOPS": "-1"}, {"TRUSTED_PROXY_HOPS": "11"}, {"TRUSTED_PROXY_HOPS": "one"},
	} {
		if config, err := Load(env(values)); err == nil {
			t.Errorf("%v: Load() = %+v, want an error", values, config)
		}
	}
}
