// Package config reads the server's configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Config is the server's configuration.
type Config struct {
	// Port is PORT, which Cloud Run sets, or 8080.
	Port int
	// DatabaseURL is DATABASE_URL, which is required.
	DatabaseURL string
	// PublicURL is PUBLIC_URL, the server's address as browsers reach it, or
	// http://localhost:<port>. Sign-in pages and provider callbacks use it.
	PublicURL string
	// AllowedEmails is ALLOWED_EMAILS, a comma-separated list of the email
	// addresses that may sign in (backend plan D10). Empty, nobody can.
	AllowedEmails []string
	// Google is GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET, and GitHub is
	// GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET. A provider without them is
	// not offered.
	Google, GitHub OAuthClient
	// ProxyHops is TRUSTED_PROXY_HOPS: how many proxies in front of the
	// server append the client's address to X-Forwarded-For, so the sign-in
	// limits can tell clients apart. 0, the default, uses the connection's
	// address and ignores the header, which clients can forge.
	ProxyHops int
}

// OAuthClient is an application registered with a sign-in provider.
type OAuthClient struct{ ID, Secret string }

// Configured reports whether the client is set.
func (c OAuthClient) Configured() bool { return c.ID != "" }

// Load reads the configuration through getenv, which is os.Getenv outside
// tests.
func Load(getenv func(string) string) (Config, error) {
	config := Config{Port: 8080, DatabaseURL: getenv("DATABASE_URL")}
	if value := getenv("PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("PORT must be a number from 1 to 65535, not %q", value)
		}
		config.Port = port
	}
	if config.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	config.PublicURL = strings.TrimSuffix(getenv("PUBLIC_URL"), "/")
	if config.PublicURL == "" {
		config.PublicURL = "http://localhost:" + strconv.Itoa(config.Port)
	}
	if parsed, err := url.Parse(config.PublicURL); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return Config{}, fmt.Errorf("PUBLIC_URL must be an http or https address with no path, not %q", config.PublicURL)
	}
	for _, email := range strings.Split(getenv("ALLOWED_EMAILS"), ",") {
		if email = strings.TrimSpace(email); email != "" {
			config.AllowedEmails = append(config.AllowedEmails, email)
		}
	}
	if value := getenv("TRUSTED_PROXY_HOPS"); value != "" {
		hops, err := strconv.Atoi(value)
		if err != nil || hops < 0 || hops > 10 {
			return Config{}, fmt.Errorf("TRUSTED_PROXY_HOPS must be a number from 0 to 10, not %q", value)
		}
		config.ProxyHops = hops
	}
	var err error
	if config.Google, err = oauthClient(getenv, "GOOGLE"); err != nil {
		return Config{}, err
	}
	if config.GitHub, err = oauthClient(getenv, "GITHUB"); err != nil {
		return Config{}, err
	}
	return config, nil
}

func oauthClient(getenv func(string) string, prefix string) (OAuthClient, error) {
	client := OAuthClient{ID: getenv(prefix + "_CLIENT_ID"), Secret: getenv(prefix + "_CLIENT_SECRET")}
	if (client.ID == "") != (client.Secret == "") {
		return OAuthClient{}, fmt.Errorf("set both %s_CLIENT_ID and %s_CLIENT_SECRET, or neither", prefix, prefix)
	}
	return client, nil
}
