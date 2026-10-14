// Package config reads the server's configuration from environment variables.
package config

import (
	"fmt"
	"strconv"
)

// Config is the server's configuration.
type Config struct {
	// Port is PORT, which Cloud Run sets, or 8080.
	Port int
}

// Load reads the configuration through getenv, which is os.Getenv outside
// tests.
func Load(getenv func(string) string) (Config, error) {
	config := Config{Port: 8080}
	if value := getenv("PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("PORT must be a number from 1 to 65535, not %q", value)
		}
		config.Port = port
	}
	return config, nil
}
