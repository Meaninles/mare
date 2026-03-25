package catalog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"mam/backend/internal/connectors"
	"mam/backend/internal/credentials"
	"mam/backend/internal/store"
)

const (
	defaultRoleMode           = "MANAGED"
	defaultAvailabilityStatus = "AVAILABLE"
	scanBatchSize             = 256
)

type scanExecutionStats struct {
	FilesScanned int
	BatchCount   int
}

type Service struct {
	store                 *store.Store
	connectorFactory      func(endpoint store.StorageEndpoint) (connectors.Connector, error)
	removableEnumerator   connectors.DeviceEnumerator
	mediaConfig           MediaConfig
	credentialVault       *credentials.Vault
	autoQueueDerivedMedia bool
	autoQueueSearchJobs   bool
	searchBridge          SearchAIBridge
	mediaJobKeys          sync.Map
	searchCapabilityFlags sync.Map
	deviceRoleSelections  sync.Map
	transferTaskControls  sync.Map
	transferWake          chan struct{}
	transferStop          chan struct{}
	transferLoopOnce      sync.Once
	transferCloseOnce     sync.Once
	transferWorkerGroup   sync.WaitGroup
}

func NewService(
	dataStore *store.Store,
	factory func(endpoint store.StorageEndpoint) (connectors.Connector, error),
	options ...ServiceOption,
) *Service {
	if factory == nil {
		factory = defaultConnectorFactory
	}

	resolvedOptions := serviceOptions{
		autoQueueDerivedMedia: true,
		autoQueueSearchJobs:   true,
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		option.apply(&resolvedOptions)
	}

	credentialVault := resolvedOptions.credentialVault
	if credentialVault == nil {
		vault, err := credentials.NewVault("")
		if err == nil {
			credentialVault = vault
		}
	}

	return &Service{
		store:                 dataStore,
		connectorFactory:      factory,
		removableEnumerator:   connectors.NewWindowsUSBEnumerator(),
		mediaConfig:           normalizeMediaConfig(resolvedOptions.mediaConfig),
		credentialVault:       credentialVault,
		autoQueueDerivedMedia: resolvedOptions.autoQueueDerivedMedia,
		autoQueueSearchJobs:   resolvedOptions.autoQueueSearchJobs,
		searchBridge:          defaultSearchBridge(resolvedOptions.searchBridge),
		transferWake:          make(chan struct{}, 1),
		transferStop:          make(chan struct{}),
	}
}

