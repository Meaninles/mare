package catalog

import (
	"context"
	"fmt"
	"os"
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
	sourceConfig, alistRuntime, err := service.prepareAListBackedEndpoint(ctx, sourceEndpoint)
	if err != nil {
		return err
	}

	sourceEntry, err := alistRuntime.StatEntry(ctx, item.SourcePath, sourceConfig.Password, true)
	if err != nil {
		return err
	}
	linkInfo, err := alistRuntime.LinkEntry(ctx, item.SourcePath, sourceConfig.Password)
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
			metadata.EngineLabel = "网络存储下载"
			metadata.CurrentSpeed = 0
			metadata.RefreshInterval = 1
			if metadata.Aria2 == nil {
				metadata.Aria2 = &transferItemAria2Metadata{}
			}
			metadata.Aria2.DownloadURI = linkInfo.URL
			metadata.Aria2.Headers = flattenLinkHeaders(linkInfo.Headers)
			metadata.Aria2.TargetDir = filepath.Dir(stagingPath)
			metadata.Aria2.TargetOut = filepath.Base(stagingPath)
			metadata.Aria2.TotalLength = item.TotalBytes
			metadata.Aria2.Status = "complete"
		})
		return service.persistTransferItemState(context.Background(), taskID, *item)
	}

	metadata := updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
		metadata.EngineKind = transferEngineKindAria2
		metadata.EngineLabel = "网络存储下载"
		metadata.RefreshInterval = 1
		if metadata.Aria2 == nil {
			metadata.Aria2 = &transferItemAria2Metadata{}
		}
		metadata.Aria2.DownloadURI = linkInfo.URL
		metadata.Aria2.Headers = flattenLinkHeaders(linkInfo.Headers)
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
			URIs:           []string{linkInfo.URL},
			Dir:            filepath.Dir(stagingPath),
			Out:            filepath.Base(stagingPath),
			Headers:        flattenLinkHeaders(linkInfo.Headers),
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
			metadata.Aria2.DownloadURI = linkInfo.URL
			metadata.Aria2.Headers = flattenLinkHeaders(linkInfo.Headers)
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
	config, runtime, err := service.prepareAListBackedEndpoint(ctx, endpoint)
	if err != nil {
		return connectors.FileEntry{}, err
	}

	targetPath := resolveEndpointAbsolutePath(endpoint, item.TargetPath)
	targetDir := path.Dir(targetPath)
	targetName := path.Base(targetPath)

	if err := service.ensureAListDirectoryTree(ctx, runtime, targetDir, endpoint.RootPath); err != nil {
		return connectors.FileEntry{}, err
	}

	item.Phase = transferPhaseCommitting
	item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseCommitting)
	item.UpdatedAt = time.Now().UTC()
	updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
		metadata.EngineKind = transferEngineKindAList
		metadata.EngineLabel = "??????"
		metadata.RefreshInterval = 1
		if metadata.AList == nil {
			metadata.AList = &transferItemAListMetadata{}
		}
		metadata.AList.TaskKind = "upload"
		metadata.AList.SourceDir = filepath.Dir(item.StagingPath)
		metadata.AList.TargetDir = targetDir
		metadata.AList.Names = []string{targetName}
	})
	if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
		return connectors.FileEntry{}, err
	}

	metadata := readTransferItemMetadata(*item)
	existingTaskID := ""
	if metadata.AList != nil {
		existingTaskID = strings.TrimSpace(metadata.AList.TaskID)
	}

	taskInfo, needCreate, err := service.resolveAListUploadTask(ctx, runtime, existingTaskID)
	if err != nil {
		return connectors.FileEntry{}, err
	}
	if needCreate {
		stageFile, err := os.Open(item.StagingPath)
		if err != nil {
			return connectors.FileEntry{}, err
		}
		defer stageFile.Close()

		stageInfo, err := stageFile.Stat()
		if err != nil {
			return connectors.FileEntry{}, err
		}

		taskInfo, err = runtime.CreateUploadTask(ctx, targetPath, config.Password, true, stageFile, stageInfo.Size())
		if err != nil {
			return connectors.FileEntry{}, err
		}
		updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
			if metadata.AList == nil {
				metadata.AList = &transferItemAListMetadata{}
			}
			metadata.AList.TaskID = taskInfo.ID
			metadata.AList.TaskKind = "upload"
			metadata.AList.TaskStatus = service.normalizeAListTaskStatus(taskInfo)
			metadata.AList.TaskState = taskInfo.State
			metadata.AList.SourceDir = filepath.Dir(item.StagingPath)
			metadata.AList.TargetDir = targetDir
			metadata.AList.Names = []string{targetName}
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
			return connectors.FileEntry{}, fmt.Errorf("alist upload task id is empty")
		}

		currentBytes := service.estimateAListCommittedBytes(item.TotalBytes, taskInfo)
		now := time.Now().UTC()
		item.CommittedBytes = max(item.CommittedBytes, currentBytes)
		item.ProgressPercent = calcTransferItemProgress(item.TotalBytes, item.StagedBytes, item.CommittedBytes, transferPhaseCommitting)
		item.UpdatedAt = now
		speed := estimateTransferSpeed(item.CommittedBytes-lastBytes, now.Sub(lastObservedAt))
		updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
			metadata.EngineKind = transferEngineKindAList
			metadata.EngineLabel = "??????"
			metadata.CurrentSpeed = speed
			metadata.RefreshInterval = 1
			if metadata.AList == nil {
				metadata.AList = &transferItemAListMetadata{}
			}
			metadata.AList.TaskID = taskInfo.ID
			metadata.AList.TaskKind = "upload"
			metadata.AList.TaskStatus = service.normalizeAListTaskStatus(taskInfo)
			metadata.AList.TaskState = taskInfo.State
			metadata.AList.SourceDir = filepath.Dir(item.StagingPath)
			metadata.AList.TargetDir = targetDir
			metadata.AList.Names = []string{targetName}
		})
		if err := service.persistTransferItemState(context.Background(), taskID, *item); err != nil {
			return connectors.FileEntry{}, err
		}
		lastBytes = item.CommittedBytes
		lastObservedAt = now

		switch service.normalizeAListTaskStatus(taskInfo) {
		case "complete":
			connector, err := service.buildConnector(endpoint)
			if err != nil {
				return connectors.FileEntry{}, err
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
			return connectors.FileEntry{}, fmt.Errorf("alist upload failed: %s", defaultString(strings.TrimSpace(taskInfo.Error), strings.TrimSpace(taskInfo.Status)))
		}

		select {
		case <-ctx.Done():
			return connectors.FileEntry{}, ctx.Err()
		case <-time.After(transferExternalPollInterval):
		}

		taskInfo, err = runtime.GetUploadTask(ctx, taskInfo.ID)
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
		if err := service.cancelAListTask(ctx, targetRuntime, metadata.AList.TaskID, metadata.AList.TaskKind); err != nil && !isAListUnknownTaskError(err) {
			return err
		}
		service.cleanupAListTargetResidue(ctx, *item)
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
		if err := service.cancelAListTask(ctx, targetRuntime, metadata.AList.TaskID, metadata.AList.TaskKind); err != nil && !isAListUnknownTaskError(err) {
			return err
		}
		if err := service.deleteAListTask(ctx, targetRuntime, metadata.AList.TaskID, metadata.AList.TaskKind); err != nil && !isAListUnknownTaskError(err) {
			return err
		}
	}
	service.cleanupAListTargetResidue(ctx, *item)

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

