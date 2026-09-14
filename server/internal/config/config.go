package config

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port          string
	DatabaseURL   string
	AllowedOrigin string
	PINPepper     string
	JWTSecret     string
}

func Load() (Config, error) {
	_ = godotenv.Load()
	cfg := Config{
		Port:          envOr("PORT", "8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		AllowedOrigin: envOr("ALLOWED_ORIGIN", "*"),
		PINPepper:     os.Getenv("PIN_PEPPER"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if len(cfg.PINPepper) < 32 {
		return Config{}, errors.New("PIN_PEPPER must be at least 32 characters")
	}
	if len(cfg.JWTSecret) < 32 {
		return Config{}, errors.New("JWT_SECRET must be at least 32 characters")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
