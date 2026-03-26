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
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"mam/backend/internal/connectors"
	"mam/backend/internal/store"
)

const (
	taskStatusQueued   = "queued"
	taskStatusPaused   = "paused"
	taskStatusCanceled = "canceled"

	transferItemStatusSkipped = "skipped"

	transferPhasePending    = "pending"
	transferPhaseStaging    = "staging"
	transferPhaseCommitting = "committing"
	transferPhaseFinalizing = "finalizing"
	transferPhaseCompleted  = "completed"
	transferPhasePaused     = "paused"
	transferPhaseCanceled   = "canceled"
	transferPhaseFailed     = "failed"

	transferSourceKindEndpoint = "endpoint"
	transferSourceKindDevice   = "device"

	transferDirectionUpload   = "upload"
	transferDirectionDownload = "download"
	transferDirectionSync     = "sync"

	transferLoopInterval          = 1500 * time.Millisecond
	transferProgressPersistWindow = 750 * time.Millisecond
	transferProgressPersistBytes  = 4 << 20
	transferCopyBufferSize        = 1 << 20
	transferConcurrentTasks       = 2
)

const (
	defaultUploadConcurrency   = 1
	defaultDownloadConcurrency = 1
	maxTransferConcurrency     = 8
)

type TransferTaskStats struct {
	GeneratedAt    time.Time `json:"generatedAt"`
	TotalTasks     int       `json:"totalTasks"`
	QueuedTasks    int       `json:"queuedTasks"`
	RunningTasks   int       `json:"runningTasks"`
	PausedTasks    int       `json:"pausedTasks"`
	FailedTasks    int       `json:"failedTasks"`
	SuccessTasks   int       `json:"successTasks"`
	UploadTasks    int       `json:"uploadTasks"`
	DownloadTasks  int       `json:"downloadTasks"`
	SyncTasks      int       `json:"syncTasks"`
	TotalItems     int       `json:"totalItems"`
	RunningItems   int       `json:"runningItems"`
	PausedItems    int       `json:"pausedItems"`
	FailedItems    int       `json:"failedItems"`
	SuccessItems   int       `json:"successItems"`
	SkippedItems   int       `json:"skippedItems"`
	TotalBytes     int64     `json:"totalBytes"`
	CompletedBytes int64     `json:"completedBytes"`
}

type TransferTaskRecord struct {
	ID              string     `json:"id"`
	TaskType        string     `json:"taskType"`
	Title           string     `json:"title"`
	Direction       string     `json:"direction"`
	Status          string     `json:"status"`
	Priority        int        `json:"priority"`
	SourceLabel     string     `json:"sourceLabel,omitempty"`
	TargetLabel     string     `json:"targetLabel,omitempty"`
	EngineSummary   string     `json:"engineSummary,omitempty"`
	ProgressPercent int        `json:"progressPercent"`
	ProgressLabel   string     `json:"progressLabel,omitempty"`
	CurrentItemName string     `json:"currentItemName,omitempty"`
	FileName        string     `json:"fileName,omitempty"`
	FilePath        string     `json:"filePath,omitempty"`
	FileSize        int64      `json:"fileSize,omitempty"`
	FileTransferred int64      `json:"fileTransferredBytes,omitempty"`
	CurrentSpeed    int64      `json:"currentSpeed,omitempty"`
	RefreshInterval int        `json:"refreshIntervalSeconds,omitempty"`
	TotalItems      int        `json:"totalItems"`
	QueuedItems     int        `json:"queuedItems"`
	RunningItems    int        `json:"runningItems"`
	PausedItems     int        `json:"pausedItems"`
	FailedItems     int        `json:"failedItems"`
	SuccessItems    int        `json:"successItems"`
	SkippedItems    int        `json:"skippedItems"`
	TotalBytes      int64      `json:"totalBytes"`
	CompletedBytes  int64      `json:"completedBytes"`
	ErrorMessage    string     `json:"errorMessage,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty"`
}