func (service *Service) ensureAria2RuntimeAvailable(ctx context.Context) (bool, error) {
	if err := service.getAria2Runtime().EnsureRunning(ctx); err != nil {
		if isOptionalSidecarStartupError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (service *Service) ensureAListRuntimeAvailable(ctx context.Context) (bool, error) {
	if err := service.getAListRuntime().EnsureRunning(ctx); err != nil {
		if isOptionalSidecarStartupError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (service *Service) reconcileInterruptedTransferItem(
	ctx context.Context,
	item *store.TransferTaskItem,
	taskStatus string,
	aria2Available bool,
	alistAvailable bool,
) error {
	if item == nil {
		return nil
	}
	before := *item

	if err := service.reconcileInterruptedTransferFiles(ctx, item); err != nil {
		return err
	}
	if aria2Available {
		if err := service.reconcileInterruptedAria2Transfer(ctx, item); err != nil {
			return err
		}
	}
	if alistAvailable {
		if err := service.reconcileInterruptedAListTransfer(ctx, item); err != nil {
			return err
		}
	}

	normalizedTaskStatus := strings.ToLower(strings.TrimSpace(taskStatus))
	switch normalizedTaskStatus {
	case taskStatusPaused:
		service.applyRecoveredTransferItemStatus(item, taskStatusPaused, transferPhasePaused)
	case taskStatusCanceled:
		service.applyRecoveredTransferItemStatus(item, taskStatusCanceled, transferPhaseCanceled)
	case taskStatusFailed:
		if !isTransferItemTerminal(item.Status) && strings.TrimSpace(strings.ToLower(item.Status)) != transferItemStatusSkipped {
			service.applyRecoveredTransferItemStatus(item, taskStatusFailed, transferPhaseFailed)
		}
	default:
		if shouldRequeueInterruptedTransferTask(normalizedTaskStatus) && canTransferItemResume(item.Status) {
			item.Status = taskStatusQueued
			item.Phase = restoreTransferPhase(*item)
			item.ErrorMessage = nil
			item.FinishedAt = nil
		}
	}

	updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
		metadata.CurrentSpeed = 0
	})
	syncTransferItemProgressFromArtifacts(item)
	if *item != before {
		item.UpdatedAt = time.Now().UTC()
	}
	return nil
}

func (service *Service) applyRecoveredTransferItemStatus(item *store.TransferTaskItem, status string, phase string) {
	if item == nil || isTransferItemTerminal(item.Status) || strings.TrimSpace(strings.ToLower(item.Status)) == transferItemStatusSkipped {
		return
	}
	item.Status = status
	item.Phase = phase
	item.ErrorMessage = nil
	item.FinishedAt = nil
}

func (service *Service) reconcileInterruptedTransferFiles(ctx context.Context, item *store.TransferTaskItem) error {
	if item == nil {
		return nil
	}

	if strings.TrimSpace(item.StagingPath) != "" {
		size, err := fileSizeIfExists(item.StagingPath)
		if err != nil {
			return err
		}
		item.StagedBytes = max(item.StagedBytes, size)
	}

	if !supportsLocalResumableTransfer(item.TargetEndpointType) ||
		strings.TrimSpace(item.TargetEndpointID) == "" ||
		strings.TrimSpace(item.TargetTempPath) == "" {
		return nil
	}

	endpoint, err := service.store.GetStorageEndpointByID(ctx, item.TargetEndpointID)
	if err != nil {
		return nil
	}

	size, err := fileSizeIfExists(resolveEndpointAbsolutePath(endpoint, item.TargetTempPath))
	if err != nil {
		return err
	}
	item.CommittedBytes = max(item.CommittedBytes, size)
	return nil
}

func (service *Service) reconcileInterruptedAria2Transfer(ctx context.Context, item *store.TransferTaskItem) error {
	metadata := readTransferItemMetadata(*item)
	if metadata.Aria2 == nil || strings.TrimSpace(metadata.Aria2.GID) == "" {
		return nil
	}

	status, needCreate, err := service.resolveAria2Transfer(ctx, service.getAria2Runtime(), item, metadata.Aria2.GID)
	if err != nil {
		return err
	}

	if needCreate {
		updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
			if metadata.Aria2 != nil {
				metadata.Aria2.TotalLength = max(metadata.Aria2.TotalLength, item.TotalBytes)
			}
		})
		return nil
	}

	item.StagedBytes = max(item.StagedBytes, status.CompletedLength)
	item.TotalBytes = max(item.TotalBytes, status.TotalLength)
	updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
		if metadata.Aria2 == nil {
			return
		}
		metadata.Aria2.GID = defaultString(strings.TrimSpace(status.GID), metadata.Aria2.GID)
		metadata.Aria2.Status = strings.TrimSpace(status.Status)
		metadata.Aria2.TotalLength = max(metadata.Aria2.TotalLength, max(status.TotalLength, item.TotalBytes))
	})
	return nil
}

