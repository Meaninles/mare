package catalog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mam/backend/internal/connectors"
	"mam/backend/internal/store"
)

func TestRestoreAssetCopiesReplicaToTargetEndpoint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataStore := newTestStore(t)
	now := time.Now().UTC().Round(time.Second)

	sourceRoot := filepath.Join(t.TempDir(), "qnap-source")
	targetRoot := filepath.Join(t.TempDir(), "local-target")
	sourcePath := filepath.Join(sourceRoot, "Projects", "Clip.mp4")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("video-data"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	sourceEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-qnap",
		Name:               "QNAP Source",
		EndpointType:       string(connectors.EndpointTypeQNAP),
		RootPath:           sourceRoot,
		RoleMode:           "MANAGED",
		IdentitySignature:  "qnap-source",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   mustJSONText(t, map[string]string{"sharePath": sourceRoot}),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	targetEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-local",
		Name:               "Local Target",
		EndpointType:       string(connectors.EndpointTypeLocal),
		RootPath:           targetRoot,
		RoleMode:           "MANAGED",
		IdentitySignature:  "local-target",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   mustJSONText(t, map[string]string{"rootPath": targetRoot}),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	for _, endpoint := range []store.StorageEndpoint{sourceEndpoint, targetEndpoint} {
		if err := dataStore.CreateStorageEndpoint(ctx, endpoint); err != nil {
			t.Fatalf("create endpoint %s: %v", endpoint.ID, err)
		}
	}

	asset := store.Asset{
		ID:               "asset-clip",
		LogicalPathKey:   "projects/clip.mp4",
		DisplayName:      "Clip.mp4",
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
		ID:           "version-source",
		Size:         int64(len("video-data")),
		MTime:        &now,
		ScanRevision: stringPointer("scan-1"),
		CreatedAt:    now,
	}
	if err := dataStore.CreateReplicaVersion(ctx, version); err != nil {
		t.Fatalf("create replica version: %v", err)
	}

	sourceReplica := store.Replica{
		ID:            "replica-source",
		AssetID:       asset.ID,
		EndpointID:    sourceEndpoint.ID,
		PhysicalPath:  sourcePath,
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

	service := NewService(dataStore, nil)

	summary, err := service.RestoreAsset(ctx, RestoreAssetRequest{
		AssetID:          asset.ID,
		SourceEndpointID: sourceEndpoint.ID,
		TargetEndpointID: targetEndpoint.ID,
	})
	if err != nil {
		t.Fatalf("restore asset: %v", err)
	}
	if summary.Status != taskStatusSuccess {
		t.Fatalf("expected success status, got %s", summary.Status)
	}
	if !summary.CreatedReplica {
		t.Fatal("expected restore to create target replica")
	}

	restoredPath := filepath.Join(targetRoot, "Projects", "Clip.mp4")
	if _, err := os.Stat(restoredPath); err != nil {
		t.Fatalf("expected restored file at %s: %v", restoredPath, err)
	}

	targetReplica, err := dataStore.GetReplicaByAssetAndEndpoint(ctx, asset.ID, targetEndpoint.ID)
	if err != nil {
		t.Fatalf("load target replica: %v", err)
	}
	if !targetReplica.ExistsFlag {
		t.Fatal("expected target replica to exist after restore")
	}
	if targetReplica.ReplicaStatus != string(ReplicaStatusActive) {
		t.Fatalf("expected target replica status ACTIVE, got %s", targetReplica.ReplicaStatus)
	}

	storedAsset, err := dataStore.GetAssetByID(ctx, asset.ID)
	if err != nil {
		t.Fatalf("load stored asset: %v", err)
	}
	if storedAsset.AssetStatus != string(AssetStatusReady) {
		t.Fatalf("expected restored asset status ready, got %s", storedAsset.AssetStatus)
	}

	tasks, err := dataStore.ListTasks(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 restore task, got %d", len(tasks))
	}
	if tasks[0].Status != taskStatusSuccess {
		t.Fatalf("expected restore task success, got %s", tasks[0].Status)
	}
}

func TestRestoreAssetsToEndpointReportsPartialFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataStore := newTestStore(t)
	now := time.Now().UTC().Round(time.Second)

	sourceRoot := filepath.Join(t.TempDir(), "source")
	targetRoot := filepath.Join(t.TempDir(), "target")
	recoverablePath := filepath.Join(sourceRoot, "Projects", "Photo.jpg")
	if err := os.MkdirAll(filepath.Dir(recoverablePath), 0o755); err != nil {
		t.Fatalf("create recoverable directory: %v", err)
	}
	if err := os.WriteFile(recoverablePath, []byte("image-data"), 0o644); err != nil {
		t.Fatalf("write recoverable file: %v", err)
	}

	sourceEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-source",
		Name:               "Source",
		EndpointType:       string(connectors.EndpointTypeLocal),
		RootPath:           sourceRoot,
		RoleMode:           "MANAGED",
		IdentitySignature:  "source",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   mustJSONText(t, map[string]string{"rootPath": sourceRoot}),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	targetEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-target",
		Name:               "Target",
		EndpointType:       string(connectors.EndpointTypeLocal),
		RootPath:           targetRoot,
		RoleMode:           "MANAGED",
		IdentitySignature:  "target",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   mustJSONText(t, map[string]string{"rootPath": targetRoot}),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	for _, endpoint := range []store.StorageEndpoint{sourceEndpoint, targetEndpoint} {
		if err := dataStore.CreateStorageEndpoint(ctx, endpoint); err != nil {
			t.Fatalf("create endpoint %s: %v", endpoint.ID, err)
		}
	}

	recoverableAsset := store.Asset{
		ID:               "asset-recoverable",
		LogicalPathKey:   "projects/photo.jpg",
		DisplayName:      "Photo.jpg",
		MediaType:        string(connectors.MediaTypeImage),
		AssetStatus:      string(AssetStatusReady),
		PrimaryTimestamp: &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	failingAsset := store.Asset{
		ID:               "asset-missing-source",
		LogicalPathKey:   "projects/lost.mov",
		DisplayName:      "Lost.mov",
		MediaType:        string(connectors.MediaTypeVideo),
		AssetStatus:      string(AssetStatusDeleted),
		PrimaryTimestamp: &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	for _, asset := range []store.Asset{recoverableAsset, failingAsset} {
		if err := dataStore.CreateAsset(ctx, asset); err != nil {
			t.Fatalf("create asset %s: %v", asset.ID, err)
		}
	}

	recoverableVersion := store.ReplicaVersion{
		ID:           "version-recoverable",
		Size:         int64(len("image-data")),
		MTime:        &now,
		ScanRevision: stringPointer("scan-2"),
		CreatedAt:    now,
	}
	if err := dataStore.CreateReplicaVersion(ctx, recoverableVersion); err != nil {
		t.Fatalf("create recoverable version: %v", err)
	}

	recoverableReplica := store.Replica{
		ID:            "replica-recoverable",
		AssetID:       recoverableAsset.ID,
		EndpointID:    sourceEndpoint.ID,
		PhysicalPath:  recoverablePath,
		ReplicaStatus: string(ReplicaStatusActive),
		ExistsFlag:    true,
		VersionID:     &recoverableVersion.ID,
		LastSeenAt:    &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	failingReplica := store.Replica{
		ID:            "replica-failing-target",
		AssetID:       failingAsset.ID,
		EndpointID:    targetEndpoint.ID,
		PhysicalPath:  filepath.Join(targetRoot, "Projects", "Lost.mov"),
		ReplicaStatus: string(ReplicaStatusMissing),
		ExistsFlag:    false,
		LastSeenAt:    &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	for _, replica := range []store.Replica{recoverableReplica, failingReplica} {
		if err := dataStore.CreateReplica(ctx, replica); err != nil {
			t.Fatalf("create replica %s: %v", replica.ID, err)
		}
	}

	service := NewService(dataStore, nil)

	summary, err := service.RestoreAssetsToEndpoint(ctx, BatchRestoreRequest{
		TargetEndpointID: targetEndpoint.ID,
		AssetIDs:         []string{recoverableAsset.ID, failingAsset.ID},
	})
	if err == nil {
		t.Fatal("expected partial batch restore failure")
	}
	if summary.SuccessCount != 1 {
		t.Fatalf("expected 1 successful restore, got %d", summary.SuccessCount)
	}
	if summary.FailedCount != 1 {
		t.Fatalf("expected 1 failed restore, got %d", summary.FailedCount)
	}
	if summary.Status != taskStatusFailed {
		t.Fatalf("expected failed batch task status, got %s", summary.Status)
	}
	if len(summary.Items) != 2 {
		t.Fatalf("expected 2 batch items, got %d", len(summary.Items))
	}

	restoredPath := filepath.Join(targetRoot, "Projects", "Photo.jpg")
	if _, statErr := os.Stat(restoredPath); statErr != nil {
		t.Fatalf("expected restored batch file at %s: %v", restoredPath, statErr)
	}

	tasks, err := dataStore.ListTasks(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 batch restore task, got %d", len(tasks))
	}
	if tasks[0].Status != taskStatusFailed {
		t.Fatalf("expected batch task failed, got %s", tasks[0].Status)
	}
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()

	dataStore, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}

	t.Cleanup(func() {
		_ = dataStore.Close()
	})

	return dataStore
}

func mustJSONText(t *testing.T, payload any) string {
	t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal JSON text: %v", err)
	}
	return string(data)
}
