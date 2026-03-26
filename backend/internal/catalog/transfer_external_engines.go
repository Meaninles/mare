package catalog

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"mam/backend/internal/connectors"
	sidecaralist "mam/backend/internal/sidecars/alist"
	sidecararia2 "mam/backend/internal/sidecars/aria2"
	"mam/backend/internal/store"
)

const (
	transferEngineKindAList = "alist"
	transferEngineKindAria2 = "aria2"

	transferExternalPollInterval = 400 * time.Millisecond
	alistRuntimeStagingMountPath = "/__mare_runtime_staging"
)

func (service *Service) stageTransferItemFromAList(ctx context.Context, taskID string, item *store.TransferTaskItem) error {
	stagingPath, err := service.ensureTransferStagingPath(item)
	if err != nil {
		return err
	}
	item.StagingPath = stagingPath

	sourceEndpoint, err := service.store.GetStorageEndpointByID(ctx, item.SourceEndpointID)
	if err != nil {
		return err
	}
	sourceConfig, alistRuntime, err := service.prepareAListEndpoint(ctx, sourceEndpoint)
	if err != nil {
		return err
	}

	sourceEntry, err := alistRuntime.StatEntry(ctx, item.SourcePath, sourceConfig.Password, true)
	if err != nil {
		return err
	}
	if item.TotalBytes <= 0 {
		item.TotalBytes = sourceEntry.Size
	}

	actualSize, err := fileSizeIfExists(stagingPath)
	if err != nil {
		return err
	}
	item.StagedBytes = actualSize
	if item.TotalBytes > 0 && item.StagedBytes >= item.TotalBytes {
		item.Phase = transferPhaseCommitting
		item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseCommitting)
		item.UpdatedAt = time.Now().UTC()
		updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
			metadata.EngineKind = transferEngineKindAria2
			metadata.EngineLabel = "AList -> Aria2 下载"
			metadata.CurrentSpeed = 0
			metadata.RefreshInterval = 1
			if metadata.Aria2 == nil {
				metadata.Aria2 = &transferItemAria2Metadata{}
			}
			metadata.Aria2.DownloadURI = sourceEntry.RawURL
			metadata.Aria2.TargetDir = filepath.Dir(stagingPath)
			metadata.Aria2.TargetOut = filepath.Base(stagingPath)
			metadata.Aria2.TotalLength = item.TotalBytes
			metadata.Aria2.Status = "complete"
		})
		return service.persistTransferItemState(context.Background(), taskID, *item)
	}

	metadata := updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
		metadata.EngineKind = transferEngineKindAria2
		metadata.EngineLabel = "AList -> Aria2 下载"
		metadata.RefreshInterval = 1
		if metadata.Aria2 == nil {
			metadata.Aria2 = &transferItemAria2Metadata{}
		}
		metadata.Aria2.DownloadURI = sourceEntry.RawURL
		metadata.Aria2.TargetDir = filepath.Dir(stagingPath)
		metadata.Aria2.TargetOut = filepath.Base(stagingPath)
		metadata.Aria2.TotalLength = max(item.TotalBytes, sourceEntry.Size)
	})
	item.Phase = transferPhaseStaging
	item.UpdatedAt = time.Now().UTC()
	if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
		return err
	}

	aria2Runtime := service.getAria2Runtime()
	gid := ""
	if metadata.Aria2 != nil {
		gid = strings.TrimSpace(metadata.Aria2.GID)
	}

	status, needCreate, err := service.resolveAria2Transfer(ctx, aria2Runtime, item, gid)
	if err != nil {
		return err
	}
	if needCreate {
		gid, err = aria2Runtime.AddURI(ctx, sidecararia2.AddRequest{
			URIs:           []string{sourceEntry.RawURL},
			Dir:            filepath.Dir(stagingPath),
			Out:            filepath.Base(stagingPath),
			Headers:        metadata.Aria2.Headers,
			Continue:       true,
			AllowOverwrite: false,
		})
		if err != nil {
			return err
		}
		updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
			if metadata.Aria2 == nil {
				metadata.Aria2 = &transferItemAria2Metadata{}
			}
			metadata.Aria2.GID = gid
			metadata.Aria2.Status = "active"
			metadata.Aria2.DownloadURI = sourceEntry.RawURL
			metadata.Aria2.TargetDir = filepath.Dir(stagingPath)
			metadata.Aria2.TargetOut = filepath.Base(stagingPath)
			metadata.Aria2.TotalLength = max(item.TotalBytes, sourceEntry.Size)
		})
		if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
			return err
		}
		status, err = aria2Runtime.TellStatus(ctx, gid)
		if err != nil && !isAria2UnknownTaskError(err) {
			return err
		}
	}

	return service.pollAria2Transfer(ctx, taskID, item, gid, status)
}