func (service *Service) RegisterEndpoint(ctx context.Context, request RegisterEndpointRequest) (EndpointRecord, error) {
	endpointType := normalizeEndpointType(request.EndpointType)
	if endpointType == "" {
		return EndpointRecord{}, errors.New("endpoint type is required")
	}

	connectionConfig, extractedCredential, err := normalizeConnectionConfig(endpointType, request.ConnectionConfig)
	if err != nil {
		return EndpointRecord{}, err
	}

	rootPath, err := resolveRootPath(endpointType, strings.TrimSpace(request.RootPath), connectionConfig)
	if err != nil {
		return EndpointRecord{}, err
	}

	identitySignature, err := resolveIdentitySignature(endpointType, rootPath, request.IdentitySignature, connectionConfig)
	if err != nil {
		return EndpointRecord{}, err
	}

	now := time.Now().UTC()
	connectionConfigText := string(connectionConfig)
	roleMode := normalizeEndpointRoleMode(request.RoleMode)
	if roleMode == "" {
		roleMode = defaultRoleMode
	}
	availabilityStatus := defaultString(strings.TrimSpace(request.AvailabilityStatus), defaultAvailabilityStatus)

	allEndpoints, err := service.store.ListStorageEndpoints(ctx)
	if err != nil {
		return EndpointRecord{}, err
	}

	var existing *store.StorageEndpoint
	for index := range allEndpoints {
		if allEndpoints[index].IdentitySignature == identitySignature {
			existing = &allEndpoints[index]
			break
		}
	}

	existingCredentialRef := ""
	existingCredentialHint := ""
	if existing != nil {
		existingCredentialRef = existing.CredentialRef
		existingCredentialHint = existing.CredentialHint
	}

	credentialRef, credentialHint, err := service.resolveEndpointCredential(
		endpointType,
		existingCredentialRef,
		existingCredentialHint,
		request.CredentialRef,
		extractedCredential,
	)
	if err != nil {
		return EndpointRecord{}, err
	}

	endpoint := store.StorageEndpoint{
		ID:                 uuid.NewString(),
		Name:               defaultString(strings.TrimSpace(request.Name), defaultEndpointName(endpointType)),
		Note:               strings.TrimSpace(request.Note),
		EndpointType:       endpointType,
		RootPath:           rootPath,
		RoleMode:           roleMode,
		IdentitySignature:  identitySignature,
		AvailabilityStatus: availabilityStatus,
		ConnectionConfig:   connectionConfigText,
		CredentialRef:      credentialRef,
		CredentialHint:     credentialHint,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if existing != nil {
		endpoint.ID = existing.ID
		endpoint.CreatedAt = existing.CreatedAt
	}

	if _, factoryErr := service.buildConnector(endpoint); factoryErr != nil {
		return EndpointRecord{}, factoryErr
	}

	if existing != nil {
		if err := service.store.UpdateStorageEndpoint(ctx, endpoint); err != nil {
			return EndpointRecord{}, err
		}
	} else {
		if err := service.store.CreateStorageEndpoint(ctx, endpoint); err != nil {
			return EndpointRecord{}, err
		}
	}

	slog.Info(
		"storage endpoint saved",
		"endpointId", endpoint.ID,
		"endpointType", endpoint.EndpointType,
		"rootPath", endpoint.RootPath,
		"updated", existing != nil,
	)
	return toEndpointRecord(endpoint), nil
}

func (service *Service) ListEndpoints(ctx context.Context) ([]EndpointRecord, error) {
	endpoints, err := service.store.ListStorageEndpoints(ctx)
	if err != nil {
		return nil, err
	}

	records := make([]EndpointRecord, 0, len(endpoints))
	for _, endpoint := range endpoints {
		records = append(records, toEndpointRecord(endpoint))
	}
	return records, nil
}

func (service *Service) ListAssets(ctx context.Context, limit, offset int) ([]AssetRecord, error) {
	assets, err := service.store.ListAssets(ctx, limit, offset)
	if err != nil {
		return nil, err
	}

	return service.buildAssetRecords(ctx, assets)
}

func (service *Service) ListTasks(ctx context.Context, limit, offset int) ([]store.Task, error) {
	return service.store.ListTasks(ctx, limit, offset)
}

func (service *Service) FullScan(ctx context.Context) (FullScanSummary, error) {
	startedAt := time.Now().UTC()
	endpoints, _, err := service.listManagedEnabledEndpoints(ctx)
	if err != nil {
		return FullScanSummary{}, err
	}

	summary := FullScanSummary{
		StartedAt:     startedAt,
		EndpointCount: len(endpoints),
	}

	for _, endpoint := range endpoints {
		endpointSummary, scanErr := service.RescanEndpoint(ctx, endpoint.ID)
		if scanErr != nil {
			summary.FailedCount++
		} else {
			summary.SuccessCount++
		}
		summary.EndpointSummaries = append(summary.EndpointSummaries, endpointSummary)
	}

	summary.FinishedAt = time.Now().UTC()
	return summary, nil
}

func (service *Service) RescanEndpoint(ctx context.Context, endpointID string) (EndpointScanSummary, error) {
	endpoint, err := service.store.GetStorageEndpointByID(ctx, endpointID)
	if err != nil {
		return EndpointScanSummary{}, err
	}
	if !isManagedEndpoint(endpoint) {
		return EndpointScanSummary{}, fmt.Errorf("endpoint %q is not a managed storage node", endpoint.Name)
	}

	task, err := service.createScanTask(ctx, endpoint)
	if err != nil {
		return EndpointScanSummary{}, err
	}

	startedAt := time.Now().UTC()
	summary := EndpointScanSummary{
		TaskID:       task.ID,
		EndpointID:   endpoint.ID,
		EndpointName: endpoint.Name,
		EndpointType: endpoint.EndpointType,
		Status:       "running",
		StartedAt:    startedAt,
	}

	slog.Info("endpoint scan started", "taskId", task.ID, "endpointId", endpoint.ID, "endpointType", endpoint.EndpointType)

	if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
		Status:     "running",
		RetryCount: task.RetryCount,
		StartedAt:  &startedAt,
		UpdatedAt:  startedAt,
	}); err != nil {
		return summary, err
	}

	connector, err := service.buildConnector(endpoint)
	if err != nil {
		return service.failTask(ctx, task, summary, err)
	}

	seenPhysicalPaths := make(map[string]struct{})
	mergeTotals := MergeStats{}

	scanStats, err := service.scanEndpoint(ctx, endpoint, connector, func(batch []ScanResult) error {
		for _, item := range batch {
			seenPhysicalPaths[item.PhysicalPath] = struct{}{}
		}

		stats, mergeErr := service.MergeScanResults(ctx, batch, task.ID)
		if mergeErr != nil {
			return mergeErr
		}

		mergeTotals.AssetsCreated += stats.AssetsCreated
		mergeTotals.AssetsUpdated += stats.AssetsUpdated
		mergeTotals.ReplicasCreated += stats.ReplicasCreated
		mergeTotals.ReplicasUpdated += stats.ReplicasUpdated
		return nil
	})
	if err != nil {
		return service.failTask(ctx, task, summary, err)
	}

	missingReplicas, err := service.markMissingReplicas(ctx, endpoint.ID, seenPhysicalPaths)
	if err != nil {
		return service.failTask(ctx, task, summary, err)
	}

	finishedAt := time.Now().UTC()
	summary.Status = "success"
	summary.FilesScanned = scanStats.FilesScanned
	summary.BatchCount = scanStats.BatchCount
	summary.AssetsCreated = mergeTotals.AssetsCreated
	summary.AssetsUpdated = mergeTotals.AssetsUpdated
	summary.ReplicasCreated = mergeTotals.ReplicasCreated
	summary.ReplicasUpdated = mergeTotals.ReplicasUpdated
	summary.MissingReplicas = missingReplicas
	summary.FinishedAt = finishedAt

	resultSummary, marshalErr := json.Marshal(summary)
	if marshalErr != nil {
		return summary, marshalErr
	}
	resultText := string(resultSummary)
	if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
		Status:        "success",
		ResultSummary: &resultText,
		RetryCount:    task.RetryCount,
		StartedAt:     &startedAt,
		FinishedAt:    &finishedAt,
		UpdatedAt:     finishedAt,
	}); err != nil {
		return summary, err
	}

	slog.Info(
		"endpoint scan completed",
		"taskId", task.ID,
		"endpointId", endpoint.ID,
		"filesScanned", summary.FilesScanned,
		"assetsCreated", summary.AssetsCreated,
		"assetsUpdated", summary.AssetsUpdated,
		"missingReplicas", summary.MissingReplicas,
	)
	return summary, nil
}

