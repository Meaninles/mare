package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mam/backend/internal/connectors"
	"mam/backend/internal/store"
)

func TestCommitTransferItemUsesCloud115SessionAndResumesExistingProgress(t *testing.T) {
	ctx := context.Background()
	dataStore := newTestStore(t)
	workspaceRoot := t.TempDir()
	fakeClient := &fakeCloud115UploadClient{
		openSession: &connectors.Cloud115UploadSession{
			UploadID: "upload-115",
			Progress: &connectors.Cloud115UploadProgress{
				FileSize:      4096,
				UploadedBytes: 1024,
				UploadedParts: 1,
				TotalParts:    4,
			},
		},
		uploadResponses: []*connectors.Cloud115UploadSession{
			{
				UploadID: "upload-115",
				Progress: &connectors.Cloud115UploadProgress{
					FileSize:      4096,
					UploadedBytes: 2048,
					UploadedParts: 2,
					TotalParts:    4,
				},
			},
			{
				UploadID: "upload-115",
				Progress: &connectors.Cloud115UploadProgress{
					FileSize:      4096,
					UploadedBytes: 3072,
					UploadedParts: 3,
					TotalParts:    4,
				},
			},
			{
				UploadID: "upload-115",
				Progress: &connectors.Cloud115UploadProgress{
					FileSize:      4096,
					UploadedBytes: 4096,
					UploadedParts: 4,
					TotalParts:    4,
					Completed:     true,
				},
			},
		},
		completeSession: &connectors.Cloud115UploadSession{
			UploadID:  "upload-115",
			Completed: true,
			Entry: &connectors.FileEntry{
				Path: "projects/clip.bin",
				Name: "clip.bin",
				Kind: connectors.EntryKindFile,
				Size: 4096,
			},
		},
	}

	service := NewService(
		dataStore,
		nil,
		MediaConfig{CacheRoot: filepath.Join(workspaceRoot, "cache", "media")},
		WithAutoQueueDerivedMedia(false),
		WithAutoQueueSearchJobs(false),
		WithCloud115UploadFactory(func(context.Context, store.StorageEndpoint) (cloud115UploadClient, cloud115UploadTarget, error) {
			return fakeClient, cloud115UploadTarget{RootID: "0", AppType: "wechatmini"}, nil
		}),
	)

	now := time.Now().UTC().Round(time.Second)
	targetEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-115-target",
		Name:               "115 网盘",
		EndpointType:       string(connectors.EndpointTypeNetwork),
		RootPath:           "/",
		RoleMode:           "MANAGED",
		IdentitySignature:  "network-115-target",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig: mustJSONText(t, map[string]any{
			"provider":     "115",
			"rootFolderId": "0",
			"appType":      "wechatmini",
		}),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := dataStore.CreateStorageEndpoint(ctx, targetEndpoint); err != nil {
		t.Fatalf("create target endpoint: %v", err)
	}

	task := store.Task{
		ID:        "task-cloud115-commit",
		TaskType:  taskTypeRestoreAsset,
		Status:    taskStatusRunning,
		Payload:   mustJSONText(t, map[string]any{"assetId": "asset-115"}),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := dataStore.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	item := newTransferTaskItem(0)
	item.TaskID = task.ID
	item.GroupKey = "asset-115"
	item.Direction = transferDirectionUpload
	item.DisplayName = "clip.bin"
	item.SourceKind = transferSourceKindEndpoint
	item.SourceLabel = "本地"
	item.SourcePath = "projects/clip.bin"
	item.TargetEndpointID = targetEndpoint.ID
	item.TargetEndpointType = targetEndpoint.EndpointType
	item.TargetLabel = targetEndpoint.Name
	item.TargetPath = "projects/clip.bin"
	item.Status = taskStatusRunning
	item.Phase = transferPhaseCommitting
	item.TotalBytes = 4096
	item.StagedBytes = 4096
	item.CreatedAt = now
	item.UpdatedAt = now

	stagingDir := filepath.Join(service.transferStateRoot(), "tasks", item.TaskID, item.ID)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatalf("create staging dir: %v", err)
	}
	item.StagingPath = filepath.Join(stagingDir, "clip.bin")
	if err := os.WriteFile(item.StagingPath, make([]byte, 4096), 0o644); err != nil {
		t.Fatalf("write staging file: %v", err)
	}

	if err := dataStore.CreateTransferTaskItems(ctx, []store.TransferTaskItem{item}); err != nil {
		t.Fatalf("create transfer item: %v", err)
	}

	entry, err := service.commitTransferItem(ctx, task.ID, &item)
	if err != nil {
		t.Fatalf("commit transfer item: %v", err)
	}

	if entry.Size != 4096 || entry.Path != "projects/clip.bin" {
		t.Fatalf("unexpected completed entry: %+v", entry)
	}
	if item.CommittedBytes != 4096 {
		t.Fatalf("expected committed bytes 4096, got %d", item.CommittedBytes)
	}

	metadata := readTransferItemMetadata(item)
	if metadata.EngineKind != transferEngineKindCloud115 {
		t.Fatalf("expected engine kind %s, got %s", transferEngineKindCloud115, metadata.EngineKind)
	}
	if metadata.Cloud115 == nil {
		t.Fatal("expected cloud115 metadata")
	}
	if metadata.Cloud115.UploadID != "upload-115" {
		t.Fatalf("expected upload id upload-115, got %q", metadata.Cloud115.UploadID)
	}
	if metadata.Cloud115.UploadedBytes != 4096 {
		t.Fatalf("expected uploaded bytes 4096, got %d", metadata.Cloud115.UploadedBytes)
	}
	if metadata.Cloud115.UploadedParts != 4 || metadata.Cloud115.TotalParts != 4 {
		t.Fatalf("expected uploaded parts 4/4, got %d/%d", metadata.Cloud115.UploadedParts, metadata.Cloud115.TotalParts)
	}
	if metadata.Cloud115.Status != "complete" {
		t.Fatalf("expected cloud115 status complete, got %s", metadata.Cloud115.Status)
	}
	if filepath.Base(metadata.Cloud115.ResumeStatePath) != cloud115TransferSessionFileName {
		t.Fatalf("expected resume state path to end with %s, got %s", cloud115TransferSessionFileName, metadata.Cloud115.ResumeStatePath)
	}

	if len(fakeClient.uploadRequests) != 3 {
		t.Fatalf("expected 3 upload calls after resuming from first part, got %d", len(fakeClient.uploadRequests))
	}
	if len(fakeClient.completeRequests) != 1 {
		t.Fatalf("expected 1 complete call, got %d", len(fakeClient.completeRequests))
	}
}

