package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mam/backend/internal/connectors"
	"mam/backend/internal/store"
)

func TestFullScanAndRescanEndpoint(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "catalog.db")
	dataStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() {
		_ = dataStore.Close()
	}()

	now := time.Now().UTC().Round(time.Second)
	endpoint := store.StorageEndpoint{
		ID:                 "endpoint-local-1",
		Name:               "Local Media",
		EndpointType:       string(connectors.EndpointTypeLocal),
		RootPath:           `D:\Media`,
		RoleMode:           "MANAGED",
		IdentitySignature:  "local-media-1",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   `{"rootPath":"D:\\Media"}`,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := dataStore.CreateStorageEndpoint(context.Background(), endpoint); err != nil {
		t.Fatalf("create storage endpoint: %v", err)
	}

	factory := &fakeConnectorFactory{
		connectorsByEndpoint: map[string]*fakeConnector{
			endpoint.ID: {
				descriptor: connectors.Descriptor{
					Name:     endpoint.Name,
					Type:     connectors.EndpointTypeLocal,
					RootPath: endpoint.RootPath,
				},
				listing: map[string][]connectors.FileEntry{
					"": {
						fileEntry(`D:\Media\Projects\Photo.JPG`, "Projects/Photo.JPG", connectors.MediaTypeImage, now),
						fileEntry(`D:\Media\Audio\Meeting.WAV`, "Audio/Meeting.WAV", connectors.MediaTypeAudio, now),
						fileEntry(`D:\Media\Notes.txt`, "Notes.txt", connectors.MediaTypeUnknown, now),
					},
				},
			},
		},
	}

	service := NewService(dataStore, factory.Build)

	fullSummary, err := service.FullScan(context.Background())
	if err != nil {
		t.Fatalf("full scan: %v", err)
	}
	if fullSummary.SuccessCount != 1 {
		t.Fatalf("expected success count 1, got %d", fullSummary.SuccessCount)
	}
	if fullSummary.EndpointSummaries[0].FilesScanned != 2 {
		t.Fatalf("expected 2 scanned media files, got %d", fullSummary.EndpointSummaries[0].FilesScanned)
	}

	assetsAfterFullScan, err := service.ListAssets(context.Background(), 20, 0)
	if err != nil {
		t.Fatalf("list assets after full scan: %v", err)
	}
	if len(assetsAfterFullScan) != 2 {
		t.Fatalf("expected 2 assets after full scan, got %d", len(assetsAfterFullScan))
	}

	factory.connectorsByEndpoint[endpoint.ID].listing = map[string][]connectors.FileEntry{
		"": {
			fileEntry(`D:\Media\Projects\Photo.JPG`, "Projects/Photo.JPG", connectors.MediaTypeImage, now.Add(time.Minute)),
			fileEntry(`D:\Media\Projects\Clip.MP4`, "Projects/Clip.MP4", connectors.MediaTypeVideo, now.Add(time.Minute)),
		},
	}

	rescanSummary, err := service.RescanEndpoint(context.Background(), endpoint.ID)
	if err != nil {
		t.Fatalf("rescan endpoint: %v", err)
	}
	if rescanSummary.FilesScanned != 2 {
		t.Fatalf("expected 2 scanned media files on rescan, got %d", rescanSummary.FilesScanned)
	}
	if rescanSummary.MissingReplicas != 1 {
		t.Fatalf("expected 1 missing replica after rescan, got %d", rescanSummary.MissingReplicas)
	}

	assetsAfterRescan, err := service.ListAssets(context.Background(), 20, 0)
	if err != nil {
		t.Fatalf("list assets after rescan: %v", err)
	}
	if len(assetsAfterRescan) != 2 {
		t.Fatalf("expected 2 visible assets after rescan, got %d", len(assetsAfterRescan))
	}

	for _, asset := range assetsAfterRescan {
		if asset.LogicalPathKey == "audio/meeting.wav" {
			t.Fatal("expected deleted assets to be hidden from the default asset list")
		}
	}

	missingAsset, err := dataStore.GetAssetByLogicalPathKey(context.Background(), "audio/meeting.wav")
	if err != nil {
		t.Fatalf("load removed asset by logical path key: %v", err)
	}
	if missingAsset.AssetStatus != string(AssetStatusDeleted) {
		t.Fatalf("expected removed asset status deleted, got %s", missingAsset.AssetStatus)
	}

	tasks, err := service.ListTasks(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].Status != "success" || tasks[1].Status != "success" {
		t.Fatalf("expected both tasks to be success, got %s and %s", tasks[0].Status, tasks[1].Status)
	}
}