type TransferTaskItemRecord struct {
	ID               string     `json:"id"`
	TaskID           string     `json:"taskId"`
	ItemIndex        int        `json:"itemIndex"`
	GroupKey         string     `json:"groupKey"`
	Direction        string     `json:"direction"`
	DisplayName      string     `json:"displayName"`
	MediaType        string     `json:"mediaType"`
	SourceLabel      string     `json:"sourceLabel,omitempty"`
	SourcePath       string     `json:"sourcePath"`
	TargetLabel      string     `json:"targetLabel,omitempty"`
	TargetPath       string     `json:"targetPath"`
	AssetID          string     `json:"assetId,omitempty"`
	LogicalPathKey   string     `json:"logicalPathKey,omitempty"`
	Status           string     `json:"status"`
	Phase            string     `json:"phase"`
	EngineKind       string     `json:"engineKind,omitempty"`
	EngineLabel      string     `json:"engineLabel,omitempty"`
	CurrentSpeed     int64      `json:"currentSpeed,omitempty"`
	RefreshInterval  int        `json:"refreshIntervalSeconds,omitempty"`
	ExternalTaskID   string     `json:"externalTaskId,omitempty"`
	ExternalStatus   string     `json:"externalStatus,omitempty"`
	ProgressPercent  int        `json:"progressPercent"`
	TotalBytes       int64      `json:"totalBytes"`
	TransferredBytes int64      `json:"transferredBytes"`
	StagedBytes      int64      `json:"stagedBytes,omitempty"`
	CommittedBytes   int64      `json:"committedBytes,omitempty"`
	ErrorMessage     string     `json:"errorMessage,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	StartedAt        *time.Time `json:"startedAt,omitempty"`
	FinishedAt       *time.Time `json:"finishedAt,omitempty"`
}

type TransferTaskListResult struct {
	GeneratedAt time.Time            `json:"generatedAt"`
	Stats       TransferTaskStats    `json:"stats"`
	Tasks       []TransferTaskRecord `json:"tasks"`
}

type TransferTaskDetailRecord struct {
	Task  TransferTaskRecord       `json:"task"`
	Items []TransferTaskItemRecord `json:"items"`
}

type TransferTaskActionSummary struct {
	Requested int      `json:"requested"`
	Updated   int      `json:"updated"`
	TaskIDs   []string `json:"taskIds"`
	Message   string   `json:"message"`
}

type transferQueueSnapshot struct {
	task   store.Task
	items  []store.TransferTaskItem
	record TransferTaskRecord
}

type transferTaskSummaryPayload struct {
	TaskID          string `json:"taskId"`
	Title           string `json:"title"`
	Direction       string `json:"direction"`
	Status          string `json:"status"`
	Priority        int    `json:"priority"`
	SourceLabel     string `json:"sourceLabel,omitempty"`
	TargetLabel     string `json:"targetLabel,omitempty"`
	EngineSummary   string `json:"engineSummary,omitempty"`
	ProgressPercent int    `json:"progressPercent"`
	ProgressLabel   string `json:"progressLabel,omitempty"`
	CurrentItemName string `json:"currentItemName,omitempty"`
	FileName        string `json:"fileName,omitempty"`
	FilePath        string `json:"filePath,omitempty"`
	FileSize        int64  `json:"fileSize,omitempty"`
	FileTransferred int64  `json:"fileTransferredBytes,omitempty"`
	CurrentSpeed    int64  `json:"currentSpeed,omitempty"`
	RefreshInterval int    `json:"refreshIntervalSeconds,omitempty"`
	TotalItems      int    `json:"totalItems"`
	QueuedItems     int    `json:"queuedItems"`
	RunningItems    int    `json:"runningItems"`
	PausedItems     int    `json:"pausedItems"`
	FailedItems     int    `json:"failedItems"`
	SuccessItems    int    `json:"successItems"`
	SkippedItems    int    `json:"skippedItems"`
	TotalBytes      int64  `json:"totalBytes"`
	CompletedBytes  int64  `json:"completedBytes"`
	ErrorMessage    string `json:"errorMessage,omitempty"`
}

type transferItemMetadata struct {
	SourceModifiedAt string                     `json:"sourceModifiedAt,omitempty"`
	SourceVersionID  string                     `json:"sourceVersionId,omitempty"`
	SourceSize       int64                      `json:"sourceSize,omitempty"`
	EngineKind       string                     `json:"engineKind,omitempty"`
	EngineLabel      string                     `json:"engineLabel,omitempty"`
	CurrentSpeed     int64                      `json:"currentSpeed,omitempty"`
	RefreshInterval  int                        `json:"refreshIntervalSeconds,omitempty"`
	Aria2            *transferItemAria2Metadata `json:"aria2,omitempty"`
	AList            *transferItemAListMetadata `json:"alist,omitempty"`
}

type TransferPreferences struct {
	UploadConcurrency   int       `json:"uploadConcurrency"`
	DownloadConcurrency int       `json:"downloadConcurrency"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type UpdateTransferPreferencesRequest struct {
	UploadConcurrency   int `json:"uploadConcurrency"`
	DownloadConcurrency int `json:"downloadConcurrency"`
}

type transferItemAria2Metadata struct {
	GID         string   `json:"gid,omitempty"`
	DownloadURI string   `json:"downloadUri,omitempty"`
	Headers     []string `json:"headers,omitempty"`
	Status      string   `json:"status,omitempty"`
	TotalLength int64    `json:"totalLength,omitempty"`
	TargetDir   string   `json:"targetDir,omitempty"`
	TargetOut   string   `json:"targetOut,omitempty"`
}

type transferItemAListMetadata struct {
	TaskID     string   `json:"taskId,omitempty"`
	TaskKind   string   `json:"taskKind,omitempty"`
	TaskStatus string   `json:"taskStatus,omitempty"`
	TaskState  int      `json:"taskState,omitempty"`
	SourceDir  string   `json:"sourceDir,omitempty"`
	TargetDir  string   `json:"targetDir,omitempty"`
	Names      []string `json:"names,omitempty"`
}

func (service *Service) Start(ctx context.Context) error {
	var startErr error

	service.transferLoopOnce.Do(func() {
		if err := ensureDirectory(service.transferStateRoot()); err != nil {
			startErr = err
			return
		}

		if err := service.ensureTransferSidecarsRunning(ctx); err != nil {
			startErr = err
			return
		}

		if err := service.normalizeInterruptedTransferTasks(ctx); err != nil {
			startErr = err
			return
		}

		service.transferWorkerGroup.Add(1)
		go service.runTransferLoop()
	})

	return startErr
}

func (service *Service) Close() {
	service.transferCloseOnce.Do(func() {
		close(service.transferStop)
		service.transferTaskControls.Range(func(_, value any) bool {
			cancel, ok := value.(context.CancelFunc)
			if ok && cancel != nil {
				cancel()
			}
			return true
		})
		service.transferWorkerGroup.Wait()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if service.aria2Runtime != nil {
			_ = service.aria2Runtime.Shutdown(shutdownCtx)
		}
		if service.alistRuntime != nil {
			_ = service.alistRuntime.Shutdown(shutdownCtx)
		}
	})
}

func (service *Service) QueueRestoreAsset(ctx context.Context, request RestoreAssetRequest) (RestoreAssetSummary, error) {
	item, asset, sourceEndpoint, targetEndpoint, err := service.buildRestoreTransferItem(ctx, request, 0)
	if err != nil {
		return RestoreAssetSummary{}, err
	}

	task, err := service.enqueueTransferTask(ctx, taskTypeRestoreAsset, request, []store.TransferTaskItem{item})
	if err != nil {
		return RestoreAssetSummary{}, err
	}

	return RestoreAssetSummary{
		TaskID:             task.ID,
		AssetID:            asset.ID,
		DisplayName:        asset.DisplayName,
		SourceEndpointID:   sourceEndpoint.ID,
		SourceEndpointName: sourceEndpoint.Name,
		TargetEndpointID:   targetEndpoint.ID,
		TargetEndpointName: targetEndpoint.Name,
		Status:             task.Status,
		ProgressPercent:    item.ProgressPercent,
		ProgressLabel:      buildQueuedTaskLabel(item.DisplayName),
		StartedAt:          task.CreatedAt,
	}, nil
}

func (service *Service) QueueRestoreAssetsToEndpoint(ctx context.Context, request BatchRestoreRequest) (BatchRestoreSummary, error) {
	targetEndpoint, err := service.store.GetStorageEndpointByID(ctx, strings.TrimSpace(request.TargetEndpointID))
	if err != nil {
		return BatchRestoreSummary{}, err
	}

	now := time.Now().UTC()
	items := make([]store.TransferTaskItem, 0, len(request.AssetIDs))
	for _, assetID := range uniqueStrings(request.AssetIDs) {
		index := len(items)
		sourceReplica, sourceEndpoint, asset, sourceErr := service.selectSourceReplicaForRestore(ctx, assetID, targetEndpoint.ID)
		if sourceErr != nil {
			items = append(items, service.newFailedTransferItem(index, transferDirectionSync, BatchRestoreItemResult{
				AssetID:          assetID,
				DisplayName:      assetID,
				TargetEndpointID: targetEndpoint.ID,
				Status:           taskStatusFailed,
				Error:            sourceErr.Error(),
			}, targetEndpoint.Name, now))
			continue
		}

		item, buildErr := service.makeRestoreTransferItem(index, asset, sourceReplica, sourceEndpoint, targetEndpoint)
		if buildErr != nil {
			items = append(items, service.newFailedTransferItem(index, transferDirectionSync, BatchRestoreItemResult{
				AssetID:          asset.ID,
				DisplayName:      asset.DisplayName,
				TargetEndpointID: targetEndpoint.ID,
				Status:           taskStatusFailed,
				Error:            buildErr.Error(),
			}, targetEndpoint.Name, now))
			continue
		}

		items = append(items, item)
	}

	if len(items) == 0 {
		return BatchRestoreSummary{}, errors.New("没有可入队的同步资产")
	}

	task, err := service.enqueueTransferTask(ctx, taskTypeRestoreBatch, request, items)
	if err != nil {
		return BatchRestoreSummary{}, err
	}

	return BatchRestoreSummary{
		TaskID:             task.ID,
		TargetEndpointID:   targetEndpoint.ID,
		TargetEndpointName: targetEndpoint.Name,
		Status:             task.Status,
		TotalAssets:        len(items),
		ProgressPercent:    0,
		ProgressLabel:      fmt.Sprintf("已加入队列，等待处理 %d 个资产", len(items)),
		StartedAt:          task.CreatedAt,
	}, nil
}

func (service *Service) QueueImportExecution(ctx context.Context, request ExecuteImportRequest) (ImportExecutionSummary, error) {
	items, deviceLabel, err := service.buildImportTransferItems(ctx, request)
	if err != nil {
		return ImportExecutionSummary{}, err
	}

	task, err := service.enqueueTransferTask(ctx, taskTypeImportExecute, request, items)
	if err != nil {
		return ImportExecutionSummary{}, err
	}

	return ImportExecutionSummary{
		TaskID:            task.ID,
		IdentitySignature: strings.TrimSpace(request.IdentitySignature),
		DeviceLabel:       deviceLabel,
		Status:            task.Status,
		TotalFiles:        len(items),
		StartedAt:         task.CreatedAt,
		ProgressPercent:   0,
		ProgressLabel:     fmt.Sprintf("已加入队列，等待处理 %d 个传输项", len(items)),
	}, nil
}

func (service *Service) ListTransferTasks(ctx context.Context, limit int) (TransferTaskListResult, error) {
	if limit <= 0 {
		limit = 120
	}

	snapshots, err := service.listTransferQueueSnapshots(ctx, maxInt(limit*4, 240))
	if err != nil {
		return TransferTaskListResult{}, err
	}
	sortTransferTaskSnapshotsForDisplay(snapshots)

	result := TransferTaskListResult{
		GeneratedAt: time.Now().UTC(),
		Tasks:       make([]TransferTaskRecord, 0, limit),
	}

	for _, snapshot := range snapshots {
		accumulateTransferTaskStats(&result.Stats, snapshot.record, snapshot.items)
	}
	for _, snapshot := range snapshots {
		if len(result.Tasks) >= limit {
			break
		}
		result.Tasks = append(result.Tasks, snapshot.record)
	}

	result.Stats.GeneratedAt = result.GeneratedAt
	return result, nil
}

func (service *Service) GetTransferTaskDetail(ctx context.Context, taskID string) (TransferTaskDetailRecord, error) {
	task, err := service.store.GetTaskByID(ctx, taskID)
	if err != nil {
		return TransferTaskDetailRecord{}, err
	}
	if !isTransferTaskType(task.TaskType) {
		return TransferTaskDetailRecord{}, fmt.Errorf("task %q is not a transfer task", taskID)
	}

	items, err := service.store.ListTransferTaskItemsByTaskID(ctx, taskID)
	if err != nil {
		return TransferTaskDetailRecord{}, err
	}

	record := buildTransferTaskRecord(task, items)
	itemRecords := make([]TransferTaskItemRecord, 0, len(items))
	for _, item := range items {
		itemRecords = append(itemRecords, buildTransferTaskItemRecord(item))
	}

	return TransferTaskDetailRecord{
		Task:  record,
		Items: itemRecords,
	}, nil
}

func (service *Service) PauseTransferTasks(ctx context.Context, taskIDs []string) (TransferTaskActionSummary, error) {
	return service.updateTransferTasks(ctx, taskIDs, "pause")
}

func (service *Service) ResumeTransferTasks(ctx context.Context, taskIDs []string) (TransferTaskActionSummary, error) {
	return service.updateTransferTasks(ctx, taskIDs, "resume")
}

func (service *Service) CancelTransferTasks(ctx context.Context, taskIDs []string) (TransferTaskActionSummary, error) {
	return service.updateTransferTasks(ctx, taskIDs, "cancel")
}

func (service *Service) DeleteTransferTasks(ctx context.Context, taskIDs []string) (TransferTaskActionSummary, error) {
	return service.updateTransferTasks(ctx, taskIDs, "delete")
}

func (service *Service) PrioritizeTransferTask(ctx context.Context, taskID string) (TransferTaskActionSummary, error) {
	normalizedTaskID := strings.TrimSpace(taskID)
	summary := TransferTaskActionSummary{
		Requested: boolToInt(normalizedTaskID != ""),
		TaskIDs:   []string{},
	}
	if normalizedTaskID == "" {
		summary.Message = "没有可处理的任务"
		return summary, nil
	}
	summary.TaskIDs = []string{normalizedTaskID}

	task, err := service.store.GetTaskByID(ctx, normalizedTaskID)
	if err != nil {
		return summary, err
	}
	if !isTransferTaskType(task.TaskType) {
		return summary, fmt.Errorf("task %q is not a transfer task", normalizedTaskID)
	}

	items, err := service.store.ListTransferTaskItemsByTaskID(ctx, normalizedTaskID)
	if err != nil {
		return summary, err
	}
	record := buildTransferTaskRecord(task, items)
	if !canTransferTaskStart(record.Status) && !canTransferTaskResume(record.Status) {
		summary.Message = "当前任务已在进行中或已结束，无需调整优先级"
		return summary, nil
	}

	snapshots, err := service.listTransferQueueSnapshots(ctx, 320)
	if err != nil {
		return summary, err
	}
	maxPriority := 0
	for _, snapshot := range snapshots {
		if snapshot.task.Priority > maxPriority {
			maxPriority = snapshot.task.Priority
		}
	}

	nextPriority := maxPriority + 1
	if err := service.store.UpdateTaskPriority(ctx, task.ID, nextPriority, time.Now().UTC()); err != nil {
		return summary, err
	}

	if canTransferTaskResume(record.Status) {
		if _, err := service.ResumeTransferTasks(ctx, []string{task.ID}); err != nil {
			return summary, err
		}
	}

	preferences, err := service.GetTransferPreferences(ctx)
	if err != nil {
		return summary, err
	}

	runningSnapshot, hasRunning, err := service.findOldestRunningTransferTaskByDirection(ctx, transferQueueSlot(record.Direction), task.ID)
	if err != nil {
		return summary, err
	}
	activeCounts := service.activeTransferTaskCounts(snapshots)
	if hasRunning && activeCounts[transferQueueSlot(record.Direction)] >= transferQueueLimit(preferences, transferQueueSlot(record.Direction)) {
		if _, err := service.PauseTransferTasks(ctx, []string{runningSnapshot.task.ID}); err != nil {
			return summary, err
		}
	}

	service.wakeTransferLoop()
	summary.Updated = 1
	summary.Message = "任务已提升优先级，将优先开始"
	return summary, nil
}

func (service *Service) runTransferLoop() {
	defer service.transferWorkerGroup.Done()

	ticker := time.NewTicker(transferLoopInterval)
	defer ticker.Stop()

	for {
		if err := service.processQueuedTransferTasks(context.Background()); err != nil {
			slog.Warn("process queued transfer tasks failed", "error", err)
		}

		select {
		case <-service.transferStop:
			return
		case <-service.transferWake:
		case <-ticker.C:
		}
	}
}

func (service *Service) processQueuedTransferTasks(ctx context.Context) error {
	snapshots, err := service.listTransferQueueSnapshots(ctx, 320)
	if err != nil {
		return err
	}

	preferences, err := service.GetTransferPreferences(ctx)
	if err != nil {
		return err
	}

	activeCounts := service.activeTransferTaskCounts(snapshots)
	queued := make([]transferQueueSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if !canTransferTaskStart(snapshot.record.Status) {
			continue
		}
		queued = append(queued, snapshot)
	}
	sortTransferQueueSnapshots(queued)

	for _, snapshot := range queued {
		slot := transferQueueSlot(snapshot.record.Direction)
		limit := transferQueueLimit(preferences, slot)
		if activeCounts[slot] >= limit {
			continue
		}
		if !service.startTransferTaskRunner(snapshot.task.ID) {
			continue
		}
		activeCounts[slot]++
	}

	return nil
}

func (service *Service) startTransferTaskRunner(taskID string) bool {
	if _, exists := service.transferTaskControls.Load(taskID); exists {
		return false
	}

	ctx, cancel := context.WithCancel(context.Background())
	if _, loaded := service.transferTaskControls.LoadOrStore(taskID, cancel); loaded {
		cancel()
		return false
	}

	service.transferWorkerGroup.Add(1)
	go func() {
		defer service.transferWorkerGroup.Done()
		defer service.transferTaskControls.Delete(taskID)

		if err := service.runTransferTask(ctx, taskID); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("transfer task runner stopped with error", "taskId", taskID, "error", err)
		}
	}()

	return true
}

func (service *Service) enqueueTransferTask(
	ctx context.Context,
	taskType string,
	payload any,
	items []store.TransferTaskItem,
) (store.Task, error) {
	if len(items) == 0 {
		return store.Task{}, errors.New("no transfer items to enqueue")
	}

	status := resolveInitialTransferTaskStatus(items)
	task, err := service.createCatalogTaskWithStatus(ctx, taskType, payload, status)
	if err != nil {
		return store.Task{}, err
	}

	now := task.CreatedAt
	for index := range items {
		items[index].TaskID = task.ID
		items[index].ItemIndex = index
		if items[index].ID == "" {
			items[index].ID = uuid.NewString()
		}
		if items[index].CreatedAt.IsZero() {
			items[index].CreatedAt = now
		}
		if items[index].UpdatedAt.IsZero() {
			items[index].UpdatedAt = now
		}
		if strings.TrimSpace(items[index].ScanRevision) == "" {
			items[index].ScanRevision = task.ID
		}
		if strings.TrimSpace(items[index].TargetTempPath) == "" && strings.TrimSpace(items[index].TargetPath) != "" {
			items[index].TargetTempPath = buildTransferTargetTempPath(items[index].TargetPath, items[index].ID)
		}
		if strings.TrimSpace(items[index].MetadataJSON) == "" {
			items[index].MetadataJSON = "{}"
		}
	}

	if err := service.store.CreateTransferTaskItems(ctx, items); err != nil {
		return store.Task{}, err
	}

	if _, err := service.refreshTransferTaskState(ctx, task.ID); err != nil {
		return store.Task{}, err
	}

	if status == taskStatusQueued || status == taskStatusRunning {
		service.wakeTransferLoop()
	}

	return task, nil
}

func (service *Service) buildRestoreTransferItem(
	ctx context.Context,
	request RestoreAssetRequest,
	index int,
) (store.TransferTaskItem, store.Asset, store.StorageEndpoint, store.StorageEndpoint, error) {
	asset, err := service.store.GetAssetByID(ctx, strings.TrimSpace(request.AssetID))
	if err != nil {
		return store.TransferTaskItem{}, store.Asset{}, store.StorageEndpoint{}, store.StorageEndpoint{}, err
	}

	sourceEndpoint, err := service.store.GetStorageEndpointByID(ctx, strings.TrimSpace(request.SourceEndpointID))
	if err != nil {
		return store.TransferTaskItem{}, store.Asset{}, store.StorageEndpoint{}, store.StorageEndpoint{}, err
	}
	targetEndpoint, err := service.store.GetStorageEndpointByID(ctx, strings.TrimSpace(request.TargetEndpointID))
	if err != nil {
		return store.TransferTaskItem{}, store.Asset{}, store.StorageEndpoint{}, store.StorageEndpoint{}, err
	}

	sourceReplica, err := service.store.GetReplicaByAssetAndEndpoint(ctx, asset.ID, sourceEndpoint.ID)
	if err != nil {
		return store.TransferTaskItem{}, store.Asset{}, store.StorageEndpoint{}, store.StorageEndpoint{}, err
	}

	item, err := service.makeRestoreTransferItem(index, asset, sourceReplica, sourceEndpoint, targetEndpoint)
	if err != nil {
		return store.TransferTaskItem{}, store.Asset{}, store.StorageEndpoint{}, store.StorageEndpoint{}, err
	}

	return item, asset, sourceEndpoint, targetEndpoint, nil
}