func (service *Service) commitTransferItemToAList(
	ctx context.Context,
	taskID string,
	item *store.TransferTaskItem,
	endpoint store.StorageEndpoint,
) (connectors.FileEntry, error) {
	_, runtime, err := service.prepareAListEndpoint(ctx, endpoint)
	if err != nil {
		return connectors.FileEntry{}, err
	}
	if err := service.ensureAListRuntimeStagingStorage(ctx, runtime); err != nil {
		return connectors.FileEntry{}, err
	}

	stageAListPath, err := service.resolveAListStagePath(item.StagingPath)
	if err != nil {
		return connectors.FileEntry{}, err
	}
	targetPath := resolveEndpointAbsolutePath(endpoint, item.TargetPath)
	targetDir := path.Dir(targetPath)
	targetName := path.Base(targetPath)
	stageName := path.Base(stageAListPath)

	if err := service.ensureAListDirectoryTree(ctx, runtime, targetDir); err != nil {
		return connectors.FileEntry{}, err
	}

	item.Phase = transferPhaseCommitting
	item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseCommitting)
	item.UpdatedAt = time.Now().UTC()
	updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
		metadata.EngineKind = transferEngineKindAList
		metadata.EngineLabel = "AList 复制上传"
		metadata.RefreshInterval = 1
		if metadata.AList == nil {
			metadata.AList = &transferItemAListMetadata{}
		}
		metadata.AList.TaskKind = "copy"
		metadata.AList.SourceDir = path.Dir(stageAListPath)
		metadata.AList.TargetDir = targetDir
		metadata.AList.Names = []string{stageName}
	})
	if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
		return connectors.FileEntry{}, err
	}

	metadata := readTransferItemMetadata(*item)
	existingTaskID := ""
	if metadata.AList != nil {
		existingTaskID = strings.TrimSpace(metadata.AList.TaskID)
	}

	taskInfo, needCreate, err := service.resolveAListCopyTask(ctx, runtime, existingTaskID)
	if err != nil {
		return connectors.FileEntry{}, err
	}
	if needCreate {
		taskInfo, err = runtime.CreateCopyTask(ctx, path.Dir(stageAListPath), targetDir, []string{stageName}, true)
		if err != nil {
			return connectors.FileEntry{}, err
		}
		updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
			if metadata.AList == nil {
				metadata.AList = &transferItemAListMetadata{}
			}
			metadata.AList.TaskID = taskInfo.ID
			metadata.AList.TaskKind = "copy"
			metadata.AList.TaskStatus = service.normalizeAListTaskStatus(taskInfo)
			metadata.AList.TaskState = taskInfo.State
			metadata.AList.SourceDir = path.Dir(stageAListPath)
			metadata.AList.TargetDir = targetDir
			metadata.AList.Names = []string{stageName}
		})
		if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
			return connectors.FileEntry{}, err
		}
	}

	lastBytes := item.CommittedBytes
	lastObservedAt := time.Now().UTC()
	for {
		if err := ctx.Err(); err != nil {
			return connectors.FileEntry{}, err
		}
		if strings.TrimSpace(taskInfo.ID) == "" {
			return connectors.FileEntry{}, fmt.Errorf("alist copy task id is empty")
		}

		currentBytes := service.estimateAListCommittedBytes(item.TotalBytes, taskInfo)
		now := time.Now().UTC()
		item.CommittedBytes = max(item.CommittedBytes, currentBytes)
		item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseCommitting)
		item.UpdatedAt = now
		speed := estimateTransferSpeed(item.CommittedBytes-lastBytes, now.Sub(lastObservedAt))
		updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
			metadata.EngineKind = transferEngineKindAList
			metadata.EngineLabel = "AList 复制上传"
			metadata.CurrentSpeed = speed
			metadata.RefreshInterval = 1
			if metadata.AList == nil {
				metadata.AList = &transferItemAListMetadata{}
			}
			metadata.AList.TaskID = taskInfo.ID
			metadata.AList.TaskKind = "copy"
			metadata.AList.TaskStatus = service.normalizeAListTaskStatus(taskInfo)
			metadata.AList.TaskState = taskInfo.State
			metadata.AList.SourceDir = path.Dir(stageAListPath)
			metadata.AList.TargetDir = targetDir
			metadata.AList.Names = []string{stageName}
		})
		if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
			return connectors.FileEntry{}, err
		}
		lastBytes = item.CommittedBytes
		lastObservedAt = now

		switch service.normalizeAListTaskStatus(taskInfo) {
		case "complete":
			connector, err := service.buildAListConnector(ctx, endpoint)
			if err != nil {
				return connectors.FileEntry{}, err
			}
			if stageName != targetName {
				if _, err := connector.StatEntry(ctx, targetPath); err != nil {
					if err := service.renameAListCopyTarget(ctx, runtime, targetDir, stageName, targetName); err != nil {
						return connectors.FileEntry{}, err
					}
				}
			}

			item.CommittedBytes = max(item.CommittedBytes, item.TotalBytes)
			item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseFinalizing)
			item.UpdatedAt = time.Now().UTC()
			updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
				metadata.CurrentSpeed = 0
				if metadata.AList != nil {
					metadata.AList.TaskStatus = "complete"
					metadata.AList.TaskState = taskInfo.State
				}
			})
			if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
				return connectors.FileEntry{}, err
			}
			entry, err := connector.StatEntry(ctx, targetPath)
			if err != nil {
				return connectors.FileEntry{}, err
			}
			return entry, nil
		case "canceled":
			return connectors.FileEntry{}, context.Canceled
		case "failed":
			return connectors.FileEntry{}, fmt.Errorf("alist copy failed: %s", defaultString(strings.TrimSpace(taskInfo.Error), strings.TrimSpace(taskInfo.Status)))
		}

		select {
		case <-ctx.Done():
			return connectors.FileEntry{}, ctx.Err()
		case <-time.After(transferExternalPollInterval):
		}

		taskInfo, err = runtime.GetCopyTask(ctx, taskInfo.ID)
		if err != nil {
			return connectors.FileEntry{}, err
		}
	}
}

