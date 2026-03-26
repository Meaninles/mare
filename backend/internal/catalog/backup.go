package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"mam/backend/internal/config"
	"mam/backend/internal/store"
)

const (
	backupFormatVersion        = 1
	backupImportModeConfigOnly = "config_only"
	backupImportModeCatalog    = "config_and_catalog"
)

type BackupPreferences struct {
	Theme               string `json:"theme"`
	UploadConcurrency   int    `json:"uploadConcurrency,omitempty"`
	DownloadConcurrency int    `json:"downloadConcurrency,omitempty"`
}

type BackupConfigurationSnapshot struct {
	Endpoints   []EndpointRecord   `json:"endpoints"`
	ImportRules []ImportRuleRecord `json:"importRules"`
}

type BackupCatalogSnapshot struct {
	Assets []AssetRecord `json:"assets"`
}

type SettingsBackupBundle struct {
	FormatVersion int                         `json:"formatVersion"`
	ExportedAt    time.Time                   `json:"exportedAt"`
	App           SettingsBackupAppInfo       `json:"app"`
	Preferences   BackupPreferences           `json:"preferences"`
	Configuration BackupConfigurationSnapshot `json:"configuration"`
	Catalog       *BackupCatalogSnapshot      `json:"catalog,omitempty"`
}

type SettingsBackupAppInfo struct {
	Name string `json:"name"`
	Env  string `json:"env"`
}

type ExportBackupRequest struct {
	Theme          string `json:"theme"`
	IncludeCatalog bool   `json:"includeCatalog"`
}

type ImportBackupRequest struct {
	Mode   string               `json:"mode"`
	Bundle SettingsBackupBundle `json:"bundle"`
}

type ImportBackupSummary struct {
	Mode              string    `json:"mode"`
	ImportedEndpoints int       `json:"importedEndpoints"`
	ImportedRules     int       `json:"importedRules"`
	ImportedAssets    int       `json:"importedAssets"`
	ImportedReplicas  int       `json:"importedReplicas"`
	ImportedVersions  int       `json:"importedVersions"`
	AppliedTheme      string    `json:"appliedTheme,omitempty"`
	ImportedAt        time.Time `json:"importedAt"`
}

func (service *Service) ExportSettingsBackup(ctx context.Context, cfg config.Config, request ExportBackupRequest) (SettingsBackupBundle, error) {
	endpoints, err := service.ListEndpoints(ctx)
	if err != nil {
		return SettingsBackupBundle{}, err
	}

	importRules, err := service.ListImportRules(ctx)
	if err != nil {
		return SettingsBackupBundle{}, err
	}
	transferPreferences, err := service.GetTransferPreferences(ctx)
	if err != nil {
		return SettingsBackupBundle{}, err
	}

	bundle := SettingsBackupBundle{
		FormatVersion: backupFormatVersion,
		ExportedAt:    time.Now().UTC(),
		App: SettingsBackupAppInfo{
			Name: cfg.AppName,
			Env:  cfg.AppEnv,
		},
		Preferences: BackupPreferences{
			Theme:               strings.TrimSpace(request.Theme),
			UploadConcurrency:   transferPreferences.UploadConcurrency,
			DownloadConcurrency: transferPreferences.DownloadConcurrency,
		},
		Configuration: BackupConfigurationSnapshot{
			Endpoints:   endpoints,
			ImportRules: importRules,
		},
	}

	if request.IncludeCatalog {
		assets, err := service.ListAssets(ctx, 100_000, 0)
		if err != nil {
			return SettingsBackupBundle{}, err
		}
		bundle.Catalog = &BackupCatalogSnapshot{Assets: assets}
	}

	slog.Info("settings backup exported",
		"includeCatalog", request.IncludeCatalog,
		"endpointCount", len(bundle.Configuration.Endpoints),
		"importRuleCount", len(bundle.Configuration.ImportRules),
		"assetCount", assetCount(bundle.Catalog),
	)
	return bundle, nil
}

