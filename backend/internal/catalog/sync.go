package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"mam/backend/internal/store"
)

const (
	taskTypeRestoreAsset = "restore_asset"
	taskTypeRestoreBatch = "restore_batch"

	taskStatusPending  = "pending"
	taskStatusRunning  = "running"
	taskStatusRetrying = "retrying"
	taskStatusSuccess  = "success"
	taskStatusFailed   = "failed"
)

type SyncEndpointRef struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	EndpointType string `json:"endpointType"`
}

type SyncReplicaRecord struct {
	ID                 string              `json:"id"`
	EndpointID         string              `json:"endpointId"`
	EndpointName       string              `json:"endpointName"`
	PhysicalPath       string              `json:"physicalPath"`
	RelativePath       string              `json:"relativePath"`
	LogicalDirectory   string              `json:"logicalDirectory"`
	ResolvedDirectory  string              `json:"resolvedDirectory"`
	MatchesLogicalPath bool                `json:"matchesLogicalPath"`
	ReplicaStatus      string              `json:"replicaStatus"`
	ExistsFlag         bool                `json:"existsFlag"`
	LastSeenAt         *time.Time          `json:"lastSeenAt,omitempty"`
	Version            *AssetVersionRecord `json:"version,omitempty"`
}

type SyncAssetRecord struct {
	ID                    string              `json:"id"`
	DisplayName           string              `json:"displayName"`
	LogicalPathKey        string              `json:"logicalPathKey"`
	CanonicalPath         string              `json:"canonicalPath"`
	CanonicalDirectory    string              `json:"canonicalDirectory"`
	MediaType             string              `json:"mediaType"`
	AssetStatus           string              `json:"assetStatus"`
	PrimaryTimestamp      *time.Time          `json:"primaryTimestamp,omitempty"`
	Poster                *AssetPreviewRecord `json:"poster,omitempty"`
	AvailableReplicaCount int                 `json:"availableReplicaCount"`
	MissingReplicaCount   int                 `json:"missingReplicaCount"`
	MissingEndpoints      []SyncEndpointRef   `json:"missingEndpoints"`
	ConsistentEndpoints   []SyncEndpointRef   `json:"consistentEndpoints"`
	UpdatedEndpoints      []SyncEndpointRef   `json:"updatedEndpoints"`
	ConflictEndpoints     []SyncEndpointRef   `json:"conflictEndpoints"`
	RecommendedSource     *SyncEndpointRef    `json:"recommendedSource,omitempty"`
	Replicas              []SyncReplicaRecord `json:"replicas"`
}

type SyncOverviewRecord struct {
	GeneratedAt       time.Time         `json:"generatedAt"`
	RecoverableAssets []SyncAssetRecord `json:"recoverableAssets"`
	ConflictAssets    []SyncAssetRecord `json:"conflictAssets"`
	RunningTasks      []store.Task      `json:"runningTasks"`
	FailedTasks       []store.Task      `json:"failedTasks"`
}

type RestoreAssetRequest struct {
	AssetID          string `json:"assetId"`
	SourceEndpointID string `json:"sourceEndpointId"`
	TargetEndpointID string `json:"targetEndpointId"`
}

type RestoreAssetSummary struct {
	TaskID             string    `json:"taskId"`
	AssetID            string    `json:"assetId"`
	DisplayName        string    `json:"displayName"`
	SourceEndpointID   string    `json:"sourceEndpointId"`
	SourceEndpointName string    `json:"sourceEndpointName"`
	TargetEndpointID   string    `json:"targetEndpointId"`
	TargetEndpointName string    `json:"targetEndpointName"`
	TargetPhysicalPath string    `json:"targetPhysicalPath,omitempty"`
	Status             string    `json:"status"`
	CreatedReplica     bool      `json:"createdReplica"`
	UpdatedReplica     bool      `json:"updatedReplica"`
	Skipped            bool      `json:"skipped"`
	ProgressPercent    int       `json:"progressPercent"`
	ProgressLabel      string    `json:"progressLabel,omitempty"`
	StartedAt          time.Time `json:"startedAt"`
	FinishedAt         time.Time `json:"finishedAt"`
	Error              string    `json:"error,omitempty"`
}