func (service *Service) makeRestoreTransferItem(
	index int,
	asset store.Asset,
	sourceReplica store.Replica,
	sourceEndpoint store.StorageEndpoint,
	targetEndpoint store.StorageEndpoint,
) (store.TransferTaskItem, error) {
	destinationPath := deriveRestoreRelativePath(sourceEndpoint.RootPath, sourceReplica.PhysicalPath, asset.LogicalPathKey)
	if destinationPath == "" {
		return store.TransferTaskItem{}, errors.New("unable to derive restore destination path")
	}

	item := newTransferTaskItem(index)
	item.GroupKey = asset.ID
	item.Direction = determineTransferDirection(sourceEndpoint.EndpointType, targetEndpoint.EndpointType)
	item.SourceKind = transferSourceKindEndpoint
	item.SourceEndpointID = sourceEndpoint.ID
	item.SourceEndpointType = sourceEndpoint.EndpointType
	item.SourceLabel = sourceEndpoint.Name
	item.SourcePath = sourceReplica.PhysicalPath
	item.TargetEndpointID = targetEndpoint.ID
	item.TargetEndpointType = targetEndpoint.EndpointType
	item.TargetLabel = targetEndpoint.Name
	item.TargetPath = destinationPath
	item.TargetTempPath = buildTransferTargetTempPath(destinationPath, item.ID)
	item.AssetID = stringPointer(asset.ID)
	item.LogicalPathKey = asset.LogicalPathKey
	item.DisplayName = asset.DisplayName
	item.MediaType = asset.MediaType
	item.Status = taskStatusQueued
	item.Phase = transferPhasePending

	metadata := transferItemMetadata{}
	if sourceReplica.VersionID != nil {
		metadata.SourceVersionID = *sourceReplica.VersionID
		version, err := service.store.GetReplicaVersionByID(context.Background(), *sourceReplica.VersionID)
		if err == nil {
			item.TotalBytes = version.Size
			item.ScanRevision = defaultString(item.ScanRevision, defaultStringPointer(version.ScanRevision, ""))
			metadata.SourceSize = version.Size
			metadata.SourceModifiedAt = timePointerString(version.MTime)
		}
	}
	if encodedMetadata, err := json.Marshal(metadata); err == nil {
		item.MetadataJSON = string(encodedMetadata)
	}

	return item, nil
}

func (service *Service) buildImportTransferItems(
	ctx context.Context,
	request ExecuteImportRequest,
) ([]store.TransferTaskItem, string, error) {
	identitySignature := strings.TrimSpace(request.IdentitySignature)
	if identitySignature == "" {
		return nil, "", errors.New("identitySignature is required")
	}

	entryPaths := uniqueStrings(request.EntryPaths)
	if len(entryPaths) == 0 {
		return nil, "", errors.New("entryPaths is required")
	}

	device, resolvedIdentity, err := service.findRemovableDeviceByIdentity(ctx, identitySignature)
	if err != nil {
		return nil, "", err
	}

	sourceConnector, err := connectors.NewRemovableConnector(connectors.RemovableConfig{
		Name:   defaultString(strings.TrimSpace(device.VolumeLabel), "导入源设备"),
		Device: device,
	})
	if err != nil {
		return nil, "", err
	}

	rules, err := service.ListImportRules(ctx)
	if err != nil {
		return nil, "", err
	}
	if len(rules) == 0 {
		return nil, "", errors.New("尚未配置导入规则")
	}

	targetEndpoints, err := service.listManagedImportEndpoints(ctx)
	if err != nil {
		return nil, "", err
	}

	deviceLabel := defaultString(strings.TrimSpace(device.VolumeLabel), strings.TrimSpace(device.MountPoint))
	items := make([]store.TransferTaskItem, 0, len(entryPaths))

	for _, entryPath := range entryPaths {
		normalizedPath := normalizeImportRelativePath(entryPath, "", "")
		entry, statErr := sourceConnector.StatEntry(ctx, normalizedPath)
		if statErr != nil {
			items = append(items, service.newFailedImportItem(len(items), normalizedPath, deviceLabel, statErr.Error()))
			continue
		}
		if entry.IsDir {
			items = append(items, service.newFailedImportItem(len(items), normalizedPath, deviceLabel, "暂不支持直接导入目录。"))
			continue
		}
		if entry.MediaType == connectors.MediaTypeUnknown {
			items = append(items, service.newFailedImportItem(len(items), normalizedPath, deviceLabel, "当前仅支持导入图片、视频和音频文件。"))
			continue
		}

		relativePath := normalizeImportRelativePath(entry.RelativePath, entry.Name, entry.Path)
		logicalPathKey, normalizeErr := NormalizeLogicalPathKey(sourceConnector.Descriptor().RootPath, relativePath)
		if normalizeErr != nil {
			items = append(items, service.newFailedImportItem(len(items), relativePath, deviceLabel, normalizeErr.Error()))
			continue
		}

		targets := resolveImportTargets(entry, relativePath, resolvedIdentity, rules, targetEndpoints)
		if len(targets) == 0 {
			items = append(items, service.newFailedImportItem(len(items), relativePath, deviceLabel, "没有匹配到可用的导入规则或目标端点。"))
			continue
		}

		var assetID *string
		if asset, assetErr := service.store.GetAssetByLogicalPathKey(ctx, logicalPathKey); assetErr == nil {
			assetID = &asset.ID
		}

		metadataPayload, _ := json.Marshal(transferItemMetadata{
			SourceModifiedAt: timePointerString(entry.ModifiedAt),
			SourceSize:       entry.Size,
		})

		for _, endpoint := range targets {
			item := newTransferTaskItem(len(items))
			item.GroupKey = relativePath
			item.Direction = determineTransferDirection(string(connectors.EndpointTypeRemovable), endpoint.EndpointType)
			item.SourceKind = transferSourceKindDevice
			item.SourceIdentitySignature = resolvedIdentity
			item.SourceLabel = deviceLabel
			item.SourcePath = relativePath
			item.TargetEndpointID = endpoint.ID
			item.TargetEndpointType = endpoint.EndpointType
			item.TargetLabel = endpoint.Name
			item.TargetPath = relativePath
			item.TargetTempPath = buildTransferTargetTempPath(relativePath, item.ID)
			item.AssetID = assetID
			item.LogicalPathKey = logicalPathKey
			item.DisplayName = defaultString(strings.TrimSpace(entry.Name), relativePath)
			item.MediaType = string(entry.MediaType)
			item.Status = taskStatusQueued
			item.Phase = transferPhasePending
			item.TotalBytes = entry.Size
			item.MetadataJSON = string(metadataPayload)
			items = append(items, item)
		}
	}

	if len(items) == 0 {
		return nil, "", errors.New("没有可入队的导入项")
	}

	return items, deviceLabel, nil
}

func (service *Service) runTransferTask(ctx context.Context, taskID string) error {
	task, err := service.store.GetTaskByID(ctx, taskID)
	if err != nil {
		return err
	}
	if !isTransferTaskType(task.TaskType) {
		return nil
	}

	startedAt := task.StartedAt
	now := time.Now().UTC()
	if startedAt == nil {
		startedAt = &now
	}

	if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
		Status:        taskStatusRunning,
		ResultSummary: task.ResultSummary,
		ErrorMessage:  nil,
		RetryCount:    task.RetryCount,
		StartedAt:     startedAt,
		UpdatedAt:     now,
	}); err != nil {
		return err
	}

	if _, err := service.refreshTransferTaskState(context.Background(), task.ID); err != nil {
		return err
	}

	items, err := service.store.ListTransferTaskItemsByTaskID(context.Background(), task.ID)
	if err != nil {
		return err
	}

	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return service.handleTransferTaskCancellation(task.ID)
		}
		if !canTransferItemRun(item.Status) {
			continue
		}

		if itemErr := service.runTransferTaskItem(ctx, task.ID, &item); itemErr != nil {
			if errors.Is(itemErr, context.Canceled) {
				return service.handleTransferTaskCancellation(task.ID)
			}
			slog.Warn("transfer item failed", "taskId", task.ID, "itemId", item.ID, "error", itemErr)
		}
	}

	_, err = service.refreshTransferTaskState(context.Background(), task.ID)
	return err
}

func (service *Service) runTransferTaskItem(ctx context.Context, taskID string, item *store.TransferTaskItem) error {
	now := time.Now().UTC()
	if item.StartedAt == nil {
		item.StartedAt = &now
	}
	item.Status = taskStatusRunning
	item.Phase = restoreTransferPhase(*item)
	item.ErrorMessage = nil
	item.UpdatedAt = now
	item.FinishedAt = nil
	if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
		return err
	}

	if err := service.stageTransferItem(ctx, taskID, item); err != nil {
		return service.failTransferItem(taskID, item, err)
	}

	entry, err := service.commitTransferItem(ctx, taskID, item)
	if err != nil {
		return service.failTransferItem(taskID, item, err)
	}

	if err := service.finalizeTransferItem(context.Background(), taskID, item, entry); err != nil {
		return service.failTransferItem(taskID, item, err)
	}

	return nil
}

func (service *Service) stageTransferItem(ctx context.Context, taskID string, item *store.TransferTaskItem) error {
	if strings.TrimSpace(item.SourceKind) == transferSourceKindEndpoint &&
		normalizeEndpointType(item.SourceEndpointType) == string(connectors.EndpointTypeNetwork) {
		return service.stageTransferItemFromAList(ctx, taskID, item)
	}

	stagingPath, err := service.ensureTransferStagingPath(item)
	if err != nil {
		return err
	}
	item.StagingPath = stagingPath

	actualSize, err := fileSizeIfExists(stagingPath)
	if err != nil {
		return err
	}
	item.StagedBytes = actualSize

	if item.TotalBytes > 0 && item.StagedBytes >= item.TotalBytes {
		item.Phase = transferPhaseCommitting
		item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseCommitting)
		item.UpdatedAt = time.Now().UTC()
		return service.persistTransferItemState(context.Background(), taskID, *item)
	}

	sourceConnector, cleanup, err := service.buildTransferSourceConnector(ctx, *item)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	reader, err := sourceConnector.ReadStream(ctx, item.SourcePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	if item.StagedBytes > 0 {
		if err := discardTransferBytes(ctx, reader, item.StagedBytes); err != nil {
			return err
		}
	}

	file, err := os.OpenFile(stagingPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Seek(item.StagedBytes, io.SeekStart); err != nil {
		return err
	}

	item.Phase = transferPhaseStaging
	item.UpdatedAt = time.Now().UTC()
	if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
		return err
	}

	lastPersistAt := time.Now().UTC()
	var bytesSincePersist int64
	if err := copyWithProgress(ctx, file, reader, func(delta int64) error {
		item.StagedBytes += delta
		bytesSincePersist += delta

		progressTotal := max(item.TotalBytes, max(item.StagedBytes, 1))
		item.ProgressPercent = calcTransferItemProgress(progressTotal, item.StagedBytes, item.CommittedBytes, transferPhaseStaging)

		now := time.Now().UTC()
		if bytesSincePersist < transferProgressPersistBytes && now.Sub(lastPersistAt) < transferProgressPersistWindow {
			return nil
		}
		if err := file.Sync(); err != nil {
			return err
		}
		item.UpdatedAt = now
		lastPersistAt = now
		bytesSincePersist = 0
		return service.persistTransferItemState(context.Background(), taskID, *item)
	}); err != nil {
		return err
	}

	if err := file.Sync(); err != nil {
		return err
	}
	if item.TotalBytes <= 0 {
		item.TotalBytes = item.StagedBytes
	}

	item.Phase = transferPhaseCommitting
	item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseCommitting)
	item.UpdatedAt = time.Now().UTC()
	return service.persistTransferItemState(context.Background(), taskID, *item)
}

func (service *Service) commitTransferItem(
	ctx context.Context,
	taskID string,
	item *store.TransferTaskItem,
) (connectors.FileEntry, error) {
	endpoint, err := service.store.GetStorageEndpointByID(ctx, item.TargetEndpointID)
	if err != nil {
		return connectors.FileEntry{}, err
	}

	if normalizeEndpointType(endpoint.EndpointType) == string(connectors.EndpointTypeNetwork) {
		return service.commitTransferItemToAList(ctx, taskID, item, endpoint)
	}

	connector, err := service.buildConnector(endpoint)
	if err != nil {
		return connectors.FileEntry{}, err
	}

	if supportsLocalResumableTransfer(endpoint.EndpointType) {
		return service.commitTransferItemToLocal(ctx, taskID, item, endpoint, connector)
	}

	return service.commitTransferItemViaConnector(ctx, taskID, item, connector)
}

