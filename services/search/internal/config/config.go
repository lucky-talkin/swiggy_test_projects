package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port             string
	ElasticsearchURL string
	Index            string
	RequestTimeout   time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Port:             env("PORT", "8083"),
		ElasticsearchURL: env("ELASTICSEARCH_URL", "http://localhost:9200"),
		Index:            env("ELASTICSEARCH_INDEX", "food-search-v1"),
		RequestTimeout:   3 * time.Second,
	}
	if value := os.Getenv("ELASTICSEARCH_TIMEOUT"); value != "" {
		timeout, err := time.ParseDuration(value)
		if err != nil || timeout <= 0 {
			return Config{}, fmt.Errorf("ELASTICSEARCH_TIMEOUT must be a positive duration")
		}
		cfg.RequestTimeout = timeout
	}
	if port, err := strconv.Atoi(cfg.Port); err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("PORT must be a valid TCP port")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