func TestNormalizeInterruptedTransferTasksReconcilesCloud115CommittedBytes(t *testing.T) {
	ctx := context.Background()
	dataStore := newTestStore(t)
	workspaceRoot := t.TempDir()

	fakeClient := &fakeCloud115UploadClient{
		listSession: &connectors.Cloud115UploadSession{
			UploadID: "upload-115-reconcile",
			Progress: &connectors.Cloud115UploadProgress{
				FileSize:      4096,
				UploadedBytes: 3072,
				UploadedParts: 3,
				TotalParts:    4,
			},
		},
	}

	service := NewService(
		dataStore,
		nil,
		MediaConfig{CacheRoot: filepath.Join(workspaceRoot, "cache", "media")},
		WithAutoQueueDerivedMedia(false),
		WithAutoQueueSearchJobs(false),
		WithCloud115UploadFactory(func(context.Context, store.StorageEndpoint) (cloud115UploadClient, cloud115UploadTarget, error) {
			return fakeClient, cloud115UploadTarget{RootID: "0", AppType: "wechatmini"}, nil
		}),
	)

	now := time.Now().UTC().Round(time.Second)
	targetEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-115-reconcile",
		Name:               "115 网盘",
		EndpointType:       string(connectors.EndpointTypeNetwork),
		RootPath:           "/",
		RoleMode:           "MANAGED",
		IdentitySignature:  "network-115-reconcile",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig: mustJSONText(t, map[string]any{
			"provider":     "115",
			"rootFolderId": "0",
			"appType":      "wechatmini",
		}),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := dataStore.CreateStorageEndpoint(ctx, targetEndpoint); err != nil {
		t.Fatalf("create target endpoint: %v", err)
	}

	task := store.Task{
		ID:        "task-cloud115-reconcile",
		TaskType:  taskTypeRestoreAsset,
		Status:    taskStatusRunning,
		Payload:   mustJSONText(t, map[string]any{"assetId": "asset-115"}),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := dataStore.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	item := newTransferTaskItem(0)
	item.TaskID = task.ID
	item.GroupKey = "asset-115"
	item.Direction = transferDirectionUpload
	item.DisplayName = "clip.bin"
	item.SourceKind = transferSourceKindEndpoint
	item.SourceLabel = "本地"
	item.SourcePath = "projects/clip.bin"
	item.TargetEndpointID = targetEndpoint.ID
	item.TargetEndpointType = targetEndpoint.EndpointType
	item.TargetLabel = targetEndpoint.Name
	item.TargetPath = "projects/clip.bin"
	item.Status = taskStatusRunning
	item.Phase = transferPhaseCommitting
	item.TotalBytes = 4096
	item.StagedBytes = 4096
	item.CommittedBytes = 1024
	item.ProgressPercent = 76
	item.CreatedAt = now
	item.UpdatedAt = now
	item.MetadataJSON = mustJSONText(t, transferItemMetadata{
		EngineKind:   transferEngineKindCloud115,
		EngineLabel:  cloud115TransferEngineLabel,
		CurrentSpeed: 8192,
		Cloud115: &transferItemCloud115Metadata{
			UploadID:        "upload-115-reconcile",
			ResumeStatePath: service.cloud115TransferResumeStatePath(item),
			RootID:          "0",
			AppType:         "wechatmini",
			Status:          "uploading",
			UploadedBytes:   1024,
			UploadedParts:   1,
			TotalParts:      4,
		},
	})

	stagingDir := filepath.Join(service.transferStateRoot(), "tasks", item.TaskID, item.ID)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatalf("create staging dir: %v", err)
	}
	item.StagingPath = filepath.Join(stagingDir, "clip.bin")
	if err := os.WriteFile(item.StagingPath, make([]byte, 4096), 0o644); err != nil {
		t.Fatalf("write staging file: %v", err)
	}
	if err := os.WriteFile(service.cloud115TransferResumeStatePath(item), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write resume state file: %v", err)
	}

	if err := dataStore.CreateTransferTaskItems(ctx, []store.TransferTaskItem{item}); err != nil {
		t.Fatalf("create transfer item: %v", err)
	}

	if err := service.normalizeInterruptedTransferTasks(ctx); err != nil {
		t.Fatalf("normalize interrupted transfers: %v", err)
	}

	storedTask, err := dataStore.GetTaskByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task after normalize: %v", err)
	}
	if storedTask.Status != taskStatusQueued {
		t.Fatalf("expected task status queued, got %s", storedTask.Status)
	}

	items, err := dataStore.ListTransferTaskItemsByTaskID(ctx, task.ID)
	if err != nil {
		t.Fatalf("list items after normalize: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 transfer item, got %d", len(items))
	}

	normalized := items[0]
	if normalized.Status != taskStatusQueued {
		t.Fatalf("expected item status queued, got %s", normalized.Status)
	}
	if normalized.Phase != transferPhaseCommitting {
		t.Fatalf("expected item phase committing, got %s", normalized.Phase)
	}
	if normalized.CommittedBytes != 3072 {
		t.Fatalf("expected committed bytes 3072, got %d", normalized.CommittedBytes)
	}

	metadata := readTransferItemMetadata(normalized)
	if metadata.CurrentSpeed != 0 {
		t.Fatalf("expected current speed reset to 0, got %d", metadata.CurrentSpeed)
	}
	if metadata.Cloud115 == nil {
		t.Fatal("expected cloud115 metadata after normalize")
	}
	if metadata.Cloud115.UploadID != "upload-115-reconcile" {
		t.Fatalf("expected upload id preserved, got %q", metadata.Cloud115.UploadID)
	}
	if metadata.Cloud115.UploadedBytes != 3072 {
		t.Fatalf("expected uploaded bytes 3072, got %d", metadata.Cloud115.UploadedBytes)
	}
	if metadata.Cloud115.UploadedParts != 3 || metadata.Cloud115.TotalParts != 4 {
		t.Fatalf("expected uploaded parts 3/4, got %d/%d", metadata.Cloud115.UploadedParts, metadata.Cloud115.TotalParts)
	}
	if len(fakeClient.listRequests) != 1 {
		t.Fatalf("expected 1 list parts call, got %d", len(fakeClient.listRequests))
	}
}

