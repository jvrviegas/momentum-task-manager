package config

import (
	"strings"
	"testing"
)

func TestBackendURLSelectsBackendMode(t *testing.T) {
	got, err := LoadFile(writeConfig(t, "[backend]\nurl = \"https://momentum.example\"\n"))
	if err != nil || got.Backend.URL != "https://momentum.example" || !got.Backend.Enabled() {
		t.Fatalf("config %#v, %v", got.Backend, err)
	}
	if got, err := LoadFile(writeConfig(t, "theme = \"dark\"\n")); err != nil || got.Backend.Enabled() {
		t.Fatalf("without a backend section: %#v, %v", got.Backend, err)
	}
	if _, err := LoadFile(writeConfig(t, "[backend]\ntoken = \"mtk_secret\"\n")); err == nil || !strings.Contains(err.Error(), "backend.token") {
		t.Fatalf("unknown backend key: %v", err)
	}
}

func TestBackendURLMustBeASecureServerAddress(t *testing.T) {
	for _, value := range []string{
		"https://momentum.example", "https://momentum.example/", "https://momentum.example:8443",
		"http://localhost:8080", "http://127.0.0.1:8080", "http://[::1]:8080",
	} {
		if err := (BackendConfig{URL: value}).Validate(); err != nil {
			t.Errorf("%q: %v", value, err)
		}
	}
	for value, want := range map[string]string{
		"http://momentum.example":                "must use https",
		"http://192.168.1.10:8080":               "must use https",
		"ftp://momentum.example":                 "must use https",
		"momentum.example":                       "must be a server address",
		"https://":                               "must be a server address",
		"https://user:secret@momentum.example":   "must be a server address",
		"https://momentum.example/v1":            "must be a server address",
		"https://momentum.example/?token=mtk_x":  "must be a server address",
		"https://momentum.example/?":             "must be a server address",
		"https://momentum.example/#fragment":     "must be a server address",
		"https://momentum example":               "must be a server address",
		"http://localhost.momentum.example:8080": "must use https",
		"http://127.0.0.1.momentum.example":      "must use https",
	} {
		err := (BackendConfig{URL: value}).Validate()
		if err == nil || !strings.Contains(err.Error(), "backend.url") || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error %v, want %q", value, err, want)
		}
	}
}