func (service *Service) commitTransferItemToLocal(
	ctx context.Context,
	taskID string,
	item *store.TransferTaskItem,
	endpoint store.StorageEndpoint,
	connector connectors.Connector,
) (connectors.FileEntry, error) {
	if strings.TrimSpace(item.TargetTempPath) == "" {
		item.TargetTempPath = buildTransferTargetTempPath(item.TargetPath, item.ID)
	}

	tempPath := resolveEndpointAbsolutePath(endpoint, item.TargetTempPath)
	finalPath := resolveEndpointAbsolutePath(endpoint, item.TargetPath)
	if err := ensureDirectory(filepath.Dir(tempPath)); err != nil {
		return connectors.FileEntry{}, err
	}

	actualCommitted, err := fileSizeIfExists(tempPath)
	if err != nil {
		return connectors.FileEntry{}, err
	}
	item.CommittedBytes = actualCommitted

	stageFile, err := os.Open(item.StagingPath)
	if err != nil {
		return connectors.FileEntry{}, err
	}
	defer stageFile.Close()

	tempFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return connectors.FileEntry{}, err
	}
	defer tempFile.Close()

	if _, err := stageFile.Seek(item.CommittedBytes, io.SeekStart); err != nil {
		return connectors.FileEntry{}, err
	}
	if _, err := tempFile.Seek(item.CommittedBytes, io.SeekStart); err != nil {
		return connectors.FileEntry{}, err
	}

	item.Phase = transferPhaseCommitting
	item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseCommitting)
	item.UpdatedAt = time.Now().UTC()
	if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
		return connectors.FileEntry{}, err
	}

	lastPersistAt := time.Now().UTC()
	var bytesSincePersist int64
	if err := copyWithProgress(ctx, tempFile, stageFile, func(delta int64) error {
		item.CommittedBytes += delta
		bytesSincePersist += delta
		item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseCommitting)

		now := time.Now().UTC()
		if bytesSincePersist < transferProgressPersistBytes && now.Sub(lastPersistAt) < transferProgressPersistWindow {
			return nil
		}
		if err := tempFile.Sync(); err != nil {
			return err
		}
		item.UpdatedAt = now
		lastPersistAt = now
		bytesSincePersist = 0
		return service.persistTransferItemState(context.Background(), taskID, *item)
	}); err != nil {
		return connectors.FileEntry{}, err
	}

	if err := tempFile.Sync(); err != nil {
		return connectors.FileEntry{}, err
	}
	_ = tempFile.Close()

	if err := ensureDirectory(filepath.Dir(finalPath)); err != nil {
		return connectors.FileEntry{}, err
	}
	_ = os.Remove(finalPath)
	if err := os.Rename(tempPath, finalPath); err != nil {
		return connectors.FileEntry{}, err
	}

	if modifiedAt := parseTransferMetadataModifiedAt(item.MetadataJSON); modifiedAt != nil {
		_ = os.Chtimes(finalPath, modifiedAt.UTC(), modifiedAt.UTC())
	}

	entry, err := connector.StatEntry(ctx, item.TargetPath)
	if err != nil {
		info, statErr := os.Stat(finalPath)
		if statErr != nil {
			return connectors.FileEntry{}, err
		}
		modifiedAt := info.ModTime().UTC()
		return connectors.FileEntry{
			Path:       finalPath,
			Name:       filepath.Base(finalPath),
			Size:       info.Size(),
			ModifiedAt: &modifiedAt,
			IsDir:      false,
		}, nil
	}
	return entry, nil
}

func (service *Service) commitTransferItemViaConnector(
	ctx context.Context,
	taskID string,
	item *store.TransferTaskItem,
	connector connectors.Connector,
) (connectors.FileEntry, error) {
	if strings.TrimSpace(item.TargetTempPath) == "" {
		item.TargetTempPath = buildTransferTargetTempPath(item.TargetPath, item.ID)
	}

	item.CommittedBytes = 0
	_ = connector.DeleteEntry(ctx, item.TargetTempPath)

	file, err := os.Open(item.StagingPath)
	if err != nil {
		return connectors.FileEntry{}, err
	}
	defer file.Close()

	item.Phase = transferPhaseCommitting
	item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseCommitting)
	item.UpdatedAt = time.Now().UTC()
	if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
		return connectors.FileEntry{}, err
	}

	if commitsTransferDirectly(item.TargetEndpointType) {
		entry, err := connector.CopyIn(ctx, item.TargetPath, file)
		if err != nil {
			return connectors.FileEntry{}, err
		}
		if strings.TrimSpace(entry.Path) == "" {
			statEntry, statErr := connector.StatEntry(ctx, item.TargetPath)
			if statErr == nil {
				entry = statEntry
			}
		}

		item.CommittedBytes = item.TotalBytes
		item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseFinalizing)
		item.UpdatedAt = time.Now().UTC()
		if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
			return connectors.FileEntry{}, err
		}

		return entry, nil
	}

	if _, err := connector.CopyIn(ctx, item.TargetTempPath, file); err != nil {
		return connectors.FileEntry{}, err
	}
	if _, statErr := connector.StatEntry(ctx, item.TargetPath); statErr == nil {
		_ = connector.DeleteEntry(ctx, item.TargetPath)
	}
	entry, err := connector.MoveEntry(ctx, item.TargetTempPath, item.TargetPath)
	if err != nil {
		return connectors.FileEntry{}, err
	}

	item.CommittedBytes = item.TotalBytes
	item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseFinalizing)
	item.UpdatedAt = time.Now().UTC()
	if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
		return connectors.FileEntry{}, err
	}

	return entry, nil
}

func (service *Service) finalizeTransferItem(
	ctx context.Context,
	taskID string,
	item *store.TransferTaskItem,
	entry connectors.FileEntry,
) error {
	item.Phase = transferPhaseFinalizing
	item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseFinalizing)
	item.UpdatedAt = time.Now().UTC()
	if err := service.persistTransferItemState(ctx, taskID, *item); err != nil {
		return err
	}

	modifiedAt := cloneTimePointer(entry.ModifiedAt)
	if modifiedAt == nil {
		modifiedAt = parseTransferMetadataModifiedAt(item.MetadataJSON)
	}

	size := entry.Size
	if size <= 0 {
		size = item.TotalBytes
	}
	if size <= 0 {
		size = parseTransferMetadataSize(item.MetadataJSON)
	}

	scanResult := ScanResult{
		EndpointID:     item.TargetEndpointID,
		PhysicalPath:   defaultString(strings.TrimSpace(entry.Path), item.TargetPath),
		LogicalPathKey: item.LogicalPathKey,
		Size:           size,
		MTime:          modifiedAt,
		MediaType:      item.MediaType,
		IsDir:          false,
	}
	if _, err := service.MergeScanResults(ctx, []ScanResult{scanResult}, defaultString(item.ScanRevision, taskID)); err != nil {
		return err
	}

	if asset, err := service.store.GetAssetByLogicalPathKey(ctx, item.LogicalPathKey); err == nil {
		item.AssetID = &asset.ID
	}
	if strings.TrimSpace(item.StagingPath) != "" {
		service.cleanupTransferStagingArtifacts(item.StagingPath)
		item.StagingPath = ""
	}

	item.TotalBytes = max(item.TotalBytes, size)
	item.StagedBytes = item.TotalBytes
	item.CommittedBytes = item.TotalBytes
	item.Status = taskStatusSuccess
	item.Phase = transferPhaseCompleted
	item.ProgressPercent = 100
	item.ErrorMessage = nil
	updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
		metadata.CurrentSpeed = 0
		if metadata.Aria2 != nil && strings.TrimSpace(metadata.Aria2.Status) == "" {
			metadata.Aria2.Status = "complete"
		}
		if metadata.AList != nil && strings.TrimSpace(metadata.AList.TaskStatus) == "" {
			metadata.AList.TaskStatus = "complete"
		}
	})
	now := time.Now().UTC()
	item.UpdatedAt = now
	item.FinishedAt = &now
	return service.persistTransferItemState(ctx, taskID, *item)
}

func (service *Service) failTransferItem(taskID string, item *store.TransferTaskItem, failure error) error {
	if errors.Is(failure, context.Canceled) {
		task, err := service.store.GetTaskByID(context.Background(), taskID)
		if err != nil {
			return failure
		}
		switch strings.TrimSpace(task.Status) {
		case taskStatusPaused:
			item.Status = taskStatusPaused
			item.Phase = transferPhasePaused
			item.ErrorMessage = nil
		case taskStatusCanceled:
			item.Status = taskStatusCanceled
			item.Phase = transferPhaseCanceled
			item.ErrorMessage = nil
		default:
			item.Status = taskStatusFailed
			item.Phase = transferPhaseFailed
			message := failure.Error()
			item.ErrorMessage = &message
		}
	} else {
		item.Status = taskStatusFailed
		item.Phase = transferPhaseFailed
		message := failure.Error()
		item.ErrorMessage = &message
	}
	updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
		metadata.CurrentSpeed = 0
	})

	item.UpdatedAt = time.Now().UTC()
	item.FinishedAt = nil
	if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
		return err
	}
	if item.Status == taskStatusFailed {
		service.cleanupAListTargetResidue(context.Background(), *item)
	}
	return failure
}

func (service *Service) handleTransferTaskCancellation(taskID string) error {
	task, err := service.store.GetTaskByID(context.Background(), taskID)
	if err != nil {
		return err
	}

	_, refreshErr := service.refreshTransferTaskState(context.Background(), taskID)
	if refreshErr != nil && !errors.Is(refreshErr, sql.ErrNoRows) {
		return refreshErr
	}

	switch strings.TrimSpace(task.Status) {
	case taskStatusPaused, taskStatusCanceled:
		return nil
	default:
		return context.Canceled
	}
}

func (service *Service) updateTransferTasks(ctx context.Context, taskIDs []string, action string) (TransferTaskActionSummary, error) {
	normalizedTaskIDs := uniqueStrings(taskIDs)
	summary := TransferTaskActionSummary{
		Requested: len(normalizedTaskIDs),
		TaskIDs:   normalizedTaskIDs,
	}
	if len(normalizedTaskIDs) == 0 {
		summary.Message = "没有可处理的任务"
		return summary, nil
	}

	for _, taskID := range normalizedTaskIDs {
		task, err := service.store.GetTaskByID(ctx, taskID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return summary, err
		}
		if !isTransferTaskType(task.TaskType) {
			continue
		}

		items, err := service.store.ListTransferTaskItemsByTaskID(ctx, taskID)
		if err != nil {
			return summary, err
		}

		switch action {
		case "pause":
			if !canTransferTaskPause(task.Status) {
				continue
			}
			service.cancelTransferTask(task.ID)
			for index := range items {
				if isTransferItemTerminal(items[index].Status) {
					continue
				}
				if err := service.pauseExternalTransferItem(ctx, &items[index]); err != nil {
					return summary, err
				}
				items[index].Status = taskStatusPaused
				items[index].Phase = transferPhasePaused
				items[index].UpdatedAt = time.Now().UTC()
				if err := service.store.UpdateTransferTaskItem(ctx, items[index]); err != nil {
					return summary, err
				}
			}
			if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
				Status:        taskStatusPaused,
				ResultSummary: task.ResultSummary,
				ErrorMessage:  nil,
				RetryCount:    task.RetryCount,
				StartedAt:     task.StartedAt,
				UpdatedAt:     time.Now().UTC(),
			}); err != nil {
				return summary, err
			}
			if _, err := service.refreshTransferTaskState(ctx, task.ID); err != nil {
				return summary, err
			}
			summary.Updated++
		case "cancel":
			if !canTransferTaskCancel(task.Status) {
				continue
			}
			service.cancelTransferTask(task.ID)
			now := time.Now().UTC()
			for index := range items {
				if isTransferItemTerminal(items[index].Status) {
					continue
				}
				if err := service.pauseExternalTransferItem(ctx, &items[index]); err != nil {
					return summary, err
				}
				items[index].Status = taskStatusCanceled
				items[index].Phase = transferPhaseCanceled
				items[index].ErrorMessage = nil
				items[index].FinishedAt = cloneTimePointer(&now)
				items[index].UpdatedAt = now
				if err := service.store.UpdateTransferTaskItem(ctx, items[index]); err != nil {
					return summary, err
				}
			}
			if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
				Status:        taskStatusCanceled,
				ResultSummary: task.ResultSummary,
				ErrorMessage:  nil,
				RetryCount:    task.RetryCount,
				StartedAt:     task.StartedAt,
				FinishedAt:    &now,
				UpdatedAt:     now,
			}); err != nil {
				return summary, err
			}
			if _, err := service.refreshTransferTaskState(ctx, task.ID); err != nil {
				return summary, err
			}
			summary.Updated++
		case "resume":
			if !canTransferTaskResume(task.Status) {
				continue
			}
			for index := range items {
				if !canTransferItemResume(items[index].Status) {
					continue
				}
				resetTransferItemForResume(&items[index])
				if err := service.store.UpdateTransferTaskItem(ctx, items[index]); err != nil {
					return summary, err
				}
			}
			retryCount := task.RetryCount
			if strings.TrimSpace(task.Status) == taskStatusFailed {
				retryCount++
			}
			if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
				Status:        taskStatusQueued,
				ResultSummary: task.ResultSummary,
				ErrorMessage:  nil,
				RetryCount:    retryCount,
				UpdatedAt:     time.Now().UTC(),
			}); err != nil {
				return summary, err
			}
			if _, err := service.refreshTransferTaskState(ctx, task.ID); err != nil {
				return summary, err
			}
			service.wakeTransferLoop()
			summary.Updated++
		case "delete":
			service.cancelTransferTask(task.ID)
			for index := range items {
				if err := service.deleteExternalTransferItem(ctx, &items[index]); err != nil {
					return summary, err
				}
			}
			service.cleanupTransferArtifacts(context.Background(), items)
			if err := service.store.DeleteTaskByID(ctx, task.ID); err != nil {
				return summary, err
			}
			summary.Updated++
		default:
			return summary, fmt.Errorf("unsupported transfer action %q", action)
		}
	}

	/*
		switch action {
		case "pause":
			summary.Message = fmt.Sprintf("已暂停 %d 个任务", summary.Updated)
		case "cancel":
			summary.Message = fmt.Sprintf("已取消 %d 个任务", summary.Updated)
		case "resume":
			summary.Message = fmt.Sprintf("已恢复 %d 个任务", summary.Updated)
		case "delete":
			summary.Message = fmt.Sprintf("已删除 %d 个任务", summary.Updated)
		}
	*/
	switch action {
	case "pause":
		summary.Message = fmt.Sprintf("已暂停 %d 个任务", summary.Updated)
	case "cancel":
		summary.Message = fmt.Sprintf("已取消 %d 个任务", summary.Updated)
	case "resume":
		summary.Message = fmt.Sprintf("已恢复 %d 个任务", summary.Updated)
	case "delete":
		summary.Message = fmt.Sprintf("已删除 %d 个任务", summary.Updated)
	}

	return summary, nil
}