func (service *Service) MergeScanResults(ctx context.Context, results []ScanResult, scanRevision string) (MergeStats, error) {
	stats := MergeStats{}
	affectedAssets := make(map[string]struct{})

	for _, result := range results {
		asset, assetCreated, assetUpdated, err := service.upsertAsset(ctx, result)
		if err != nil {
			return stats, err
		}
		if assetCreated {
			stats.AssetsCreated++
		}
		if assetUpdated {
			stats.AssetsUpdated++
		}

		replicaCreated, replicaUpdated, err := service.upsertReplica(ctx, asset, result, scanRevision)
		if err != nil {
			return stats, err
		}
		if replicaCreated {
			stats.ReplicasCreated++
		}
		if replicaUpdated {
			stats.ReplicasUpdated++
		}

		affectedAssets[asset.ID] = struct{}{}
	}

	for assetID := range affectedAssets {
		if err := service.syncAssetStatus(ctx, assetID); err != nil {
			return stats, err
		}
		if err := service.queueDerivedMediaForAsset(ctx, assetID); err != nil {
			return stats, err
		}
	}

	return stats, nil
}

func (service *Service) queueDerivedMediaForAsset(ctx context.Context, assetID string) error {
	if !service.autoQueueDerivedMedia {
		return nil
	}

	asset, err := service.store.GetAssetByID(ctx, assetID)
	if err != nil {
		return err
	}

	replicas, err := service.store.ListReplicasByAssetID(ctx, assetID)
	if err != nil {
		return err
	}

	service.maybeQueueDerivedMedia(asset, replicas)
	return nil
}

