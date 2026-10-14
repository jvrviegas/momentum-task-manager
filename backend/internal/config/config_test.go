package config

import (
	"reflect"
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
		"GITHUB_CLIENT_ID": "github-id", "GITHUB_CLIENT_SECRET": "github-secret",
	}))
	want := Config{
		Port: 9090, DatabaseURL: "postgres://localhost/momentum", PublicURL: "https://momentum.example",
		AllowedEmails: []string{"alice@example.com", "bob@example.com"},
		Google:        OAuthClient{"google-id", "google-secret"}, GitHub: OAuthClient{"github-id", "github-secret"},
	}
	if err != nil || !reflect.DeepEqual(config, want) {
		t.Fatalf("Load() = %+v, %v; want %+v", config, err, want)
	}
	if !config.Google.Configured() || (OAuthClient{}).Configured() {
		t.Fatal("Configured() is wrong")
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
	} {
		if config, err := Load(env(values)); err == nil {
			t.Errorf("%v: Load() = %+v, want an error", values, config)
		}
	}
}