func (service *Service) normalizeInterruptedTransferTasks(ctx context.Context) error {
	aria2Available, err := service.ensureAria2RuntimeAvailable(ctx)
	if err != nil {
		return err
	}
	alistAvailable, err := service.ensureAListRuntimeAvailable(ctx)
	if err != nil {
		return err
	}

	tasks, err := service.store.ListTasks(ctx, 400, 0)
	if err != nil {
		return err
	}

	for _, task := range tasks {
		if !isTransferTaskType(task.TaskType) {
			continue
		}

		status := strings.TrimSpace(strings.ToLower(task.Status))

		items, err := service.store.ListTransferTaskItemsByTaskID(ctx, task.ID)
		if err != nil {
			return err
		}

		changed := false
		for index := range items {
			original := items[index]
			if err := service.reconcileInterruptedTransferItem(ctx, &items[index], status, aria2Available, alistAvailable); err != nil {
				return err
			}
			if items[index] != original {
				if err := service.store.UpdateTransferTaskItem(ctx, items[index]); err != nil {
					return err
				}
				changed = true
			}
		}

		if shouldRequeueInterruptedTransferTask(status) {
			if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
				Status:        taskStatusQueued,
				ResultSummary: task.ResultSummary,
				ErrorMessage:  nil,
				RetryCount:    task.RetryCount,
				StartedAt:     task.StartedAt,
				UpdatedAt:     time.Now().UTC(),
			}); err != nil {
				return err
			}
			changed = true
		}

		if changed {
			if _, err := service.refreshTransferTaskState(ctx, task.ID); err != nil {
				return err
			}
		}
	}

	return nil
}

func (service *Service) persistTransferItemState(ctx context.Context, taskID string, item store.TransferTaskItem) error {
	if err := service.store.UpdateTransferTaskItem(ctx, item); err != nil {
		return err
	}
	_, err := service.refreshTransferTaskState(ctx, taskID)
	return err
}

func (service *Service) refreshTransferTaskState(ctx context.Context, taskID string) (TransferTaskRecord, error) {
	task, err := service.store.GetTaskByID(ctx, taskID)
	if err != nil {
		return TransferTaskRecord{}, err
	}
	items, err := service.store.ListTransferTaskItemsByTaskID(ctx, taskID)
	if err != nil {
		return TransferTaskRecord{}, err
	}

	record := buildTransferTaskRecord(task, items)
	summaryPayload := transferTaskSummaryPayload{
		TaskID:          record.ID,
		Title:           record.Title,
		Direction:       record.Direction,
		Status:          record.Status,
		Priority:        record.Priority,
		SourceLabel:     record.SourceLabel,
		TargetLabel:     record.TargetLabel,
		EngineSummary:   record.EngineSummary,
		ProgressPercent: record.ProgressPercent,
		ProgressLabel:   record.ProgressLabel,
		CurrentItemName: record.CurrentItemName,
		FileName:        record.FileName,
		FilePath:        record.FilePath,
		FileSize:        record.FileSize,
		FileTransferred: record.FileTransferred,
		CurrentSpeed:    record.CurrentSpeed,
		RefreshInterval: record.RefreshInterval,
		TotalItems:      record.TotalItems,
		QueuedItems:     record.QueuedItems,
		RunningItems:    record.RunningItems,
		PausedItems:     record.PausedItems,
		FailedItems:     record.FailedItems,
		SuccessItems:    record.SuccessItems,
		SkippedItems:    record.SkippedItems,
		TotalBytes:      record.TotalBytes,
		CompletedBytes:  record.CompletedBytes,
		ErrorMessage:    record.ErrorMessage,
	}
	encodedSummary, err := json.Marshal(summaryPayload)
	if err != nil {
		return TransferTaskRecord{}, err
	}
	resultSummary := string(encodedSummary)

	var errorMessage *string
	if strings.TrimSpace(record.ErrorMessage) != "" {
		value := record.ErrorMessage
		errorMessage = &value
	}

	if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
		Status:        record.Status,
		ResultSummary: &resultSummary,
		ErrorMessage:  errorMessage,
		RetryCount:    task.RetryCount,
		StartedAt:     record.StartedAt,
		FinishedAt:    record.FinishedAt,
		UpdatedAt:     time.Now().UTC(),
	}); err != nil {
		return TransferTaskRecord{}, err
	}

	record.UpdatedAt = time.Now().UTC()
	return record, nil
}

func (service *Service) buildTransferSourceConnector(
	ctx context.Context,
	item store.TransferTaskItem,
) (connectors.Connector, func(), error) {
	switch strings.TrimSpace(item.SourceKind) {
	case transferSourceKindEndpoint:
		endpoint, err := service.store.GetStorageEndpointByID(ctx, item.SourceEndpointID)
		if err != nil {
			return nil, nil, err
		}
		connector, err := service.buildConnector(endpoint)
		return connector, nil, err
	case transferSourceKindDevice:
		device, _, err := service.findRemovableDeviceByIdentity(ctx, item.SourceIdentitySignature)
		if err != nil {
			return nil, nil, err
		}
		connector, err := connectors.NewRemovableConnector(connectors.RemovableConfig{
			Name:   defaultString(item.SourceLabel, "导入源设备"),
			Device: device,
		})
		return connector, nil, err
	default:
		return nil, nil, fmt.Errorf("unsupported transfer source kind %q", item.SourceKind)
	}
}

func (service *Service) ensureTransferStagingPath(item *store.TransferTaskItem) (string, error) {
	if strings.TrimSpace(item.StagingPath) != "" {
		if err := ensureDirectory(filepath.Dir(item.StagingPath)); err != nil {
			return "", err
		}
		return item.StagingPath, nil
	}

	stagingDir := filepath.Join(service.transferStateRoot(), "tasks", item.TaskID, item.ID)
	if err := ensureDirectory(stagingDir); err != nil {
		return "", err
	}

	return filepath.Join(stagingDir, transferStagingFileName(*item)), nil
}

func transferStagingFileName(item store.TransferTaskItem) string {
	candidates := []string{
		path.Base(strings.TrimSpace(item.TargetPath)),
		path.Base(strings.TrimSpace(item.SourcePath)),
		strings.TrimSpace(item.DisplayName),
	}
	for _, candidate := range candidates {
		name := sanitizeTransferStagingFileName(candidate)
		if name != "" {
			return name
		}
	}

	extension := filepath.Ext(strings.TrimSpace(item.TargetPath))
	if extension == "" {
		extension = filepath.Ext(strings.TrimSpace(item.SourcePath))
	}
	return shortTaskID(item.ID) + extension
}

func sanitizeTransferStagingFileName(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	replacer := strings.NewReplacer(
		"<", "_",
		">", "_",
		":", "_",
		"\"", "_",
		"/", "_",
		"\\", "_",
		"|", "_",
		"?", "_",
		"*", "_",
	)
	trimmed = replacer.Replace(trimmed)
	trimmed = strings.Trim(trimmed, ". ")
	if trimmed == "" {
		return ""
	}

	upper := strings.ToUpper(trimmed)
	switch upper {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return "_" + trimmed
	default:
		return trimmed
	}
}

func (service *Service) transferStateRoot() string {
	return filepath.Join(filepath.Dir(service.mediaConfig.CacheRoot), "transfers")
}

func (service *Service) wakeTransferLoop() {
	select {
	case service.transferWake <- struct{}{}:
	default:
	}
}

func (service *Service) cancelTransferTask(taskID string) {
	value, ok := service.transferTaskControls.Load(taskID)
	if !ok {
		return
	}
	cancel, valid := value.(context.CancelFunc)
	if valid && cancel != nil {
		cancel()
	}
}

func (service *Service) activeTransferTaskIDs() map[string]struct{} {
	active := make(map[string]struct{})
	service.transferTaskControls.Range(func(key, _ any) bool {
		taskID, ok := key.(string)
		if ok && strings.TrimSpace(taskID) != "" {
			active[taskID] = struct{}{}
		}
		return true
	})
	return active
}

func (service *Service) activeTransferTaskCounts(snapshots []transferQueueSnapshot) map[string]int {
	activeTaskIDs := service.activeTransferTaskIDs()
	counts := map[string]int{
		transferDirectionUpload:   0,
		transferDirectionDownload: 0,
		transferDirectionSync:     0,
	}
	for _, snapshot := range snapshots {
		if _, ok := activeTaskIDs[snapshot.task.ID]; !ok && strings.TrimSpace(snapshot.record.Status) != taskStatusRunning {
			continue
		}
		slot := transferQueueSlot(snapshot.record.Direction)
		counts[slot]++
	}
	return counts
}

func (service *Service) listTransferQueueSnapshots(ctx context.Context, limit int) ([]transferQueueSnapshot, error) {
	tasks, err := service.store.ListTasks(ctx, limit, 0)
	if err != nil {
		return nil, err
	}

	snapshots := make([]transferQueueSnapshot, 0, len(tasks))
	for _, task := range tasks {
		if !isTransferTaskType(task.TaskType) {
			continue
		}
		items, itemErr := service.store.ListTransferTaskItemsByTaskID(ctx, task.ID)
		if itemErr != nil {
			return nil, itemErr
		}
		snapshots = append(snapshots, transferQueueSnapshot{
			task:   task,
			items:  items,
			record: buildTransferTaskRecord(task, items),
		})
	}
	return snapshots, nil
}

func (service *Service) cleanupTransferArtifacts(ctx context.Context, items []store.TransferTaskItem) {
	for _, item := range items {
		if strings.TrimSpace(item.StagingPath) != "" {
			service.cleanupTransferStagingArtifacts(item.StagingPath)
		}
		if strings.TrimSpace(item.TargetTempPath) == "" {
			continue
		}

		switch {
		case supportsLocalResumableTransfer(item.TargetEndpointType) && strings.TrimSpace(item.TargetEndpointID) != "":
			endpoint, err := service.store.GetStorageEndpointByID(ctx, item.TargetEndpointID)
			if err == nil {
				_ = os.Remove(resolveEndpointAbsolutePath(endpoint, item.TargetTempPath))
			}
		case strings.TrimSpace(item.TargetEndpointID) != "":
			endpoint, err := service.store.GetStorageEndpointByID(ctx, item.TargetEndpointID)
			if err == nil {
				connector, buildErr := service.buildConnector(endpoint)
				if buildErr == nil {
					_ = connector.DeleteEntry(ctx, item.TargetTempPath)
				}
			}
		}
	}
}