func (service *Service) scanEndpoint(
	ctx context.Context,
	endpoint store.StorageEndpoint,
	connector connectors.Connector,
	emit func(batch []ScanResult) error,
) (scanExecutionStats, error) {
	descriptor := connector.Descriptor()
	queue := []string{""}
	stats := scanExecutionStats{}
	batch := make([]ScanResult, 0, scanBatchSize)

	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return stats, err
		}

		currentPath := queue[0]
		queue = queue[1:]

		entries, err := connector.ListEntries(ctx, connectors.ListEntriesRequest{
			Path:               currentPath,
			Recursive:          false,
			IncludeDirectories: true,
			MediaOnly:          false,
		})
		if err != nil {
			return stats, err
		}

		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return stats, err
			}

			if entry.IsDir {
				nextPath := strings.TrimSpace(entry.RelativePath)
				if nextPath == "" {
					nextPath = strings.TrimSpace(entry.Path)
				}
				if nextPath != "" {
					queue = append(queue, nextPath)
				}
				continue
			}

			if entry.MediaType == connectors.MediaTypeUnknown {
				continue
			}

			physicalPath := strings.TrimSpace(entry.Path)
			if physicalPath == "" {
				physicalPath = strings.TrimSpace(entry.RelativePath)
			}
			logicalPathSource := physicalPath
			if strings.TrimSpace(entry.RelativePath) != "" {
				logicalPathSource = entry.RelativePath
			}

			logicalPathKey, err := NormalizeLogicalPathKey(descriptor.RootPath, logicalPathSource)
			if err != nil {
				return stats, fmt.Errorf("normalize logical path for %q: %w", logicalPathSource, err)
			}

			batch = append(batch, ScanResult{
				EndpointID:     endpoint.ID,
				PhysicalPath:   physicalPath,
				LogicalPathKey: logicalPathKey,
				Size:           entry.Size,
				MTime:          entry.ModifiedAt,
				MediaType:      string(entry.MediaType),
				IsDir:          false,
			})
			stats.FilesScanned++

			if len(batch) >= scanBatchSize {
				if err := emit(batch); err != nil {
					return stats, err
				}
				stats.BatchCount++
				batch = batch[:0]
			}
		}
	}

	if len(batch) > 0 {
		if err := emit(batch); err != nil {
			return stats, err
		}
		stats.BatchCount++
	}

	return stats, nil
}

func (service *Service) upsertAsset(ctx context.Context, result ScanResult) (store.Asset, bool, bool, error) {
	now := time.Now().UTC()
	asset, err := service.store.GetAssetByLogicalPathKey(ctx, result.LogicalPathKey)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return store.Asset{}, false, false, err
		}

		asset = store.Asset{
			ID:               uuid.NewString(),
			LogicalPathKey:   result.LogicalPathKey,
			DisplayName:      path.Base(result.LogicalPathKey),
			MediaType:        result.MediaType,
			AssetStatus:      string(AssetStatusReady),
			PrimaryTimestamp: cloneTimePointer(result.MTime),
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := service.store.CreateAsset(ctx, asset); err != nil {
			return store.Asset{}, false, false, err
		}
		return asset, true, false, nil
	}

	updated := false
	displayName := path.Base(result.LogicalPathKey)
	if asset.DisplayName != displayName {
		asset.DisplayName = displayName
		updated = true
	}

	if shouldReplaceMediaType(asset.MediaType, result.MediaType) {
		asset.MediaType = result.MediaType
		updated = true
	}

	nextPrimaryTimestamp := selectPrimaryTimestamp(asset.PrimaryTimestamp, result.MTime)
	if !sameTimePointer(asset.PrimaryTimestamp, nextPrimaryTimestamp) {
		asset.PrimaryTimestamp = cloneTimePointer(nextPrimaryTimestamp)
		updated = true
	}

	if updated {
		asset.UpdatedAt = now
		if err := service.store.UpdateAsset(ctx, asset); err != nil {
			return store.Asset{}, false, false, err
		}
	}

	return asset, false, updated, nil
}

