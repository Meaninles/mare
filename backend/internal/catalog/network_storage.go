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
	networkStorageProvider115      = "115"
	networkStorageDriver115Cloud   = "115 Cloud"
	defaultNetworkStoragePageSize  = 1000
	defaultNetworkStorageCacheTTL  = 30
	defaultNetworkStorageLoginMode = "manual"
	networkStorageLoginModeQRCode  = "qrcode"
	networkStorageLoginModeManual  = "manual"
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

type PreviewNetworkStorageRequest struct {
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	StorageKey   string `json:"storageKey,omitempty"`
	MountPath    string `json:"mountPath,omitempty"`
	RootFolderID string `json:"rootFolderId"`
	LoginMethod  string `json:"loginMethod,omitempty"`
	AppType      string `json:"appType,omitempty"`
	PageSize     int    `json:"pageSize,omitempty"`
	Credential   string `json:"credential"`
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

func (service *Service) PreviewNetworkStorageConnector(
	ctx context.Context,
	request PreviewNetworkStorageRequest,
) (connectors.Connector, error) {
	payload, err := normalizeNetworkStorageConnectionConfig(map[string]any{
		"provider":     strings.TrimSpace(request.Provider),
		"storageKey":   strings.TrimSpace(request.StorageKey),
		"mountPath":    strings.TrimSpace(request.MountPath),
		"rootFolderId": strings.TrimSpace(request.RootFolderID),
		"loginMethod":  strings.TrimSpace(request.LoginMethod),
		"appType":      strings.TrimSpace(request.AppType),
		"pageSize":     request.PageSize,
		"credential":   strings.TrimSpace(request.Credential),
	})
	if err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	rootPath := strings.TrimSpace(firstNonEmptyMapString(payload, "mountPath"))
	if rootPath == "" {
		return nil, errors.New("network storage mount path is required")
	}

	endpoint := store.StorageEndpoint{
		ID:               "preview-" + uuid.NewString(),
		Name:             defaultString(strings.TrimSpace(request.Name), "网络存储"),
		EndpointType:     string(connectors.EndpointTypeNetwork),
		RootPath:         rootPath,
		ConnectionConfig: string(encoded),
	}
	return service.buildNetworkStorageConnector(ctx, endpoint)
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
		return alistEndpointConfig{}, nil, wrapNetworkStorageRuntimeError(err)
	}
	if _, err := runtime.ListEntries(ctx, storageSpec.MountPath, "", true); err != nil {
		return alistEndpointConfig{}, nil, explainNetworkStorageListError(config, err)
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
			WebProxy:        true,
			ProxyRange:      true,
		}, nil
	default:
		return sidecaralist.StorageSpec{}, fmt.Errorf("unsupported network storage provider: %s", config.Provider)
	}
}

func normalizeNetworkStorageProvider(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "115":
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
	case "wechatmini", "alipaymini", "qandroid", "tv", "android", "ios", "web":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "wechatmini"
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

func wrapNetworkStorageRuntimeError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "user not login"), strings.Contains(message, "登录超时"):
		return fmt.Errorf("115 登录已失效，请编辑该网盘端点后重新扫码登录。推荐使用微信小程序、支付宝小程序、115生活或电视设备类型，避免使用安卓、iOS、网页")
	case strings.Contains(message, "repeat login"):
		return fmt.Errorf("115 检测到重复登录，当前凭证不可用。请重新扫码登录，并优先选择微信小程序、支付宝小程序、115生活或电视设备类型")
	default:
		return err
	}
}

func explainNetworkStorageListError(config networkStorageEndpointConfig, err error) error {
	if err == nil {
		return nil
	}

	wrapped := wrapNetworkStorageRuntimeError(err)
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	rootFolderID := strings.TrimSpace(config.RootFolderID)
	if rootFolderID == "" {
		rootFolderID = "0"
	}

	if strings.Contains(message, "failed get objs") ||
		strings.Contains(message, "failed to list objs") ||
		strings.Contains(message, "unexpected error") {
		if rootFolderID != "0" {
			return fmt.Errorf("115 网盘目录 ID %s 无法访问，请确认该目录仍存在且当前凭证有权限访问；如需排查，可先把根目录 ID 改成 0 验证整盘访问。原始错误：%w", rootFolderID, wrapped)
		}
		return fmt.Errorf("115 网盘根目录访问失败，通常是当前凭证不可用，或网络存储服务暂时无法取到目录列表。原始错误：%w", wrapped)
	}

	return wrapped
}