func (service *Service) cleanupTransferStagingArtifacts(stagingPath string) {
	stagingPath = strings.TrimSpace(stagingPath)
	if stagingPath == "" {
		return
	}

	_ = os.Remove(stagingPath)
	_ = os.Remove(stagingPath + ".aria2")
	service.pruneEmptyTransferTaskDirs(filepath.Dir(stagingPath))
}

func (service *Service) pruneEmptyTransferTaskDirs(directory string) {
	root := filepath.Join(service.transferStateRoot(), "tasks")
	current := filepath.Clean(strings.TrimSpace(directory))
	if current == "" {
		return
	}

	for {
		relative, err := filepath.Rel(root, current)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return
		}

		if removeErr := os.Remove(current); removeErr != nil {
			return
		}

		parent := filepath.Dir(current)
		if parent == current {
			return
		}
		current = parent
	}
}

func (service *Service) createCatalogTaskWithStatus(ctx context.Context, taskType string, payload any, status string) (store.Task, error) {
	now := time.Now().UTC()
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return store.Task{}, err
	}

	task := store.Task{
		ID:        uuid.NewString(),
		TaskType:  taskType,
		Status:    defaultString(strings.TrimSpace(status), taskStatusPending),
		Payload:   string(encodedPayload),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := service.store.CreateTask(ctx, task); err != nil {
		return store.Task{}, err
	}
	return task, nil
}

func buildTransferTaskRecord(task store.Task, items []store.TransferTaskItem) TransferTaskRecord {
	record := TransferTaskRecord{
		ID:        task.ID,
		TaskType:  task.TaskType,
		Status:    resolveTransferTaskStatus(task.Status, items),
		Priority:  task.Priority,
		CreatedAt: task.CreatedAt,
		UpdatedAt: task.UpdatedAt,
	}

	sourceLabels := make([]string, 0, len(items))
	targetLabels := make([]string, 0, len(items))
	engineLabels := make([]string, 0, len(items))
	var (
		totalProgress int
		progressCount int
		currentItem   string
		errorMessage  string
		currentSpeed  int64
		refreshEvery  int
		fileItem      store.TransferTaskItem
		hasFileItem   bool
	)

	for _, item := range items {
		status := strings.ToLower(strings.TrimSpace(item.Status))
		metadata := readTransferItemMetadata(item)
		switch status {
		case taskStatusQueued, taskStatusPending, taskStatusRetrying:
			record.QueuedItems++
		case taskStatusRunning:
			record.RunningItems++
		case taskStatusPaused:
			record.PausedItems++
		case taskStatusFailed:
			record.FailedItems++
		case taskStatusSuccess:
			record.SuccessItems++
		case transferItemStatusSkipped:
			record.SkippedItems++
		}

		record.TotalItems++
		itemTotalBytes := item.TotalBytes
		if itemTotalBytes <= 0 {
			itemTotalBytes = parseTransferMetadataSize(item.MetadataJSON)
		}
		record.TotalBytes += itemTotalBytes
		record.CompletedBytes += estimateTransferCompletedBytes(item)

		itemProgress := clampInt(item.ProgressPercent, 0, 100)
		if itemProgress == 0 && itemTotalBytes > 0 {
			itemProgress = calcTransferItemProgress(itemTotalBytes, item.StagedBytes, item.CommittedBytes, item.Phase)
		}
		totalProgress += itemProgress
		progressCount++
		if !hasFileItem || shouldPreferTransferDisplayItem(item, fileItem) {
			fileItem = item
			hasFileItem = true
		}

		if strings.TrimSpace(item.SourceLabel) != "" {
			sourceLabels = append(sourceLabels, item.SourceLabel)
		}
		if strings.TrimSpace(item.TargetLabel) != "" {
			targetLabels = append(targetLabels, item.TargetLabel)
		}
		if label := defaultString(strings.TrimSpace(metadata.EngineLabel), strings.TrimSpace(metadata.EngineKind)); label != "" {
			engineLabels = append(engineLabels, label)
		}

		if currentItem == "" {
			switch status {
			case taskStatusRunning:
				currentItem = defaultString(strings.TrimSpace(item.DisplayName), strings.TrimSpace(item.TargetPath))
			case taskStatusQueued, taskStatusPending, taskStatusRetrying:
				currentItem = defaultString(strings.TrimSpace(item.DisplayName), strings.TrimSpace(item.TargetPath))
			}
		}

		if errorMessage == "" && item.ErrorMessage != nil && strings.TrimSpace(*item.ErrorMessage) != "" {
			errorMessage = strings.TrimSpace(*item.ErrorMessage)
		}
		if status == taskStatusRunning {
			currentSpeed = max(currentSpeed, metadata.CurrentSpeed)
			if metadata.RefreshInterval > 0 && (refreshEvery == 0 || metadata.RefreshInterval < refreshEvery) {
				refreshEvery = metadata.RefreshInterval
			}
		}

		record.Direction = defaultString(strings.TrimSpace(record.Direction), strings.TrimSpace(item.Direction))
		record.StartedAt = earliestTimePointer(record.StartedAt, item.StartedAt)
		record.FinishedAt = latestTimePointer(record.FinishedAt, item.FinishedAt)
	}

	if record.Direction == "" {
		record.Direction = determineTransferDirection("", "")
	}
	record.SourceLabel = compactTransferLabel(sourceLabels)
	record.TargetLabel = compactTransferLabel(targetLabels)
	record.EngineSummary = compactTransferLabel(engineLabels)
	record.CurrentItemName = compactTransferLabel([]string{currentItem})
	if hasFileItem {
		fileSize := fileItem.TotalBytes
		if fileSize <= 0 {
			fileSize = parseTransferMetadataSize(fileItem.MetadataJSON)
		}
		record.FileName = transferDisplayItemName(fileItem)
		record.FilePath = transferDisplayItemPath(fileItem)
		record.FileSize = fileSize
		record.FileTransferred = estimateTransferCompletedBytes(fileItem)
		if record.FileSize > 0 {
			record.FileTransferred = minInt64(record.FileTransferred, record.FileSize)
		}
	}
	record.CurrentSpeed = currentSpeed
	record.RefreshInterval = refreshEvery
	record.StartedAt = preferTimePointer(task.StartedAt, record.StartedAt)
	if !isTransferTaskTerminal(record.Status) {
		record.FinishedAt = nil
	} else {
		record.FinishedAt = preferTimePointer(task.FinishedAt, record.FinishedAt)
	}
	if record.TotalBytes > 0 {
		record.CompletedBytes = minInt64(record.CompletedBytes, record.TotalBytes)
		record.ProgressPercent = clampInt(int((record.CompletedBytes*100)/record.TotalBytes), 0, 100)
	} else if progressCount > 0 {
		record.ProgressPercent = clampInt(totalProgress/progressCount, 0, 100)
	}
	if record.Status == taskStatusSuccess {
		record.ProgressPercent = 100
		record.CompletedBytes = max(record.CompletedBytes, record.TotalBytes)
	}
	record.ErrorMessage = defaultString(errorMessage, defaultStringPointer(task.ErrorMessage, ""))
	record.Title = transferTaskTitle(task.TaskType, record.TotalItems, firstTransferDisplayName(items), record.SourceLabel, record.TargetLabel)
	record.ProgressLabel = transferTaskProgressLabel(record)
	return record
}

func buildTransferTaskItemRecord(item store.TransferTaskItem) TransferTaskItemRecord {
	progressPercent := clampInt(item.ProgressPercent, 0, 100)
	totalBytes := item.TotalBytes
	if totalBytes <= 0 {
		totalBytes = parseTransferMetadataSize(item.MetadataJSON)
	}
	if progressPercent == 0 && totalBytes > 0 {
		progressPercent = calcTransferItemProgress(totalBytes, item.StagedBytes, item.CommittedBytes, item.Phase)
	}
	metadata := readTransferItemMetadata(item)

	record := TransferTaskItemRecord{
		ID:               item.ID,
		TaskID:           item.TaskID,
		ItemIndex:        item.ItemIndex,
		GroupKey:         item.GroupKey,
		Direction:        item.Direction,
		DisplayName:      defaultString(strings.TrimSpace(item.DisplayName), path.Base(strings.TrimSpace(item.TargetPath))),
		MediaType:        item.MediaType,
		SourceLabel:      item.SourceLabel,
		SourcePath:       item.SourcePath,
		TargetLabel:      item.TargetLabel,
		TargetPath:       item.TargetPath,
		AssetID:          defaultStringPointer(item.AssetID, ""),
		LogicalPathKey:   item.LogicalPathKey,
		Status:           item.Status,
		Phase:            defaultString(strings.TrimSpace(item.Phase), restoreTransferPhase(item)),
		EngineKind:       strings.TrimSpace(metadata.EngineKind),
		EngineLabel:      strings.TrimSpace(metadata.EngineLabel),
		CurrentSpeed:     metadata.CurrentSpeed,
		RefreshInterval:  metadata.RefreshInterval,
		ExternalTaskID:   transferItemExternalTaskID(item),
		ExternalStatus:   transferItemExternalStatus(item),
		ProgressPercent:  progressPercent,
		TotalBytes:       totalBytes,
		TransferredBytes: estimateTransferCompletedBytes(item),
		StagedBytes:      item.StagedBytes,
		CommittedBytes:   item.CommittedBytes,
		ErrorMessage:     defaultStringPointer(item.ErrorMessage, ""),
		CreatedAt:        item.CreatedAt,
		UpdatedAt:        item.UpdatedAt,
		StartedAt:        cloneTimePointer(item.StartedAt),
		FinishedAt:       cloneTimePointer(item.FinishedAt),
	}
	if record.TotalBytes > 0 {
		record.TransferredBytes = minInt64(record.TransferredBytes, record.TotalBytes)
	}
	return record
}

func accumulateTransferTaskStats(stats *TransferTaskStats, record TransferTaskRecord, _ []store.TransferTaskItem) {
	if stats == nil {
		return
	}

	stats.TotalTasks++
	switch strings.ToLower(strings.TrimSpace(record.Status)) {
	case taskStatusQueued:
		stats.QueuedTasks++
	case taskStatusRunning:
		stats.RunningTasks++
	case taskStatusPaused:
		stats.PausedTasks++
	case taskStatusFailed:
		stats.FailedTasks++
	case taskStatusSuccess:
		stats.SuccessTasks++
	}

	switch strings.ToLower(strings.TrimSpace(record.Direction)) {
	case transferDirectionUpload:
		stats.UploadTasks++
	case transferDirectionDownload:
		stats.DownloadTasks++
	default:
		stats.SyncTasks++
	}

	stats.TotalItems += record.TotalItems
	stats.RunningItems += record.RunningItems
	stats.PausedItems += record.PausedItems
	stats.FailedItems += record.FailedItems
	stats.SuccessItems += record.SuccessItems
	stats.SkippedItems += record.SkippedItems
	stats.TotalBytes += record.TotalBytes
	stats.CompletedBytes += record.CompletedBytes
}

func newTransferTaskItem(index int) store.TransferTaskItem {
	now := time.Now().UTC()
	return store.TransferTaskItem{
		ID:              uuid.NewString(),
		ItemIndex:       index,
		Status:          taskStatusPending,
		Phase:           transferPhasePending,
		MetadataJSON:    "{}",
		CreatedAt:       now,
		UpdatedAt:       now,
		ProgressPercent: 0,
	}
}

func (service *Service) newFailedImportItem(index int, relativePath string, deviceLabel string, message string) store.TransferTaskItem {
	item := newTransferTaskItem(index)
	item.GroupKey = defaultString(strings.TrimSpace(relativePath), fmt.Sprintf("import-%d", index))
	item.Direction = transferDirectionUpload
	item.SourceKind = transferSourceKindDevice
	item.SourceLabel = deviceLabel
	item.SourcePath = relativePath
	item.DisplayName = defaultString(strings.TrimSpace(path.Base(relativePath)), defaultString(strings.TrimSpace(relativePath), "未命名文件"))
	item.Status = taskStatusFailed
	item.Phase = transferPhaseFailed
	item.ErrorMessage = stringPointer(defaultString(strings.TrimSpace(message), "导入项初始化失败"))
	item.FinishedAt = cloneTimePointer(&item.UpdatedAt)
	return item
}