func (service *Service) upsertReplica(
	ctx context.Context,
	asset store.Asset,
	result ScanResult,
	scanRevision string,
) (bool, bool, error) {
	replica, err := service.store.GetReplicaByAssetAndEndpoint(ctx, asset.ID, result.EndpointID)
	replicaExists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, false, err
	}

	versionID, _, err := service.resolveReplicaVersion(ctx, replica.VersionID, result, scanRevision)
	if err != nil {
		return false, false, err
	}

	now := time.Now().UTC()
	lastSeenAt := now

	if !replicaExists {
		newReplica := store.Replica{
			ID:            uuid.NewString(),
			AssetID:       asset.ID,
			EndpointID:    result.EndpointID,
			PhysicalPath:  result.PhysicalPath,
			ReplicaStatus: string(ReplicaStatusActive),
			ExistsFlag:    true,
			VersionID:     versionID,
			LastSeenAt:    &lastSeenAt,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := service.store.CreateReplica(ctx, newReplica); err != nil {
			return false, false, err
		}
		return true, false, nil
	}

	replica.AssetID = asset.ID
	replica.EndpointID = result.EndpointID
	replica.PhysicalPath = result.PhysicalPath
	replica.ReplicaStatus = string(ReplicaStatusActive)
	replica.ExistsFlag = true
	replica.VersionID = versionID
	replica.LastSeenAt = &lastSeenAt
	replica.UpdatedAt = now
	if err := service.store.UpdateReplica(ctx, replica); err != nil {
		return false, false, err
	}

	return false, true, nil
}

func (service *Service) resolveReplicaVersion(
	ctx context.Context,
	currentVersionID *string,
	result ScanResult,
	scanRevision string,
) (*string, bool, error) {
	if currentVersionID != nil {
		version, err := service.store.GetReplicaVersionByID(ctx, *currentVersionID)
		if err == nil && versionMatches(version, result) {
			return currentVersionID, false, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, false, err
		}
	}

	now := time.Now().UTC()
	version := store.ReplicaVersion{
		ID:           uuid.NewString(),
		Size:         result.Size,
		MTime:        cloneTimePointer(result.MTime),
		ScanRevision: stringPointer(scanRevision),
		CreatedAt:    now,
	}
	if err := service.store.CreateReplicaVersion(ctx, version); err != nil {
		return nil, false, err
	}

	return &version.ID, true, nil
}

func (service *Service) markMissingReplicas(
	ctx context.Context,
	endpointID string,
	seenPhysicalPaths map[string]struct{},
) (int, error) {
	replicas, err := service.store.ListReplicasByEndpointID(ctx, endpointID)
	if err != nil {
		return 0, err
	}

	now := time.Now().UTC()
	affectedAssets := make(map[string]struct{})
	missingCount := 0

	for _, replica := range replicas {
		if _, ok := seenPhysicalPaths[replica.PhysicalPath]; ok {
			continue
		}
		if !replica.ExistsFlag && strings.EqualFold(replica.ReplicaStatus, string(ReplicaStatusMissing)) {
			continue
		}

		replica.ExistsFlag = false
		replica.ReplicaStatus = string(ReplicaStatusMissing)
		replica.UpdatedAt = now
		if err := service.store.UpdateReplica(ctx, replica); err != nil {
			return missingCount, err
		}

		missingCount++
		affectedAssets[replica.AssetID] = struct{}{}
	}

	for assetID := range affectedAssets {
		if err := service.syncAssetStatus(ctx, assetID); err != nil {
			return missingCount, err
		}
	}

	return missingCount, nil
}

