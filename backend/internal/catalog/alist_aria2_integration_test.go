//go:build integration && windows

package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mam/backend/internal/connectors"
	"mam/backend/internal/store"
)

const (
	integrationTransferPayloadSize = int64(768 << 20)
	integrationSignatureBlockSize  = int64(1 << 20)
)

type fileSampleDigest struct {
	Size int64
	Hash string
}

func TestAListAria2TransferPauseResumeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping AList + Aria2 integration test in short mode")
	}

	alistBinary, aria2Binary, err := resolveIntegrationRuntimeBinaries()
	if err != nil {
		t.Skipf("skipping integration test: %v", err)
	}
	t.Setenv("MAM_ALIST_BINARY", alistBinary)
	t.Setenv("MAM_ARIA2_BINARY", aria2Binary)

	ctx := context.Background()
	dataStore := newTestStore(t)
	workspaceRoot := t.TempDir()

	sourceStorageRoot := filepath.Join(workspaceRoot, "alist-source")
	localRestoreRoot := filepath.Join(workspaceRoot, "local-restore")
	targetStorageRoot := filepath.Join(workspaceRoot, "alist-target")
	for _, directory := range []string{sourceStorageRoot, localRestoreRoot, targetStorageRoot} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create integration directory %s: %v", directory, err)
		}
	}
	sourceFilePath := filepath.Join(sourceStorageRoot, "projects", "clip.bin")
	if err := createSparseTransferFixture(sourceFilePath, integrationTransferPayloadSize); err != nil {
		t.Fatalf("create sparse transfer fixture: %v", err)
	}

	sourceDigest, err := computeFileSampleDigest(sourceFilePath)
	if err != nil {
		t.Fatalf("compute source digest: %v", err)
	}

	now := time.Now().UTC().Round(time.Second)
	sourceEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-alist-source",
		Name:               "AList 源端",
		EndpointType:       string(connectors.EndpointTypeAList),
		RootPath:           "/source",
		RoleMode:           "MANAGED",
		IdentitySignature:  "alist-source",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig: mustJSONText(t, map[string]any{
			"mountPath": "/source",
			"driver":    "Local",
			"addition": map[string]any{
				"root_folder_path": sourceStorageRoot,
			},
			"remark":          "integration-source",
			"cacheExpiration": 30,
		}),
		CreatedAt: now,
		UpdatedAt: now,
	}
	localEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-local-restore",
		Name:               "本地恢复目录",
		EndpointType:       string(connectors.EndpointTypeLocal),
		RootPath:           localRestoreRoot,
		RoleMode:           "MANAGED",
		IdentitySignature:  "local-restore",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   mustJSONText(t, map[string]any{"rootPath": localRestoreRoot}),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	targetEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-alist-target",
		Name:               "AList 目标端",
		EndpointType:       string(connectors.EndpointTypeAList),
		RootPath:           "/upload",
		RoleMode:           "MANAGED",
		IdentitySignature:  "alist-target",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig: mustJSONText(t, map[string]any{
			"mountPath": "/upload",
			"driver":    "Local",
			"addition": map[string]any{
				"root_folder_path": targetStorageRoot,
			},
			"remark":          "integration-target",
			"cacheExpiration": 30,
		}),
		CreatedAt: now,
		UpdatedAt: now,
	}
	for _, endpoint := range []store.StorageEndpoint{sourceEndpoint, localEndpoint, targetEndpoint} {
		if err := dataStore.CreateStorageEndpoint(ctx, endpoint); err != nil {
			t.Fatalf("create endpoint %s: %v", endpoint.ID, err)
		}
	}

	asset := store.Asset{
		ID:               "asset-large-transfer",
		LogicalPathKey:   "projects/clip.bin",
		DisplayName:      "clip.bin",
		MediaType:        string(connectors.MediaTypeVideo),
		AssetStatus:      string(AssetStatusReady),
		PrimaryTimestamp: &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := dataStore.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("create asset: %v", err)
	}

	version := store.ReplicaVersion{
		ID:           "version-alist-source",
		Size:         sourceDigest.Size,
		MTime:        &now,
		ScanRevision: stringPointer("integration-scan"),
		CreatedAt:    now,
	}
	if err := dataStore.CreateReplicaVersion(ctx, version); err != nil {
		t.Fatalf("create replica version: %v", err)
	}

	sourceReplica := store.Replica{
		ID:            "replica-alist-source",
		AssetID:       asset.ID,
		EndpointID:    sourceEndpoint.ID,
		PhysicalPath:  "/source/projects/clip.bin",
		ReplicaStatus: string(ReplicaStatusActive),
		ExistsFlag:    true,
		VersionID:     &version.ID,
		LastSeenAt:    &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := dataStore.CreateReplica(ctx, sourceReplica); err != nil {
		t.Fatalf("create source replica: %v", err)
	}

	service := NewService(
		dataStore,
		nil,
		MediaConfig{CacheRoot: filepath.Join(workspaceRoot, "cache", "media"), FFmpegPath: "ffmpeg"},
		WithAutoQueueDerivedMedia(false),
		WithAutoQueueSearchJobs(false),
	)
	if err := service.Start(ctx); err != nil {
		t.Fatalf("start transfer service: %v", err)
	}
	t.Cleanup(service.Close)

	downloadSummary, err := service.QueueRestoreAsset(ctx, RestoreAssetRequest{
		AssetID:          asset.ID,
		SourceEndpointID: sourceEndpoint.ID,
		TargetEndpointID: localEndpoint.ID,
	})
	if err != nil {
		t.Fatalf("queue alist -> local transfer: %v", err)
	}

	waitForTransferCondition(t, service, downloadSummary.TaskID, 2*time.Minute, func(detail TransferTaskDetailRecord) bool {
		item := requireSingleTransferItem(t, detail)
		return detail.Task.Status == taskStatusRunning &&
			item.Status == taskStatusRunning &&
			item.EngineKind == transferEngineKindAria2 &&
			item.ProgressPercent > 0 &&
			item.ProgressPercent < 70 &&
			item.StagedBytes > 0
	})

	downloadPauseSummary, err := service.PauseTransferTasks(ctx, []string{downloadSummary.TaskID})
	if err != nil {
		t.Fatalf("pause alist -> local transfer: %v", err)
	}
	if downloadPauseSummary.Updated != 1 {
		t.Fatalf("expected 1 paused download task, got %d", downloadPauseSummary.Updated)
	}

	downloadPausedDetail := waitForTransferStatus(t, service, downloadSummary.TaskID, taskStatusPaused, 30*time.Second)
	downloadPausedItem := requireSingleTransferItem(t, downloadPausedDetail)
	if downloadPausedItem.StagedBytes <= 0 || downloadPausedItem.StagedBytes >= downloadPausedItem.TotalBytes {
		t.Fatalf("expected partial aria2 staging progress before pause, got staged=%d total=%d", downloadPausedItem.StagedBytes, downloadPausedItem.TotalBytes)
	}
	if downloadPausedItem.ExternalStatus != "paused" {
		t.Fatalf("expected aria2 external status paused, got %s", downloadPausedItem.ExternalStatus)
	}

	downloadResumeSummary, err := service.ResumeTransferTasks(ctx, []string{downloadSummary.TaskID})
	if err != nil {
		t.Fatalf("resume alist -> local transfer: %v", err)
	}
	if downloadResumeSummary.Updated != 1 {
		t.Fatalf("expected 1 resumed download task, got %d", downloadResumeSummary.Updated)
	}

	downloadCompletedDetail := waitForTransferStatus(t, service, downloadSummary.TaskID, taskStatusSuccess, 3*time.Minute)
	downloadCompletedItem := requireSingleTransferItem(t, downloadCompletedDetail)
	if downloadCompletedItem.ProgressPercent != 100 {
		t.Fatalf("expected completed download item progress 100, got %d", downloadCompletedItem.ProgressPercent)
	}

	localRestoredPath := filepath.Join(localRestoreRoot, "projects", "clip.bin")
	assertFileDigestMatches(t, localRestoredPath, sourceDigest)

	uploadSummary, err := service.QueueRestoreAsset(ctx, RestoreAssetRequest{
		AssetID:          asset.ID,
		SourceEndpointID: localEndpoint.ID,
		TargetEndpointID: targetEndpoint.ID,
	})
	if err != nil {
		t.Fatalf("queue local -> alist transfer: %v", err)
	}

	waitForTransferCondition(t, service, uploadSummary.TaskID, 2*time.Minute, func(detail TransferTaskDetailRecord) bool {
		item := requireSingleTransferItem(t, detail)
		return detail.Task.Status == taskStatusRunning &&
			item.Status == taskStatusRunning &&
			item.EngineKind == transferEngineKindAList &&
			item.ProgressPercent > 70 &&
			item.ProgressPercent < 95 &&
			item.CommittedBytes > 0
	})

	uploadPauseSummary, err := service.PauseTransferTasks(ctx, []string{uploadSummary.TaskID})
	if err != nil {
		t.Fatalf("pause local -> alist transfer: %v", err)
	}
	if uploadPauseSummary.Updated != 1 {
		t.Fatalf("expected 1 paused upload task, got %d", uploadPauseSummary.Updated)
	}

	uploadPausedDetail := waitForTransferStatus(t, service, uploadSummary.TaskID, taskStatusPaused, 30*time.Second)
	uploadPausedItem := requireSingleTransferItem(t, uploadPausedDetail)
	if uploadPausedItem.CommittedBytes <= 0 || uploadPausedItem.CommittedBytes >= uploadPausedItem.TotalBytes {
		t.Fatalf("expected partial alist commit progress before pause, got committed=%d total=%d", uploadPausedItem.CommittedBytes, uploadPausedItem.TotalBytes)
	}
	if uploadPausedItem.ExternalStatus != "canceled" {
		t.Fatalf("expected alist external status canceled after pause, got %s", uploadPausedItem.ExternalStatus)
	}

	uploadResumeSummary, err := service.ResumeTransferTasks(ctx, []string{uploadSummary.TaskID})
	if err != nil {
		t.Fatalf("resume local -> alist transfer: %v", err)
	}
	if uploadResumeSummary.Updated != 1 {
		t.Fatalf("expected 1 resumed upload task, got %d", uploadResumeSummary.Updated)
	}

	uploadCompletedDetail := waitForTransferStatus(t, service, uploadSummary.TaskID, taskStatusSuccess, 3*time.Minute)
	uploadCompletedItem := requireSingleTransferItem(t, uploadCompletedDetail)
	if uploadCompletedItem.ProgressPercent != 100 {
		t.Fatalf("expected completed upload item progress 100, got %d", uploadCompletedItem.ProgressPercent)
	}
	if uploadCompletedItem.EngineKind != transferEngineKindAList {
		t.Fatalf("expected upload item engine kind %s, got %s", transferEngineKindAList, uploadCompletedItem.EngineKind)
	}

	uploadedFilePath := filepath.Join(targetStorageRoot, "projects", "clip.bin")
	assertFileDigestMatches(t, uploadedFilePath, sourceDigest)

	targetConnector, err := service.buildConnector(targetEndpoint)
	if err != nil {
		t.Fatalf("build target alist connector: %v", err)
	}
	targetEntry, err := targetConnector.StatEntry(ctx, "projects/clip.bin")
	if err != nil {
		t.Fatalf("stat uploaded alist entry: %v", err)
	}
	if targetEntry.Size != sourceDigest.Size {
		t.Fatalf("expected uploaded alist entry size %d, got %d", sourceDigest.Size, targetEntry.Size)
	}
}

