package config

import (
	"log"
	"os"
)

type Config struct {
	Port          string
	InternalKey   string
	DBPath        string
	APIVersion    string
	Environment   string
	RabbitURL     string
	EncryptionKey string
}

func Load() *Config {
	requireEnv := func(key string) string {
		val := os.Getenv(key)
		if val == "" {
			log.Fatalf("Fatal: environment variable %s is required and not set", key)
		}
		return val
	}

	return &Config{
		Port:          requireEnv("WHATSAPP_GO_PORT"),
		InternalKey:   requireEnv("WHATSAPP_GO_INTERNAL_KEY"),
		DBPath:        requireEnv("WHATSAPP_GO_DB_PATH"),
		APIVersion:    requireEnv("WHATSAPP_GO_API_VERSION"),
		Environment:   requireEnv("ENVIRONMENT"),
		RabbitURL:     requireEnv("RABBITMQ_URL"),
		EncryptionKey: requireEnv("WHATSAPP_GO_ENCRYPTION_KEY"),
	}
}