func (service *Service) syncAssetStatus(ctx context.Context, assetID string) error {
	asset, err := service.store.GetAssetByID(ctx, assetID)
	if err != nil {
		return err
	}

	replicas, err := service.store.ListReplicasByAssetID(ctx, assetID)
	if err != nil {
		return err
	}

	statusInputs := make([]ReplicaStatusSnapshot, 0, len(replicas))
	diffInputs := make([]ReplicaDiffInput, 0, len(replicas))
	for _, replica := range replicas {
		statusInputs = append(statusInputs, ReplicaStatusSnapshot{
			ReplicaStatus: replica.ReplicaStatus,
			ExistsFlag:    replica.ExistsFlag,
		})

		var size *int64
		var mtime *time.Time
		if replica.VersionID != nil {
			version, versionErr := service.store.GetReplicaVersionByID(ctx, *replica.VersionID)
			if versionErr != nil && !errors.Is(versionErr, sql.ErrNoRows) {
				return versionErr
			}
			if versionErr == nil {
				size = &version.Size
				mtime = cloneTimePointer(version.MTime)
			}
		}

		diffInputs = append(diffInputs, ReplicaDiffInput{
			ReplicaID:     replica.ID,
			EndpointID:    replica.EndpointID,
			ReplicaStatus: replica.ReplicaStatus,
			ExistsFlag:    replica.ExistsFlag,
			Size:          size,
			MTime:         mtime,
		})
	}

	nextStatus := AggregateAssetStatus(statusInputs)
	diffResult := AnalyzeReplicaDifferences(nil, diffInputs)
	if len(diffResult.ConflictEndpointIDs) > 0 && nextStatus != AssetStatusDeleted && nextStatus != AssetStatusPendingDelete {
		nextStatus = AssetStatusConflict
	}

	nextStatusText := string(nextStatus)
	if asset.AssetStatus == nextStatusText {
		return nil
	}

	asset.AssetStatus = nextStatusText
	asset.UpdatedAt = time.Now().UTC()
	return service.store.UpdateAsset(ctx, asset)
}

func (service *Service) createScanTask(ctx context.Context, endpoint store.StorageEndpoint) (store.Task, error) {
	now := time.Now().UTC()
	payload, err := json.Marshal(map[string]string{
		"endpointId":   endpoint.ID,
		"endpointName": endpoint.Name,
		"endpointType": endpoint.EndpointType,
	})
	if err != nil {
		return store.Task{}, err
	}

	task := store.Task{
		ID:        uuid.NewString(),
		TaskType:  "scan_endpoint",
		Status:    "pending",
		Payload:   string(payload),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := service.store.CreateTask(ctx, task); err != nil {
		return store.Task{}, err
	}
	return task, nil
}

func (service *Service) failTask(
	ctx context.Context,
	task store.Task,
	summary EndpointScanSummary,
	failure error,
) (EndpointScanSummary, error) {
	finishedAt := time.Now().UTC()
	summary.Status = "error"
	summary.Error = failure.Error()
	summary.FinishedAt = finishedAt

	resultSummary, marshalErr := json.Marshal(summary)
	if marshalErr != nil {
		return summary, marshalErr
	}
	errorText := failure.Error()
	resultText := string(resultSummary)
	updateErr := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
		Status:        "error",
		ResultSummary: &resultText,
		ErrorMessage:  &errorText,
		RetryCount:    task.RetryCount,
		StartedAt:     &summary.StartedAt,
		FinishedAt:    &finishedAt,
		UpdatedAt:     finishedAt,
	})
	if updateErr != nil {
		return summary, updateErr
	}

	slog.Error(
		"endpoint scan failed",
		"taskId", task.ID,
		"endpointId", summary.EndpointID,
		"endpointType", summary.EndpointType,
		"error", failure,
	)
	return summary, failure
}

