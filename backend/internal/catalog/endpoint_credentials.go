package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"mam/backend/internal/connectors"
	"mam/backend/internal/store"
)

const storedCredentialHint = "已保存在当前机器"

type endpointCredentialInput struct {
	Secret string
	Hint   string
}

func (service *Service) buildConnector(endpoint store.StorageEndpoint) (connectors.Connector, error) {
	hydrated, err := service.hydrateEndpointForConnector(endpoint)
	if err != nil {
		return nil, err
	}
	if normalizeEndpointType(hydrated.EndpointType) == string(connectors.EndpointTypeNetwork) {
		return service.buildNetworkStorageConnector(context.Background(), hydrated)
	}
	if normalizeEndpointType(hydrated.EndpointType) == string(connectors.EndpointTypeAList) {
		return service.buildAListConnector(context.Background(), hydrated)
	}
	return service.connectorFactory(hydrated)
}

func (service *Service) hydrateEndpointForConnector(endpoint store.StorageEndpoint) (store.StorageEndpoint, error) {
	normalizedEndpointType := normalizeEndpointType(endpoint.EndpointType)
	if normalizedEndpointType != string(connectors.EndpointTypeCloud115) &&
		normalizedEndpointType != string(connectors.EndpointTypeNetwork) {
		return endpoint, nil
	}

	config := json.RawMessage(endpoint.ConnectionConfig)
	_, extracted, err := normalizeConnectionConfig(endpoint.EndpointType, config)
	if err != nil {
		return endpoint, err
	}
	if extracted != nil && strings.TrimSpace(extracted.Secret) != "" {
		return endpoint, nil
	}

	credentialRef := strings.TrimSpace(endpoint.CredentialRef)
	if credentialRef == "" {
		return endpoint, nil
	}
	if service.credentialVault == nil {
		return endpoint, fmt.Errorf("credential vault is not configured for endpoint %q", endpoint.Name)
	}

	record, err := service.credentialVault.Resolve(credentialRef)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return endpoint, fmt.Errorf("credential for endpoint %q is not available on this machine; please rebind it", endpoint.Name)
		}
		return endpoint, err
	}

	var hydratedConfig json.RawMessage
	switch normalizedEndpointType {
	case string(connectors.EndpointTypeCloud115):
		hydratedConfig, err = injectCloud115Credential(config, record.Secret)
	case string(connectors.EndpointTypeNetwork):
		hydratedConfig, err = injectNetworkStorageCredential(config, record.Secret)
	default:
		return endpoint, nil
	}
	if err != nil {
		return endpoint, err
	}

	endpoint.ConnectionConfig = string(hydratedConfig)
	return endpoint, nil
}

func (service *Service) resolveEndpointCredential(
	endpointType string,
	existingRef string,
	existingHint string,
	requestedRef string,
	extracted *endpointCredentialInput,
) (string, string, error) {
	resolvedRef := strings.TrimSpace(requestedRef)
	if resolvedRef == "" {
		resolvedRef = strings.TrimSpace(existingRef)
	}

	resolvedHint := strings.TrimSpace(existingHint)
	if extracted == nil || strings.TrimSpace(extracted.Secret) == "" {
		if resolvedRef == "" {
			return "", "", nil
		}
		if resolvedHint == "" {
			resolvedHint = defaultCredentialHint(endpointType)
		}
		return resolvedRef, resolvedHint, nil
	}

	if service.credentialVault == nil {
		return "", "", errors.New("credential vault is not configured")
	}

	record, err := service.credentialVault.Put(
		credentialProviderForEndpoint(endpointType),
		resolvedRef,
		extracted.Secret,
		defaultString(strings.TrimSpace(extracted.Hint), defaultCredentialHint(endpointType)),
	)
	if err != nil {
		return "", "", err
	}

	return record.Ref, defaultString(strings.TrimSpace(record.Hint), defaultCredentialHint(endpointType)), nil
}

