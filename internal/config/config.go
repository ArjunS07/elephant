package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration, read once at startup.
type Config struct {
	DatabaseURL   string
	SecretKey     string
	EncryptionKey string
	PublicHost    string
	ModelboxURL   string
}

// Read config from env
func Load() Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found or error loading .env file")
	}

	publicHost := os.Getenv("PUBLIC_HOST")
	if publicHost == "" {
		publicHost = "http://localhost:8080"
	}

	modelboxURL := os.Getenv("MODELBOX_URL")
	if modelboxURL == "" {
		modelboxURL = "http://127.0.0.1:8081"
	}

	return Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		SecretKey:     os.Getenv("SECRET_KEY"),
		EncryptionKey: os.Getenv("ENCRYPTION_KEY"),
		PublicHost:    publicHost,
		ModelboxURL:   modelboxURL,
	}
}