func waitForTransferStatus(
	t *testing.T,
	service *Service,
	taskID string,
	expectedStatus string,
	timeout time.Duration,
) TransferTaskDetailRecord {
	t.Helper()
	return waitForTransferCondition(t, service, taskID, timeout, func(detail TransferTaskDetailRecord) bool {
		return detail.Task.Status == expectedStatus
	})
}

func waitForTransferCondition(
	t *testing.T,
	service *Service,
	taskID string,
	timeout time.Duration,
	condition func(TransferTaskDetailRecord) bool,
) TransferTaskDetailRecord {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastDetail TransferTaskDetailRecord
	for time.Now().Before(deadline) {
		detail, err := service.GetTransferTaskDetail(context.Background(), taskID)
		if err == nil {
			lastDetail = detail
			if condition(detail) {
				return detail
			}
			if isTransferTaskTerminal(detail.Task.Status) && !condition(detail) {
				item := requireSingleTransferItem(t, detail)
				t.Fatalf(
					"transfer task %s reached terminal status %s before condition matched: itemStatus=%s phase=%s progress=%d error=%s external=%s",
					taskID,
					detail.Task.Status,
					item.Status,
					item.Phase,
					item.ProgressPercent,
					item.ErrorMessage,
					item.ExternalStatus,
				)
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	itemSummary := "no item"
	if len(lastDetail.Items) > 0 {
		item := lastDetail.Items[0]
		itemSummary = fmt.Sprintf(
			"itemStatus=%s phase=%s progress=%d staged=%d committed=%d external=%s",
			item.Status,
			item.Phase,
			item.ProgressPercent,
			item.StagedBytes,
			item.CommittedBytes,
			item.ExternalStatus,
		)
	}
	t.Fatalf(
		"timed out waiting for transfer task %s condition after %s: taskStatus=%s %s",
		taskID,
		timeout,
		lastDetail.Task.Status,
		itemSummary,
	)
	return TransferTaskDetailRecord{}
}

func requireSingleTransferItem(t *testing.T, detail TransferTaskDetailRecord) TransferTaskItemRecord {
	t.Helper()
	if len(detail.Items) != 1 {
		t.Fatalf("expected 1 transfer item, got %d", len(detail.Items))
	}
	return detail.Items[0]
}

func createSparseTransferFixture(targetPath string, size int64) error {
	if size < integrationSignatureBlockSize*3 {
		return fmt.Errorf("fixture size %d is too small for integration markers", size)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	if err := file.Truncate(size); err != nil {
		return err
	}

	for offset, label := range integrationMarkerOffsets(size) {
		if _, err := file.Seek(offset, 0); err != nil {
			return err
		}
		block := repeatedMarkerBlock(label, integrationSignatureBlockSize)
		if _, err := file.Write(block); err != nil {
			return err
		}
	}

	return file.Sync()
}

func repeatedMarkerBlock(label string, size int64) []byte {
	pattern := []byte(label)
	if len(pattern) == 0 {
		pattern = []byte("mare")
	}

	block := make([]byte, size)
	for index := range block {
		block[index] = pattern[index%len(pattern)]
	}
	return block
}

func integrationMarkerOffsets(size int64) map[int64]string {
	middleOffset := max((size/2)-(integrationSignatureBlockSize/2), int64(0))
	finalOffset := max(size-integrationSignatureBlockSize, int64(0))
	return map[int64]string{
		0:            "mare-start",
		middleOffset: "mare-middle",
		finalOffset:  "mare-end",
	}
}

func computeFileSampleDigest(targetPath string) (fileSampleDigest, error) {
	file, err := os.Open(targetPath)
	if err != nil {
		return fileSampleDigest{}, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fileSampleDigest{}, err
	}

	size := info.Size()
	hasher := sha256.New()
	offsets := []int64{
		0,
		max((size/2)-(integrationSignatureBlockSize/2), int64(0)),
		max(size-integrationSignatureBlockSize, int64(0)),
	}
	seenOffsets := make(map[int64]struct{}, len(offsets))
	for _, offset := range offsets {
		if _, exists := seenOffsets[offset]; exists {
			continue
		}
		seenOffsets[offset] = struct{}{}

		chunkSize := min(integrationSignatureBlockSize, size-offset)
		if chunkSize <= 0 {
			continue
		}
		buffer := make([]byte, chunkSize)
		if _, err := file.ReadAt(buffer, offset); err != nil {
			return fileSampleDigest{}, err
		}
		hasher.Write([]byte(fmt.Sprintf("%d:", offset)))
		hasher.Write(buffer)
	}

	return fileSampleDigest{
		Size: size,
		Hash: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

func assertFileDigestMatches(t *testing.T, targetPath string, expected fileSampleDigest) {
	t.Helper()

	actual, err := computeFileSampleDigest(targetPath)
	if err != nil {
		t.Fatalf("compute digest for %s: %v", targetPath, err)
	}
	if actual != expected {
		t.Fatalf("unexpected file digest for %s: got %+v want %+v", targetPath, actual, expected)
	}
}

func resolveIntegrationRuntimeBinaries() (string, string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	projectRoot := filepath.Clean(filepath.Join(workingDirectory, "..", "..", ".."))
	alistBinary := filepath.Join(projectRoot, ".tools", "runtime", "alist", "extracted", "alist.exe")
	if _, err := os.Stat(alistBinary); err != nil {
		return "", "", fmt.Errorf("alist binary not found at %s", alistBinary)
	}

	aria2Pattern := filepath.Join(projectRoot, ".tools", "runtime", "aria2", "extracted", "*", "aria2c.exe")
	matches, err := filepath.Glob(aria2Pattern)
	if err != nil || len(matches) == 0 {
		return "", "", fmt.Errorf("aria2 binary not found at %s", aria2Pattern)
	}

	return alistBinary, matches[0], nil
}

func min(left int64, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