func TestListAssetsTracksAvailableAndMissingReplicaCounts(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "catalog.db")
	dataStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() {
		_ = dataStore.Close()
	}()

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Second)

	endpoints := []store.StorageEndpoint{
		{
			ID:                 "endpoint-local",
			Name:               "Local Media",
			EndpointType:       string(connectors.EndpointTypeLocal),
			RootPath:           `D:\Media`,
			RoleMode:           "MANAGED",
			IdentitySignature:  "local-media",
			AvailabilityStatus: "AVAILABLE",
			ConnectionConfig:   `{"rootPath":"D:\\Media"}`,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 "endpoint-qnap",
			Name:               "QNAP SMB",
			EndpointType:       string(connectors.EndpointTypeQNAP),
			RootPath:           `\\qnap\share\Media`,
			RoleMode:           "MANAGED",
			IdentitySignature:  "qnap-media",
			AvailabilityStatus: "AVAILABLE",
			ConnectionConfig:   `{"sharePath":"\\\\qnap\\share\\Media"}`,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 "endpoint-cloud",
			Name:               "115 Cloud",
			EndpointType:       string(connectors.EndpointTypeCloud115),
			RootPath:           "0",
			RoleMode:           "MANAGED",
			IdentitySignature:  "cloud-media",
			AvailabilityStatus: "AVAILABLE",
			ConnectionConfig:   `{"rootId":"0","accessToken":"token","appType":"windows"}`,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}

	for _, endpoint := range endpoints {
		if err := dataStore.CreateStorageEndpoint(ctx, endpoint); err != nil {
			t.Fatalf("create storage endpoint %s: %v", endpoint.ID, err)
		}
	}

	factory := &fakeConnectorFactory{
		connectorsByEndpoint: map[string]*fakeConnector{
			"endpoint-local": {
				descriptor: connectors.Descriptor{
					Name:     "Local Media",
					Type:     connectors.EndpointTypeLocal,
					RootPath: `D:\Media`,
				},
				listing: map[string][]connectors.FileEntry{
					"": {
						fileEntry(`D:\Media\Projects\Photo.JPG`, "Projects/Photo.JPG", connectors.MediaTypeImage, now),
					},
				},
			},
			"endpoint-qnap": {
				descriptor: connectors.Descriptor{
					Name:     "QNAP SMB",
					Type:     connectors.EndpointTypeQNAP,
					RootPath: `\\qnap\share\Media`,
				},
				listing: map[string][]connectors.FileEntry{
					"": {
						fileEntry(`\\qnap\share\Media\Projects\Photo.JPG`, "Projects/Photo.JPG", connectors.MediaTypeImage, now),
					},
				},
			},
			"endpoint-cloud": {
				descriptor: connectors.Descriptor{
					Name:     "115 Cloud",
					Type:     connectors.EndpointTypeCloud115,
					RootPath: "0",
				},
				listing: map[string][]connectors.FileEntry{
					"": {
						fileEntry("115://0/Projects/Photo.JPG", "Projects/Photo.JPG", connectors.MediaTypeImage, now),
					},
				},
			},
		},
	}

	service := NewService(dataStore, factory.Build)

	fullSummary, err := service.FullScan(ctx)
	if err != nil {
		t.Fatalf("full scan: %v", err)
	}
	if fullSummary.SuccessCount != 3 {
		t.Fatalf("expected success count 3, got %d", fullSummary.SuccessCount)
	}

	assetsAfterFullScan, err := service.ListAssets(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list assets after full scan: %v", err)
	}
	if len(assetsAfterFullScan) != 1 {
		t.Fatalf("expected 1 merged asset after full scan, got %d", len(assetsAfterFullScan))
	}
	if assetsAfterFullScan[0].CanonicalPath != "projects/photo.jpg" {
		t.Fatalf("expected canonical path projects/photo.jpg, got %s", assetsAfterFullScan[0].CanonicalPath)
	}
	if assetsAfterFullScan[0].CanonicalDirectory != "projects" {
		t.Fatalf("expected canonical directory projects, got %s", assetsAfterFullScan[0].CanonicalDirectory)
	}
	if assetsAfterFullScan[0].AvailableReplicaCount != 3 {
		t.Fatalf("expected 3 available replicas after full scan, got %d", assetsAfterFullScan[0].AvailableReplicaCount)
	}
	if assetsAfterFullScan[0].MissingReplicaCount != 0 {
		t.Fatalf("expected 0 missing replicas after full scan, got %d", assetsAfterFullScan[0].MissingReplicaCount)
	}
	resolvedDirectories := make(map[string]string)
	for _, replica := range assetsAfterFullScan[0].Replicas {
		if replica.RelativePath != "projects/photo.jpg" {
			t.Fatalf("expected replica relative path projects/photo.jpg, got %s", replica.RelativePath)
		}
		if replica.LogicalDirectory != "projects" {
			t.Fatalf("expected replica logical directory projects, got %s", replica.LogicalDirectory)
		}
		if !replica.MatchesLogicalPath {
			t.Fatalf("expected replica %s to match canonical logical path", replica.EndpointID)
		}
		resolvedDirectories[replica.EndpointID] = replica.ResolvedDirectory
	}
	if resolvedDirectories["endpoint-local"] != `D:\Media\projects` {
		t.Fatalf("expected local resolved directory D:\\Media\\projects, got %s", resolvedDirectories["endpoint-local"])
	}
	if resolvedDirectories["endpoint-qnap"] != `\\qnap\share\Media\projects` {
		t.Fatalf("expected qnap resolved directory \\\\qnap\\share\\Media\\projects, got %s", resolvedDirectories["endpoint-qnap"])
	}
	if resolvedDirectories["endpoint-cloud"] != "0:/projects" {
		t.Fatalf("expected cloud resolved directory 0:/projects, got %s", resolvedDirectories["endpoint-cloud"])
	}

	factory.connectorsByEndpoint["endpoint-qnap"].listing = map[string][]connectors.FileEntry{
		"": {},
	}

	rescanSummary, err := service.RescanEndpoint(ctx, "endpoint-qnap")
	if err != nil {
		t.Fatalf("rescan qnap endpoint: %v", err)
	}
	if rescanSummary.MissingReplicas != 1 {
		t.Fatalf("expected 1 missing replica after rescan, got %d", rescanSummary.MissingReplicas)
	}

	assetsAfterRescan, err := service.ListAssets(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list assets after rescan: %v", err)
	}
	if len(assetsAfterRescan) != 1 {
		t.Fatalf("expected 1 merged asset after rescan, got %d", len(assetsAfterRescan))
	}

	asset := assetsAfterRescan[0]
	if asset.AssetStatus != "partial" {
		t.Fatalf("expected asset status partial after one replica removal, got %s", asset.AssetStatus)
	}
	if asset.AvailableReplicaCount != 2 {
		t.Fatalf("expected 2 available replicas after rescan, got %d", asset.AvailableReplicaCount)
	}
	if asset.MissingReplicaCount != 1 {
		t.Fatalf("expected 1 missing replica after rescan, got %d", asset.MissingReplicaCount)
	}
	if len(asset.Replicas) != 3 {
		t.Fatalf("expected 3 replica records to remain for traceability, got %d", len(asset.Replicas))
	}

	missingByEndpoint := make(map[string]bool)
	availableByEndpoint := make(map[string]bool)
	for _, replica := range asset.Replicas {
		if replica.ExistsFlag {
			availableByEndpoint[replica.EndpointID] = true
			continue
		}
		missingByEndpoint[replica.EndpointID] = true
	}

	if !missingByEndpoint["endpoint-qnap"] {
		t.Fatal("expected qnap replica to be marked missing")
	}
	if len(availableByEndpoint) != 2 {
		t.Fatalf("expected 2 endpoints to remain available, got %d", len(availableByEndpoint))
	}
}

