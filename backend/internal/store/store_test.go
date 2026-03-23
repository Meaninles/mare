package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreCRUDFlows(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "catalog.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Second)

	endpoint := StorageEndpoint{
		ID:                 "endpoint-1",
		Name:               "Local Media",
		EndpointType:       "LOCAL",
		RootPath:           "D:/Media",
		RoleMode:           "MANAGED",
		IdentitySignature:  "sig-local-media",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   `{"type":"local"}`,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := store.CreateStorageEndpoint(ctx, endpoint); err != nil {
		t.Fatalf("create storage endpoint: %v", err)
	}

	endpoints, err := store.ListStorageEndpoints(ctx)
	if err != nil {
		t.Fatalf("list storage endpoints: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(endpoints))
	}

	endpoint.Name = "Primary Local Media"
	endpoint.UpdatedAt = now.Add(time.Minute)
	if err := store.UpdateStorageEndpoint(ctx, endpoint); err != nil {
		t.Fatalf("update storage endpoint: %v", err)
	}

	storedEndpoint, err := store.GetStorageEndpointByID(ctx, endpoint.ID)
	if err != nil {
		t.Fatalf("get storage endpoint: %v", err)
	}
	if storedEndpoint.Name != "Primary Local Media" {
		t.Fatalf("expected updated endpoint name, got %s", storedEndpoint.Name)
	}

	asset := Asset{
		ID:             "asset-1",
		LogicalPathKey: "projects/a/clip001.mp4",
		DisplayName:    "clip001.mp4",
		MediaType:      "video",
		AssetStatus:    "ready",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := store.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("create asset: %v", err)
	}

	storedAsset, err := store.GetAssetByLogicalPathKey(ctx, asset.LogicalPathKey)
	if err != nil {
		t.Fatalf("get asset by logical path: %v", err)
	}
	if storedAsset.ID != asset.ID {
		t.Fatalf("expected asset id %s, got %s", asset.ID, storedAsset.ID)
	}

	if err := store.UpdateAssetStatus(ctx, asset.ID, "partial", now.Add(2*time.Minute)); err != nil {
		t.Fatalf("update asset status: %v", err)
	}

	version := ReplicaVersion{
		ID:        "version-1",
		Size:      2048,
		CreatedAt: now,
	}

	if err := store.CreateReplicaVersion(ctx, version); err != nil {
		t.Fatalf("create replica version: %v", err)
	}

	replica := Replica{
		ID:            "replica-1",
		AssetID:       asset.ID,
		EndpointID:    endpoint.ID,
		PhysicalPath:  "D:/Media/projects/a/clip001.mp4",
		ReplicaStatus: "ACTIVE",
		ExistsFlag:    true,
		VersionID:     &version.ID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := store.CreateReplica(ctx, replica); err != nil {
		t.Fatalf("create replica: %v", err)
	}

	replicasByAsset, err := store.ListReplicasByAssetID(ctx, asset.ID)
	if err != nil {
		t.Fatalf("list replicas by asset: %v", err)
	}
	if len(replicasByAsset) != 1 {
		t.Fatalf("expected 1 replica for asset, got %d", len(replicasByAsset))
	}

	replicasByEndpoint, err := store.ListReplicasByEndpointID(ctx, endpoint.ID)
	if err != nil {
		t.Fatalf("list replicas by endpoint: %v", err)
	}
	if len(replicasByEndpoint) != 1 {
		t.Fatalf("expected 1 replica for endpoint, got %d", len(replicasByEndpoint))
	}

	task := Task{
		ID:        "task-1",
		TaskType:  "scan",
		Status:    "pending",
		Payload:   `{"endpointId":"endpoint-1"}`,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	resultSummary := "scan completed"
	startedAt := now.Add(30 * time.Second)
	finishedAt := now.Add(time.Minute)
	if err := store.UpdateTaskStatus(ctx, task.ID, TaskStatusUpdate{
		Status:        "success",
		ResultSummary: &resultSummary,
		RetryCount:    0,
		StartedAt:     &startedAt,
		FinishedAt:    &finishedAt,
		UpdatedAt:     now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("update task status: %v", err)
	}

	tasks, err := store.ListTasks(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Status != "success" {
		t.Fatalf("expected task status success, got %s", tasks[0].Status)
	}

	if err := store.DeleteStorageEndpoint(ctx, endpoint.ID); err == nil {
		t.Fatal("expected deleting endpoint with replica reference to fail, got nil")
	}
}