func defaultConnectorFactory(endpoint store.StorageEndpoint) (connectors.Connector, error) {
	switch normalizeEndpointType(endpoint.EndpointType) {
	case string(connectors.EndpointTypeLocal):
		var config struct {
			RootPath string `json:"rootPath"`
		}
		_ = json.Unmarshal([]byte(endpoint.ConnectionConfig), &config)
		return connectors.NewLocalConnector(connectors.LocalConfig{
			Name:     endpoint.Name,
			RootPath: defaultString(config.RootPath, endpoint.RootPath),
		})
	case string(connectors.EndpointTypeQNAP):
		var config struct {
			SharePath string `json:"sharePath"`
		}
		_ = json.Unmarshal([]byte(endpoint.ConnectionConfig), &config)
		return connectors.NewQNAPConnector(connectors.QNAPConfig{
			Name:      endpoint.Name,
			SharePath: defaultString(config.SharePath, endpoint.RootPath),
		})
	case string(connectors.EndpointTypeCloud115):
		var config struct {
			RootID      string `json:"rootId"`
			AccessToken string `json:"accessToken"`
			AppType     string `json:"appType"`
		}
		_ = json.Unmarshal([]byte(endpoint.ConnectionConfig), &config)
		return connectors.NewCloud115Connector(connectors.Cloud115Config{
			Name:        endpoint.Name,
			RootID:      defaultString(config.RootID, endpoint.RootPath),
			AccessToken: config.AccessToken,
			AppType:     config.AppType,
		}, nil)
	case string(connectors.EndpointTypeRemovable):
		var config struct {
			Device connectors.DeviceInfo `json:"device"`
		}
		_ = json.Unmarshal([]byte(endpoint.ConnectionConfig), &config)
		if strings.TrimSpace(config.Device.MountPoint) == "" {
			config.Device.MountPoint = endpoint.RootPath
		}
		return connectors.NewRemovableConnector(connectors.RemovableConfig{
			Name:   endpoint.Name,
			Device: config.Device,
		})
	default:
		return nil, fmt.Errorf("unsupported endpoint type: %s", endpoint.EndpointType)
	}
}

func normalizeEndpointType(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "LOCAL":
		return string(connectors.EndpointTypeLocal)
	case "QNAP", "QNAP_SMB":
		return string(connectors.EndpointTypeQNAP)
	case "115", "CLOUD115", "CLOUD_115":
		return string(connectors.EndpointTypeCloud115)
	case "REMOVABLE", "REMOVABLE_DRIVE":
		return string(connectors.EndpointTypeRemovable)
	default:
		return ""
	}
}

func normalizeConnectionConfig(endpointType string, raw json.RawMessage) (json.RawMessage, *endpointCredentialInput, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return json.RawMessage(`{}`), nil, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, fmt.Errorf("invalid connection config: %w", err)
	}

	extractedCredential := extractEndpointCredential(endpointType, payload)

	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("normalize connection config: %w", err)
	}

	switch endpointType {
	case string(connectors.EndpointTypeLocal), string(connectors.EndpointTypeQNAP), string(connectors.EndpointTypeCloud115), string(connectors.EndpointTypeRemovable):
		return normalized, extractedCredential, nil
	default:
		return nil, nil, fmt.Errorf("unsupported endpoint type: %s", endpointType)
	}
}

func resolveRootPath(endpointType, explicitRoot string, connectionConfig json.RawMessage) (string, error) {
	if strings.TrimSpace(explicitRoot) != "" {
		return strings.TrimSpace(explicitRoot), nil
	}

	switch endpointType {
	case string(connectors.EndpointTypeLocal):
		var config struct {
			RootPath string `json:"rootPath"`
		}
		if err := json.Unmarshal(connectionConfig, &config); err != nil {
			return "", err
		}
		if strings.TrimSpace(config.RootPath) == "" {
			return "", errors.New("root path is required")
		}
		return strings.TrimSpace(config.RootPath), nil
	case string(connectors.EndpointTypeQNAP):
		var config struct {
			SharePath string `json:"sharePath"`
		}
		if err := json.Unmarshal(connectionConfig, &config); err != nil {
			return "", err
		}
		if strings.TrimSpace(config.SharePath) == "" {
			return "", errors.New("share path is required")
		}
		return strings.TrimSpace(config.SharePath), nil
	case string(connectors.EndpointTypeCloud115):
		var config struct {
			RootID string `json:"rootId"`
		}
		if err := json.Unmarshal(connectionConfig, &config); err != nil {
			return "", err
		}
		if strings.TrimSpace(config.RootID) == "" {
			return "", errors.New("115 root id is required")
		}
		return strings.TrimSpace(config.RootID), nil
	case string(connectors.EndpointTypeRemovable):
		var config struct {
			Device connectors.DeviceInfo `json:"device"`
		}
		if err := json.Unmarshal(connectionConfig, &config); err != nil {
			return "", err
		}
		if strings.TrimSpace(config.Device.MountPoint) == "" {
			return "", errors.New("removable device mount point is required")
		}
		return strings.TrimSpace(config.Device.MountPoint), nil
	default:
		return "", errors.New("unsupported endpoint type")
	}
}

