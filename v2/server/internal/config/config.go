// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
)

// Config holds the server runtime configuration.
type Config struct {
	Addr        string // HTTP listen address, e.g. ":8080"
	DatabaseURL string // Postgres DSN
	JWTSecret   string // HMAC secret for verifying client JWTs
}

// Load reads configuration from environment variables, applying defaults for
// local development. DatabaseURL is required.
func Load() (Config, error) {
	c := Config{
		Addr:        getenv("COWORK_ADDR", ":8080"),
		DatabaseURL: os.Getenv("COWORK_DATABASE_URL"),
		JWTSecret:   getenv("COWORK_JWT_SECRET", "dev-insecure-secret"),
	}
	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("COWORK_DATABASE_URL is required")
	}
	return c, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
