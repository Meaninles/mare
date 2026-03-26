package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mam/backend/internal/connectors"
	"mam/backend/internal/store"
)

func TestResumeTransferTasksRequeuesFailedNetworkStorageItem(t *testing.T) {
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
		ID:        "task-resume-failed-network-storage",
		TaskType:  taskTypeRestoreAsset,
		Status:    taskStatusFailed,
		Payload:   mustJSONText(t, map[string]any{"assetId": "asset-1"}),
		CreatedAt: now,
		UpdatedAt: now.Add(2 * time.Minute),
	}
	if err := dataStore.CreateTask(ctx, task); err != nil {
		t.Fatalf("create transfer task: %v", err)
	}

	item := newTransferTaskItem(0)
	item.TaskID = task.ID
	item.GroupKey = "asset-1"
	item.Direction = transferDirectionUpload
	item.DisplayName = "clip.bin"
	item.SourceKind = transferSourceKindEndpoint
	item.SourceLabel = "本地"
	item.SourcePath = "projects/clip.bin"
	item.TargetEndpointID = "endpoint-network-target"
	item.TargetEndpointType = string(connectors.EndpointTypeNetwork)
	item.TargetLabel = "网络存储"
	item.TargetPath = "projects/clip.bin"
	item.Status = taskStatusFailed
	item.Phase = transferPhaseFailed
	item.TotalBytes = 100
	item.StagedBytes = 100
	item.CommittedBytes = 45
	item.ProgressPercent = 81
	item.CreatedAt = now
	item.UpdatedAt = now.Add(2 * time.Minute)
	finishedAt := now.Add(3 * time.Minute)
	item.FinishedAt = &finishedAt
	errorMessage := "alist copy failed: token invalid"
	item.ErrorMessage = &errorMessage
	item.MetadataJSON = mustJSONText(t, transferItemMetadata{
		CurrentSpeed: 2048,
		AList: &transferItemAListMetadata{
			TaskID:     "alist-copy-1",
			TaskStatus: "failed",
			TaskState:  7,
			TargetDir:  "/upload/projects",
			Names:      []string{"clip.bin"},
		},
	})
	if err := dataStore.CreateTransferTaskItems(ctx, []store.TransferTaskItem{item}); err != nil {
		t.Fatalf("create transfer task item: %v", err)
	}

	summary, err := service.ResumeTransferTasks(ctx, []string{task.ID})
	if err != nil {
		t.Fatalf("resume failed transfer task: %v", err)
	}
	if summary.Updated != 1 {
		t.Fatalf("expected 1 resumed task, got %d", summary.Updated)
	}

	storedItems, err := dataStore.ListTransferTaskItemsByTaskID(ctx, task.ID)
	if err != nil {
		t.Fatalf("list resumed transfer items: %v", err)
	}
	if len(storedItems) != 1 {
		t.Fatalf("expected 1 resumed transfer item, got %d", len(storedItems))
	}

	resumed := storedItems[0]
	if resumed.Status != taskStatusQueued {
		t.Fatalf("expected resumed item status %s, got %s", taskStatusQueued, resumed.Status)
	}
	if resumed.Phase != transferPhaseCommitting {
		t.Fatalf("expected resumed item phase %s, got %s", transferPhaseCommitting, resumed.Phase)
	}
	if resumed.CommittedBytes != 0 {
		t.Fatalf("expected resumed item committed bytes reset to 0, got %d", resumed.CommittedBytes)
	}
	if resumed.ProgressPercent != 70 {
		t.Fatalf("expected resumed item progress 70 after resetting commit state, got %d", resumed.ProgressPercent)
	}
	if resumed.ErrorMessage != nil {
		t.Fatalf("expected resumed item error cleared, got %q", *resumed.ErrorMessage)
	}
	if resumed.FinishedAt != nil {
		t.Fatalf("expected resumed item finished_at cleared, got %v", resumed.FinishedAt)
	}

	metadata := parseTransferItemMetadata(resumed.MetadataJSON)
	if metadata.CurrentSpeed != 0 {
		t.Fatalf("expected resumed item speed reset to 0, got %d", metadata.CurrentSpeed)
	}
	if metadata.AList == nil {
		t.Fatal("expected resumed item to preserve AList metadata envelope")
	}
	if metadata.AList.TaskID != "" {
		t.Fatalf("expected resumed item AList task id cleared, got %q", metadata.AList.TaskID)
	}
	if metadata.AList.TaskStatus != "" {
		t.Fatalf("expected resumed item AList task status cleared, got %q", metadata.AList.TaskStatus)
	}
	if metadata.AList.TaskState != 0 {
		t.Fatalf("expected resumed item AList task state reset to 0, got %d", metadata.AList.TaskState)
	}

	detail, err := service.GetTransferTaskDetail(ctx, task.ID)
	if err != nil {
		t.Fatalf("load resumed transfer detail: %v", err)
	}
	if detail.Task.Status != taskStatusQueued {
		t.Fatalf("expected resumed task status %s, got %s", taskStatusQueued, detail.Task.Status)
	}
	if len(detail.Items) != 1 {
		t.Fatalf("expected 1 transfer detail item, got %d", len(detail.Items))
	}
	if detail.Items[0].Status != taskStatusQueued {
		t.Fatalf("expected transfer detail item status %s, got %s", taskStatusQueued, detail.Items[0].Status)
	}
	if detail.Items[0].ExternalTaskID != "" || detail.Items[0].ExternalStatus != "" {
		t.Fatalf("expected stale external AList task state cleared, got id=%q status=%q", detail.Items[0].ExternalTaskID, detail.Items[0].ExternalStatus)
	}
}