func (service *Service) pauseExternalTransferItem(ctx context.Context, item *store.TransferTaskItem) error {
	metadata := readTransferItemMetadata(*item)

	if metadata.Aria2 != nil && strings.TrimSpace(metadata.Aria2.GID) != "" {
		if err := service.getAria2Runtime().Pause(ctx, metadata.Aria2.GID); err != nil && !isAria2UnknownTaskError(err) {
			return err
		}
		updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
			metadata.CurrentSpeed = 0
			if metadata.Aria2 != nil {
				metadata.Aria2.Status = "paused"
			}
		})
	}

	if metadata.AList != nil && strings.TrimSpace(metadata.AList.TaskID) != "" {
		targetRuntime := service.getAListRuntime()
		if err := targetRuntime.CancelCopyTask(ctx, metadata.AList.TaskID); err != nil && !isAListUnknownTaskError(err) {
			return err
		}
		updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
			metadata.CurrentSpeed = 0
			if metadata.AList != nil {
				metadata.AList.TaskStatus = "canceled"
				metadata.AList.TaskState = 7
			}
		})
	}

	return nil
}

func (service *Service) deleteExternalTransferItem(ctx context.Context, item *store.TransferTaskItem) error {
	metadata := readTransferItemMetadata(*item)

	if metadata.Aria2 != nil && strings.TrimSpace(metadata.Aria2.GID) != "" {
		if err := service.getAria2Runtime().Remove(ctx, metadata.Aria2.GID); err != nil && !isAria2UnknownTaskError(err) {
			return err
		}
	}

	if metadata.AList != nil && strings.TrimSpace(metadata.AList.TaskID) != "" {
		targetRuntime := service.getAListRuntime()
		if err := targetRuntime.CancelCopyTask(ctx, metadata.AList.TaskID); err != nil && !isAListUnknownTaskError(err) {
			return err
		}
	}

	updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
		metadata.CurrentSpeed = 0
		if metadata.Aria2 != nil {
			metadata.Aria2.Status = "removed"
		}
		if metadata.AList != nil {
			metadata.AList.TaskStatus = "canceled"
			metadata.AList.TaskState = 7
		}
	})
	return nil
}

func (service *Service) resolveAria2Transfer(
	ctx context.Context,
	runtime *sidecararia2.Runtime,
	item *store.TransferTaskItem,
	gid string,
) (sidecararia2.Status, bool, error) {
	gid = strings.TrimSpace(gid)
	if gid == "" {
		return sidecararia2.Status{}, true, nil
	}

	status, err := runtime.TellStatus(ctx, gid)
	if err != nil {
		if isAria2UnknownTaskError(err) {
			updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
				if metadata.Aria2 != nil {
					metadata.Aria2.GID = ""
					metadata.Aria2.Status = ""
				}
			})
			return sidecararia2.Status{}, true, nil
		}
		return sidecararia2.Status{}, false, err
	}

	switch strings.ToLower(strings.TrimSpace(status.Status)) {
	case "error", "removed":
		_ = runtime.Remove(ctx, gid)
		updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
			if metadata.Aria2 != nil {
				metadata.Aria2.GID = ""
				metadata.Aria2.Status = status.Status
			}
		})
		return sidecararia2.Status{}, true, nil
	default:
		return status, false, nil
	}
}

