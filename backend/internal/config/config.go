package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppName             string
	AppEnv              string
	HTTPHost            string
	HTTPPort            int
	HTTPReadTimeout     time.Duration
	HTTPWriteTimeout    time.Duration
	HTTPIdleTimeout     time.Duration
	HTTPShutdownTimeout time.Duration
	CatalogDBPath       string
	PostgresDSN         string
	RedisAddr           string
	OpenSearchURL       string
	RabbitMQURL         string
	MinIOEndpoint       string
	MinIOBucket         string
	FFmpegPath          string
	AIServiceURL        string
}

func Load() (Config, error) {
	httpPort, err := getInt("HTTP_PORT", 8080)
	if err != nil {
		return Config{}, err
	}

	readTimeout, err := getDuration("HTTP_READ_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}

	writeTimeout, err := getDuration("HTTP_WRITE_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}

	idleTimeout, err := getDuration("HTTP_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := getDuration("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	return Config{
		AppName:             getString("APP_NAME", "MAM Backend"),
		AppEnv:              getString("APP_ENV", "development"),
		HTTPHost:            getString("HTTP_HOST", "0.0.0.0"),
		HTTPPort:            httpPort,
		HTTPReadTimeout:     readTimeout,
		HTTPWriteTimeout:    writeTimeout,
		HTTPIdleTimeout:     idleTimeout,
		HTTPShutdownTimeout: shutdownTimeout,
		CatalogDBPath:       getString("CATALOG_DB_PATH", "./data/mam.db"),
		PostgresDSN:         getString("POSTGRES_DSN", ""),
		RedisAddr:           getString("REDIS_ADDR", ""),
		OpenSearchURL:       getString("OPENSEARCH_URL", ""),
		RabbitMQURL:         getString("RABBITMQ_URL", ""),
		MinIOEndpoint:       getString("MINIO_ENDPOINT", ""),
		MinIOBucket:         getString("MINIO_BUCKET", ""),
		FFmpegPath:          getString("FFMPEG_PATH", "ffmpeg"),
		AIServiceURL:        getString("AI_SERVICE_URL", ""),
	}, nil
}

func (cfg Config) HTTPAddress() string {
	return fmt.Sprintf("%s:%d", cfg.HTTPHost, cfg.HTTPPort)
}

func getString(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func getInt(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}

	return parsed, nil
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", key, err)
	}

	return parsed, nil
}
