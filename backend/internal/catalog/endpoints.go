package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"mam/backend/internal/store"
)

func (service *Service) UpdateEndpoint(ctx context.Context, endpointID string, request UpdateEndpointRequest) (EndpointRecord, error) {
	endpointID = strings.TrimSpace(endpointID)
	if endpointID == "" {
		return EndpointRecord{}, fmt.Errorf("endpoint id is required")
	}

	existing, err := service.store.GetStorageEndpointByID(ctx, endpointID)
	if err != nil {
		return EndpointRecord{}, err
	}

	endpointType, endpointTypeErr := resolveRequestedEndpointType(
		defaultString(strings.TrimSpace(request.EndpointType), existing.EndpointType),
		request.ConnectionConfig,
	)
	if endpointTypeErr != nil {
		return EndpointRecord{}, endpointTypeErr
	}
	if endpointType == "" {
		slog.Warn(
			"update endpoint missing type",
			"endpointId", existing.ID,
			"requestedType", strings.TrimSpace(request.EndpointType),
			"existingType", existing.EndpointType,
			"connectionConfigKeys", summarizeJSONKeys(request.ConnectionConfig),
		)
		return EndpointRecord{}, fmt.Errorf("endpoint type is required")
	}

	connectionConfigInput := request.ConnectionConfig
	if len(strings.TrimSpace(string(connectionConfigInput))) == 0 {
		connectionConfigInput = json.RawMessage(existing.ConnectionConfig)
	}

	connectionConfig, extractedCredential, err := normalizeConnectionConfig(endpointType, connectionConfigInput)
	if err != nil {
		return EndpointRecord{}, err
	}

	rootPathInput := strings.TrimSpace(request.RootPath)
	if rootPathInput == "" {
		rootPathInput = existing.RootPath
	}

	rootPath, err := resolveRootPath(endpointType, rootPathInput, connectionConfig)
	if err != nil {
		return EndpointRecord{}, err
	}

	identitySignature, err := resolveIdentitySignature(
		endpointType,
		rootPath,
		strings.TrimSpace(request.IdentitySignature),
		connectionConfig,
	)
	if err != nil {
		return EndpointRecord{}, err
	}

	allEndpoints, err := service.store.ListStorageEndpoints(ctx)
	if err != nil {
		return EndpointRecord{}, err
	}
	for _, candidate := range allEndpoints {
		if candidate.ID == existing.ID {
			continue
		}
		if candidate.IdentitySignature == identitySignature {
			return EndpointRecord{}, fmt.Errorf("storage endpoint %q already exists", candidate.Name)
		}
	}

	now := time.Now().UTC()
	roleMode := normalizeEndpointRoleMode(request.RoleMode)
	if roleMode == "" {
		roleMode = defaultString(existing.RoleMode, defaultRoleMode)
	}
	credentialRef, credentialHint, err := service.resolveEndpointCredential(
		endpointType,
		existing.CredentialRef,
		existing.CredentialHint,
		request.CredentialRef,
		extractedCredential,
	)
	if err != nil {
		return EndpointRecord{}, err
	}

	endpoint := store.StorageEndpoint{
		ID:                 existing.ID,
		Name:               defaultString(strings.TrimSpace(request.Name), existing.Name),
		Note:               strings.TrimSpace(request.Note),
		EndpointType:       endpointType,
		RootPath:           rootPath,
		RoleMode:           roleMode,
		IdentitySignature:  identitySignature,
		AvailabilityStatus: defaultString(strings.TrimSpace(request.AvailabilityStatus), defaultString(existing.AvailabilityStatus, defaultAvailabilityStatus)),
		ConnectionConfig:   string(connectionConfig),
		CredentialRef:      credentialRef,
		CredentialHint:     credentialHint,
		CreatedAt:          existing.CreatedAt,
		UpdatedAt:          now,
	}

	if _, err := service.buildConnector(endpoint); err != nil {
		return EndpointRecord{}, err
	}

	if err := service.store.UpdateStorageEndpoint(ctx, endpoint); err != nil {
		return EndpointRecord{}, err
	}

	slog.Info("storage endpoint updated", "endpointId", endpoint.ID, "endpointType", endpoint.EndpointType, "rootPath", endpoint.RootPath)
	return toEndpointRecord(endpoint), nil
}