func TestFullScanIgnoresImportSourceEndpoints(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "catalog.db")
	dataStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() {
		_ = dataStore.Close()
	}()

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Second)

	endpoints := []store.StorageEndpoint{
		{
			ID:                 "endpoint-managed",
			Name:               "Managed SSD",
			EndpointType:       string(connectors.EndpointTypeLocal),
			RootPath:           `D:\Media`,
			RoleMode:           "MANAGED",
			IdentitySignature:  "managed-media",
			AvailabilityStatus: "AVAILABLE",
			ConnectionConfig:   `{"rootPath":"D:\\Media"}`,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 "endpoint-import-source",
			Name:               "Import Source",
			EndpointType:       string(connectors.EndpointTypeLocal),
			RootPath:           `E:\Card`,
			RoleMode:           "IMPORT_SOURCE",
			IdentitySignature:  "import-source",
			AvailabilityStatus: "AVAILABLE",
			ConnectionConfig:   `{"rootPath":"E:\\Card"}`,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}

	for _, endpoint := range endpoints {
		if err := dataStore.CreateStorageEndpoint(ctx, endpoint); err != nil {
			t.Fatalf("create storage endpoint %s: %v", endpoint.ID, err)
		}
	}

	service := NewService(dataStore, (&fakeConnectorFactory{
		connectorsByEndpoint: map[string]*fakeConnector{
			"endpoint-managed": {
				descriptor: connectors.Descriptor{
					Name:     "Managed SSD",
					Type:     connectors.EndpointTypeLocal,
					RootPath: `D:\Media`,
				},
				listing: map[string][]connectors.FileEntry{
					"": {
						fileEntry(`D:\Media\Projects\Photo.JPG`, "Projects/Photo.JPG", connectors.MediaTypeImage, now),
					},
				},
			},
			"endpoint-import-source": {
				descriptor: connectors.Descriptor{
					Name:     "Import Source",
					Type:     connectors.EndpointTypeLocal,
					RootPath: `E:\Card`,
				},
				listing: map[string][]connectors.FileEntry{
					"": {
						fileEntry(`E:\Card\Projects\Photo.JPG`, "Projects/Photo.JPG", connectors.MediaTypeImage, now),
					},
				},
			},
		},
	}).Build)

	summary, err := service.FullScan(ctx)
	if err != nil {
		t.Fatalf("full scan: %v", err)
	}
	if summary.EndpointCount != 1 {
		t.Fatalf("expected only 1 managed endpoint to participate in full scan, got %d", summary.EndpointCount)
	}
	if summary.SuccessCount != 1 {
		t.Fatalf("expected 1 successful managed endpoint scan, got %d", summary.SuccessCount)
	}

	assets, err := service.ListAssets(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list assets: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset after managed scan, got %d", len(assets))
	}
	if assets[0].AvailableReplicaCount != 1 {
		t.Fatalf("expected 1 managed replica, got %d", assets[0].AvailableReplicaCount)
	}
}

