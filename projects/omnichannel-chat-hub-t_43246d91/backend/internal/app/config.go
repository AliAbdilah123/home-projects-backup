package app

import "os"

type Config struct {
	HTTPAddr             string
	DatabaseURL          string
	InternalWebhookToken string
}

func ConfigFromEnv() Config {
	cfg := Config{
		HTTPAddr:             getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:          getenv("DATABASE_URL", "file:data/app.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"),
		InternalWebhookToken: getenv("INTERNAL_WEBHOOK_TOKEN", ""),
	}
	return cfg
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