func (service *Service) DeleteEndpoint(ctx context.Context, endpointID string) (DeleteEndpointSummary, error) {
	endpointID = strings.TrimSpace(endpointID)
	if endpointID == "" {
		return DeleteEndpointSummary{}, fmt.Errorf("endpoint id is required")
	}

	endpoint, err := service.store.GetStorageEndpointByID(ctx, endpointID)
	if err != nil {
		return DeleteEndpointSummary{}, err
	}

	replicas, err := service.store.ListReplicasByEndpointID(ctx, endpointID)
	if err != nil {
		return DeleteEndpointSummary{}, err
	}

	affectedAssetIDs := make(map[string]struct{}, len(replicas))
	for _, replica := range replicas {
		affectedAssetIDs[replica.AssetID] = struct{}{}
	}

	updatedImportRuleCount, err := service.removeEndpointFromImportRules(ctx, endpointID)
	if err != nil {
		return DeleteEndpointSummary{}, err
	}

	if len(replicas) > 0 {
		if err := service.store.DeleteReplicasByEndpointID(ctx, endpointID); err != nil {
			return DeleteEndpointSummary{}, err
		}
	}

	if err := service.store.DeleteStorageEndpoint(ctx, endpointID); err != nil {
		return DeleteEndpointSummary{}, err
	}

	deletedAssetCount := 0
	for assetID := range affectedAssetIDs {
		if err := service.syncAssetStatus(ctx, assetID); err != nil {
			return DeleteEndpointSummary{}, err
		}

		asset, err := service.store.GetAssetByID(ctx, assetID)
		if err != nil {
			return DeleteEndpointSummary{}, err
		}
		if strings.EqualFold(asset.AssetStatus, string(AssetStatusDeleted)) {
			deletedAssetCount++
		}
	}

	summary := DeleteEndpointSummary{
		EndpointID:             endpoint.ID,
		EndpointName:           endpoint.Name,
		EndpointType:           endpoint.EndpointType,
		RemovedReplicaCount:    len(replicas),
		AffectedAssetCount:     len(affectedAssetIDs),
		DeletedAssetCount:      deletedAssetCount,
		UpdatedImportRuleCount: updatedImportRuleCount,
		DeletedAt:              time.Now().UTC(),
	}

	slog.Info(
		"storage endpoint deleted",
		"endpointId", endpoint.ID,
		"endpointType", endpoint.EndpointType,
		"removedReplicaCount", len(replicas),
		"affectedAssetCount", len(affectedAssetIDs),
	)
	return summary, nil
}

func (service *Service) removeEndpointFromImportRules(ctx context.Context, endpointID string) (int, error) {
	rules, err := service.store.ListImportRules(ctx)
	if err != nil {
		return 0, err
	}
	if len(rules) == 0 {
		return 0, nil
	}

	now := time.Now().UTC()
	updatedRules := make([]store.ImportRule, 0, len(rules))
	updatedCount := 0

	for _, rule := range rules {
		var targetEndpointIDs []string
		if err := json.Unmarshal([]byte(rule.TargetEndpointIDs), &targetEndpointIDs); err != nil {
			return 0, fmt.Errorf("decode import rule target endpoints: %w", err)
		}

		nextTargetEndpointIDs := make([]string, 0, len(targetEndpointIDs))
		removed := false
		for _, targetEndpointID := range uniqueStrings(targetEndpointIDs) {
			if targetEndpointID == endpointID {
				removed = true
				continue
			}
			nextTargetEndpointIDs = append(nextTargetEndpointIDs, targetEndpointID)
		}

		if !removed {
			updatedRules = append(updatedRules, rule)
			continue
		}

		updatedCount++
		if len(nextTargetEndpointIDs) == 0 {
			continue
		}

		encodedTargets, err := json.Marshal(nextTargetEndpointIDs)
		if err != nil {
			return 0, err
		}

		rule.TargetEndpointIDs = string(encodedTargets)
		rule.UpdatedAt = now
		updatedRules = append(updatedRules, rule)
	}

	if updatedCount == 0 {
		return 0, nil
	}

	if err := service.store.ReplaceImportRules(ctx, updatedRules); err != nil {
		return 0, err
	}

	return updatedCount, nil
}