func (service *Service) reconcileInterruptedAListTransfer(ctx context.Context, item *store.TransferTaskItem) error {
	metadata := readTransferItemMetadata(*item)
	if metadata.AList == nil || strings.TrimSpace(metadata.AList.TaskID) == "" {
		return nil
	}

	taskInfo, needCreate, err := service.resolveAListTask(ctx, service.getAListRuntime(), metadata.AList.TaskID, metadata.AList.TaskKind)
	if err != nil {
		return err
	}

	if needCreate {
		service.cleanupAListTargetResidue(ctx, *item)
		item.CommittedBytes = 0
		updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
			if metadata.AList == nil {
				return
			}
			metadata.AList.TaskID = ""
			metadata.AList.TaskStatus = ""
			metadata.AList.TaskState = 0
		})
		return nil
	}

	item.TotalBytes = max(item.TotalBytes, taskInfo.TotalBytes)
	item.CommittedBytes = max(item.CommittedBytes, service.estimateAListCommittedBytes(item.TotalBytes, taskInfo))
	updateTransferItemMetadata(item, func(metadata *transferItemMetadata) {
		if metadata.AList == nil {
			return
		}
		metadata.AList.TaskID = defaultString(strings.TrimSpace(taskInfo.ID), metadata.AList.TaskID)
		metadata.AList.TaskStatus = service.normalizeAListTaskStatus(taskInfo)
		metadata.AList.TaskState = taskInfo.State
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
			metadata.EngineLabel = "网络存储下载"
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

func (service *Service) resolveAListTask(
	ctx context.Context,
	runtime *sidecaralist.Runtime,
	taskID string,
	taskKind string,
) (sidecaralist.TaskInfo, bool, error) {
	switch strings.ToLower(strings.TrimSpace(taskKind)) {
	case "", "copy":
		return service.resolveAListCopyTask(ctx, runtime, taskID)
	case "upload":
		return service.resolveAListUploadTask(ctx, runtime, taskID)
	default:
		return sidecaralist.TaskInfo{}, true, nil
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

func (service *Service) resolveAListUploadTask(ctx context.Context, runtime *sidecaralist.Runtime, taskID string) (sidecaralist.TaskInfo, bool, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return sidecaralist.TaskInfo{}, true, nil
	}

	taskInfo, err := runtime.GetUploadTask(ctx, taskID)
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

func (service *Service) cancelAListTask(ctx context.Context, runtime *sidecaralist.Runtime, taskID string, taskKind string) error {
	switch strings.ToLower(strings.TrimSpace(taskKind)) {
	case "upload":
		return runtime.CancelUploadTask(ctx, taskID)
	default:
		return runtime.CancelCopyTask(ctx, taskID)
	}
}

func (service *Service) deleteAListTask(ctx context.Context, runtime *sidecaralist.Runtime, taskID string, taskKind string) error {
	switch strings.ToLower(strings.TrimSpace(taskKind)) {
	case "upload":
		return runtime.DeleteUploadTask(ctx, taskID)
	default:
		return nil
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

func (service *Service) ensureAListDirectoryTree(ctx context.Context, runtime *sidecaralist.Runtime, targetDir string, storageRoot string) error {
	targetDir = normalizeAListEndpointPath(targetDir)
	if targetDir == "" || targetDir == "/" {
		return nil
	}

	floor := normalizeAListEndpointPath(storageRoot)
	current := ""
	relative := strings.TrimPrefix(targetDir, "/")
	if floor != "" && floor != "/" {
		if targetDir == floor {
			return nil
		}
		if targetDir != floor && !strings.HasPrefix(targetDir, floor+"/") {
			return fmt.Errorf("alist target dir %q escapes storage root %q", targetDir, floor)
		}
		current = floor
		relative = strings.TrimPrefix(strings.TrimPrefix(targetDir, floor), "/")
	}

	segments := strings.Split(relative, "/")
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

func (service *Service) cleanupAListTargetResidue(ctx context.Context, item store.TransferTaskItem) {
	metadata := readTransferItemMetadata(item)
	if metadata.AList == nil {
		return
	}

	targetDir := strings.TrimSpace(metadata.AList.TargetDir)
	names := uniqueStrings(metadata.AList.Names)
	if strings.TrimSpace(item.TargetPath) != "" {
		names = append(names, path.Base(item.TargetPath))
		names = uniqueStrings(names)
	}
	if targetDir == "" || len(names) == 0 {
		return
	}

	_ = service.getAListRuntime().RemoveEntry(ctx, targetDir, names)
}

func flattenLinkHeaders(values map[string][]string) []string {
	headers := make([]string, 0, len(values))
	for key, items := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		for _, item := range items {
			trimmed := strings.TrimSpace(item)
			if trimmed == "" {
				continue
			}
			headers = append(headers, fmt.Sprintf("%s: %s", key, trimmed))
		}
	}
	return headers
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