func (service *Service) MigrateEndpointCredentials(ctx context.Context) error {
	if service.credentialVault == nil {
		return nil
	}

	endpoints, err := service.store.ListStorageEndpoints(ctx)
	if err != nil {
		return err
	}

	for _, endpoint := range endpoints {
		normalizedConfig, extracted, err := normalizeConnectionConfig(endpoint.EndpointType, json.RawMessage(endpoint.ConnectionConfig))
		if err != nil {
			return err
		}

		nextRef, nextHint, err := service.resolveEndpointCredential(
			endpoint.EndpointType,
			endpoint.CredentialRef,
			endpoint.CredentialHint,
			endpoint.CredentialRef,
			extracted,
		)
		if err != nil {
			return err
		}

		normalizedConfigText := string(normalizedConfig)
		if normalizedConfigText == endpoint.ConnectionConfig &&
			strings.TrimSpace(nextRef) == strings.TrimSpace(endpoint.CredentialRef) &&
			strings.TrimSpace(nextHint) == strings.TrimSpace(endpoint.CredentialHint) {
			continue
		}

		endpoint.ConnectionConfig = normalizedConfigText
		endpoint.CredentialRef = nextRef
		endpoint.CredentialHint = nextHint
		endpoint.UpdatedAt = time.Now().UTC()
		if err := service.store.UpdateStorageEndpoint(ctx, endpoint); err != nil {
			return err
		}
	}

	return nil
}

func injectCloud115Credential(connectionConfig json.RawMessage, secret string) (json.RawMessage, error) {
	if len(strings.TrimSpace(string(connectionConfig))) == 0 {
		connectionConfig = json.RawMessage(`{}`)
	}

	var payload map[string]any
	if err := json.Unmarshal(connectionConfig, &payload); err != nil {
		return nil, fmt.Errorf("invalid connection config: %w", err)
	}

	payload["accessToken"] = strings.TrimSpace(secret)
	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("normalize hydrated connection config: %w", err)
	}
	return normalized, nil
}

func extractEndpointCredential(endpointType string, payload map[string]any) *endpointCredentialInput {
	normalizedEndpointType := normalizeEndpointType(endpointType)
	if normalizedEndpointType != string(connectors.EndpointTypeCloud115) &&
		normalizedEndpointType != string(connectors.EndpointTypeNetwork) {
		return nil
	}

	secret := firstNonEmptyMapString(payload, "accessToken", "cookies", "credential", "cookie", "token")
	delete(payload, "accessToken")
	delete(payload, "cookies")
	delete(payload, "credential")
	delete(payload, "cookie")
	delete(payload, "token")
	delete(payload, "credentialRef")
	delete(payload, "credentialHint")
	delete(payload, "hasCredential")

	if strings.TrimSpace(secret) == "" {
		return nil
	}

	return &endpointCredentialInput{
		Secret: strings.TrimSpace(secret),
		Hint:   defaultCredentialHint(endpointType),
	}
}

func credentialProviderForEndpoint(endpointType string) string {
	switch normalizeEndpointType(endpointType) {
	case string(connectors.EndpointTypeCloud115):
		return "cloud115"
	case string(connectors.EndpointTypeNetwork):
		return "network-storage-115"
	default:
		return strings.ToLower(strings.TrimSpace(endpointType))
	}
}

func defaultCredentialHint(endpointType string) string {
	switch normalizeEndpointType(endpointType) {
	case string(connectors.EndpointTypeCloud115):
		return storedCredentialHint
	case string(connectors.EndpointTypeNetwork):
		return "已保存在当前机器的网盘凭证"
	default:
		return storedCredentialHint
	}
}

func injectNetworkStorageCredential(connectionConfig json.RawMessage, secret string) (json.RawMessage, error) {
	if len(strings.TrimSpace(string(connectionConfig))) == 0 {
		connectionConfig = json.RawMessage(`{}`)
	}

	var payload map[string]any
	if err := json.Unmarshal(connectionConfig, &payload); err != nil {
		return nil, fmt.Errorf("invalid connection config: %w", err)
	}

	payload["credential"] = strings.TrimSpace(secret)
	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("normalize hydrated network storage config: %w", err)
	}
	return normalized, nil
}

func firstNonEmptyMapString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		if strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}