func TestResumeTransferTasksPreservesCloud115CommittedBytes(t *testing.T) {
	ctx := context.Background()
	dataStore := newTestStore(t)

	service := NewService(
		dataStore,
		nil,
		WithAutoQueueDerivedMedia(false),
		WithAutoQueueSearchJobs(false),
	)

	now := time.Now().UTC().Round(time.Second)
	task := store.Task{
		ID:        "task-cloud115-resume",
		TaskType:  taskTypeRestoreAsset,
		Status:    taskStatusFailed,
		Payload:   mustJSONText(t, map[string]any{"assetId": "asset-115"}),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := dataStore.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	item := newTransferTaskItem(0)
	item.TaskID = task.ID
	item.GroupKey = "asset-115"
	item.Direction = transferDirectionUpload
	item.DisplayName = "clip.bin"
	item.SourceKind = transferSourceKindEndpoint
	item.SourceLabel = "本地"
	item.SourcePath = "projects/clip.bin"
	item.TargetEndpointID = "endpoint-115-resume"
	item.TargetEndpointType = string(connectors.EndpointTypeNetwork)
	item.TargetLabel = "115 网盘"
	item.TargetPath = "projects/clip.bin"
	item.Status = taskStatusFailed
	item.Phase = transferPhaseFailed
	item.TotalBytes = 4096
	item.StagedBytes = 4096
	item.CommittedBytes = 2048
	item.ProgressPercent = 82
	item.CreatedAt = now
	item.UpdatedAt = now
	item.MetadataJSON = mustJSONText(t, transferItemMetadata{
		EngineKind:   transferEngineKindCloud115,
		EngineLabel:  cloud115TransferEngineLabel,
		CurrentSpeed: 1024,
		Cloud115: &transferItemCloud115Metadata{
			UploadID:      "upload-115-resume",
			RootID:        "0",
			AppType:       "wechatmini",
			Status:        "failed",
			UploadedBytes: 2048,
			UploadedParts: 2,
			TotalParts:    4,
		},
	})
	if err := dataStore.CreateTransferTaskItems(ctx, []store.TransferTaskItem{item}); err != nil {
		t.Fatalf("create transfer item: %v", err)
	}

	summary, err := service.ResumeTransferTasks(ctx, []string{task.ID})
	if err != nil {
		t.Fatalf("resume transfer task: %v", err)
	}
	if summary.Updated != 1 {
		t.Fatalf("expected 1 resumed task, got %d", summary.Updated)
	}

	items, err := dataStore.ListTransferTaskItemsByTaskID(ctx, task.ID)
	if err != nil {
		t.Fatalf("list resumed items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 resumed item, got %d", len(items))
	}

	resumed := items[0]
	if resumed.CommittedBytes != 2048 {
		t.Fatalf("expected committed bytes preserved, got %d", resumed.CommittedBytes)
	}
	if resumed.Phase != transferPhaseCommitting {
		t.Fatalf("expected resumed phase committing, got %s", resumed.Phase)
	}

	metadata := readTransferItemMetadata(resumed)
	if metadata.CurrentSpeed != 0 {
		t.Fatalf("expected resumed current speed 0, got %d", metadata.CurrentSpeed)
	}
	if metadata.Cloud115 == nil {
		t.Fatal("expected resumed cloud115 metadata")
	}
	if metadata.Cloud115.Status != "queued" {
		t.Fatalf("expected resumed cloud115 status queued, got %s", metadata.Cloud115.Status)
	}
}

func TestNormalizeInterruptedTransferTasksUsesRemoteCloud115ProgressAsSourceOfTruth(t *testing.T) {
	ctx := context.Background()
	dataStore := newTestStore(t)
	workspaceRoot := t.TempDir()

	fakeClient := &fakeCloud115UploadClient{
		listSession: &connectors.Cloud115UploadSession{
			UploadID: "upload-115-remote-truth",
			Progress: &connectors.Cloud115UploadProgress{
				FileSize:      4096,
				UploadedBytes: 2048,
				UploadedParts: 2,
				TotalParts:    4,
			},
		},
	}

	service := NewService(
		dataStore,
		nil,
		MediaConfig{CacheRoot: filepath.Join(workspaceRoot, "cache", "media")},
		WithAutoQueueDerivedMedia(false),
		WithAutoQueueSearchJobs(false),
		WithCloud115UploadFactory(func(context.Context, store.StorageEndpoint) (cloud115UploadClient, cloud115UploadTarget, error) {
			return fakeClient, cloud115UploadTarget{RootID: "0", AppType: "wechatmini"}, nil
		}),
	)

	now := time.Now().UTC().Round(time.Second)
	targetEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-115-remote-truth",
		Name:               "115 缃戠洏",
		EndpointType:       string(connectors.EndpointTypeNetwork),
		RootPath:           "/",
		RoleMode:           "MANAGED",
		IdentitySignature:  "network-115-remote-truth",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig: mustJSONText(t, map[string]any{
			"provider":     "115",
			"rootFolderId": "0",
			"appType":      "wechatmini",
		}),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := dataStore.CreateStorageEndpoint(ctx, targetEndpoint); err != nil {
		t.Fatalf("create target endpoint: %v", err)
	}

	task := store.Task{
		ID:        "task-cloud115-remote-truth",
		TaskType:  taskTypeRestoreAsset,
		Status:    taskStatusRunning,
		Payload:   mustJSONText(t, map[string]any{"assetId": "asset-115"}),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := dataStore.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	item := newTransferTaskItem(0)
	item.TaskID = task.ID
	item.GroupKey = "asset-115"
	item.Direction = transferDirectionUpload
	item.DisplayName = "clip.bin"
	item.SourceKind = transferSourceKindEndpoint
	item.SourceLabel = "鏈湴"
	item.SourcePath = "projects/clip.bin"
	item.TargetEndpointID = targetEndpoint.ID
	item.TargetEndpointType = targetEndpoint.EndpointType
	item.TargetLabel = targetEndpoint.Name
	item.TargetPath = "projects/clip.bin"
	item.Status = taskStatusRunning
	item.Phase = transferPhaseCommitting
	item.TotalBytes = 4096
	item.StagedBytes = 4096
	item.CommittedBytes = 3072
	item.ProgressPercent = 88
	item.CreatedAt = now
	item.UpdatedAt = now
	item.MetadataJSON = mustJSONText(t, transferItemMetadata{
		EngineKind:  transferEngineKindCloud115,
		EngineLabel: cloud115TransferEngineLabel,
		Cloud115: &transferItemCloud115Metadata{
			UploadID:        "upload-115-remote-truth",
			ResumeStatePath: service.cloud115TransferResumeStatePath(item),
			RootID:          "0",
			AppType:         "wechatmini",
			Status:          "uploading",
			UploadedBytes:   3072,
			UploadedParts:   3,
			TotalParts:      4,
			ParentID:        23,
			FileName:        "clip.bin",
			PartSize:        1024,
			SessionURL:      "https://example.invalid/upload",
			SessionCallback: json.RawMessage(`{"token":"callback"}`),
		},
	})

	stagingDir := filepath.Join(service.transferStateRoot(), "tasks", item.TaskID, item.ID)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatalf("create staging dir: %v", err)
	}
	item.StagingPath = filepath.Join(stagingDir, "clip.bin")
	if err := os.WriteFile(item.StagingPath, make([]byte, 4096), 0o644); err != nil {
		t.Fatalf("write staging file: %v", err)
	}

	if err := dataStore.CreateTransferTaskItems(ctx, []store.TransferTaskItem{item}); err != nil {
		t.Fatalf("create transfer item: %v", err)
	}

	if err := service.normalizeInterruptedTransferTasks(ctx); err != nil {
		t.Fatalf("normalize interrupted transfers: %v", err)
	}

	items, err := dataStore.ListTransferTaskItemsByTaskID(ctx, task.ID)
	if err != nil {
		t.Fatalf("list items after normalize: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 transfer item, got %d", len(items))
	}
	if items[0].CommittedBytes != 2048 {
		t.Fatalf("expected committed bytes corrected to remote 2048, got %d", items[0].CommittedBytes)
	}
	metadata := readTransferItemMetadata(items[0])
	if metadata.Cloud115 == nil {
		t.Fatal("expected cloud115 metadata after normalize")
	}
	if metadata.Cloud115.UploadedBytes != 2048 || metadata.Cloud115.UploadedParts != 2 {
		t.Fatalf("expected cloud115 metadata corrected to 2048 bytes / 2 parts, got %d / %d", metadata.Cloud115.UploadedBytes, metadata.Cloud115.UploadedParts)
	}
}

func TestCommitTransferItemReconcilesCloud115ProgressAfterUploadError(t *testing.T) {
	ctx := context.Background()
	dataStore := newTestStore(t)
	workspaceRoot := t.TempDir()
	fakeClient := &fakeCloud115UploadClient{
		openSession: &connectors.Cloud115UploadSession{
			UploadID:  "upload-115-reconcile-error",
			UploadURL: "https://example.invalid/upload",
			Callback:  json.RawMessage(`{"token":"callback"}`),
			ParentID:  23,
			FileName:  "clip.bin",
			PartSize:  1024,
			Progress: &connectors.Cloud115UploadProgress{
				FileSize:      4096,
				UploadedBytes: 1024,
				UploadedParts: 1,
				TotalParts:    4,
			},
		},
		listSession: &connectors.Cloud115UploadSession{
			UploadID:  "upload-115-reconcile-error",
			UploadURL: "https://example.invalid/upload",
			Callback:  json.RawMessage(`{"token":"callback"}`),
			ParentID:  23,
			FileName:  "clip.bin",
			PartSize:  1024,
			Progress: &connectors.Cloud115UploadProgress{
				FileSize:      4096,
				UploadedBytes: 2048,
				UploadedParts: 2,
				TotalParts:    4,
			},
		},
		uploadOutcomes: []fakeCloud115UploadOutcome{
			{
				err: &connectors.ConnectorError{
					Code:      connectors.ErrorCodeUnavailable,
					Connector: connectors.EndpointTypeNetwork,
					Operation: "upload_session_upload_parts",
					Message:   "temporary timeout",
					Temporary: true,
				},
			},
			{
				session: &connectors.Cloud115UploadSession{
					UploadID: "upload-115-reconcile-error",
					Progress: &connectors.Cloud115UploadProgress{
						FileSize:      4096,
						UploadedBytes: 3072,
						UploadedParts: 3,
						TotalParts:    4,
					},
				},
			},
			{
				session: &connectors.Cloud115UploadSession{
					UploadID: "upload-115-reconcile-error",
					Progress: &connectors.Cloud115UploadProgress{
						FileSize:      4096,
						UploadedBytes: 4096,
						UploadedParts: 4,
						TotalParts:    4,
						Completed:     true,
					},
				},
			},
		},
		completeSession: &connectors.Cloud115UploadSession{
			UploadID:  "upload-115-reconcile-error",
			Completed: true,
			Entry: &connectors.FileEntry{
				Path: "projects/clip.bin",
				Name: "clip.bin",
				Kind: connectors.EntryKindFile,
				Size: 4096,
			},
		},
	}

	service := NewService(
		dataStore,
		nil,
		MediaConfig{CacheRoot: filepath.Join(workspaceRoot, "cache", "media")},
		WithAutoQueueDerivedMedia(false),
		WithAutoQueueSearchJobs(false),
		WithCloud115UploadFactory(func(context.Context, store.StorageEndpoint) (cloud115UploadClient, cloud115UploadTarget, error) {
			return fakeClient, cloud115UploadTarget{RootID: "0", AppType: "wechatmini"}, nil
		}),
	)

	now := time.Now().UTC().Round(time.Second)
	targetEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-115-reconcile-error",
		Name:               "115 缃戠洏",
		EndpointType:       string(connectors.EndpointTypeNetwork),
		RootPath:           "/",
		RoleMode:           "MANAGED",
		IdentitySignature:  "network-115-reconcile-error",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig: mustJSONText(t, map[string]any{
			"provider":     "115",
			"rootFolderId": "0",
			"appType":      "wechatmini",
		}),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := dataStore.CreateStorageEndpoint(ctx, targetEndpoint); err != nil {
		t.Fatalf("create target endpoint: %v", err)
	}

	task := store.Task{
		ID:        "task-cloud115-reconcile-error",
		TaskType:  taskTypeRestoreAsset,
		Status:    taskStatusRunning,
		Payload:   mustJSONText(t, map[string]any{"assetId": "asset-115"}),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := dataStore.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	item := newTransferTaskItem(0)
	item.TaskID = task.ID
	item.GroupKey = "asset-115"
	item.Direction = transferDirectionUpload
	item.DisplayName = "clip.bin"
	item.SourceKind = transferSourceKindEndpoint
	item.SourceLabel = "鏈湴"
	item.SourcePath = "projects/clip.bin"
	item.TargetEndpointID = targetEndpoint.ID
	item.TargetEndpointType = targetEndpoint.EndpointType
	item.TargetLabel = targetEndpoint.Name
	item.TargetPath = "projects/clip.bin"
	item.Status = taskStatusRunning
	item.Phase = transferPhaseCommitting
	item.TotalBytes = 4096
	item.StagedBytes = 4096
	item.CreatedAt = now
	item.UpdatedAt = now

	stagingDir := filepath.Join(service.transferStateRoot(), "tasks", item.TaskID, item.ID)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatalf("create staging dir: %v", err)
	}
	item.StagingPath = filepath.Join(stagingDir, "clip.bin")
	if err := os.WriteFile(item.StagingPath, make([]byte, 4096), 0o644); err != nil {
		t.Fatalf("write staging file: %v", err)
	}

	if err := dataStore.CreateTransferTaskItems(ctx, []store.TransferTaskItem{item}); err != nil {
		t.Fatalf("create transfer item: %v", err)
	}

	entry, err := service.commitTransferItem(ctx, task.ID, &item)
	if err != nil {
		t.Fatalf("commit transfer item: %v", err)
	}
	if entry.Size != 4096 {
		t.Fatalf("expected completed entry size 4096, got %d", entry.Size)
	}
	if item.CommittedBytes != 4096 {
		t.Fatalf("expected committed bytes 4096, got %d", item.CommittedBytes)
	}
	if len(fakeClient.listRequests) == 0 {
		t.Fatal("expected upload error recovery to reconcile remote parts via list")
	}
}

type fakeCloud115UploadClient struct {
	openSession     *connectors.Cloud115UploadSession
	listSession     *connectors.Cloud115UploadSession
	uploadResponses []*connectors.Cloud115UploadSession
	completeSession *connectors.Cloud115UploadSession
	statEntry       connectors.FileEntry
	openErr         error
	completeErr     error
	statErr         error
	uploadOutcomes  []fakeCloud115UploadOutcome

	openRequests     []connectors.Cloud115UploadSessionRequest
	listRequests     []connectors.Cloud115UploadSessionRequest
	uploadRequests   []connectors.Cloud115UploadSessionRequest
	completeRequests []connectors.Cloud115UploadSessionRequest
	abortRequests    []connectors.Cloud115UploadSessionRequest
	statRequests     []fakeCloud115StatRequest

	uploadIndex int
}

type fakeCloud115UploadOutcome struct {
	session *connectors.Cloud115UploadSession
	err     error
}

type fakeCloud115StatRequest struct {
	rootID string
	path   string
}

func (client *fakeCloud115UploadClient) OpenUploadSession(
	_ context.Context,
	request connectors.Cloud115UploadSessionRequest,
) (*connectors.Cloud115UploadSession, error) {
	client.openRequests = append(client.openRequests, request)
	if client.openErr != nil {
		return nil, client.openErr
	}
	if client.openSession == nil {
		return nil, fmt.Errorf("unexpected open upload session call")
	}
	return client.openSession, nil
}

func (client *fakeCloud115UploadClient) ListUploadSessionParts(
	_ context.Context,
	request connectors.Cloud115UploadSessionRequest,
) (*connectors.Cloud115UploadSession, error) {
	client.listRequests = append(client.listRequests, request)
	if client.listSession == nil {
		return nil, fmt.Errorf("unexpected list upload session parts call")
	}
	return client.listSession, nil
}

func (client *fakeCloud115UploadClient) UploadSessionParts(
	_ context.Context,
	request connectors.Cloud115UploadSessionRequest,
) (*connectors.Cloud115UploadSession, error) {
	client.uploadRequests = append(client.uploadRequests, request)
	if client.uploadIndex < len(client.uploadOutcomes) {
		outcome := client.uploadOutcomes[client.uploadIndex]
		client.uploadIndex++
		if outcome.err != nil {
			return nil, outcome.err
		}
		if outcome.session == nil {
			return nil, fmt.Errorf("unexpected nil upload session outcome #%d", client.uploadIndex)
		}
		return outcome.session, nil
	}
	if client.uploadIndex >= len(client.uploadResponses) {
		return nil, fmt.Errorf("unexpected upload session parts call #%d", client.uploadIndex+1)
	}
	response := client.uploadResponses[client.uploadIndex]
	client.uploadIndex++
	return response, nil
}

func (client *fakeCloud115UploadClient) CompleteUploadSession(
	_ context.Context,
	request connectors.Cloud115UploadSessionRequest,
) (*connectors.Cloud115UploadSession, error) {
	client.completeRequests = append(client.completeRequests, request)
	if client.completeErr != nil {
		return nil, client.completeErr
	}
	if client.completeSession == nil {
		return nil, fmt.Errorf("unexpected complete upload session call")
	}
	return client.completeSession, nil
}

func (client *fakeCloud115UploadClient) AbortUploadSession(
	_ context.Context,
	request connectors.Cloud115UploadSessionRequest,
) (*connectors.Cloud115UploadSession, error) {
	client.abortRequests = append(client.abortRequests, request)
	return &connectors.Cloud115UploadSession{StateDeleted: true}, nil
}

func (client *fakeCloud115UploadClient) StatEntry(
	_ context.Context,
	rootID string,
	path string,
) (connectors.FileEntry, error) {
	client.statRequests = append(client.statRequests, fakeCloud115StatRequest{rootID: rootID, path: path})
	if client.statErr != nil {
		return connectors.FileEntry{}, client.statErr
	}
	if client.statEntry.Path == "" && client.statEntry.Size == 0 {
		return connectors.FileEntry{}, fmt.Errorf("unexpected stat entry call")
	}
	return client.statEntry, nil
}
