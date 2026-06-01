package app

import (
	"os"
	"strings"
)

type Config struct {
	HTTPAddr             string
	DatabaseURL          string
	InternalWebhookToken string
	BaileysWorkerURL     string
	EnableDevWebhooks    bool
}

func ConfigFromEnv() Config {
	cfg := Config{
		HTTPAddr:             getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:          getenv("DATABASE_URL", "file:data/app.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"),
		InternalWebhookToken: getenv("INTERNAL_WEBHOOK_TOKEN", ""),
		BaileysWorkerURL:     getenv("BAILEYS_WORKER_URL", "http://127.0.0.1:8090"),
		EnableDevWebhooks:    getenvBool("ENABLE_DEV_WEBHOOKS", false),
	}
	return cfg
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