type BatchRestoreRequest struct {
	TargetEndpointID string   `json:"targetEndpointId"`
	AssetIDs         []string `json:"assetIds"`
}

type BatchRestoreItemResult struct {
	AssetID          string `json:"assetId"`
	DisplayName      string `json:"displayName"`
	SourceEndpointID string `json:"sourceEndpointId,omitempty"`
	TargetEndpointID string `json:"targetEndpointId"`
	Status           string `json:"status"`
	Skipped          bool   `json:"skipped"`
	Error            string `json:"error,omitempty"`
}

type BatchRestoreSummary struct {
	TaskID             string                   `json:"taskId"`
	TargetEndpointID   string                   `json:"targetEndpointId"`
	TargetEndpointName string                   `json:"targetEndpointName"`
	Status             string                   `json:"status"`
	TotalAssets        int                      `json:"totalAssets"`
	SuccessCount       int                      `json:"successCount"`
	FailedCount        int                      `json:"failedCount"`
	SkippedCount       int                      `json:"skippedCount"`
	ProgressPercent    int                      `json:"progressPercent"`
	ProgressLabel      string                   `json:"progressLabel,omitempty"`
	Items              []BatchRestoreItemResult `json:"items"`
	StartedAt          time.Time                `json:"startedAt"`
	FinishedAt         time.Time                `json:"finishedAt"`
	Error              string                   `json:"error,omitempty"`
}

type RetrySyncTaskSummary = RetryTaskSummary

type restoreExecutionResult struct {
	asset              store.Asset
	sourceEndpoint     store.StorageEndpoint
	targetEndpoint     store.StorageEndpoint
	targetPhysicalPath string
	createdReplica     bool
	updatedReplica     bool
	skipped            bool
}

func (service *Service) GetSyncOverview(ctx context.Context) (SyncOverviewRecord, error) {
	enabledEndpoints, endpointLookup, err := service.listManagedEnabledEndpoints(ctx)
	if err != nil {
		return SyncOverviewRecord{}, err
	}

	expectedEndpointIDs := make([]string, 0, len(enabledEndpoints))
	for _, endpoint := range enabledEndpoints {
		expectedEndpointIDs = append(expectedEndpointIDs, endpoint.ID)
	}

	assets, err := service.store.ListAssets(ctx, 10_000, 0)
	if err != nil {
		return SyncOverviewRecord{}, err
	}

	recoverableAssets := make([]SyncAssetRecord, 0)
	conflictAssets := make([]SyncAssetRecord, 0)

	for _, asset := range assets {
		record, diffResult, err := service.buildSyncAssetRecord(ctx, asset, enabledEndpoints, endpointLookup, expectedEndpointIDs)
		if err != nil {
			return SyncOverviewRecord{}, err
		}

		if len(diffResult.ConflictEndpointIDs) > 0 {
			conflictAssets = append(conflictAssets, record)
			continue
		}

		if len(record.MissingEndpoints) > 0 && record.RecommendedSource != nil {
			recoverableAssets = append(recoverableAssets, record)
		}
	}

	tasks, err := service.store.ListTasks(ctx, 200, 0)
	if err != nil {
		return SyncOverviewRecord{}, err
	}

	runningTasks := make([]store.Task, 0)
	failedTasks := make([]store.Task, 0)
	for _, task := range tasks {
		if !isSyncTaskType(task.TaskType) {
			continue
		}

		status := strings.ToLower(strings.TrimSpace(task.Status))
		switch status {
		case taskStatusPending, taskStatusRunning, taskStatusRetrying:
			runningTasks = append(runningTasks, task)
		case taskStatusFailed, "error":
			failedTasks = append(failedTasks, task)
		}
	}

	return SyncOverviewRecord{
		GeneratedAt:       time.Now().UTC(),
		RecoverableAssets: recoverableAssets,
		ConflictAssets:    conflictAssets,
		RunningTasks:      runningTasks,
		FailedTasks:       failedTasks,
	}, nil
}

