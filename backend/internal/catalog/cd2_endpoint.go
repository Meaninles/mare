package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"mam/backend/internal/connectors"
	"mam/backend/internal/store"
)

type cd2EndpointConfig struct {
	RootPath  string `json:"rootPath"`
	CloudName string `json:"cloudName,omitempty"`
	UserName  string `json:"userName,omitempty"`
}

func (service *Service) buildCD2Connector(endpoint store.StorageEndpoint) (connectors.Connector, error) {
	if service.cd2fsService == nil {
		return nil, errors.New("cd2 file service is not configured")
	}

	config, err := parseCD2EndpointConfig(json.RawMessage(endpoint.ConnectionConfig))
	if err != nil {
		return nil, err
	}

	rootPath := normalizeCD2CatalogPath(defaultString(strings.TrimSpace(config.RootPath), endpoint.RootPath))
	if rootPath == "" || rootPath == "/" {
		return nil, errors.New("cd2 root path is required")
	}
	if err := service.previewCD2Endpoint(context.Background(), rootPath); err != nil {
		return nil, err
	}

	return connectors.NewCD2Connector(connectors.CD2Config{
		Name:     endpoint.Name,
		RootPath: rootPath,
		Service:  service.cd2fsService,
	})
}

func normalizeCD2ConnectionConfig(payload map[string]any) (map[string]any, error) {
	rootPath := normalizeCD2CatalogPath(firstNonEmptyMapString(payload, "rootPath", "root_path", "cd2RootPath", "cd2_root_path"))
	if rootPath == "" || rootPath == "/" {
		return nil, errors.New("cd2 root path is required")
	}

	return map[string]any{
		"rootPath":  rootPath,
		"cloudName": strings.TrimSpace(firstNonEmptyMapString(payload, "cloudName", "cloud_name")),
		"userName":  strings.TrimSpace(firstNonEmptyMapString(payload, "userName", "cloudUserName", "cloud_user_name")),
	}, nil
}

func parseCD2EndpointConfig(raw json.RawMessage) (cd2EndpointConfig, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		raw = json.RawMessage(`{}`)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return cd2EndpointConfig{}, fmt.Errorf("invalid cd2 endpoint config: %w", err)
	}

	normalized, err := normalizeCD2ConnectionConfig(payload)
	if err != nil {
		return cd2EndpointConfig{}, err
	}

	encoded, err := json.Marshal(normalized)
	if err != nil {
		return cd2EndpointConfig{}, err
	}

	var config cd2EndpointConfig
	if err := json.Unmarshal(encoded, &config); err != nil {
		return cd2EndpointConfig{}, err
	}
	return config, nil
}

func normalizeCD2CatalogPath(value string) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(value, `\`, "/"))
	if normalized == "" {
		return ""
	}
	cleaned := path.Clean(normalized)
	if cleaned == "." {
		return ""
	}
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	return cleaned
}

func (service *Service) previewCD2Endpoint(ctx context.Context, rootPath string) error {
	if service.cd2fsService == nil {
		return errors.New("cd2 file service is not configured")
	}
	_, err := service.cd2fsService.Stat(ctx, normalizeCD2CatalogPath(rootPath))
	return err
}