func (service *Service) newFailedTransferItem(
	index int,
	direction string,
	result BatchRestoreItemResult,
	targetLabel string,
	now time.Time,
) store.TransferTaskItem {
	item := newTransferTaskItem(index)
	item.CreatedAt = now
	item.UpdatedAt = now
	item.GroupKey = defaultString(strings.TrimSpace(result.AssetID), fmt.Sprintf("restore-%d", index))
	item.Direction = defaultString(strings.TrimSpace(direction), transferDirectionSync)
	item.TargetEndpointID = strings.TrimSpace(result.TargetEndpointID)
	item.TargetLabel = targetLabel
	item.AssetID = stringPointer(strings.TrimSpace(result.AssetID))
	item.DisplayName = defaultString(strings.TrimSpace(result.DisplayName), defaultString(strings.TrimSpace(result.AssetID), "未命名资产"))
	if result.Skipped {
		item.Status = transferItemStatusSkipped
		item.Phase = transferPhaseCompleted
		item.ProgressPercent = 100
		item.FinishedAt = &now
		return item
	}
	item.Status = taskStatusFailed
	item.Phase = transferPhaseFailed
	item.ErrorMessage = stringPointer(defaultString(strings.TrimSpace(result.Error), "恢复任务初始化失败"))
	item.FinishedAt = &now
	return item
}

func resolveInitialTransferTaskStatus(items []store.TransferTaskItem) string {
	if len(items) == 0 {
		return taskStatusPending
	}

	hasRunnable := false
	hasPaused := false
	hasFailed := false
	hasCanceled := false
	allSuccessful := true

	for _, item := range items {
		switch strings.ToLower(strings.TrimSpace(item.Status)) {
		case taskStatusQueued, taskStatusPending, taskStatusRetrying, taskStatusRunning:
			hasRunnable = true
			allSuccessful = false
		case taskStatusPaused:
			hasPaused = true
			allSuccessful = false
		case taskStatusFailed:
			hasFailed = true
			allSuccessful = false
		case taskStatusCanceled:
			hasCanceled = true
			allSuccessful = false
		case taskStatusSuccess, transferItemStatusSkipped:
		default:
			allSuccessful = false
		}
	}

	switch {
	case hasRunnable:
		return taskStatusQueued
	case hasPaused:
		return taskStatusPaused
	case hasFailed:
		return taskStatusFailed
	case hasCanceled && !allSuccessful:
		return taskStatusCanceled
	case allSuccessful:
		return taskStatusSuccess
	default:
		return taskStatusPending
	}
}

func canTransferTaskStart(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case taskStatusQueued, taskStatusPending, taskStatusRetrying:
		return true
	default:
		return false
	}
}

func canTransferTaskPause(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case taskStatusQueued, taskStatusPending, taskStatusRetrying, taskStatusRunning:
		return true
	default:
		return false
	}
}

func canTransferTaskResume(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case taskStatusPaused, taskStatusFailed, taskStatusCanceled:
		return true
	default:
		return false
	}
}

func canTransferTaskCancel(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case taskStatusQueued, taskStatusPending, taskStatusRetrying, taskStatusRunning, taskStatusPaused:
		return true
	default:
		return false
	}
}

func canTransferItemRun(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case taskStatusQueued, taskStatusPending, taskStatusRetrying:
		return true
	default:
		return false
	}
}

func isTransferTaskTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case taskStatusSuccess, taskStatusFailed, taskStatusCanceled:
		return true
	default:
		return false
	}
}

func isTransferItemTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case taskStatusSuccess, taskStatusFailed, taskStatusCanceled, transferItemStatusSkipped:
		return true
	default:
		return false
	}
}

func isTransferTaskType(taskType string) bool {
	switch strings.TrimSpace(taskType) {
	case taskTypeRestoreAsset, taskTypeRestoreBatch, taskTypeImportExecute:
		return true
	default:
		return false
	}
}

func supportsLocalResumableTransfer(endpointType string) bool {
	switch normalizeEndpointType(endpointType) {
	case string(connectors.EndpointTypeLocal), string(connectors.EndpointTypeQNAP), string(connectors.EndpointTypeRemovable):
		return true
	default:
		return false
	}
}

func resolveTransferTaskStatus(currentStatus string, items []store.TransferTaskItem) string {
	if len(items) == 0 {
		switch strings.ToLower(strings.TrimSpace(currentStatus)) {
		case taskStatusQueued, taskStatusPaused, taskStatusCanceled, taskStatusRunning, taskStatusFailed, taskStatusSuccess:
			return strings.ToLower(strings.TrimSpace(currentStatus))
		case taskStatusRetrying, taskStatusPending:
			return taskStatusQueued
		default:
			return taskStatusPending
		}
	}

	counts := make(map[string]int)
	for _, item := range items {
		counts[strings.ToLower(strings.TrimSpace(item.Status))]++
	}

	switch {
	case counts[taskStatusRunning] > 0:
		return taskStatusRunning
	case counts[taskStatusQueued]+counts[taskStatusPending]+counts[taskStatusRetrying] > 0:
		return taskStatusQueued
	case counts[taskStatusPaused] > 0:
		return taskStatusPaused
	case counts[taskStatusFailed] > 0:
		return taskStatusFailed
	case counts[taskStatusCanceled] > 0:
		return taskStatusCanceled
	case counts[taskStatusSuccess]+counts[transferItemStatusSkipped] == len(items):
		return taskStatusSuccess
	default:
		switch strings.ToLower(strings.TrimSpace(currentStatus)) {
		case taskStatusPaused, taskStatusCanceled, taskStatusRunning, taskStatusFailed, taskStatusSuccess:
			return strings.ToLower(strings.TrimSpace(currentStatus))
		default:
			return taskStatusPending
		}
	}
}

func transferTaskTitle(taskType string, totalItems int, itemName string, sourceLabel string, targetLabel string) string {
	switch strings.TrimSpace(taskType) {
	case taskTypeRestoreAsset:
		if strings.TrimSpace(itemName) != "" {
			return fmt.Sprintf("恢复 %s", compactTransferLabel([]string{itemName}))
		}
		return "恢复文件"
	case taskTypeRestoreBatch:
		if strings.TrimSpace(targetLabel) != "" {
			return fmt.Sprintf("批量恢复到 %s", targetLabel)
		}
		if totalItems > 0 {
			return fmt.Sprintf("批量恢复 %d 项", totalItems)
		}
		return "批量恢复"
	case taskTypeImportExecute:
		if strings.TrimSpace(sourceLabel) != "" {
			return fmt.Sprintf("从 %s 导入", sourceLabel)
		}
		if totalItems > 0 {
			return fmt.Sprintf("批量导入 %d 项", totalItems)
		}
		return "批量导入"
	default:
		return "传输任务"
	}
}

func transferTaskProgressLabel(record TransferTaskRecord) string {
	switch strings.ToLower(strings.TrimSpace(record.Status)) {
	case taskStatusRunning:
		if strings.TrimSpace(record.CurrentItemName) != "" {
			return fmt.Sprintf("正在传输 %s（%d%%）", record.CurrentItemName, record.ProgressPercent)
		}
		return fmt.Sprintf("正在处理 %d/%d 项（%d%%）", record.SuccessItems+record.SkippedItems+record.FailedItems+record.RunningItems, maxInt(record.TotalItems, 1), record.ProgressPercent)
	case taskStatusQueued:
		return buildQueuedTaskLabel(record.CurrentItemName)
	case taskStatusPaused:
		return fmt.Sprintf("已暂停，待继续 %d 项", maxInt(record.TotalItems-record.SuccessItems-record.SkippedItems, 0))
	case taskStatusFailed:
		if record.FailedItems > 0 {
			return fmt.Sprintf("失败 %d 项，可恢复继续", record.FailedItems)
		}
		return "任务失败，可恢复继续"
	case taskStatusCanceled:
		return "任务已取消"
	case taskStatusSuccess:
		return fmt.Sprintf("已完成 %d/%d 项", record.TotalItems, record.TotalItems)
	default:
		return "等待处理"
	}
}

func compactTransferLabel(values []string) string {
	normalized := uniqueStrings(values)
	if len(normalized) == 0 {
		return ""
	}
	if len(normalized) == 1 {
		return trimTransferLabel(normalized[0], 24)
	}
	return fmt.Sprintf("%s 等 %d 个", trimTransferLabel(normalized[0], 16), len(normalized))
}

func determineTransferDirection(sourceEndpointType string, targetEndpointType string) string {
	sourceType := normalizeEndpointType(sourceEndpointType)
	targetType := normalizeEndpointType(targetEndpointType)
	sourceRemote := sourceType == string(connectors.EndpointTypeNetwork)
	targetRemote := targetType == string(connectors.EndpointTypeNetwork)

	switch {
	case targetRemote && !sourceRemote:
		return transferDirectionUpload
	case sourceRemote && !targetRemote:
		return transferDirectionDownload
	case sourceType == string(connectors.EndpointTypeRemovable) && targetType != string(connectors.EndpointTypeRemovable):
		return transferDirectionUpload
	case targetType == string(connectors.EndpointTypeRemovable) && sourceType != string(connectors.EndpointTypeRemovable):
		return transferDirectionDownload
	default:
		return transferDirectionSync
	}
}

func transferQueueSlot(direction string) string {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case transferDirectionUpload:
		return transferDirectionUpload
	case transferDirectionDownload:
		return transferDirectionDownload
	default:
		return transferDirectionSync
	}
}

func shouldRequeueInterruptedTransferTask(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case taskStatusQueued, taskStatusRunning, taskStatusPending, taskStatusRetrying:
		return true
	default:
		return false
	}
}

func transferQueueLimit(preferences TransferPreferences, slot string) int {
	switch transferQueueSlot(slot) {
	case transferDirectionUpload:
		return clampInt(preferences.UploadConcurrency, 1, maxTransferConcurrency)
	case transferDirectionDownload:
		return clampInt(preferences.DownloadConcurrency, 1, maxTransferConcurrency)
	default:
		return maxInt(
			clampInt(preferences.UploadConcurrency, 1, maxTransferConcurrency),
			clampInt(preferences.DownloadConcurrency, 1, maxTransferConcurrency),
		)
	}
}

func sortTransferQueueSnapshots(snapshots []transferQueueSnapshot) {
	sort.SliceStable(snapshots, func(left, right int) bool {
		if snapshots[left].task.Priority != snapshots[right].task.Priority {
			return snapshots[left].task.Priority > snapshots[right].task.Priority
		}
		if !snapshots[left].task.CreatedAt.Equal(snapshots[right].task.CreatedAt) {
			return snapshots[left].task.CreatedAt.Before(snapshots[right].task.CreatedAt)
		}
		return snapshots[left].task.ID < snapshots[right].task.ID
	})
}

func sortTransferTaskSnapshotsForDisplay(snapshots []transferQueueSnapshot) {
	sort.SliceStable(snapshots, func(left, right int) bool {
		leftRank := transferTaskDisplayRank(snapshots[left].record.Status)
		rightRank := transferTaskDisplayRank(snapshots[right].record.Status)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if snapshots[left].task.Priority != snapshots[right].task.Priority {
			return snapshots[left].task.Priority > snapshots[right].task.Priority
		}

		switch leftRank {
		case 0:
			leftStarted := defaultTimePointer(snapshots[left].task.StartedAt, snapshots[left].task.CreatedAt)
			rightStarted := defaultTimePointer(snapshots[right].task.StartedAt, snapshots[right].task.CreatedAt)
			if !leftStarted.Equal(rightStarted) {
				return leftStarted.Before(rightStarted)
			}
		case 1:
			if !snapshots[left].task.CreatedAt.Equal(snapshots[right].task.CreatedAt) {
				return snapshots[left].task.CreatedAt.Before(snapshots[right].task.CreatedAt)
			}
		default:
			if !snapshots[left].task.UpdatedAt.Equal(snapshots[right].task.UpdatedAt) {
				return snapshots[left].task.UpdatedAt.After(snapshots[right].task.UpdatedAt)
			}
		}

		return snapshots[left].task.ID < snapshots[right].task.ID
	})
}