func TestRescanEndpointRejectsImportSourceRole(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "catalog.db")
	dataStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() {
		_ = dataStore.Close()
	}()

	now := time.Now().UTC().Round(time.Second)
	endpoint := store.StorageEndpoint{
		ID:                 "endpoint-import-source",
		Name:               "Import Source",
		EndpointType:       string(connectors.EndpointTypeLocal),
		RootPath:           `E:\Card`,
		RoleMode:           "IMPORT_SOURCE",
		IdentitySignature:  "import-source",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   `{"rootPath":"E:\\Card"}`,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := dataStore.CreateStorageEndpoint(context.Background(), endpoint); err != nil {
		t.Fatalf("create import source endpoint: %v", err)
	}

	service := NewService(dataStore, (&fakeConnectorFactory{
		connectorsByEndpoint: map[string]*fakeConnector{
			endpoint.ID: {
				descriptor: connectors.Descriptor{
					Name:     endpoint.Name,
					Type:     connectors.EndpointTypeLocal,
					RootPath: endpoint.RootPath,
				},
			},
		},
	}).Build)

	if _, err := service.RescanEndpoint(context.Background(), endpoint.ID); err == nil {
		t.Fatal("expected rescan to reject import source endpoint")
	}
}

func TestListAssetsAddsSyntheticMissingReplicasForManagedEndpointsWithoutReplicaRows(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "catalog.db")
	dataStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() {
		_ = dataStore.Close()
	}()

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Second)

	endpoints := []store.StorageEndpoint{
		{
			ID:                 "endpoint-local",
			Name:               "Local Media",
			EndpointType:       string(connectors.EndpointTypeLocal),
			RootPath:           `D:\Media`,
			RoleMode:           "MANAGED",
			IdentitySignature:  "local-media",
			AvailabilityStatus: "AVAILABLE",
			ConnectionConfig:   `{"rootPath":"D:\\Media"}`,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 "endpoint-qnap",
			Name:               "QNAP SMB",
			EndpointType:       string(connectors.EndpointTypeQNAP),
			RootPath:           `\\qnap\share\Media`,
			RoleMode:           "MANAGED",
			IdentitySignature:  "qnap-media",
			AvailabilityStatus: "AVAILABLE",
			ConnectionConfig:   `{"sharePath":"\\\\qnap\\share\\Media"}`,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 "endpoint-cloud",
			Name:               "115 Cloud",
			EndpointType:       string(connectors.EndpointTypeCloud115),
			RootPath:           "0",
			RoleMode:           "MANAGED",
			IdentitySignature:  "cloud-media",
			AvailabilityStatus: "AVAILABLE",
			ConnectionConfig:   `{"rootId":"0","accessToken":"token","appType":"windows"}`,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}

	for _, endpoint := range endpoints {
		if err := dataStore.CreateStorageEndpoint(ctx, endpoint); err != nil {
			t.Fatalf("create storage endpoint %s: %v", endpoint.ID, err)
		}
	}

	asset := store.Asset{
		ID:               "asset-1",
		LogicalPathKey:   "projects/clip.mp4",
		DisplayName:      "clip.mp4",
		MediaType:        "video",
		AssetStatus:      string(AssetStatusReady),
		CreatedAt:        now,
		UpdatedAt:        now,
		PrimaryTimestamp: &now,
	}
	if err := dataStore.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("create asset: %v", err)
	}

	replica := store.Replica{
		ID:            "replica-cloud",
		AssetID:       asset.ID,
		EndpointID:    "endpoint-cloud",
		PhysicalPath:  "115://0/projects/clip.mp4",
		ReplicaStatus: string(ReplicaStatusActive),
		ExistsFlag:    true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := dataStore.CreateReplica(ctx, replica); err != nil {
		t.Fatalf("create replica: %v", err)
	}

	service := NewService(dataStore, (&fakeConnectorFactory{
		connectorsByEndpoint: map[string]*fakeConnector{
			"endpoint-cloud": {
				descriptor: connectors.Descriptor{
					Name:     "115 Cloud",
					Type:     connectors.EndpointTypeCloud115,
					RootPath: "0",
					Capabilities: connectors.Capabilities{
						CanReadStream: true,
					},
				},
			},
		},
	}).Build)

	assets, err := service.ListAssets(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list assets: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(assets))
	}

	record := assets[0]
	if record.AvailableReplicaCount != 1 {
		t.Fatalf("expected 1 available replica, got %d", record.AvailableReplicaCount)
	}
	if record.MissingReplicaCount != 2 {
		t.Fatalf("expected 2 missing replicas, got %d", record.MissingReplicaCount)
	}
	if len(record.Replicas) != 3 {
		t.Fatalf("expected 3 replica records including placeholders, got %d", len(record.Replicas))
	}

	missingByEndpoint := make(map[string]bool)
	for _, candidate := range record.Replicas {
		if candidate.ExistsFlag {
			continue
		}
		missingByEndpoint[candidate.EndpointID] = true
	}

	if !missingByEndpoint["endpoint-local"] {
		t.Fatal("expected local endpoint to be represented as missing")
	}
	if !missingByEndpoint["endpoint-qnap"] {
		t.Fatal("expected qnap endpoint to be represented as missing")
	}
}