func (service *Service) RestoreAsset(ctx context.Context, request RestoreAssetRequest) (RestoreAssetSummary, error) {
	return service.QueueRestoreAsset(ctx, request)
}

func (service *Service) RestoreAssetsToEndpoint(ctx context.Context, request BatchRestoreRequest) (BatchRestoreSummary, error) {
	return service.QueueRestoreAssetsToEndpoint(ctx, request)
}

func (service *Service) buildSyncAssetRecord(
	ctx context.Context,
	asset store.Asset,
	endpoints []store.StorageEndpoint,
	endpointLookup map[string]store.StorageEndpoint,
	expectedEndpointIDs []string,
) (SyncAssetRecord, ReplicaDiffResult, error) {
	replicas, err := service.store.ListReplicasByAssetID(ctx, asset.ID)
	if err != nil {
		return SyncAssetRecord{}, ReplicaDiffResult{}, err
	}

	diffInputs := make([]ReplicaDiffInput, 0, len(replicas))

	for _, replica := range replicas {
		_, size, mtime, versionErr := service.buildVersionRecord(ctx, replica.VersionID)
		if versionErr != nil {
			return SyncAssetRecord{}, ReplicaDiffResult{}, versionErr
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

	diffResult := AnalyzeReplicaDifferences(expectedEndpointIDs, diffInputs)
	replicaRecords, availableReplicaCount, missingReplicaCount, err := service.buildSyncReplicaRecords(
		ctx,
		replicas,
		endpoints,
		endpointLookup,
		asset.LogicalPathKey,
	)
	if err != nil {
		return SyncAssetRecord{}, ReplicaDiffResult{}, err
	}
	posterRecord, err := service.buildPosterRecord(ctx, asset)
	if err != nil {
		return SyncAssetRecord{}, ReplicaDiffResult{}, err
	}

	recommendedSource, _, err := service.selectPreferredSourceEndpoint(
		ctx,
		replicas,
		"",
		preferredSourceEndpointIDs(diffResult),
	)
	if err != nil {
		return SyncAssetRecord{}, ReplicaDiffResult{}, err
	}

	record := SyncAssetRecord{
		ID:                    asset.ID,
		DisplayName:           asset.DisplayName,
		LogicalPathKey:        asset.LogicalPathKey,
		CanonicalPath:         canonicalLogicalPath(asset.LogicalPathKey),
		CanonicalDirectory:    canonicalDirectoryPath(asset.LogicalPathKey),
		MediaType:             asset.MediaType,
		AssetStatus:           asset.AssetStatus,
		PrimaryTimestamp:      asset.PrimaryTimestamp,
		Poster:                posterRecord,
		AvailableReplicaCount: availableReplicaCount,
		MissingReplicaCount:   missingReplicaCount,
		MissingEndpoints:      buildSyncEndpointRefs(diffResult.MissingEndpointIDs, endpointLookup),
		ConsistentEndpoints:   buildSyncEndpointRefs(diffResult.ConsistentEndpointIDs, endpointLookup),
		UpdatedEndpoints:      buildSyncEndpointRefs(diffResult.UpdatedEndpointIDs, endpointLookup),
		ConflictEndpoints:     buildSyncEndpointRefs(diffResult.ConflictEndpointIDs, endpointLookup),
		Replicas:              replicaRecords,
	}
	if recommendedSource != nil && len(diffResult.ConflictEndpointIDs) == 0 {
		record.RecommendedSource = &SyncEndpointRef{
			ID:           recommendedSource.ID,
			Name:         recommendedSource.Name,
			EndpointType: recommendedSource.EndpointType,
		}
	}

	_ = endpoints
	return record, diffResult, nil
}

func (service *Service) executeRestoreAsset(
	ctx context.Context,
	request RestoreAssetRequest,
	scanRevision string,
	progress func(progressPercent int, progressLabel string) error,
) (restoreExecutionResult, error) {
	assetID := strings.TrimSpace(request.AssetID)
	sourceEndpointID := strings.TrimSpace(request.SourceEndpointID)
	targetEndpointID := strings.TrimSpace(request.TargetEndpointID)
	if assetID == "" || sourceEndpointID == "" || targetEndpointID == "" {
		return restoreExecutionResult{}, errors.New("assetId, sourceEndpointId and targetEndpointId are required")
	}
	if sourceEndpointID == targetEndpointID {
		return restoreExecutionResult{}, errors.New("source and target endpoints must be different")
	}

	asset, err := service.store.GetAssetByID(ctx, assetID)
	if err != nil {
		return restoreExecutionResult{}, err
	}

	sourceEndpoint, err := service.store.GetStorageEndpointByID(ctx, sourceEndpointID)
	if err != nil {
		return restoreExecutionResult{}, err
	}

	targetEndpoint, err := service.store.GetStorageEndpointByID(ctx, targetEndpointID)
	if err != nil {
		return restoreExecutionResult{}, err
	}

	sourceReplica, err := service.store.GetReplicaByAssetAndEndpoint(ctx, assetID, sourceEndpointID)
	if err != nil {
		return restoreExecutionResult{}, err
	}
	if !sourceReplica.ExistsFlag || normalizeReplicaLifecycle(sourceReplica.ReplicaStatus, sourceReplica.ExistsFlag) != replicaLifecycleAvailable {
		return restoreExecutionResult{}, errors.New("source replica is not available for restore")
	}

	sourceConnector, err := service.buildConnector(sourceEndpoint)
	if err != nil {
		return restoreExecutionResult{}, err
	}
	targetConnector, err := service.buildConnector(targetEndpoint)
	if err != nil {
		return restoreExecutionResult{}, err
	}

	if !sourceConnector.Descriptor().Capabilities.CanReadStream {
		return restoreExecutionResult{}, errors.New("source endpoint does not support read stream")
	}
	if !targetConnector.Descriptor().Capabilities.CanWrite {
		return restoreExecutionResult{}, errors.New("target endpoint does not support write")
	}

	var sourceVersion *store.ReplicaVersion
	if sourceReplica.VersionID != nil {
		version, versionErr := service.store.GetReplicaVersionByID(ctx, *sourceReplica.VersionID)
		if versionErr != nil && !errors.Is(versionErr, sql.ErrNoRows) {
			return restoreExecutionResult{}, versionErr
		}
		if versionErr == nil {
			versionCopy := version
			sourceVersion = &versionCopy
		}
	}

	targetReplica, targetReplicaErr := service.store.GetReplicaByAssetAndEndpoint(ctx, assetID, targetEndpointID)
	targetReplicaExists := targetReplicaErr == nil
	if targetReplicaErr != nil && !errors.Is(targetReplicaErr, sql.ErrNoRows) {
		return restoreExecutionResult{}, targetReplicaErr
	}

	if targetReplicaExists && targetReplica.ExistsFlag && targetReplica.VersionID != nil && sourceReplica.VersionID != nil && *targetReplica.VersionID == *sourceReplica.VersionID {
		if err := service.syncAssetStatus(ctx, assetID); err != nil {
			return restoreExecutionResult{}, err
		}
		return restoreExecutionResult{
			asset:              asset,
			sourceEndpoint:     sourceEndpoint,
			targetEndpoint:     targetEndpoint,
			targetPhysicalPath: targetReplica.PhysicalPath,
			skipped:            true,
		}, nil
	}

	if progress != nil {
		if err := progress(12, "读取源副本"); err != nil {
			return restoreExecutionResult{}, err
		}
	}

	localSourcePath, cleanup, err := service.materializeReplicaToLocalFile(ctx, sourceEndpoint, sourceReplica)
	if err != nil {
		return restoreExecutionResult{}, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	sourceReader, err := os.Open(localSourcePath)
	if err != nil {
		return restoreExecutionResult{}, err
	}
	defer sourceReader.Close()

	if progress != nil {
		if err := progress(48, "写入目标位置"); err != nil {
			return restoreExecutionResult{}, err
		}
	}

	destinationPath := deriveRestoreRelativePath(sourceEndpoint.RootPath, sourceReplica.PhysicalPath, asset.LogicalPathKey)
	if destinationPath == "" {
		return restoreExecutionResult{}, errors.New("unable to derive restore destination path")
	}

	copiedEntry, err := targetConnector.CopyIn(ctx, destinationPath, sourceReader)
	if err != nil {
		return restoreExecutionResult{}, err
	}

	if progress != nil {
		if err := progress(82, "登记副本信息"); err != nil {
			return restoreExecutionResult{}, err
		}
	}

	scanResult := ScanResult{
		EndpointID:     targetEndpoint.ID,
		PhysicalPath:   defaultString(strings.TrimSpace(copiedEntry.Path), destinationPath),
		LogicalPathKey: asset.LogicalPathKey,
		Size:           copiedEntry.Size,
		MTime:          cloneTimePointer(copiedEntry.ModifiedAt),
		MediaType:      asset.MediaType,
		IsDir:          false,
	}
	if scanResult.Size == 0 && sourceVersion != nil {
		scanResult.Size = sourceVersion.Size
	}
	if scanResult.MTime == nil && sourceVersion != nil {
		scanResult.MTime = cloneTimePointer(sourceVersion.MTime)
	}

	versionID, _, err := service.resolveReplicaVersion(ctx, targetReplica.VersionID, scanResult, scanRevision)
	if err != nil {
		return restoreExecutionResult{}, err
	}

	now := time.Now().UTC()
	lastSeenAt := now
	result := restoreExecutionResult{
		asset:              asset,
		sourceEndpoint:     sourceEndpoint,
		targetEndpoint:     targetEndpoint,
		targetPhysicalPath: scanResult.PhysicalPath,
	}

	if !targetReplicaExists {
		replica := store.Replica{
			ID:            uuid.NewString(),
			AssetID:       asset.ID,
			EndpointID:    targetEndpoint.ID,
			PhysicalPath:  scanResult.PhysicalPath,
			ReplicaStatus: string(ReplicaStatusActive),
			ExistsFlag:    true,
			VersionID:     versionID,
			LastSeenAt:    &lastSeenAt,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := service.store.CreateReplica(ctx, replica); err != nil {
			return restoreExecutionResult{}, err
		}
		result.createdReplica = true
	} else {
		targetReplica.PhysicalPath = scanResult.PhysicalPath
		targetReplica.ReplicaStatus = string(ReplicaStatusActive)
		targetReplica.ExistsFlag = true
		targetReplica.VersionID = versionID
		targetReplica.LastSeenAt = &lastSeenAt
		targetReplica.UpdatedAt = now
		if err := service.store.UpdateReplica(ctx, targetReplica); err != nil {
			return restoreExecutionResult{}, err
		}
		result.updatedReplica = true
	}

	if err := service.syncAssetStatus(ctx, asset.ID); err != nil {
		return restoreExecutionResult{}, err
	}

	if progress != nil {
		if err := progress(96, "刷新资产状态"); err != nil {
			return restoreExecutionResult{}, err
		}
	}

	return result, nil
}

func (service *Service) selectSourceReplicaForRestore(
	ctx context.Context,
	assetID string,
	targetEndpointID string,
) (store.Replica, store.StorageEndpoint, store.Asset, error) {
	asset, err := service.store.GetAssetByID(ctx, assetID)
	if err != nil {
		return store.Replica{}, store.StorageEndpoint{}, store.Asset{}, err
	}

	replicas, err := service.store.ListReplicasByAssetID(ctx, assetID)
	if err != nil {
		return store.Replica{}, store.StorageEndpoint{}, store.Asset{}, err
	}

	diffInputs, err := service.buildReplicaDiffInputs(ctx, replicas)
	if err != nil {
		return store.Replica{}, store.StorageEndpoint{}, store.Asset{}, err
	}

	diffResult := AnalyzeReplicaDifferences(nil, diffInputs)
	if len(diffResult.ConflictEndpointIDs) > 0 {
		return store.Replica{}, store.StorageEndpoint{}, store.Asset{}, errors.New("asset has conflicting replicas and needs manual review before restore")
	}

	endpoint, candidate, err := service.selectPreferredSourceEndpoint(
		ctx,
		replicas,
		targetEndpointID,
		preferredSourceEndpointIDs(diffResult),
	)
	if err != nil {
		return store.Replica{}, store.StorageEndpoint{}, store.Asset{}, err
	}
	if candidate == nil || endpoint == nil {
		return store.Replica{}, store.StorageEndpoint{}, store.Asset{}, errors.New("no healthy source replica is available")
	}

	return *candidate, *endpoint, asset, nil
}

func (service *Service) selectPreferredSourceEndpoint(
	ctx context.Context,
	replicas []store.Replica,
	excludedEndpointID string,
	preferredEndpointIDs []string,
) (*store.StorageEndpoint, *store.Replica, error) {
	type candidate struct {
		endpoint  store.StorageEndpoint
		replica   store.Replica
		preferred bool
		priority  int
	}

	preferredEndpointSet := make(map[string]struct{}, len(preferredEndpointIDs))
	for _, endpointID := range preferredEndpointIDs {
		normalized := strings.TrimSpace(endpointID)
		if normalized == "" {
			continue
		}
		preferredEndpointSet[normalized] = struct{}{}
	}

	candidates := make([]candidate, 0, len(replicas))
	for _, replica := range replicas {
		if strings.TrimSpace(excludedEndpointID) != "" && replica.EndpointID == excludedEndpointID {
			continue
		}
		if !replica.ExistsFlag || normalizeReplicaLifecycle(replica.ReplicaStatus, replica.ExistsFlag) != replicaLifecycleAvailable {
			continue
		}

		endpoint, err := service.store.GetStorageEndpointByID(ctx, replica.EndpointID)
		if err != nil {
			return nil, nil, err
		}
		connector, err := service.buildConnector(endpoint)
		if err != nil {
			continue
		}
		if !connector.Descriptor().Capabilities.CanReadStream {
			continue
		}

		candidates = append(candidates, candidate{
			endpoint:  endpoint,
			replica:   replica,
			preferred: len(preferredEndpointSet) == 0 || endpointIDInSet(replica.EndpointID, preferredEndpointSet),
			priority:  readableReplicaPriority(endpoint.EndpointType),
		})
	}

	if len(candidates) == 0 {
		return nil, nil, nil
	}

	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].preferred != candidates[right].preferred {
			return candidates[left].preferred
		}
		if candidates[left].priority != candidates[right].priority {
			return candidates[left].priority < candidates[right].priority
		}
		return strings.ToLower(candidates[left].endpoint.Name) < strings.ToLower(candidates[right].endpoint.Name)
	})

	return &candidates[0].endpoint, &candidates[0].replica, nil
}

func (service *Service) createCatalogTask(ctx context.Context, taskType string, payload any) (store.Task, error) {
	now := time.Now().UTC()
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return store.Task{}, err
	}

	task := store.Task{
		ID:        uuid.NewString(),
		TaskType:  taskType,
		Status:    taskStatusPending,
		Payload:   string(encodedPayload),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := service.store.CreateTask(ctx, task); err != nil {
		return store.Task{}, err
	}
	return task, nil
}

func (service *Service) failBatchRestoreTask(
	ctx context.Context,
	task store.Task,
	summary BatchRestoreSummary,
	startedAt time.Time,
	failure error,
) (BatchRestoreSummary, error) {
	finishedAt := time.Now().UTC()
	summary.Status = taskStatusFailed
	summary.Error = failure.Error()
	summary.FinishedAt = finishedAt

	resultText, marshalErr := json.Marshal(summary)
	if marshalErr != nil {
		return summary, marshalErr
	}
	resultSummary := string(resultText)
	errorText := failure.Error()
	if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
		Status:        taskStatusFailed,
		ResultSummary: &resultSummary,
		ErrorMessage:  &errorText,
		RetryCount:    task.RetryCount,
		StartedAt:     &startedAt,
		FinishedAt:    &finishedAt,
		UpdatedAt:     finishedAt,
	}); err != nil {
		return summary, err
	}

	slog.Error("batch restore failed", "taskId", task.ID, "targetEndpointId", summary.TargetEndpointID, "error", failure)
	return summary, failure
}

