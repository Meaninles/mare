package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"mam/backend/internal/connectors"
	sidecaralist "mam/backend/internal/sidecars/alist"
	"mam/backend/internal/store"
)

const (
	networkStorageProvider115       = "115"
	networkStorageDriver115Cloud    = "115 Cloud"
	defaultNetworkStoragePageSize   = 1000
	defaultNetworkStorageCacheTTL   = 30
	defaultNetworkStorageLoginMode  = "manual"
	networkStorageLoginModeQRCode   = "qrcode"
	networkStorageLoginModeManual   = "manual"
)

type networkStorageEndpointConfig struct {
	Provider     string `json:"provider"`
	StorageKey   string `json:"storageKey"`
	MountPath    string `json:"mountPath"`
	Driver       string `json:"driver"`
	RootFolderID string `json:"rootFolderId"`
	LoginMethod  string `json:"loginMethod"`
	AppType      string `json:"appType"`
	PageSize     int    `json:"pageSize"`
	Credential   string `json:"credential,omitempty"`
}

func (service *Service) buildNetworkStorageConnector(
	ctx context.Context,
	endpoint store.StorageEndpoint,
) (connectors.Connector, error) {
	config, runtime, err := service.prepareNetworkStorageEndpoint(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	return connectors.NewAListConnector(connectors.AListConfig{
		Name:            endpoint.Name,
		RootPath:        config.MountPath,
		MountPath:       config.MountPath,
		Driver:          config.Driver,
		Addition:        config.Addition,
		Remark:          fmt.Sprintf("network-storage:%s", endpoint.ID),
		CacheExpiration: defaultNetworkStorageCacheTTL,
	}, runtime)
}

func (service *Service) prepareNetworkStorageEndpoint(
	ctx context.Context,
	endpoint store.StorageEndpoint,
) (alistEndpointConfig, *sidecaralist.Runtime, error) {
	config, err := parseNetworkStorageEndpointConfig(json.RawMessage(endpoint.ConnectionConfig))
	if err != nil {
		return alistEndpointConfig{}, nil, err
	}
	if strings.TrimSpace(config.Credential) == "" {
		return alistEndpointConfig{}, nil, errors.New("115 网盘凭证不能为空，请先扫码登录或手动填写凭证")
	}

	storageSpec, err := buildNetworkStorageStorageSpec(config)
	if err != nil {
		return alistEndpointConfig{}, nil, err
	}

	runtime := service.getAListRuntime()
	if _, err := runtime.EnsureStorage(ctx, storageSpec); err != nil {
		return alistEndpointConfig{}, nil, err
	}

	return alistEndpointConfig{
		MountPath:       storageSpec.MountPath,
		Driver:          storageSpec.Driver,
		Addition:        storageSpec.Addition,
		CacheExpiration: storageSpec.CacheExpiration,
	}, runtime, nil
}

func normalizeNetworkStorageConnectionConfig(payload map[string]any) (map[string]any, error) {
	provider := normalizeNetworkStorageProvider(firstNonEmptyMapString(payload, "provider"))
	if provider == "" {
		provider = networkStorageProvider115
	}
	if provider != networkStorageProvider115 {
		return nil, fmt.Errorf("unsupported network storage provider: %s", provider)
	}

	storageKey := strings.TrimSpace(firstNonEmptyMapString(payload, "storageKey"))
	if storageKey == "" {
		storageKey = uuid.NewString()
	}

	mountPath := normalizeAListEndpointPath(firstNonEmptyMapString(payload, "mountPath"))
	if mountPath == "" {
		mountPath = networkStorageMountPath(provider, storageKey)
	}

	rootFolderID := strings.TrimSpace(firstNonEmptyMapString(payload, "rootFolderId", "root_folder_id"))
	if rootFolderID == "" {
		rootFolderID = "0"
	}

	driver := strings.TrimSpace(firstNonEmptyMapString(payload, "driver"))
	if driver == "" {
		driver = networkStorageDriverForProvider(provider)
	}

	loginMethod := normalizeNetworkStorageLoginMethod(firstNonEmptyMapString(payload, "loginMethod", "login_mode"))
	credential := strings.TrimSpace(firstNonEmptyMapString(payload, "credential", "cookie", "accessToken", "token"))
	if loginMethod == "" {
		if credential != "" {
			loginMethod = networkStorageLoginModeManual
		} else {
			loginMethod = networkStorageLoginModeQRCode
		}
	}

	pageSize := defaultNetworkStoragePageSize
	switch value := payload["pageSize"].(type) {
	case float64:
		if value > 0 {
			pageSize = int(value)
		}
	case int:
		if value > 0 {
			pageSize = value
		}
	}

	normalized := map[string]any{
		"provider":     provider,
		"storageKey":   storageKey,
		"mountPath":    mountPath,
		"driver":       driver,
		"rootFolderId": rootFolderID,
		"loginMethod":  loginMethod,
		"appType":      normalizeNetworkStorageAppType(firstNonEmptyMapString(payload, "appType")),
		"pageSize":     pageSize,
	}
	if credential != "" {
		normalized["credential"] = credential
	}

	return normalized, nil
}

func parseNetworkStorageEndpointConfig(raw json.RawMessage) (networkStorageEndpointConfig, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		raw = json.RawMessage(`{}`)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return networkStorageEndpointConfig{}, fmt.Errorf("invalid network storage config: %w", err)
	}

	normalized, err := normalizeNetworkStorageConnectionConfig(payload)
	if err != nil {
		return networkStorageEndpointConfig{}, err
	}

	encoded, err := json.Marshal(normalized)
	if err != nil {
		return networkStorageEndpointConfig{}, err
	}

	var config networkStorageEndpointConfig
	if err := json.Unmarshal(encoded, &config); err != nil {
		return networkStorageEndpointConfig{}, err
	}
	return config, nil
}

