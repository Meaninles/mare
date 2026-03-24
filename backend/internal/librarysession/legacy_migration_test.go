package librarysession

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"mam/backend/internal/store"
)

func TestMigrateLegacyCatalogCreatesValidatedDefaultLibrary(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "mam.db")

	sourceStore, err := store.CreateSQLiteStore(sourcePath)
	if err != nil {
		t.Fatalf("create legacy source store: %v", err)
	}

	now := time.Now().UTC().Round(time.Second)
	endpoint := store.StorageEndpoint{
		ID:                 "endpoint-1",
		Name:               "Legacy Local",
		EndpointType:       "LOCAL",
		RootPath:           "D:/Legacy",
		RoleMode:           "MANAGED",
		IdentitySignature:  "legacy-local",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   `{"rootPath":"D:/Legacy"}`,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := sourceStore.CreateStorageEndpoint(ctx, endpoint); err != nil {
		t.Fatalf("create legacy endpoint: %v", err)
	}

	asset := store.Asset{
		ID:             "asset-1",
		LogicalPathKey: "trip/day01/img0001.cr3",
		DisplayName:    "img0001.cr3",
		MediaType:      "image",
		AssetStatus:    "ready",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := sourceStore.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("create legacy asset: %v", err)
	}

	version := store.ReplicaVersion{
		ID:        "version-1",
		Size:      4096,
		CreatedAt: now,
	}
	if err := sourceStore.CreateReplicaVersion(ctx, version); err != nil {
		t.Fatalf("create legacy replica version: %v", err)
	}

	replica := store.Replica{
		ID:            "replica-1",
		AssetID:       asset.ID,
		EndpointID:    endpoint.ID,
		PhysicalPath:  "D:/Legacy/trip/day01/img0001.cr3",
		ReplicaStatus: "ACTIVE",
		ExistsFlag:    true,
		VersionID:     &version.ID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := sourceStore.CreateReplica(ctx, replica); err != nil {
		t.Fatalf("create legacy replica: %v", err)
	}

	task := store.Task{
		ID:        "task-1",
		TaskType:  "scan_endpoint",
		Status:    "success",
		Payload:   `{"endpointId":"endpoint-1"}`,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := sourceStore.CreateTask(ctx, task); err != nil {
		t.Fatalf("create legacy task: %v", err)
	}

	rule := store.ImportRule{
		ID:                "rule-1",
		RuleType:          "media_type",
		MatchValue:        "image",
		TargetEndpointIDs: `["endpoint-1"]`,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := sourceStore.ReplaceImportRules(ctx, []store.ImportRule{rule}); err != nil {
		t.Fatalf("replace import rules: %v", err)
	}

	if err := sourceStore.Close(); err != nil {
		t.Fatalf("close legacy source store: %v", err)
	}

	manager := NewManager("Mare", "ffmpeg")
	t.Cleanup(func() {
		_, _ = manager.CloseLibrary(ctx)
	})

	status, err := manager.LegacyCatalogStatus(ctx, sourcePath)
	if err != nil {
		t.Fatalf("inspect legacy status: %v", err)
	}
	if !status.Available {
		t.Fatalf("expected legacy catalog to be available, got %+v", status)
	}
	if status.SourceSummary == nil || status.SourceSummary.AssetCount != 1 {
		t.Fatalf("expected legacy asset count 1, got %+v", status.SourceSummary)
	}

	targetPath := filepath.Join(tempDir, "default-library.maredb")
	result, err := manager.MigrateLegacyCatalog(ctx, sourcePath, targetPath, "Migrated Default Library")
	if err != nil {
		t.Fatalf("migrate legacy catalog: %v", err)
	}

	if !result.CountsMatch {
		t.Fatalf("expected migration validation to pass, got %+v", result)
	}
	if result.SourceSummary != result.TargetSummary {
		t.Fatalf("expected source and target summary to match, got source=%+v target=%+v", result.SourceSummary, result.TargetSummary)
	}
	if !result.Library.Ready {
		t.Fatalf("expected migrated library session to be ready, got %+v", result.Library)
	}
	if !samePath(result.TargetPath, targetPath) {
		t.Fatalf("expected migrated target path %s, got %s", targetPath, result.TargetPath)
	}

	targetStore, err := store.OpenSQLiteStore(targetPath)
	if err != nil {
		t.Fatalf("open migrated target store: %v", err)
	}
	if err := targetStore.Close(); err != nil {
		t.Fatalf("close migrated target store: %v", err)
	}
}
