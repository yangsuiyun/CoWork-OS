// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
)

// Config holds the server runtime configuration.
type Config struct {
	Addr string // HTTP listen address, e.g. ":8080"
	// DatabaseURL is the request-path DSN; it MUST use an RLS-scoped role
	// (cowork_app), never a superuser, so tenant isolation is enforced.
	DatabaseURL string
	// ProjectorDatabaseURL is the system-consumer DSN; it uses a BYPASSRLS
	// role (cowork_projector) to read all tenants. Defaults to DatabaseURL.
	ProjectorDatabaseURL string
	JWTSecret            string // HMAC secret for verifying client JWTs
}

// Load reads configuration from environment variables, applying defaults for
// local development. DatabaseURL is required.
func Load() (Config, error) {
	c := Config{
		Addr:                 getenv("COWORK_ADDR", ":8080"),
		DatabaseURL:          os.Getenv("COWORK_DATABASE_URL"),
		ProjectorDatabaseURL: os.Getenv("COWORK_PROJECTOR_DATABASE_URL"),
		JWTSecret:            getenv("COWORK_JWT_SECRET", "dev-insecure-secret"),
	}
	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("COWORK_DATABASE_URL is required")
	}
	if c.ProjectorDatabaseURL == "" {
		c.ProjectorDatabaseURL = c.DatabaseURL
	}
	return c, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
