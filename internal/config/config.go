package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration, read once at startup.
type Config struct {
	DatabaseURL   string
	SecretKey     string
	EncryptionKey string
	PublicHost    string
}

// Load reads configuration from the environment. A .env file is loaded
// best-effort (ignored if absent, since env may be provided another way).
func Load() Config {
	_ = godotenv.Load()

	publicHost := os.Getenv("PUBLIC_HOST")
	if publicHost == "" {
		publicHost = "http://localhost:8080"
	}

	return Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		SecretKey:     os.Getenv("SECRET_KEY"),
		EncryptionKey: os.Getenv("ENCRYPTION_KEY"),
		PublicHost:    publicHost,
	}
}