func TestUpdateEndpointPersistsNoteAndConfigChanges(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "catalog.db")
	dataStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() {
		_ = dataStore.Close()
	}()

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Second)
	endpoint := store.StorageEndpoint{
		ID:                 "endpoint-local",
		Name:               "Local Media",
		Note:               "old-note",
		EndpointType:       string(connectors.EndpointTypeLocal),
		RootPath:           `D:\Media`,
		RoleMode:           "MANAGED",
		IdentitySignature:  "local-media",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   `{"rootPath":"D:\\Media"}`,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := dataStore.CreateStorageEndpoint(ctx, endpoint); err != nil {
		t.Fatalf("create storage endpoint: %v", err)
	}

	service := NewService(dataStore, func(endpoint store.StorageEndpoint) (connectors.Connector, error) {
		return &fakeConnector{
			descriptor: connectors.Descriptor{
				Name:     endpoint.Name,
				Type:     connectors.EndpointTypeLocal,
				RootPath: endpoint.RootPath,
			},
		}, nil
	})

	record, err := service.UpdateEndpoint(ctx, endpoint.ID, UpdateEndpointRequest{
		Name:               "Archive SSD",
		Note:               "cold-storage",
		EndpointType:       "LOCAL",
		RootPath:           `E:\Archive`,
		RoleMode:           "MANAGED",
		AvailabilityStatus: "DISABLED",
		ConnectionConfig:   json.RawMessage(`{"rootPath":"E:\\Archive"}`),
	})
	if err != nil {
		t.Fatalf("update endpoint: %v", err)
	}

	if record.Name != "Archive SSD" {
		t.Fatalf("expected updated endpoint name, got %s", record.Name)
	}
	if record.Note != "cold-storage" {
		t.Fatalf("expected updated endpoint note, got %s", record.Note)
	}
	if record.RootPath != `E:\Archive` {
		t.Fatalf("expected updated root path, got %s", record.RootPath)
	}
	if record.AvailabilityStatus != "DISABLED" {
		t.Fatalf("expected updated availability status, got %s", record.AvailabilityStatus)
	}

	storedEndpoint, err := dataStore.GetStorageEndpointByID(ctx, endpoint.ID)
	if err != nil {
		t.Fatalf("load updated endpoint: %v", err)
	}
	if storedEndpoint.Note != "cold-storage" {
		t.Fatalf("expected stored note to be updated, got %s", storedEndpoint.Note)
	}
	if storedEndpoint.RootPath != `E:\Archive` {
		t.Fatalf("expected stored root path to be updated, got %s", storedEndpoint.RootPath)
	}
}

