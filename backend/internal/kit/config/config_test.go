package config

import (
	"os"
	"testing"
)

func TestLoad_Success(t *testing.T) {
	envs := map[string]string{
		"WHATSAPP_GO_PORT":           "8080",
		"WHATSAPP_GO_INTERNAL_KEY":   "test-internal-key",
		"WHATSAPP_GO_DB_PATH":        "/tmp/test.db",
		"WHATSAPP_GO_API_VERSION":    "v1",
		"ENVIRONMENT":                "development",
		"RABBITMQ_URL":               "amqp://guest:guest@localhost:5672/",
		"WHATSAPP_GO_ENCRYPTION_KEY": "01234567890123456789012345678901",
	}

	for k, v := range envs {
		os.Setenv(k, v)
		defer os.Unsetenv(k)
	}

	cfg := Load()
	if cfg.Port != "8080" {
		t.Errorf("expected Port to be 8080, got %s", cfg.Port)
	}
	if cfg.InternalKey != "test-internal-key" {
		t.Errorf("expected InternalKey to be test-internal-key, got %s", cfg.InternalKey)
	}
	if cfg.DBPath != "/tmp/test.db" {
		t.Errorf("expected DBPath to be /tmp/test.db, got %s", cfg.DBPath)
	}
	if cfg.APIVersion != "v1" {
		t.Errorf("expected APIVersion to be v1, got %s", cfg.APIVersion)
	}
	if cfg.Environment != "development" {
		t.Errorf("expected Environment to be development, got %s", cfg.Environment)
	}
	if cfg.RabbitURL != "amqp://guest:guest@localhost:5672/" {
		t.Errorf("expected RabbitURL to match, got %s", cfg.RabbitURL)
	}
	if cfg.EncryptionKey != "01234567890123456789012345678901" {
		t.Errorf("expected EncryptionKey to match, got %s", cfg.EncryptionKey)
	}
}
