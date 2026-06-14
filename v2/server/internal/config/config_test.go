package config

import (
	"strings"
	"testing"
)

func TestLoadRequiresProjectorDatabaseURL(t *testing.T) {
	t.Setenv("COWORK_DATABASE_URL", "postgres://cowork_app:cowork@localhost:5432/coworkos?sslmode=disable")
	t.Setenv("COWORK_PROJECTOR_DATABASE_URL", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "COWORK_PROJECTOR_DATABASE_URL is required") {
		t.Fatalf("want missing projector DSN error, got %v", err)
	}
}

func TestLoadReturnsBothDatabaseURLs(t *testing.T) {
	t.Setenv("COWORK_DATABASE_URL", "postgres://cowork_app:cowork@localhost:5432/coworkos?sslmode=disable")
	t.Setenv("COWORK_PROJECTOR_DATABASE_URL", "postgres://cowork_projector:cowork@localhost:5432/coworkos?sslmode=disable")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DatabaseURL == "" || cfg.ProjectorDatabaseURL == "" || cfg.DatabaseURL == cfg.ProjectorDatabaseURL {
		t.Fatalf("unexpected database URLs: %+v", cfg)
	}
}