func resolveIdentitySignature(
	endpointType string,
	rootPath string,
	explicitIdentity string,
	connectionConfig json.RawMessage,
) (string, error) {
	if strings.TrimSpace(explicitIdentity) != "" {
		return strings.TrimSpace(explicitIdentity), nil
	}

	switch endpointType {
	case string(connectors.EndpointTypeRemovable):
		var config struct {
			Device connectors.DeviceInfo `json:"device"`
		}
		if err := json.Unmarshal(connectionConfig, &config); err != nil {
			return "", err
		}
		return connectors.GenerateDeviceIdentity(config.Device), nil
	default:
		sum := sha256.Sum256([]byte(strings.Join([]string{
			strings.ToLower(endpointType),
			strings.ToLower(canonicalizePath(rootPath)),
		}, "|")))
		return hex.EncodeToString(sum[:]), nil
	}
}

func toEndpointRecord(endpoint store.StorageEndpoint) EndpointRecord {
	return EndpointRecord{
		ID:                 endpoint.ID,
		Name:               endpoint.Name,
		Note:               endpoint.Note,
		EndpointType:       endpoint.EndpointType,
		RootPath:           endpoint.RootPath,
		RoleMode:           endpoint.RoleMode,
		IdentitySignature:  endpoint.IdentitySignature,
		AvailabilityStatus: endpoint.AvailabilityStatus,
		CredentialRef:      endpoint.CredentialRef,
		CredentialHint:     endpoint.CredentialHint,
		HasCredential:      strings.TrimSpace(endpoint.CredentialRef) != "",
		ConnectionConfig:   json.RawMessage(endpoint.ConnectionConfig),
		CreatedAt:          endpoint.CreatedAt,
		UpdatedAt:          endpoint.UpdatedAt,
	}
}

func versionMatches(version store.ReplicaVersion, result ScanResult) bool {
	return version.Size == result.Size && sameTimePointer(version.MTime, result.MTime)
}

func shouldReplaceMediaType(current, next string) bool {
	current = strings.TrimSpace(strings.ToLower(current))
	next = strings.TrimSpace(strings.ToLower(next))
	if next == "" || next == string(connectors.MediaTypeUnknown) {
		return false
	}
	return current == "" || current == string(connectors.MediaTypeUnknown)
}

func selectPrimaryTimestamp(current, candidate *time.Time) *time.Time {
	if current == nil {
		return cloneTimePointer(candidate)
	}
	if candidate == nil {
		return cloneTimePointer(current)
	}
	if candidate.Before(*current) {
		return cloneTimePointer(candidate)
	}
	return cloneTimePointer(current)
}

func sameTimePointer(left, right *time.Time) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return left.UTC().Equal(right.UTC())
	}
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := value.UTC()
	return &clone
}

func stringPointer(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	copied := value
	return &copied
}

func defaultEndpointName(endpointType string) string {
	switch endpointType {
	case string(connectors.EndpointTypeLocal):
		return "Local Endpoint"
	case string(connectors.EndpointTypeQNAP):
		return "QNAP SMB"
	case string(connectors.EndpointTypeCloud115):
		return "115 Cloud"
	case string(connectors.EndpointTypeRemovable):
		return "Removable Drive"
	default:
		return "Storage Endpoint"
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