func TestDeleteEndpointRemovesReplicaRecordsAndUpdatesImportRules(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "catalog.db")
	dataStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() {
		_ = dataStore.Close()
	}()

	ctx := context.Background()
	now := time.Now().UTC().Round(time.Second)

	localEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-local",
		Name:               "Local Media",
		EndpointType:       string(connectors.EndpointTypeLocal),
		RootPath:           `D:\Media`,
		RoleMode:           "MANAGED",
		IdentitySignature:  "local-media",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   `{"rootPath":"D:\\Media"}`,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	qnapEndpoint := store.StorageEndpoint{
		ID:                 "endpoint-qnap",
		Name:               "QNAP SMB",
		EndpointType:       string(connectors.EndpointTypeQNAP),
		RootPath:           `\\qnap\share\Media`,
		RoleMode:           "MANAGED",
		IdentitySignature:  "qnap-media",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   `{"sharePath":"\\\\qnap\\share\\Media"}`,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	for _, endpoint := range []store.StorageEndpoint{localEndpoint, qnapEndpoint} {
		if err := dataStore.CreateStorageEndpoint(ctx, endpoint); err != nil {
			t.Fatalf("create storage endpoint %s: %v", endpoint.ID, err)
		}
	}

	asset := store.Asset{
		ID:               "asset-1",
		LogicalPathKey:   "projects/clip.mp4",
		DisplayName:      "clip.mp4",
		MediaType:        "video",
		AssetStatus:      string(AssetStatusReady),
		CreatedAt:        now,
		UpdatedAt:        now,
		PrimaryTimestamp: &now,
	}
	if err := dataStore.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("create asset: %v", err)
	}

	for _, replica := range []store.Replica{
		{
			ID:            "replica-local",
			AssetID:       asset.ID,
			EndpointID:    localEndpoint.ID,
			PhysicalPath:  `D:\Media\Projects\clip.mp4`,
			ReplicaStatus: string(ReplicaStatusActive),
			ExistsFlag:    true,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "replica-qnap",
			AssetID:       asset.ID,
			EndpointID:    qnapEndpoint.ID,
			PhysicalPath:  `\\qnap\share\Media\Projects\clip.mp4`,
			ReplicaStatus: string(ReplicaStatusActive),
			ExistsFlag:    true,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	} {
		if err := dataStore.CreateReplica(ctx, replica); err != nil {
			t.Fatalf("create replica %s: %v", replica.ID, err)
		}
	}

	targetEndpointIDs, err := json.Marshal([]string{localEndpoint.ID, qnapEndpoint.ID})
	if err != nil {
		t.Fatalf("marshal import rule targets: %v", err)
	}
	if err := dataStore.ReplaceImportRules(ctx, []store.ImportRule{
		{
			ID:                "rule-1",
			RuleType:          "media_type",
			MatchValue:        "video",
			TargetEndpointIDs: string(targetEndpointIDs),
			CreatedAt:         now,
			UpdatedAt:         now,
		},
	}); err != nil {
		t.Fatalf("create import rule: %v", err)
	}

	service := NewService(dataStore, func(endpoint store.StorageEndpoint) (connectors.Connector, error) {
		return &fakeConnector{
			descriptor: connectors.Descriptor{
				Name:     endpoint.Name,
				RootPath: endpoint.RootPath,
			},
		}, nil
	})

	summary, err := service.DeleteEndpoint(ctx, localEndpoint.ID)
	if err != nil {
		t.Fatalf("delete endpoint: %v", err)
	}

	if summary.RemovedReplicaCount != 1 {
		t.Fatalf("expected 1 removed replica, got %d", summary.RemovedReplicaCount)
	}
	if summary.UpdatedImportRuleCount != 1 {
		t.Fatalf("expected 1 updated import rule, got %d", summary.UpdatedImportRuleCount)
	}

	if _, err := dataStore.GetStorageEndpointByID(ctx, localEndpoint.ID); err == nil {
		t.Fatal("expected deleted endpoint to be removed from storage_endpoints")
	}

	replicas, err := dataStore.ListReplicasByAssetID(ctx, asset.ID)
	if err != nil {
		t.Fatalf("list remaining replicas: %v", err)
	}
	if len(replicas) != 1 || replicas[0].EndpointID != qnapEndpoint.ID {
		t.Fatalf("expected only qnap replica to remain, got %+v", replicas)
	}

	storedAsset, err := dataStore.GetAssetByID(ctx, asset.ID)
	if err != nil {
		t.Fatalf("load stored asset: %v", err)
	}
	if storedAsset.AssetStatus != string(AssetStatusReady) {
		t.Fatalf("expected asset to remain ready with one surviving replica, got %s", storedAsset.AssetStatus)
	}

	rules, err := dataStore.ListImportRules(ctx)
	if err != nil {
		t.Fatalf("list import rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 import rule to remain, got %d", len(rules))
	}

	var remainingTargets []string
	if err := json.Unmarshal([]byte(rules[0].TargetEndpointIDs), &remainingTargets); err != nil {
		t.Fatalf("decode remaining import rule targets: %v", err)
	}
	if len(remainingTargets) != 1 || remainingTargets[0] != qnapEndpoint.ID {
		t.Fatalf("expected qnap to remain as the only import target, got %#v", remainingTargets)
	}
}

