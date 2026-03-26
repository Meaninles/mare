package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"mam/backend/internal/connectors"
	sidecaralist "mam/backend/internal/sidecars/alist"
	sidecararia2 "mam/backend/internal/sidecars/aria2"
	"mam/backend/internal/store"
)

type alistEndpointConfig struct {
	MountPath       string `json:"mountPath"`
	Driver          string `json:"driver"`
	Addition        string `json:"addition"`
	Password        string `json:"password"`
	Remark          string `json:"remark"`
	CacheExpiration int    `json:"cacheExpiration"`
}

func (service *Service) getAListRuntime() *sidecaralist.Runtime {
	if service.alistRuntime == nil {
		service.alistRuntime = sidecaralist.NewRuntime(service.transferStateRoot())
	}
	return service.alistRuntime
}

func (service *Service) getAria2Runtime() *sidecararia2.Runtime {
	if service.aria2Runtime == nil {
		service.aria2Runtime = sidecararia2.NewRuntime(service.transferStateRoot())
	}
	return service.aria2Runtime
}

func (service *Service) buildAListConnector(ctx context.Context, endpoint store.StorageEndpoint) (connectors.Connector, error) {
	config, runtime, err := service.prepareAListEndpoint(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	return connectors.NewAListConnector(connectors.AListConfig{
		Name:            endpoint.Name,
		RootPath:        endpoint.RootPath,
		MountPath:       config.MountPath,
		Driver:          config.Driver,
		Addition:        config.Addition,
		Password:        config.Password,
		Remark:          config.Remark,
		CacheExpiration: config.CacheExpiration,
	}, runtime)
}

func (service *Service) prepareAListEndpoint(ctx context.Context, endpoint store.StorageEndpoint) (alistEndpointConfig, *sidecaralist.Runtime, error) {
	config, err := parseAListEndpointConfig(json.RawMessage(endpoint.ConnectionConfig), endpoint.RootPath)
	if err != nil {
		return alistEndpointConfig{}, nil, err
	}

	runtime := service.getAListRuntime()
	if _, err := runtime.EnsureStorage(ctx, sidecaralist.StorageSpec{
		MountPath:       config.MountPath,
		Driver:          config.Driver,
		Addition:        config.Addition,
		Remark:          config.Remark,
		CacheExpiration: config.CacheExpiration,
	}); err != nil {
		return alistEndpointConfig{}, nil, err
	}

	return config, runtime, nil
}

func normalizeAListConnectionConfig(payload map[string]any, explicitRoot string) (map[string]any, error) {
	mountPath := firstNonEmptyMapString(payload, "mountPath", "mount_path")
	driver := firstNonEmptyMapString(payload, "driver", "storageDriver", "storage_driver")
	password := firstNonEmptyMapString(payload, "password")
	remark := firstNonEmptyMapString(payload, "remark")
	cacheExpiration := 30
	if raw, ok := payload["cacheExpiration"]; ok {
		switch value := raw.(type) {
		case float64:
			if value > 0 {
				cacheExpiration = int(value)
			}
		case int:
			if value > 0 {
				cacheExpiration = value
			}
		}
	}

	addition, err := normalizeAListAdditionValue(payload["addition"])
	if err != nil {
		return nil, err
	}
	if addition == "" {
		addition, err = normalizeAListAdditionValue(payload["storageAddition"])
		if err != nil {
			return nil, err
		}
	}

	if strings.TrimSpace(mountPath) == "" {
		mountPath = alistMountPathForRoot(explicitRoot)
	}
	if strings.TrimSpace(driver) == "" {
		driver = "Local"
	}

	return map[string]any{
		"mountPath":       normalizeAListEndpointPath(mountPath),
		"driver":          strings.TrimSpace(driver),
		"addition":        defaultString(strings.TrimSpace(addition), "{}"),
		"password":        strings.TrimSpace(password),
		"remark":          strings.TrimSpace(remark),
		"cacheExpiration": cacheExpiration,
	}, nil
}

func parseAListEndpointConfig(raw json.RawMessage, rootPath string) (alistEndpointConfig, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		raw = json.RawMessage(`{}`)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return alistEndpointConfig{}, fmt.Errorf("invalid alist connection config: %w", err)
	}

	normalized, err := normalizeAListConnectionConfig(payload, rootPath)
	if err != nil {
		return alistEndpointConfig{}, err
	}

	encoded, err := json.Marshal(normalized)
	if err != nil {
		return alistEndpointConfig{}, err
	}

	var config alistEndpointConfig
	if err := json.Unmarshal(encoded, &config); err != nil {
		return alistEndpointConfig{}, err
	}
	if config.CacheExpiration <= 0 {
		config.CacheExpiration = 30
	}
	if strings.TrimSpace(config.MountPath) == "" {
		return alistEndpointConfig{}, errors.New("alist mount path is required")
	}
	if strings.TrimSpace(config.Driver) == "" {
		return alistEndpointConfig{}, errors.New("alist storage driver is required")
	}
	if strings.TrimSpace(config.Addition) == "" {
		config.Addition = "{}"
	}
	return config, nil
}

func normalizeAListAdditionValue(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return "", nil
		}
		var payload any
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			encoded, err := json.Marshal(payload)
			if err == nil {
				return string(encoded), nil
			}
		}
		return trimmed, nil
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return "", fmt.Errorf("normalize alist addition: %w", err)
		}
		return string(encoded), nil
	}
}

func normalizeAListEndpointPath(value string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(value, `\`, "/"))
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func alistMountPathForRoot(rootPath string) string {
	normalized := normalizeAListEndpointPath(rootPath)
	if normalized == "" || normalized == "/" {
		return ""
	}

	segments := strings.Split(strings.TrimPrefix(normalized, "/"), "/")
	if len(segments) == 0 || strings.TrimSpace(segments[0]) == "" {
		return ""
	}
	return "/" + segments[0]
}

func buildAListRuntimeStorageSpec(mountPath string, rootFolder string) sidecaralist.StorageSpec {
	addition, _ := normalizeAListAdditionValue(map[string]any{
		"root_folder_path": filepath.Clean(rootFolder),
	})
	return sidecaralist.StorageSpec{
		MountPath:       normalizeAListEndpointPath(mountPath),
		Driver:          "Local",
		Addition:        addition,
		Remark:          "mare-runtime",
		CacheExpiration: 30,
	}
}