func buildSyncEndpointRefs(endpointIDs []string, lookup map[string]store.StorageEndpoint) []SyncEndpointRef {
	refs := make([]SyncEndpointRef, 0, len(endpointIDs))
	for _, endpointID := range endpointIDs {
		endpoint, ok := lookup[endpointID]
		if !ok {
			refs = append(refs, SyncEndpointRef{ID: endpointID, Name: endpointID})
			continue
		}
		refs = append(refs, SyncEndpointRef{
			ID:           endpoint.ID,
			Name:         endpoint.Name,
			EndpointType: endpoint.EndpointType,
		})
	}
	return refs
}

func deriveRestoreRelativePath(rootPath, physicalPath, fallback string) string {
	_ = rootPath
	_ = physicalPath
	return canonicalLogicalPath(fallback)
}

func isSyncTaskType(taskType string) bool {
	switch strings.TrimSpace(taskType) {
	case taskTypeRestoreAsset, taskTypeRestoreBatch:
		return true
	default:
		return false
	}
}

func drainReader(reader io.Reader) error {
	_, err := io.Copy(io.Discard, reader)
	return err
}

func (service *Service) buildReplicaDiffInputs(ctx context.Context, replicas []store.Replica) ([]ReplicaDiffInput, error) {
	diffInputs := make([]ReplicaDiffInput, 0, len(replicas))

	for _, replica := range replicas {
		var size *int64
		var mtime *time.Time
		if replica.VersionID != nil {
			version, err := service.store.GetReplicaVersionByID(ctx, *replica.VersionID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			if err == nil {
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

	return diffInputs, nil
}

func preferredSourceEndpointIDs(diffResult ReplicaDiffResult) []string {
	switch {
	case len(diffResult.UpdatedEndpointIDs) > 0:
		return diffResult.UpdatedEndpointIDs
	case len(diffResult.ConsistentEndpointIDs) > 0:
		return diffResult.ConsistentEndpointIDs
	default:
		return nil
	}
}

func endpointIDInSet(endpointID string, values map[string]struct{}) bool {
	_, ok := values[strings.TrimSpace(endpointID)]
	return ok
}

func (service *Service) updateRestoreTaskProgress(
	ctx context.Context,
	taskID string,
	retryCount int,
	startedAt time.Time,
	summary RestoreAssetSummary,
) error {
	resultText, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	value := string(resultText)
	now := time.Now().UTC()
	return service.store.UpdateTaskStatus(ctx, taskID, store.TaskStatusUpdate{
		Status:        taskStatusRunning,
		ResultSummary: &value,
		RetryCount:    retryCount,
		StartedAt:     &startedAt,
		UpdatedAt:     now,
	})
}

func (service *Service) updateBatchRestoreProgress(
	ctx context.Context,
	taskID string,
	retryCount int,
	startedAt time.Time,
	summary BatchRestoreSummary,
) error {
	resultText, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	value := string(resultText)
	now := time.Now().UTC()
	return service.store.UpdateTaskStatus(ctx, taskID, store.TaskStatusUpdate{
		Status:        taskStatusRunning,
		ResultSummary: &value,
		RetryCount:    retryCount,
		StartedAt:     &startedAt,
		UpdatedAt:     now,
	})
}

func calcProgressPercent(processedCount, totalCount int) int {
	if totalCount <= 0 {
		return 0
	}

	percent := (processedCount * 100) / totalCount
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

func buildBatchRestoreProgressLabel(summary BatchRestoreSummary, displayName string) string {
	if summary.TotalAssets <= 0 {
		return "开始处理资产"
	}

	return fmt.Sprintf(
		"已处理 %d/%d 个资产，当前：%s",
		len(summary.Items),
		summary.TotalAssets,
		defaultString(strings.TrimSpace(displayName), "未命名资产"),
	)
}
