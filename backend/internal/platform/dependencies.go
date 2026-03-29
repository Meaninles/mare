package platform

import "mam/backend/internal/config"

type DependencyState struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
	Target     string `json:"target"`
}

type DependencySnapshot struct {
	Items []DependencyState `json:"items"`
}

func NewDependencySnapshot(cfg config.Config) DependencySnapshot {
	snapshot := DependencySnapshot{
		Items: []DependencyState{
			{Name: "postgres", Configured: cfg.PostgresDSN != "", Target: cfg.PostgresDSN},
			{Name: "redis", Configured: cfg.RedisAddr != "", Target: cfg.RedisAddr},
			{Name: "opensearch", Configured: cfg.OpenSearchURL != "", Target: cfg.OpenSearchURL},
			{Name: "rabbitmq", Configured: cfg.RabbitMQURL != "", Target: cfg.RabbitMQURL},
			{Name: "minio", Configured: cfg.MinIOEndpoint != "", Target: cfg.MinIOEndpoint},
			{Name: "ffmpeg", Configured: cfg.FFmpegPath != "", Target: cfg.FFmpegPath},
			{Name: "ai-service", Configured: cfg.AIServiceURL != "", Target: cfg.AIServiceURL},
			{Name: "clouddrive2", Configured: cfg.CD2Enabled, Target: cfg.CD2BaseURL},
			{Name: "clouddrive2-grpc", Configured: cfg.CD2Enabled, Target: cfg.CD2GRPCTarget},
		},
	}
	return snapshot
}