func (service *Service) pollAria2Transfer(
	ctx context.Context,
	taskID string,
	item *store.TransferTaskItem,
	gid string,
	initialStatus sidecararia2.Status,
) error {
	aria2Runtime := service.getAria2Runtime()
	status := initialStatus
	lastPersistAt := time.Now().UTC()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		if strings.TrimSpace(status.GID) == "" {
			refreshed, err := aria2Runtime.TellStatus(ctx, gid)
			if err != nil {
				if isAria2UnknownTaskError(err) {
					return fmt.Errorf("aria2 task %q not found", gid)
				}
				return err
			}
			status = refreshed
		}

		item.StagedBytes = max(item.StagedBytes, status.CompletedLength)
		if item.TotalBytes <= 0 {
			item.TotalBytes = max(status.TotalLength, item.StagedBytes)
		}
		item.ProgressPercent = calcTransferItemProgress(max(item.TotalBytes, 1), item.StagedBytes, item.CommittedBytes, transferPhaseStaging)
		item.UpdatedAt = time.Now().UTC()
		updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
			metadata.EngineKind = transferEngineKindAria2
			metadata.EngineLabel = "AList -> Aria2 下载"
			metadata.CurrentSpeed = status.DownloadSpeed
			metadata.RefreshInterval = 1
			if metadata.Aria2 == nil {
				metadata.Aria2 = &transferItemAria2Metadata{}
			}
			metadata.Aria2.GID = gid
			metadata.Aria2.Status = status.Status
			metadata.Aria2.TotalLength = max(status.TotalLength, item.TotalBytes)
		})

		if time.Since(lastPersistAt) >= transferProgressPersistWindow {
			if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
				return err
			}
			lastPersistAt = time.Now().UTC()
		}

		switch strings.ToLower(strings.TrimSpace(status.Status)) {
		case "complete":
			actualSize, err := fileSizeIfExists(item.StagingPath)
			if err != nil {
				return err
			}
			item.StagedBytes = actualSize
			if item.TotalBytes <= 0 {
				item.TotalBytes = max(actualSize, status.TotalLength)
			}
			item.Phase = transferPhaseCommitting
			item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseCommitting)
			item.UpdatedAt = time.Now().UTC()
			updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
				metadata.CurrentSpeed = 0
				if metadata.Aria2 != nil {
					metadata.Aria2.Status = "complete"
				}
			})
			return service.persistTransferItemState(context.Background(), taskID, *item)
		case "error", "removed":
			return fmt.Errorf("aria2 download failed: %s", defaultString(strings.TrimSpace(status.ErrorMessage), strings.TrimSpace(status.ErrorCode)))
		case "paused":
			if err := aria2Runtime.Unpause(ctx, gid); err != nil && !isAria2UnknownTaskError(err) {
				return err
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(transferExternalPollInterval):
		}

		nextStatus, err := aria2Runtime.TellStatus(ctx, gid)
		if err != nil {
			if isAria2UnknownTaskError(err) {
				return fmt.Errorf("aria2 task %q not found", gid)
			}
			return err
		}
		status = nextStatus
	}
}

func (service *Service) resolveAListCopyTask(ctx context.Context, runtime *sidecaralist.Runtime, taskID string) (sidecaralist.TaskInfo, bool, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return sidecaralist.TaskInfo{}, true, nil
	}

	taskInfo, err := runtime.GetCopyTask(ctx, taskID)
	if err != nil {
		if isAListUnknownTaskError(err) {
			return sidecaralist.TaskInfo{}, true, nil
		}
		return sidecaralist.TaskInfo{}, false, err
	}

	switch service.normalizeAListTaskStatus(taskInfo) {
	case "failed", "canceled":
		return sidecaralist.TaskInfo{}, true, nil
	default:
		return taskInfo, false, nil
	}
}

func (service *Service) ensureAListRuntimeStagingStorage(ctx context.Context, runtime *sidecaralist.Runtime) error {
	if err := ensureDirectory(filepath.Join(service.transferStateRoot(), "tasks")); err != nil {
		return err
	}
	_, err := runtime.EnsureStorage(ctx, buildAListRuntimeStorageSpec(alistRuntimeStagingMountPath, filepath.Join(service.transferStateRoot(), "tasks")))
	return err
}