func TestListTransferTasksSortsQueueForDisplay(t *testing.T) {
	ctx := context.Background()
	dataStore := newTestStore(t)
	service := NewService(
		dataStore,
		nil,
		WithAutoQueueDerivedMedia(false),
		WithAutoQueueSearchJobs(false),
	)

	base := time.Now().UTC().Round(time.Second)
	runningStarted := base.Add(3 * time.Minute)
	successFinished := base.Add(9 * time.Minute)
	tasks := []store.Task{
		{
			ID:        "task-running",
			TaskType:  taskTypeRestoreAsset,
			Status:    taskStatusRunning,
			Priority:  0,
			Payload:   mustJSONText(t, map[string]any{"task": "running"}),
			CreatedAt: base,
			UpdatedAt: base.Add(6 * time.Minute),
			StartedAt: &runningStarted,
		},
		{
			ID:        "task-queued-high",
			TaskType:  taskTypeRestoreAsset,
			Status:    taskStatusQueued,
			Priority:  10,
			Payload:   mustJSONText(t, map[string]any{"task": "queued-high"}),
			CreatedAt: base.Add(1 * time.Minute),
			UpdatedAt: base.Add(1 * time.Minute),
		},
		{
			ID:        "task-queued-low",
			TaskType:  taskTypeRestoreAsset,
			Status:    taskStatusQueued,
			Priority:  1,
			Payload:   mustJSONText(t, map[string]any{"task": "queued-low"}),
			CreatedAt: base.Add(2 * time.Minute),
			UpdatedAt: base.Add(2 * time.Minute),
		},
		{
			ID:        "task-failed",
			TaskType:  taskTypeRestoreAsset,
			Status:    taskStatusFailed,
			Priority:  99,
			Payload:   mustJSONText(t, map[string]any{"task": "failed"}),
			CreatedAt: base.Add(3 * time.Minute),
			UpdatedAt: base.Add(8 * time.Minute),
		},
		{
			ID:         "task-success",
			TaskType:   taskTypeRestoreAsset,
			Status:     taskStatusSuccess,
			Priority:   999,
			Payload:    mustJSONText(t, map[string]any{"task": "success"}),
			CreatedAt:  base.Add(4 * time.Minute),
			UpdatedAt:  base.Add(9 * time.Minute),
			FinishedAt: &successFinished,
		},
	}
	for _, task := range tasks {
		if err := dataStore.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	items := []store.TransferTaskItem{
		buildTestTransferTaskItem(tasks[0], taskStatusRunning, transferDirectionDownload),
		buildTestTransferTaskItem(tasks[1], taskStatusQueued, transferDirectionDownload),
		buildTestTransferTaskItem(tasks[2], taskStatusQueued, transferDirectionDownload),
		buildTestTransferTaskItem(tasks[3], taskStatusFailed, transferDirectionUpload),
		buildTestTransferTaskItem(tasks[4], taskStatusSuccess, transferDirectionUpload),
	}
	if err := dataStore.CreateTransferTaskItems(ctx, items); err != nil {
		t.Fatalf("create transfer task items: %v", err)
	}

	result, err := service.ListTransferTasks(ctx, 10)
	if err != nil {
		t.Fatalf("list transfer tasks: %v", err)
	}
	if len(result.Tasks) != 5 {
		t.Fatalf("expected 5 transfer tasks, got %d", len(result.Tasks))
	}

	expectedOrder := []string{
		"task-running",
		"task-queued-high",
		"task-queued-low",
		"task-failed",
		"task-success",
	}
	for index, expectedID := range expectedOrder {
		if result.Tasks[index].ID != expectedID {
			t.Fatalf("expected task order %v, got [%s %s %s %s %s]",
				expectedOrder,
				result.Tasks[0].ID,
				result.Tasks[1].ID,
				result.Tasks[2].ID,
				result.Tasks[3].ID,
				result.Tasks[4].ID,
			)
		}
	}
}

func TestNormalizeInterruptedTransferTasksRequeuesInterruptedItemFromLocalArtifacts(t *testing.T) {
	ctx := context.Background()
	dataStore := newTestStore(t)
	workspaceRoot := t.TempDir()
	restoreRoot := filepath.Join(workspaceRoot, "restore")
	if err := os.MkdirAll(restoreRoot, 0o755); err != nil {
		t.Fatalf("create restore root: %v", err)
	}

	service := NewService(
		dataStore,
		nil,
		MediaConfig{CacheRoot: filepath.Join(workspaceRoot, "cache", "media")},
		WithAutoQueueDerivedMedia(false),
		WithAutoQueueSearchJobs(false),
	)

	now := time.Now().UTC().Round(time.Second)
	targetEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-local-target",
		Name:               "本地恢复目录",
		EndpointType:       string(connectors.EndpointTypeLocal),
		RootPath:           restoreRoot,
		RoleMode:           "MANAGED",
		IdentitySignature:  "local-target",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   mustJSONText(t, map[string]any{"rootPath": restoreRoot}),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := dataStore.CreateStorageEndpoint(ctx, targetEndpoint); err != nil {
		t.Fatalf("create target endpoint: %v", err)
	}

	task := store.Task{
		ID:        "task-startup-requeue",
		TaskType:  taskTypeRestoreAsset,
		Status:    taskStatusRunning,
		Payload:   mustJSONText(t, map[string]any{"assetId": "asset-1"}),
		CreatedAt: now,
		UpdatedAt: now.Add(2 * time.Minute),
	}
	if err := dataStore.CreateTask(ctx, task); err != nil {
		t.Fatalf("create transfer task: %v", err)
	}

	item := newTransferTaskItem(0)
	item.TaskID = task.ID
	item.GroupKey = "asset-1"
	item.Direction = transferDirectionDownload
	item.DisplayName = "clip.bin"
	item.SourceKind = transferSourceKindEndpoint
	item.SourceEndpointType = string(connectors.EndpointTypeNetwork)
	item.SourceLabel = "115 网盘"
	item.SourcePath = "projects/clip.bin"
	item.TargetEndpointID = targetEndpoint.ID
	item.TargetEndpointType = targetEndpoint.EndpointType
	item.TargetLabel = targetEndpoint.Name
	item.TargetPath = "projects/clip.bin"
	item.TargetTempPath = buildTransferTargetTempPath(item.TargetPath, item.ID)
	item.Status = taskStatusRunning
	item.Phase = transferPhaseStaging
	item.TotalBytes = 100
	item.StagedBytes = 12
	item.CommittedBytes = 5
	item.ProgressPercent = 8
	item.CreatedAt = now
	item.UpdatedAt = now.Add(2 * time.Minute)
	item.MetadataJSON = mustJSONText(t, transferItemMetadata{
		CurrentSpeed: 4096,
	})

	stagingDir := filepath.Join(service.transferStateRoot(), "tasks", item.TaskID, item.ID)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatalf("create staging dir: %v", err)
	}
	item.StagingPath = filepath.Join(stagingDir, "clip.bin")
	if err := os.WriteFile(item.StagingPath, make([]byte, 64), 0o644); err != nil {
		t.Fatalf("write staging file: %v", err)
	}

	targetTempPath := resolveEndpointAbsolutePath(targetEndpoint, item.TargetTempPath)
	if err := os.MkdirAll(filepath.Dir(targetTempPath), 0o755); err != nil {
		t.Fatalf("create target temp dir: %v", err)
	}
	if err := os.WriteFile(targetTempPath, make([]byte, 23), 0o644); err != nil {
		t.Fatalf("write target temp file: %v", err)
	}

	if err := dataStore.CreateTransferTaskItems(ctx, []store.TransferTaskItem{item}); err != nil {
		t.Fatalf("create transfer task item: %v", err)
	}

	if err := service.normalizeInterruptedTransferTasks(ctx); err != nil {
		t.Fatalf("normalize interrupted transfers: %v", err)
	}

	storedTask, err := dataStore.GetTaskByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("load normalized task: %v", err)
	}
	if storedTask.Status != taskStatusQueued {
		t.Fatalf("expected normalized task status %s, got %s", taskStatusQueued, storedTask.Status)
	}

	storedItems, err := dataStore.ListTransferTaskItemsByTaskID(ctx, task.ID)
	if err != nil {
		t.Fatalf("list normalized items: %v", err)
	}
	if len(storedItems) != 1 {
		t.Fatalf("expected 1 normalized item, got %d", len(storedItems))
	}

	normalized := storedItems[0]
	if normalized.Status != taskStatusQueued {
		t.Fatalf("expected normalized item status %s, got %s", taskStatusQueued, normalized.Status)
	}
	if normalized.Phase != transferPhaseCommitting {
		t.Fatalf("expected normalized item phase %s, got %s", transferPhaseCommitting, normalized.Phase)
	}
	if normalized.StagedBytes != 64 {
		t.Fatalf("expected normalized staged bytes 64, got %d", normalized.StagedBytes)
	}
	if normalized.CommittedBytes != 23 {
		t.Fatalf("expected normalized committed bytes 23, got %d", normalized.CommittedBytes)
	}

	expectedProgress := calcTransferItemProgress(normalized.TotalBytes, normalized.StagedBytes, normalized.CommittedBytes, normalized.Phase)
	if normalized.ProgressPercent != expectedProgress {
		t.Fatalf("expected normalized progress %d, got %d", expectedProgress, normalized.ProgressPercent)
	}

	metadata := parseTransferItemMetadata(normalized.MetadataJSON)
	if metadata.CurrentSpeed != 0 {
		t.Fatalf("expected normalized current speed 0, got %d", metadata.CurrentSpeed)
	}
}

