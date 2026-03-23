package catalog

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
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
	if len(assetsAfterRescan) != 3 {
		t.Fatalf("expected 3 assets after rescan, got %d", len(assetsAfterRescan))
	}

	var missingAssetFound bool
	for _, asset := range assetsAfterRescan {
		if asset.LogicalPathKey == "audio/meeting.wav" {
			missingAssetFound = true
			if asset.AssetStatus != "missing" {
				t.Fatalf("expected removed asset status missing, got %s", asset.AssetStatus)
			}
		}
	}
	if !missingAssetFound {
		t.Fatal("expected removed asset to remain in catalog with missing status")
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
	if assetsAfterFullScan[0].AvailableReplicaCount != 3 {
		t.Fatalf("expected 3 available replicas after full scan, got %d", assetsAfterFullScan[0].AvailableReplicaCount)
	}
	if assetsAfterFullScan[0].MissingReplicaCount != 0 {
		t.Fatalf("expected 0 missing replicas after full scan, got %d", assetsAfterFullScan[0].MissingReplicaCount)
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
	descriptor connectors.Descriptor
	listing    map[string][]connectors.FileEntry
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

func (connector *fakeConnector) DeleteEntry(context.Context, string) error {
	return nil
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
