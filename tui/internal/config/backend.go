package config

import (
	"fmt"
	"net"
	"net/url"
)

// BackendConfig selects backend mode: with a URL, Momentum keeps tasks on
// that Momentum server instead of in Taskwarrior (backend plan T13).
type BackendConfig struct {
	URL string `toml:"url"`
}

// Enabled reports whether the config selects backend mode.
func (c BackendConfig) Enabled() bool { return c.URL != "" }

// Validate accepts a server's address, such as https://momentum.example. It
// must use HTTPS, because requests carry the sign-in token, unless the server
// is on this machine.
func (c BackendConfig) Validate() error {
	if c.URL == "" {
		return nil
	}
	parsed, err := url.Parse(c.URL)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return fmt.Errorf("config key %q must be a server address such as https://momentum.example", "backend.url")
	}
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !loopback(parsed.Hostname())) {
		return fmt.Errorf("config key %q must use https, or http for a server on this machine", "backend.url")
	}
	return nil
}

func loopback(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}
