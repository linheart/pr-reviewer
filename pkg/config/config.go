package config

import (
	"os"
	"time"
)

type Config struct {
	Port        string
	DatabaseURL string

	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

func FromEnv() Config {
	return Config{
		Port:        getenv("PORT", "8080"),
		DatabaseURL: getenv("DATABASE_URL", "postgres://postgres:postgres@db:5432/app?sslmode=disable"),

		ReadTimeout:  parseDuration("SERVER_READ_TIMEOUT", 15*time.Second),
		WriteTimeout: parseDuration("SERVER_WRITE_TIMEOUT", 15*time.Second),
		IdleTimeout:  parseDuration("SERVER_IDLE_TIMEOUT", 60*time.Second),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseDuration(env string, def time.Duration) time.Duration {
	if v := os.Getenv(env); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
