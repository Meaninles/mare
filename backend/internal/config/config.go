package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppName                     string
	AppEnv                      string
	HTTPHost                    string
	HTTPPort                    int
	HTTPReadTimeout             time.Duration
	HTTPWriteTimeout            time.Duration
	HTTPIdleTimeout             time.Duration
	HTTPShutdownTimeout         time.Duration
	CatalogDBPath               string
	PostgresDSN                 string
	RedisAddr                   string
	OpenSearchURL               string
	RabbitMQURL                 string
	MinIOEndpoint               string
	MinIOBucket                 string
	FFmpegPath                  string
	AIServiceURL                string
	LogFilePath                 string
	CD2Enabled                  bool
	CD2Mode                     string
	CD2BaseURL                  string
	CD2ExpectedName             string
	CD2ExpectedVersion          string
	CD2ProbeTimeout             time.Duration
	CD2GRPCTarget               string
	CD2GRPCUseTLS               bool
	CD2GRPCDialTimeout          time.Duration
	CD2GRPCRequestTimeout       time.Duration
	CD2AuthUserName             string
	CD2AuthPassword             string
	CD2AuthAPIToken             string
	CD2AuthProfilePath          string
	CD2ManagedTokenRef          string
	CD2ManagedTokenFriendlyName string
	CD2ManagedTokenRootDir      string
	CD2PersistManagedToken      bool
}

func Load() (Config, error) {
	catalogDBPath := getString("CATALOG_DB_PATH", "./data/mam.db")
	httpPort, err := getInt("HTTP_PORT", 8080)
	if err != nil {
		return Config{}, err
	}

	readTimeout, err := getDuration("HTTP_READ_TIMEOUT", 5*time.Minute)
	if err != nil {
		return Config{}, err
	}

	writeTimeout, err := getDuration("HTTP_WRITE_TIMEOUT", 5*time.Minute)
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

	cd2ProbeTimeout, err := getDuration("CD2_PROBE_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	cd2BaseURL := getString("CD2_BASE_URL", "http://127.0.0.1:29798")

	cd2GRPCDialTimeout, err := getDuration("CD2_GRPC_DIAL_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	cd2GRPCRequestTimeout, err := getDuration("CD2_GRPC_REQUEST_TIMEOUT", 15*time.Second)
	if err != nil {
		return Config{}, err
	}

	return Config{
		AppName:                     getString("APP_NAME", "MAM Backend"),
		AppEnv:                      getString("APP_ENV", "development"),
		HTTPHost:                    getString("HTTP_HOST", "0.0.0.0"),
		HTTPPort:                    httpPort,
		HTTPReadTimeout:             readTimeout,
		HTTPWriteTimeout:            writeTimeout,
		HTTPIdleTimeout:             idleTimeout,
		HTTPShutdownTimeout:         shutdownTimeout,
		CatalogDBPath:               catalogDBPath,
		PostgresDSN:                 getString("POSTGRES_DSN", ""),
		RedisAddr:                   getString("REDIS_ADDR", ""),
		OpenSearchURL:               getString("OPENSEARCH_URL", ""),
		RabbitMQURL:                 getString("RABBITMQ_URL", ""),
		MinIOEndpoint:               getString("MINIO_ENDPOINT", ""),
		MinIOBucket:                 getString("MINIO_BUCKET", ""),
		FFmpegPath:                  getString("FFMPEG_PATH", "ffmpeg"),
		AIServiceURL:                getString("AI_SERVICE_URL", ""),
		LogFilePath:                 getString("LOG_FILE_PATH", filepath.Join(".", "data", "logs", "backend.log")),
		CD2Enabled:                  getBool("CD2_ENABLED", true),
		CD2Mode:                     getString("CD2_MODE", "external"),
		CD2BaseURL:                  cd2BaseURL,
		CD2ExpectedName:             getString("CD2_EXPECTED_NAME", "CloudDrive2"),
		CD2ExpectedVersion:          getString("CD2_EXPECTED_VERSION", ""),
		CD2ProbeTimeout:             cd2ProbeTimeout,
		CD2GRPCTarget:               getString("CD2_GRPC_TARGET", defaultCD2GRPCTarget(cd2BaseURL)),
		CD2GRPCUseTLS:               getBool("CD2_GRPC_USE_TLS", false),
		CD2GRPCDialTimeout:          cd2GRPCDialTimeout,
		CD2GRPCRequestTimeout:       cd2GRPCRequestTimeout,
		CD2AuthUserName:             getString("CD2_AUTH_USER_NAME", ""),
		CD2AuthPassword:             getString("CD2_AUTH_PASSWORD", ""),
		CD2AuthAPIToken:             getString("CD2_AUTH_API_TOKEN", ""),
		CD2AuthProfilePath:          getString("CD2_AUTH_PROFILE_PATH", ""),
		CD2ManagedTokenRef:          getString("CD2_MANAGED_TOKEN_REF", "cred_cd2_managed_token"),
		CD2ManagedTokenFriendlyName: getString("CD2_MANAGED_TOKEN_FRIENDLY_NAME", "mam-backend"),
		CD2ManagedTokenRootDir:      getString("CD2_MANAGED_TOKEN_ROOT_DIR", "/"),
		CD2PersistManagedToken:      getBool("CD2_PERSIST_MANAGED_TOKEN", true),
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

func getBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	switch value {
	case "1", "true", "TRUE", "True", "yes", "YES", "Yes", "on", "ON", "On":
		return true
	case "0", "false", "FALSE", "False", "no", "NO", "No", "off", "OFF", "Off":
		return false
	default:
		return fallback
	}
}

func defaultCD2GRPCTarget(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return "127.0.0.1:29798"
	}

	parsed, err := url.Parse(trimmed)
	if err == nil && strings.TrimSpace(parsed.Host) != "" {
		return parsed.Host
	}

	trimmed = strings.TrimPrefix(trimmed, "http://")
	trimmed = strings.TrimPrefix(trimmed, "https://")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "127.0.0.1:29798"
	}
	return trimmed
}