func (service *Service) resolveAListStagePath(stagingPath string) (string, error) {
	tasksRoot := filepath.Join(service.transferStateRoot(), "tasks")
	absoluteStagingPath := filepath.Clean(stagingPath)
	relativePath, err := filepath.Rel(tasksRoot, absoluteStagingPath)
	if err != nil {
		return "", err
	}
	relativePath = filepath.ToSlash(relativePath)
	if relativePath == "." || strings.HasPrefix(relativePath, "../") || relativePath == ".." {
		return "", fmt.Errorf("staging path %q is outside transfer tasks root", stagingPath)
	}
	return normalizeAListEndpointPath(path.Join(alistRuntimeStagingMountPath, relativePath)), nil
}

func (service *Service) ensureAListDirectoryTree(ctx context.Context, runtime *sidecaralist.Runtime, targetDir string) error {
	targetDir = normalizeAListEndpointPath(targetDir)
	if targetDir == "" || targetDir == "/" {
		return nil
	}

	segments := strings.Split(strings.TrimPrefix(targetDir, "/"), "/")
	current := ""
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		current = normalizeAListEndpointPath(path.Join(current, segment))
		if err := runtime.MakeDirectory(ctx, current); err != nil && !isAListAlreadyExistsError(err) {
			return err
		}
	}
	return nil
}

func (service *Service) renameAListCopyTarget(ctx context.Context, runtime *sidecaralist.Runtime, targetDir string, sourceName string, targetName string) error {
	sourceName = strings.TrimSpace(sourceName)
	targetName = strings.TrimSpace(targetName)
	if sourceName == "" || targetName == "" || sourceName == targetName {
		return nil
	}

	targetPath := normalizeAListEndpointPath(path.Join(targetDir, targetName))
	sourcePath := normalizeAListEndpointPath(path.Join(targetDir, sourceName))
	if entry, err := runtime.StatEntry(ctx, targetPath, "", true); err == nil && !entry.IsDir {
		_ = runtime.RemoveEntry(ctx, path.Dir(targetPath), []string{path.Base(targetPath)})
	}
	return runtime.RenameEntry(ctx, sourcePath, targetName, true)
}

func (service *Service) normalizeAListTaskStatus(taskInfo sidecaralist.TaskInfo) string {
	if taskInfo.State == 2 && strings.TrimSpace(taskInfo.Error) == "" {
		return "complete"
	}

	lowerError := strings.ToLower(strings.TrimSpace(taskInfo.Error))
	if taskInfo.State == 7 || strings.Contains(lowerError, "context canceled") {
		return "canceled"
	}
	if lowerError != "" {
		return "failed"
	}

	status := strings.ToLower(strings.TrimSpace(taskInfo.Status))
	switch status {
	case "complete", "success", "done":
		return "complete"
	case "cancel", "canceled", "cancelled", "stopped":
		return "canceled"
	case "failed", "error":
		return "failed"
	case "running", "active", "doing", "pending", "waiting", "queued":
		return status
	}

	switch taskInfo.State {
	case 0:
		return "pending"
	case 1:
		return "running"
	default:
		return defaultString(status, "running")
	}
}

func (service *Service) estimateAListCommittedBytes(totalBytes int64, taskInfo sidecaralist.TaskInfo) int64 {
	if totalBytes <= 0 {
		totalBytes = taskInfo.TotalBytes
	}
	if totalBytes <= 0 {
		return 0
	}
	progressPercent := normalizeAListProgressPercent(taskInfo.Progress)
	return minInt64(int64(progressPercent)*totalBytes/100, totalBytes)
}

func normalizeAListProgressPercent(progress float64) int {
	switch {
	case progress <= 0:
		return 0
	case progress <= 1.0001:
		return clampInt(int(progress*100), 0, 100)
	default:
		return clampInt(int(progress), 0, 100)
	}
}

func estimateTransferSpeed(deltaBytes int64, elapsed time.Duration) int64 {
	if deltaBytes <= 0 || elapsed <= 0 {
		return 0
	}
	return int64(float64(deltaBytes) / elapsed.Seconds())
}

func isAria2UnknownTaskError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such download") ||
		strings.Contains(message, "not found") ||
		strings.Contains(message, "gid")
}

func isAListUnknownTaskError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") ||
		strings.Contains(message, "no rows") ||
		strings.Contains(message, "tid")
}

func isAListAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "exist") || strings.Contains(message, "already")
}
