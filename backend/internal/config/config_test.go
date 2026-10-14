package config

import "testing"

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadDefaultsPortTo8080(t *testing.T) {
	config, err := Load(env(nil))
	if err != nil || config.Port != 8080 {
		t.Fatalf("Load() = %+v, %v; want port 8080", config, err)
	}
}

func TestLoadReadsPort(t *testing.T) {
	config, err := Load(env(map[string]string{"PORT": "9090"}))
	if err != nil || config.Port != 9090 {
		t.Fatalf("Load() = %+v, %v; want port 9090", config, err)
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	for _, port := range []string{"http", "0", "65536", "-1", "80 "} {
		if config, err := Load(env(map[string]string{"PORT": port})); err == nil {
			t.Errorf("PORT=%q: Load() = %+v, want an error", port, config)
		}
	}
}