func TestCleanupTransferArtifactsRemovesAria2CompanionAndTempFiles(t *testing.T) {
	ctx := context.Background()
	dataStore := newTestStore(t)
	workspaceRoot := t.TempDir()
	restoreRoot := filepath.Join(workspaceRoot, "restore")
	if err := os.MkdirAll(restoreRoot, 0o755); err != nil {
		t.Fatalf("create restore root: %v", err)
	}

	service := NewService(
		dataStore,
		nil,
		MediaConfig{CacheRoot: filepath.Join(workspaceRoot, "cache", "media")},
		WithAutoQueueDerivedMedia(false),
		WithAutoQueueSearchJobs(false),
	)

	now := time.Now().UTC().Round(time.Second)
	targetEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-local-cleanup",
		Name:               "本地恢复目录",
		EndpointType:       string(connectors.EndpointTypeLocal),
		RootPath:           restoreRoot,
		RoleMode:           "MANAGED",
		IdentitySignature:  "local-cleanup",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   mustJSONText(t, map[string]any{"rootPath": restoreRoot}),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := dataStore.CreateStorageEndpoint(ctx, targetEndpoint); err != nil {
		t.Fatalf("create target endpoint: %v", err)
	}

	item := newTransferTaskItem(0)
	item.TaskID = "task-cleanup"
	item.TargetEndpointID = targetEndpoint.ID
	item.TargetEndpointType = targetEndpoint.EndpointType
	item.TargetPath = "projects/clip.bin"
	item.TargetTempPath = buildTransferTargetTempPath(item.TargetPath, item.ID)

	stagingDir := filepath.Join(service.transferStateRoot(), "tasks", item.TaskID, item.ID)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatalf("create staging dir: %v", err)
	}
	item.StagingPath = filepath.Join(stagingDir, "clip.bin")
	if err := os.WriteFile(item.StagingPath, []byte("staging"), 0o644); err != nil {
		t.Fatalf("write staging file: %v", err)
	}
	if err := os.WriteFile(item.StagingPath+".aria2", []byte("control"), 0o644); err != nil {
		t.Fatalf("write aria2 control file: %v", err)
	}

	targetTempPath := resolveEndpointAbsolutePath(targetEndpoint, item.TargetTempPath)
	if err := os.MkdirAll(filepath.Dir(targetTempPath), 0o755); err != nil {
		t.Fatalf("create target temp dir: %v", err)
	}
	if err := os.WriteFile(targetTempPath, []byte("temp"), 0o644); err != nil {
		t.Fatalf("write target temp file: %v", err)
	}

	service.cleanupTransferArtifacts(ctx, []store.TransferTaskItem{item})

	if _, err := os.Stat(item.StagingPath); !os.IsNotExist(err) {
		t.Fatalf("expected staging file removed, got err=%v", err)
	}
	if _, err := os.Stat(item.StagingPath + ".aria2"); !os.IsNotExist(err) {
		t.Fatalf("expected aria2 control file removed, got err=%v", err)
	}
	if _, err := os.Stat(targetTempPath); !os.IsNotExist(err) {
		t.Fatalf("expected target temp file removed, got err=%v", err)
	}
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Fatalf("expected staging directory pruned, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Dir(stagingDir)); !os.IsNotExist(err) {
		t.Fatalf("expected task staging directory pruned, got err=%v", err)
	}
}

func buildTestTransferTaskItem(task store.Task, status string, direction string) store.TransferTaskItem {
	item := newTransferTaskItem(0)
	item.TaskID = task.ID
	item.GroupKey = task.ID
	item.Direction = direction
	item.DisplayName = task.ID + ".bin"
	item.SourceKind = transferSourceKindEndpoint
	item.SourceLabel = "source"
	item.SourcePath = "source/" + task.ID + ".bin"
	item.TargetEndpointType = string(connectors.EndpointTypeLocal)
	item.TargetLabel = "target"
	item.TargetPath = "target/" + task.ID + ".bin"
	item.Status = status
	item.Phase = restoreTransferPhase(item)
	item.TotalBytes = 128
	item.CreatedAt = task.CreatedAt
	item.UpdatedAt = task.UpdatedAt
	item.StartedAt = task.StartedAt
	item.FinishedAt = task.FinishedAt
	return item
}
