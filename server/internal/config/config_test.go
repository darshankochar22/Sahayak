package config

import "testing"

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PORT", "")
	t.Setenv("ALLOWED_ORIGIN", "")
	t.Setenv("PIN_PEPPER", "pepper-that-is-at-least-thirty-two-bytes")
	t.Setenv("JWT_SECRET", "jwt-secret-that-is-at-least-thirty-two-bytes")

	if _, err := Load(); err == nil {
		t.Fatal("expected missing DATABASE_URL to return an error")
	}
}

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("PORT", "")
	t.Setenv("ALLOWED_ORIGIN", "")
	t.Setenv("PIN_PEPPER", "pepper-that-is-at-least-thirty-two-bytes")
	t.Setenv("JWT_SECRET", "jwt-secret-that-is-at-least-thirty-two-bytes")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Port != "8080" {
		t.Fatalf("expected default port 8080, got %s", cfg.Port)
	}
	if cfg.AllowedOrigin != "*" {
		t.Fatalf("expected default origin *, got %s", cfg.AllowedOrigin)
	}
}

func TestLoadUsesConfiguredValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://configured")
	t.Setenv("PORT", "9090")
	t.Setenv("ALLOWED_ORIGIN", "https://app.example.com")
	t.Setenv("PIN_PEPPER", "pepper-that-is-at-least-thirty-two-bytes")
	t.Setenv("JWT_SECRET", "jwt-secret-that-is-at-least-thirty-two-bytes")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.DatabaseURL != "postgres://configured" || cfg.Port != "9090" || cfg.AllowedOrigin != "https://app.example.com" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadRequiresLongAuthenticationSecrets(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("PIN_PEPPER", "short")
	t.Setenv("JWT_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected short secrets to fail")
	}
}