func buildNetworkStorageStorageSpec(config networkStorageEndpointConfig) (sidecaralist.StorageSpec, error) {
	switch normalizeNetworkStorageProvider(config.Provider) {
	case networkStorageProvider115:
		addition, err := normalizeAListAdditionValue(map[string]any{
			"cookie":         strings.TrimSpace(config.Credential),
			"root_folder_id": defaultString(strings.TrimSpace(config.RootFolderID), "0"),
			"page_size":      max(config.PageSize, defaultNetworkStoragePageSize),
		})
		if err != nil {
			return sidecaralist.StorageSpec{}, err
		}

		return sidecaralist.StorageSpec{
			MountPath:       normalizeAListEndpointPath(config.MountPath),
			Driver:          networkStorageDriver115Cloud,
			Addition:        addition,
			Remark:          fmt.Sprintf("mare-network:%s", strings.TrimSpace(config.StorageKey)),
			CacheExpiration: defaultNetworkStorageCacheTTL,
		}, nil
	default:
		return sidecaralist.StorageSpec{}, fmt.Errorf("unsupported network storage provider: %s", config.Provider)
	}
}

func normalizeNetworkStorageProvider(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "115", "CLOUD115", "CLOUD_115":
		return networkStorageProvider115
	default:
		return ""
	}
}

func normalizeNetworkStorageLoginMethod(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case networkStorageLoginModeQRCode:
		return networkStorageLoginModeQRCode
	case "", networkStorageLoginModeManual:
		return networkStorageLoginModeManual
	default:
		return ""
	}
}

func normalizeNetworkStorageAppType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "android", "ios", "web":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "windows"
	}
}

func networkStorageDriverForProvider(provider string) string {
	switch normalizeNetworkStorageProvider(provider) {
	case networkStorageProvider115:
		return networkStorageDriver115Cloud
	default:
		return ""
	}
}

func networkStorageMountPath(provider string, storageKey string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	storageKey = strings.TrimSpace(storageKey)
	if storageKey == "" {
		storageKey = uuid.NewString()
	}
	return normalizeAListEndpointPath(fmt.Sprintf("/network/%s/%s", provider, storageKey))
}
