package config

import (
	"log"
	"os"
)

type Config struct {
	Port               string
	InternalKey        string
	DBPath             string
	APIVersion         string
	Environment        string
	RabbitURL          string
	EncryptionKey      string
	BackendURL         string
	BackendInternalKey string
	MediaTempDir       string
}

func Load() *Config {
	requireEnv := func(key string) string {
		val := os.Getenv(key)
		if val == "" {
			log.Fatalf("Fatal: environment variable %s is required and not set", key)
		}
		return val
	}

	mediaTempDir := os.Getenv("MEDIA_TEMP_DIR")
	if mediaTempDir == "" {
		mediaTempDir = os.TempDir()
	}

	return &Config{
		Port:               requireEnv("CHATNIP_PORT"),
		InternalKey:        requireEnv("CHATNIP_INTERNAL_KEY"),
		DBPath:             requireEnv("CHATNIP_DB_PATH"),
		APIVersion:         requireEnv("CHATNIP_API_VERSION"),
		Environment:        requireEnv("ENVIRONMENT"),
		RabbitURL:          requireEnv("RABBITMQ_URL"),
		EncryptionKey:      requireEnv("CHATNIP_ENCRYPTION_KEY"),
		BackendURL:         os.Getenv("BACKEND_API_URL"),
		BackendInternalKey: os.Getenv("BACKEND_INTERNAL_API_KEY"),
		MediaTempDir:       mediaTempDir,
	}
}