func TestRegisterEndpointStoresCloudCredentialOutsideLibraryDB(t *testing.T) {
	credentialRoot := t.TempDir()
	t.Setenv("MAM_CREDENTIAL_VAULT_DIR", credentialRoot)

	dbPath := filepath.Join(t.TempDir(), "catalog.db")
	dataStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	defer func() {
		_ = dataStore.Close()
	}()

	service := NewService(dataStore, func(endpoint store.StorageEndpoint) (connectors.Connector, error) {
		return &fakeConnector{
			descriptor: connectors.Descriptor{
				Name:     endpoint.Name,
				Type:     connectors.EndpointTypeCloud115,
				RootPath: endpoint.RootPath,
			},
		}, nil
	})

	record, err := service.RegisterEndpoint(context.Background(), RegisterEndpointRequest{
		Name:               "115 Cloud",
		EndpointType:       string(connectors.EndpointTypeCloud115),
		RootPath:           "0",
		RoleMode:           "MANAGED",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   json.RawMessage(`{"rootId":"0","accessToken":"token-123","appType":"windows"}`),
	})
	if err != nil {
		t.Fatalf("register endpoint: %v", err)
	}

	if !record.HasCredential {
		t.Fatal("expected cloud endpoint to report a stored local credential")
	}
	if strings.TrimSpace(record.CredentialRef) == "" {
		t.Fatal("expected cloud endpoint to expose a credential ref")
	}

	storedEndpoint, err := dataStore.GetStorageEndpointByID(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("load stored endpoint: %v", err)
	}
	if strings.Contains(strings.ToLower(storedEndpoint.ConnectionConfig), "token-123") {
		t.Fatalf("expected library db to exclude raw credential secret, got %s", storedEndpoint.ConnectionConfig)
	}
	if strings.TrimSpace(storedEndpoint.CredentialRef) == "" {
		t.Fatal("expected stored endpoint to keep a credential ref")
	}
	if !strings.Contains(storedEndpoint.ConnectionConfig, `"rootId":"0"`) {
		t.Fatalf("expected stored connection config to keep portable metadata, got %s", storedEndpoint.ConnectionConfig)
	}
}