func transferTaskDisplayRank(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case taskStatusRunning:
		return 0
	case taskStatusQueued, taskStatusPending, taskStatusRetrying:
		return 1
	case taskStatusPaused:
		return 2
	case taskStatusFailed:
		return 3
	case taskStatusCanceled:
		return 4
	case taskStatusSuccess:
		return 5
	default:
		return 6
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func defaultTimePointer(value *time.Time, fallback time.Time) time.Time {
	if value != nil {
		return value.UTC()
	}
	return fallback.UTC()
}

func (service *Service) findOldestRunningTransferTaskByDirection(
	ctx context.Context,
	direction string,
	excludeTaskID string,
) (transferQueueSnapshot, bool, error) {
	snapshots, err := service.listTransferQueueSnapshots(ctx, 320)
	if err != nil {
		return transferQueueSnapshot{}, false, err
	}

	var selected transferQueueSnapshot
	found := false
	for _, snapshot := range snapshots {
		if snapshot.task.ID == excludeTaskID {
			continue
		}
		if transferQueueSlot(snapshot.record.Direction) != transferQueueSlot(direction) {
			continue
		}
		if strings.TrimSpace(snapshot.record.Status) != taskStatusRunning {
			continue
		}
		if !found {
			selected = snapshot
			found = true
			continue
		}
		leftStarted := defaultTimePointer(snapshot.task.StartedAt, snapshot.task.CreatedAt)
		rightStarted := defaultTimePointer(selected.task.StartedAt, selected.task.CreatedAt)
		if leftStarted.Before(rightStarted) {
			selected = snapshot
		}
	}

	return selected, found, nil
}

func buildQueuedTaskLabel(displayName string) string {
	if strings.TrimSpace(displayName) == "" {
		return "已加入传输队列，等待处理"
	}
	return fmt.Sprintf("已加入队列，等待传输 %s", compactTransferLabel([]string{displayName}))
}

func buildTransferTargetTempPath(targetPath string, itemID string) string {
	cleanPath := strings.TrimPrefix(canonicalizePath(targetPath), "/")
	baseName := path.Base(cleanPath)
	if baseName == "." || baseName == "/" || baseName == "" {
		baseName = shortTaskID(itemID)
	}
	tempName := fmt.Sprintf(".%s.mam-part-%s", baseName, shortTaskID(itemID))
	dirName := path.Dir(cleanPath)
	if dirName == "." || dirName == "/" || dirName == "" {
		return tempName
	}
	return path.Join(dirName, tempName)
}

func commitsTransferDirectly(endpointType string) bool {
	switch normalizeEndpointType(endpointType) {
	case string(connectors.EndpointTypeNetwork):
		return true
	default:
		return false
	}
}

func resolveEndpointAbsolutePath(endpoint store.StorageEndpoint, relativePath string) string {
	if normalizeEndpointType(endpoint.EndpointType) == string(connectors.EndpointTypeNetwork) {
		normalized := strings.TrimSpace(strings.ReplaceAll(relativePath, `\`, "/"))
		if normalized == "" || normalized == "." {
			return canonicalizePath(endpoint.RootPath)
		}
		if strings.HasPrefix(normalized, "/") {
			return canonicalizePath(normalized)
		}
		return canonicalizePath(path.Join(endpoint.RootPath, normalized))
	}
	if filepath.IsAbs(relativePath) {
		return filepath.Clean(relativePath)
	}
	cleanRelativePath := strings.TrimPrefix(canonicalizePath(relativePath), "/")
	return filepath.Join(endpoint.RootPath, filepath.FromSlash(cleanRelativePath))
}

func restoreTransferPhase(item store.TransferTaskItem) string {
	switch strings.ToLower(strings.TrimSpace(item.Status)) {
	case taskStatusPaused:
		return transferPhasePaused
	case taskStatusCanceled:
		return transferPhaseCanceled
	case taskStatusFailed:
		return transferPhaseFailed
	case taskStatusSuccess, transferItemStatusSkipped:
		return transferPhaseCompleted
	}

	totalBytes := item.TotalBytes
	if totalBytes <= 0 {
		totalBytes = parseTransferMetadataSize(item.MetadataJSON)
	}
	switch {
	case item.CommittedBytes > 0 && totalBytes > 0 && item.CommittedBytes >= totalBytes:
		return transferPhaseFinalizing
	case item.CommittedBytes > 0:
		return transferPhaseCommitting
	case item.StagedBytes > 0 && totalBytes > 0 && item.StagedBytes >= totalBytes:
		return transferPhaseCommitting
	case item.StagedBytes > 0:
		return transferPhaseStaging
	default:
		return transferPhasePending
	}
}

func canTransferItemResume(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case taskStatusSuccess, transferItemStatusSkipped:
		return false
	default:
		return true
	}
}

func resetTransferItemForResume(item *store.TransferTaskItem) {
	item.Status = taskStatusQueued
	item.ErrorMessage = nil
	item.FinishedAt = nil
	item.UpdatedAt = time.Now().UTC()

	if requiresFreshCommitOnResume(*item) {
		item.CommittedBytes = 0
	}

	updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
		metadata.CurrentSpeed = 0
		if requiresFreshCommitOnResume(*item) && metadata.AList != nil {
			metadata.AList.TaskID = ""
			metadata.AList.TaskStatus = ""
			metadata.AList.TaskState = 0
		}
	})

	item.Phase = restoreTransferPhase(*item)
	progressTotal := item.TotalBytes
	if progressTotal <= 0 {
		progressTotal = parseTransferMetadataSize(item.MetadataJSON)
	}
	item.ProgressPercent = calcTransferItemProgress(progressTotal, item.StagedBytes, item.CommittedBytes, item.Phase)
}

func syncTransferItemProgressFromArtifacts(item *store.TransferTaskItem) {
	totalBytes := max(item.TotalBytes, parseTransferMetadataSize(item.MetadataJSON))
	totalBytes = max(totalBytes, item.StagedBytes)
	totalBytes = max(totalBytes, item.CommittedBytes)
	item.TotalBytes = totalBytes

	progressTotal := totalBytes
	if progressTotal <= 0 {
		progressTotal = parseTransferMetadataSize(item.MetadataJSON)
	}
	item.ProgressPercent = calcTransferItemProgress(progressTotal, item.StagedBytes, item.CommittedBytes, item.Phase)
}

func requiresFreshCommitOnResume(item store.TransferTaskItem) bool {
	switch normalizeEndpointType(item.TargetEndpointType) {
	case string(connectors.EndpointTypeNetwork):
		return true
	default:
		return false
	}
}

func calcTransferItemProgress(totalBytes int64, stagedBytes int64, committedBytes int64, phase string) int {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case transferPhaseCompleted:
		return 100
	case transferPhaseFinalizing:
		return 95
	case transferPhasePaused, transferPhaseCanceled, transferPhaseFailed:
		if totalBytes > 0 {
			return clampInt(int((minInt64(max(stagedBytes, committedBytes), totalBytes)*100)/totalBytes), 0, 100)
		}
		return 0
	}

	total := totalBytes
	if total <= 0 {
		total = max(stagedBytes, committedBytes)
	}
	if total <= 0 {
		switch strings.ToLower(strings.TrimSpace(phase)) {
		case transferPhaseCommitting:
			return 70
		case transferPhaseFinalizing:
			return 95
		case transferPhaseStaging:
			return 10
		default:
			return 0
		}
	}

	stagedRatio := float64(minInt64(stagedBytes, total)) / float64(total)
	committedRatio := float64(minInt64(committedBytes, total)) / float64(total)

	switch strings.ToLower(strings.TrimSpace(phase)) {
	case transferPhaseCommitting:
		return clampInt(int(70+(committedRatio*25)), 0, 95)
	case transferPhaseFinalizing:
		return 95
	case transferPhaseStaging:
		fallthrough
	default:
		return clampInt(int(stagedRatio*70), 0, 70)
	}
}

func estimateTransferCompletedBytes(item store.TransferTaskItem) int64 {
	totalBytes := item.TotalBytes
	if totalBytes <= 0 {
		totalBytes = parseTransferMetadataSize(item.MetadataJSON)
	}
	switch strings.ToLower(strings.TrimSpace(item.Status)) {
	case taskStatusSuccess, transferItemStatusSkipped:
		if totalBytes > 0 {
			return totalBytes
		}
		return max(item.StagedBytes, item.CommittedBytes)
	}
	if totalBytes <= 0 {
		return max(item.StagedBytes, item.CommittedBytes)
	}
	progressPercent := clampInt(item.ProgressPercent, 0, 100)
	if progressPercent == 0 {
		progressPercent = calcTransferItemProgress(totalBytes, item.StagedBytes, item.CommittedBytes, defaultString(strings.TrimSpace(item.Phase), restoreTransferPhase(item)))
	}
	return minInt64(int64(progressPercent)*totalBytes/100, totalBytes)
}

func parseTransferMetadataModifiedAt(metadataJSON string) *time.Time {
	metadata := parseTransferItemMetadata(metadataJSON)
	if strings.TrimSpace(metadata.SourceModifiedAt) == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, metadata.SourceModifiedAt)
		if err == nil {
			value := parsed.UTC()
			return &value
		}
	}
	return nil
}

func parseTransferMetadataSize(metadataJSON string) int64 {
	metadata := parseTransferItemMetadata(metadataJSON)
	return max(metadata.SourceSize, 0)
}

func parseTransferItemMetadata(metadataJSON string) transferItemMetadata {
	if strings.TrimSpace(metadataJSON) == "" {
		return transferItemMetadata{}
	}
	var metadata transferItemMetadata
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return transferItemMetadata{}
	}
	return metadata
}

func fileSizeIfExists(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	return info.Size(), nil
}

func discardTransferBytes(ctx context.Context, reader io.Reader, bytesToDiscard int64) error {
	if bytesToDiscard <= 0 {
		return nil
	}
	written, err := io.CopyN(io.Discard, &contextAwareReader{ctx: ctx, reader: reader}, bytesToDiscard)
	if err != nil {
		return err
	}
	if written != bytesToDiscard {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func copyWithProgress(ctx context.Context, writer io.Writer, reader io.Reader, onProgress func(delta int64) error) error {
	buffer := make([]byte, transferCopyBufferSize)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		readCount, readErr := reader.Read(buffer)
		if readCount > 0 {
			writtenTotal := 0
			for writtenTotal < readCount {
				if err := ctx.Err(); err != nil {
					return err
				}
				written, writeErr := writer.Write(buffer[writtenTotal:readCount])
				if writeErr != nil {
					return writeErr
				}
				if written <= 0 {
					return io.ErrShortWrite
				}
				writtenTotal += written
			}
			if onProgress != nil {
				if err := onProgress(int64(readCount)); err != nil {
					return err
				}
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

func timePointerString(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func shortTaskID(value string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(value), "-", "")
	if len(normalized) <= 8 {
		return normalized
	}
	return normalized[:8]
}

func clampInt(value int, minimum int, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func minInt64(left int64, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func firstTransferDisplayName(items []store.TransferTaskItem) string {
	for _, item := range items {
		if strings.TrimSpace(item.DisplayName) != "" {
			return item.DisplayName
		}
	}
	return ""
}

func shouldPreferTransferDisplayItem(candidate store.TransferTaskItem, current store.TransferTaskItem) bool {
	candidateRank := transferDisplayItemRank(candidate.Status)
	currentRank := transferDisplayItemRank(current.Status)
	if candidateRank != currentRank {
		return candidateRank < currentRank
	}
	if candidate.ItemIndex != current.ItemIndex {
		return candidate.ItemIndex < current.ItemIndex
	}
	return strings.TrimSpace(candidate.ID) < strings.TrimSpace(current.ID)
}

func transferDisplayItemRank(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case taskStatusRunning:
		return 0
	case taskStatusQueued, taskStatusPending, taskStatusRetrying:
		return 1
	case taskStatusPaused:
		return 2
	case taskStatusFailed:
		return 3
	case taskStatusCanceled:
		return 4
	case taskStatusSuccess:
		return 5
	case transferItemStatusSkipped:
		return 6
	default:
		return 7
	}
}

func transferDisplayItemName(item store.TransferTaskItem) string {
	if name := strings.TrimSpace(item.DisplayName); name != "" {
		return name
	}

	for _, candidate := range []string{strings.TrimSpace(item.SourcePath), strings.TrimSpace(item.TargetPath)} {
		if candidate == "" {
			continue
		}
		if baseName := strings.TrimSpace(path.Base(candidate)); baseName != "" && baseName != "." && baseName != "/" {
			return baseName
		}
	}

	return "未命名文件"
}

func transferDisplayItemPath(item store.TransferTaskItem) string {
	for _, candidate := range []string{strings.TrimSpace(item.SourcePath), strings.TrimSpace(item.TargetPath)} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func trimTransferLabel(value string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) == 0 || maxRunes <= 0 || len(runes) <= maxRunes {
		return strings.TrimSpace(value)
	}
	return string(runes[:maxRunes]) + "..."
}

func earliestTimePointer(current *time.Time, candidate *time.Time) *time.Time {
	if candidate == nil {
		return cloneTimePointer(current)
	}
	if current == nil || candidate.Before(*current) {
		return cloneTimePointer(candidate)
	}
	return cloneTimePointer(current)
}

func latestTimePointer(current *time.Time, candidate *time.Time) *time.Time {
	if candidate == nil {
		return cloneTimePointer(current)
	}
	if current == nil || candidate.After(*current) {
		return cloneTimePointer(candidate)
	}
	return cloneTimePointer(current)
}

func preferTimePointer(primary *time.Time, fallback *time.Time) *time.Time {
	if primary != nil {
		return cloneTimePointer(primary)
	}
	return cloneTimePointer(fallback)
}

func defaultStringPointer(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return defaultString(strings.TrimSpace(*value), fallback)
}

type contextAwareReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextAwareReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