func (service *Service) ImportSettingsBackup(ctx context.Context, request ImportBackupRequest) (ImportBackupSummary, error) {
	mode := normalizeBackupImportMode(request.Mode)
	if mode == "" {
		return ImportBackupSummary{}, errors.New("unsupported backup import mode")
	}
	if request.Bundle.FormatVersion != backupFormatVersion {
		return ImportBackupSummary{}, fmt.Errorf("unsupported backup format version: %d", request.Bundle.FormatVersion)
	}

	summary := ImportBackupSummary{
		Mode:         mode,
		AppliedTheme: strings.TrimSpace(request.Bundle.Preferences.Theme),
		ImportedAt:   time.Now().UTC(),
	}

	if mode == backupImportModeCatalog {
		if err := service.store.ClearTasks(ctx); err != nil {
			return summary, err
		}
		if err := service.store.ClearCatalogSnapshot(ctx); err != nil {
			return summary, err
		}
		if err := service.store.DeleteAllStorageEndpoints(ctx); err != nil {
			return summary, err
		}
	}

	if _, err := service.UpdateTransferPreferences(ctx, UpdateTransferPreferencesRequest{
		UploadConcurrency:   request.Bundle.Preferences.UploadConcurrency,
		DownloadConcurrency: request.Bundle.Preferences.DownloadConcurrency,
	}); err != nil {
		return summary, err
	}

	existingEndpoints := map[string]store.StorageEndpoint{}
	if mode == backupImportModeConfigOnly {
		currentEndpoints, err := service.store.ListStorageEndpoints(ctx)
		if err != nil {
			return summary, err
		}
		for _, endpoint := range currentEndpoints {
			existingEndpoints[endpoint.ID] = endpoint
		}
	}

	for _, endpointRecord := range request.Bundle.Configuration.Endpoints {
		endpointType, err := resolveRequestedEndpointType(endpointRecord.EndpointType, endpointRecord.ConnectionConfig)
		if err != nil {
			return summary, err
		}
		if endpointType == "" {
			return summary, fmt.Errorf("unsupported storage endpoint type in backup: %s", endpointRecord.EndpointType)
		}

		endpoint := store.StorageEndpoint{
			ID:                 endpointRecord.ID,
			Name:               endpointRecord.Name,
			Note:               endpointRecord.Note,
			EndpointType:       endpointType,
			RootPath:           endpointRecord.RootPath,
			RoleMode:           endpointRecord.RoleMode,
			IdentitySignature:  endpointRecord.IdentitySignature,
			AvailabilityStatus: endpointRecord.AvailabilityStatus,
			ConnectionConfig:   string(endpointRecord.ConnectionConfig),
			CredentialRef:      endpointRecord.CredentialRef,
			CredentialHint:     endpointRecord.CredentialHint,
			CreatedAt:          endpointRecord.CreatedAt,
			UpdatedAt:          endpointRecord.UpdatedAt,
		}

		if mode == backupImportModeConfigOnly {
			if _, exists := existingEndpoints[endpoint.ID]; exists {
				if err := service.store.UpdateStorageEndpoint(ctx, endpoint); err != nil {
					return summary, err
				}
			} else {
				if err := service.store.CreateStorageEndpoint(ctx, endpoint); err != nil {
					return summary, err
				}
			}
		} else {
			if err := service.store.CreateStorageEndpoint(ctx, endpoint); err != nil {
				return summary, err
			}
		}

		summary.ImportedEndpoints++
	}

	rules := make([]store.ImportRule, 0, len(request.Bundle.Configuration.ImportRules))
	for _, ruleRecord := range request.Bundle.Configuration.ImportRules {
		targetEndpointIDs, err := json.Marshal(uniqueStrings(ruleRecord.TargetEndpointIDs))
		if err != nil {
			return summary, err
		}
		rules = append(rules, store.ImportRule{
			ID:                ruleRecord.ID,
			RuleType:          ruleRecord.RuleType,
			MatchValue:        ruleRecord.MatchValue,
			TargetEndpointIDs: string(targetEndpointIDs),
			CreatedAt:         ruleRecord.CreatedAt,
			UpdatedAt:         ruleRecord.UpdatedAt,
		})
	}
	if err := service.store.ReplaceImportRules(ctx, rules); err != nil {
		return summary, err
	}
	summary.ImportedRules = len(rules)

	if mode == backupImportModeCatalog {
		if request.Bundle.Catalog == nil {
			return summary, errors.New("backup bundle does not contain catalog snapshot data")
		}
		if err := service.importCatalogSnapshot(ctx, request.Bundle.Catalog, &summary); err != nil {
			return summary, err
		}
	}

	slog.Info("settings backup imported",
		"mode", summary.Mode,
		"endpointCount", summary.ImportedEndpoints,
		"importRuleCount", summary.ImportedRules,
		"assetCount", summary.ImportedAssets,
		"replicaCount", summary.ImportedReplicas,
	)
	return summary, nil
}

func normalizeBackupImportMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case backupImportModeConfigOnly:
		return backupImportModeConfigOnly
	case backupImportModeCatalog:
		return backupImportModeCatalog
	default:
		return ""
	}
}

func (service *Service) importCatalogSnapshot(ctx context.Context, snapshot *BackupCatalogSnapshot, summary *ImportBackupSummary) error {
	if snapshot == nil {
		return nil
	}

	importedVersionIDs := make(map[string]struct{})
	for _, assetRecord := range snapshot.Assets {
		asset := store.Asset{
			ID:                 assetRecord.ID,
			LogicalPathKey:     assetRecord.LogicalPathKey,
			DisplayName:        assetRecord.DisplayName,
			MediaType:          assetRecord.MediaType,
			AssetStatus:        assetRecord.AssetStatus,
			PrimaryTimestamp:   cloneTimePointer(assetRecord.PrimaryTimestamp),
			PrimaryThumbnailID: nil,
			CreatedAt:          assetRecord.CreatedAt,
			UpdatedAt:          assetRecord.UpdatedAt,
		}
		if err := service.store.CreateAsset(ctx, asset); err != nil {
			return err
		}
		summary.ImportedAssets++

		for _, replicaRecord := range assetRecord.Replicas {
			if strings.TrimSpace(replicaRecord.PhysicalPath) == "" {
				continue
			}

			var versionID *string
			if replicaRecord.Version != nil {
				if _, seen := importedVersionIDs[replicaRecord.Version.ID]; !seen {
					version := store.ReplicaVersion{
						ID:           replicaRecord.Version.ID,
						Size:         replicaRecord.Version.Size,
						MTime:        cloneTimePointer(replicaRecord.Version.MTime),
						ScanRevision: replicaRecord.Version.ScanRevision,
						CreatedAt:    replicaRecord.Version.CreatedAt,
					}
					if err := service.store.CreateReplicaVersion(ctx, version); err != nil {
						return err
					}
					importedVersionIDs[version.ID] = struct{}{}
					summary.ImportedVersions++
				}
				versionID = &replicaRecord.Version.ID
			}

			replica := store.Replica{
				ID:            replicaRecord.ID,
				AssetID:       asset.ID,
				EndpointID:    replicaRecord.EndpointID,
				PhysicalPath:  replicaRecord.PhysicalPath,
				ReplicaStatus: replicaRecord.ReplicaStatus,
				ExistsFlag:    replicaRecord.ExistsFlag,
				VersionID:     versionID,
				LastSeenAt:    cloneTimePointer(replicaRecord.LastSeenAt),
				CreatedAt:     asset.CreatedAt,
				UpdatedAt:     asset.UpdatedAt,
			}
			if err := service.store.CreateReplica(ctx, replica); err != nil {
				return err
			}
			summary.ImportedReplicas++
		}

		if assetRecord.AudioMetadata != nil {
			if err := service.store.SaveAssetMediaMetadata(ctx, store.AssetMediaMetadata{
				AssetID:         asset.ID,
				DurationSeconds: assetRecord.AudioMetadata.DurationSeconds,
				CodecName:       assetRecord.AudioMetadata.CodecName,
				SampleRateHz:    assetRecord.AudioMetadata.SampleRateHz,
				ChannelCount:    assetRecord.AudioMetadata.ChannelCount,
				CreatedAt:       asset.CreatedAt,
				UpdatedAt:       asset.UpdatedAt,
			}); err != nil {
				return err
			}
		}
	}

	return nil
}

func assetCount(snapshot *BackupCatalogSnapshot) int {
	if snapshot == nil {
		return 0
	}
	return len(snapshot.Assets)
}
