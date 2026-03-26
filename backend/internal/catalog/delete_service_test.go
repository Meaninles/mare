package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"mam/backend/internal/connectors"
	"mam/backend/internal/store"
)

func TestDeleteReplicaUpdatesAssetVisibilityAndStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataStore := newTestStore(t)
	now := time.Now().UTC().Round(time.Second)

	localEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-local",
		Name:               "Local",
		EndpointType:       string(connectors.EndpointTypeLocal),
		RootPath:           `D:\Library`,
		RoleMode:           "MANAGED",
		IdentitySignature:  "local-delete",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   mustJSONText(t, map[string]string{"rootPath": `D:\Library`}),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	cloudEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-removable",
		Name:               "Removable Media",
		EndpointType:       string(connectors.EndpointTypeRemovable),
		RootPath:           `E:\Library`,
		RoleMode:           "MANAGED",
		IdentitySignature:  "removable-delete",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   mustJSONText(t, map[string]any{"device": map[string]any{"mountPoint": `E:\Library`}}),
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	for _, endpoint := range []store.StorageEndpoint{localEndpoint, cloudEndpoint} {
		if err := dataStore.CreateStorageEndpoint(ctx, endpoint); err != nil {
			t.Fatalf("create endpoint %s: %v", endpoint.ID, err)
		}
	}

	asset := store.Asset{
		ID:               "asset-delete",
		LogicalPathKey:   "projects/scene.jpg",
		DisplayName:      "Scene.jpg",
		MediaType:        string(connectors.MediaTypeImage),
		AssetStatus:      string(AssetStatusReady),
		PrimaryTimestamp: &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := dataStore.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("create asset: %v", err)
	}

	localReplica := store.Replica{
		ID:            "replica-local",
		AssetID:       asset.ID,
		EndpointID:    localEndpoint.ID,
		PhysicalPath:  `D:\Library\Projects\Scene.jpg`,
		ReplicaStatus: string(ReplicaStatusActive),
		ExistsFlag:    true,
		LastSeenAt:    &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	cloudReplica := store.Replica{
		ID:            "replica-removable",
		AssetID:       asset.ID,
		EndpointID:    cloudEndpoint.ID,
		PhysicalPath:  `E:\Library\Projects\Scene.jpg`,
		ReplicaStatus: string(ReplicaStatusActive),
		ExistsFlag:    true,
		LastSeenAt:    &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	for _, replica := range []store.Replica{localReplica, cloudReplica} {
		if err := dataStore.CreateReplica(ctx, replica); err != nil {
			t.Fatalf("create replica %s: %v", replica.ID, err)
		}
	}

	factory := &fakeConnectorFactory{
		connectorsByEndpoint: map[string]*fakeConnector{
			localEndpoint.ID: {
				descriptor: connectors.Descriptor{
					Name:     localEndpoint.Name,
					Type:     connectors.EndpointTypeLocal,
					RootPath: localEndpoint.RootPath,
					Capabilities: connectors.Capabilities{
						CanDelete: true,
					},
				},
			},
			cloudEndpoint.ID: {
				descriptor: connectors.Descriptor{
					Name:     cloudEndpoint.Name,
					Type:     connectors.EndpointTypeRemovable,
					RootPath: cloudEndpoint.RootPath,
					Capabilities: connectors.Capabilities{
						CanDelete: true,
					},
				},
			},
		},
	}

	service := NewService(dataStore, factory.Build, WithAutoQueueDerivedMedia(false))

	firstSummary, err := service.DeleteReplica(ctx, DeleteReplicaRequest{
		AssetID:          asset.ID,
		TargetEndpointID: cloudEndpoint.ID,
	})
	if err != nil {
		t.Fatalf("delete first replica: %v", err)
	}
	if !firstSummary.ReplicaDeleted {
		t.Fatal("expected first delete to mark replica as deleted")
	}
	if firstSummary.AssetRemoved {
		t.Fatal("expected asset to remain visible after deleting only one replica")
	}
	if firstSummary.RemainingAvailableCopies != 1 {
		t.Fatalf("expected 1 remaining available replica, got %d", firstSummary.RemainingAvailableCopies)
	}
	if firstSummary.AssetStatus != string(AssetStatusPartial) {
		t.Fatalf("expected partial asset status after first delete, got %s", firstSummary.AssetStatus)
	}
	if len(factory.connectorsByEndpoint[cloudEndpoint.ID].deletedPaths) != 1 {
		t.Fatal("expected delete connector to be called exactly once for cloud replica")
	}
	if factory.connectorsByEndpoint[cloudEndpoint.ID].deletedPaths[0] != cloudReplica.PhysicalPath {
		t.Fatalf("expected cloud delete path %s, got %s", cloudReplica.PhysicalPath, factory.connectorsByEndpoint[cloudEndpoint.ID].deletedPaths[0])
	}

	storedCloudReplica, err := dataStore.GetReplicaByAssetAndEndpoint(ctx, asset.ID, cloudEndpoint.ID)
	if err != nil {
		t.Fatalf("load cloud replica: %v", err)
	}
	if storedCloudReplica.ExistsFlag {
		t.Fatal("expected cloud replica to be marked absent after delete")
	}
	if storedCloudReplica.ReplicaStatus != string(ReplicaStatusDeleted) {
		t.Fatalf("expected cloud replica status deleted, got %s", storedCloudReplica.ReplicaStatus)
	}

	storedAsset, err := dataStore.GetAssetByID(ctx, asset.ID)
	if err != nil {
		t.Fatalf("load stored asset after first delete: %v", err)
	}
	if storedAsset.AssetStatus != string(AssetStatusPartial) {
		t.Fatalf("expected stored asset status partial, got %s", storedAsset.AssetStatus)
	}

	visibleAssets, err := service.ListAssets(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list visible assets after first delete: %v", err)
	}
	if len(visibleAssets) != 1 {
		t.Fatalf("expected asset to remain visible after first delete, got %d visible assets", len(visibleAssets))
	}

	secondSummary, err := service.DeleteReplica(ctx, DeleteReplicaRequest{
		AssetID:          asset.ID,
		TargetEndpointID: localEndpoint.ID,
	})
	if err != nil {
		t.Fatalf("delete last replica: %v", err)
	}
	if !secondSummary.AssetRemoved {
		t.Fatal("expected last replica delete to remove asset from visible catalog")
	}
	if secondSummary.RemainingAvailableCopies != 0 {
		t.Fatalf("expected 0 remaining available replicas, got %d", secondSummary.RemainingAvailableCopies)
	}
	if secondSummary.AssetStatus != string(AssetStatusDeleted) {
		t.Fatalf("expected deleted asset status after removing last replica, got %s", secondSummary.AssetStatus)
	}
	if len(factory.connectorsByEndpoint[localEndpoint.ID].deletedPaths) != 1 {
		t.Fatal("expected local delete connector to be called exactly once")
	}
	if factory.connectorsByEndpoint[localEndpoint.ID].deletedPaths[0] != localReplica.PhysicalPath {
		t.Fatalf("expected local delete path %s, got %s", localReplica.PhysicalPath, factory.connectorsByEndpoint[localEndpoint.ID].deletedPaths[0])
	}

	visibleAssets, err = service.ListAssets(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list visible assets after last delete: %v", err)
	}
	if len(visibleAssets) != 0 {
		t.Fatalf("expected no visible assets after deleting every replica, got %d", len(visibleAssets))
	}

	storedAsset, err = dataStore.GetAssetByID(ctx, asset.ID)
	if err != nil {
		t.Fatalf("load stored asset after last delete: %v", err)
	}
	if storedAsset.AssetStatus != string(AssetStatusDeleted) {
		t.Fatalf("expected stored asset status deleted, got %s", storedAsset.AssetStatus)
	}

	tasks, err := dataStore.ListTasks(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 delete tasks, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.TaskType != taskTypeDeleteReplica {
			t.Fatalf("expected delete task type %s, got %s", taskTypeDeleteReplica, task.TaskType)
		}
		if task.Status != taskStatusSuccess {
			t.Fatalf("expected successful delete task, got %s", task.Status)
		}
	}
}

func TestDeleteReplicaFailurePreservesReplicaAndRecordsTaskError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataStore := newTestStore(t)
	now := time.Now().UTC().Round(time.Second)

	endpoint := store.StorageEndpoint{
		ID:                 "endpoint-failing-delete",
		Name:               "Failing Endpoint",
		EndpointType:       string(connectors.EndpointTypeLocal),
		RootPath:           `D:\Delete`,
		RoleMode:           "MANAGED",
		IdentitySignature:  "failing-delete",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   mustJSONText(t, map[string]string{"rootPath": `D:\Delete`}),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := dataStore.CreateStorageEndpoint(ctx, endpoint); err != nil {
		t.Fatalf("create endpoint: %v", err)
	}

	asset := store.Asset{
		ID:               "asset-delete-error",
		LogicalPathKey:   "projects/failure.mov",
		DisplayName:      "Failure.mov",
		MediaType:        string(connectors.MediaTypeVideo),
		AssetStatus:      string(AssetStatusReady),
		PrimaryTimestamp: &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := dataStore.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("create asset: %v", err)
	}

	replica := store.Replica{
		ID:            "replica-delete-error",
		AssetID:       asset.ID,
		EndpointID:    endpoint.ID,
		PhysicalPath:  `D:\Delete\Projects\Failure.mov`,
		ReplicaStatus: string(ReplicaStatusActive),
		ExistsFlag:    true,
		LastSeenAt:    &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := dataStore.CreateReplica(ctx, replica); err != nil {
		t.Fatalf("create replica: %v", err)
	}

	factory := &fakeConnectorFactory{
		connectorsByEndpoint: map[string]*fakeConnector{
			endpoint.ID: {
				descriptor: connectors.Descriptor{
					Name:     endpoint.Name,
					Type:     connectors.EndpointTypeLocal,
					RootPath: endpoint.RootPath,
					Capabilities: connectors.Capabilities{
						CanDelete: true,
					},
				},
				deleteErr: errors.New("delete failed"),
			},
		},
	}

	service := NewService(dataStore, factory.Build, WithAutoQueueDerivedMedia(false))

	summary, err := service.DeleteReplica(ctx, DeleteReplicaRequest{
		AssetID:          asset.ID,
		TargetEndpointID: endpoint.ID,
	})
	if err == nil {
		t.Fatal("expected delete replica to fail")
	}
	if summary.Status != taskStatusFailed {
		t.Fatalf("expected failed summary status, got %s", summary.Status)
	}
	if summary.Error == "" {
		t.Fatal("expected delete summary to carry error text")
	}
	if len(factory.connectorsByEndpoint[endpoint.ID].deletedPaths) != 1 {
		t.Fatal("expected failing connector to still receive one delete call")
	}

	storedReplica, err := dataStore.GetReplicaByAssetAndEndpoint(ctx, asset.ID, endpoint.ID)
	if err != nil {
		t.Fatalf("load stored replica: %v", err)
	}
	if !storedReplica.ExistsFlag {
		t.Fatal("expected replica to remain available after failed delete")
	}
	if storedReplica.ReplicaStatus != string(ReplicaStatusActive) {
		t.Fatalf("expected replica status ACTIVE after failed delete, got %s", storedReplica.ReplicaStatus)
	}

	storedAsset, err := dataStore.GetAssetByID(ctx, asset.ID)
	if err != nil {
		t.Fatalf("load stored asset: %v", err)
	}
	if storedAsset.AssetStatus != string(AssetStatusReady) {
		t.Fatalf("expected asset status to remain ready after failed delete, got %s", storedAsset.AssetStatus)
	}

	tasks, err := dataStore.ListTasks(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 delete task, got %d", len(tasks))
	}
	if tasks[0].Status != taskStatusFailed {
		t.Fatalf("expected failed delete task status, got %s", tasks[0].Status)
	}
	if tasks[0].ErrorMessage == nil || *tasks[0].ErrorMessage == "" {
		t.Fatal("expected failed delete task to record error message")
	}
}