type fakeConnectorFactory struct {
	connectorsByEndpoint map[string]*fakeConnector
}

func (factory *fakeConnectorFactory) Build(endpoint store.StorageEndpoint) (connectors.Connector, error) {
	connector, ok := factory.connectorsByEndpoint[endpoint.ID]
	if !ok {
		return nil, errors.New("connector not found")
	}
	return connector, nil
}

type fakeConnector struct {
	descriptor   connectors.Descriptor
	listing      map[string][]connectors.FileEntry
	deleteErr    error
	deletedPaths []string
}

func (connector *fakeConnector) Descriptor() connectors.Descriptor {
	return connector.descriptor
}

func (connector *fakeConnector) HealthCheck(context.Context) (connectors.HealthStatus, error) {
	return connectors.HealthStatusReady, nil
}

func (connector *fakeConnector) ListEntries(_ context.Context, request connectors.ListEntriesRequest) ([]connectors.FileEntry, error) {
	return connector.listing[request.Path], nil
}

func (connector *fakeConnector) StatEntry(context.Context, string) (connectors.FileEntry, error) {
	return connectors.FileEntry{}, nil
}

func (connector *fakeConnector) ReadStream(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func (connector *fakeConnector) CopyIn(context.Context, string, io.Reader) (connectors.FileEntry, error) {
	return connectors.FileEntry{}, nil
}

func (connector *fakeConnector) CopyOut(context.Context, string, io.Writer) error {
	return nil
}

func (connector *fakeConnector) DeleteEntry(_ context.Context, path string) error {
	connector.deletedPaths = append(connector.deletedPaths, path)
	return connector.deleteErr
}

func (connector *fakeConnector) RenameEntry(context.Context, string, string) (connectors.FileEntry, error) {
	return connectors.FileEntry{}, nil
}

func (connector *fakeConnector) MoveEntry(context.Context, string, string) (connectors.FileEntry, error) {
	return connectors.FileEntry{}, nil
}

func (connector *fakeConnector) MakeDirectory(context.Context, string) (connectors.FileEntry, error) {
	return connectors.FileEntry{}, nil
}

func fileEntry(path string, relativePath string, mediaType connectors.MediaType, modifiedAt time.Time) connectors.FileEntry {
	mtime := modifiedAt.UTC()
	return connectors.FileEntry{
		Path:         path,
		RelativePath: relativePath,
		Name:         filepath.Base(relativePath),
		Kind:         connectors.EntryKindFile,
		MediaType:    mediaType,
		Size:         1024,
		ModifiedAt:   &mtime,
		IsDir:        false,
	}
}
