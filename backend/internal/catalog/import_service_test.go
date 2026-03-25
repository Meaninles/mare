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

func TestListImportDevicesRecognizesManagedRemovableDriveAcrossMountChanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataStore := newTestStore(t)
	now := time.Now().UTC().Round(time.Second)

	managedDevice := connectors.DeviceInfo{
		MountPoint:         `E:\`,
		VolumeLabel:        "Travel SSD",
		FileSystem:         "exFAT",
		VolumeSerialNumber: "VOL-123",
		InterfaceType:      "USB",
		Model:              "Portable SSD",
		PNPDeviceID:        "USBSTOR\\DISK&VEN_FAKE",
	}
	identitySignature := connectors.GenerateDeviceIdentity(managedDevice)

	endpoint := store.StorageEndpoint{
		ID:                 "endpoint-removable-1",
		Name:               "旅行素材盘",
		EndpointType:       string(connectors.EndpointTypeRemovable),
		RootPath:           managedDevice.MountPoint,
		RoleMode:           "MANAGED",
		IdentitySignature:  identitySignature,
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   mustJSONText(t, map[string]any{"device": managedDevice}),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := dataStore.CreateStorageEndpoint(ctx, endpoint); err != nil {
		t.Fatalf("create managed removable endpoint: %v", err)
	}

	reinsertedDevice := managedDevice
	reinsertedDevice.MountPoint = `F:\`

	service := NewService(dataStore, nil, WithAutoQueueDerivedMedia(false))
	service.removableEnumerator = &stubCatalogDeviceEnumerator{
		devices: []connectors.DeviceInfo{reinsertedDevice},
	}

	devices, err := service.ListImportDevices(ctx)
	if err != nil {
		t.Fatalf("list import devices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 import device, got %d", len(devices))
	}

	record := devices[0]
	if record.IdentitySignature != identitySignature {
		t.Fatalf("expected same identity signature after mount change, got %s", record.IdentitySignature)
	}
	if record.KnownEndpoint == nil {
		t.Fatal("expected known endpoint match for reinserted removable drive")
	}
	if record.KnownEndpoint.ID != endpoint.ID {
		t.Fatalf("expected endpoint %s, got %s", endpoint.ID, record.KnownEndpoint.ID)
	}
	if record.SuggestedRole != deviceRoleManagedStorage {
		t.Fatalf("expected suggested role %s, got %s", deviceRoleManagedStorage, record.SuggestedRole)
	}
}

func TestSelectImportDeviceRoleRegistersManagedStorageAndTracksImportSource(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataStore := newTestStore(t)
	service := NewService(dataStore, nil, WithAutoQueueDerivedMedia(false))

	device := connectors.DeviceInfo{
		MountPoint:         `E:\`,
		VolumeLabel:        "T7",
		FileSystem:         "exFAT",
		VolumeSerialNumber: "VOL-T7",
		InterfaceType:      "USB",
		Model:              "Portable SSD T7",
		PNPDeviceID:        "USBSTOR\\T7",
	}
	identitySignature := connectors.GenerateDeviceIdentity(device)
	service.removableEnumerator = &stubCatalogDeviceEnumerator{
		devices: []connectors.DeviceInfo{device},
	}

	managedResult, err := service.SelectImportDeviceRole(ctx, SelectImportDeviceRoleRequest{
		IdentitySignature: identitySignature,
		Role:              deviceRoleManagedStorage,
		Name:              "T7",
	})
	if err != nil {
		t.Fatalf("select managed storage role: %v", err)
	}
	if managedResult.Role != deviceRoleManagedStorage {
		t.Fatalf("expected managed storage role, got %s", managedResult.Role)
	}
	if managedResult.Endpoint == nil {
		t.Fatal("expected managed storage selection to create/update endpoint")
	}
	if managedResult.Device.CurrentSessionRole != deviceRoleManagedStorage {
		t.Fatalf("expected current session role %s, got %s", deviceRoleManagedStorage, managedResult.Device.CurrentSessionRole)
	}

	storedEndpoint, err := dataStore.GetStorageEndpointByID(ctx, managedResult.Endpoint.ID)
	if err != nil {
		t.Fatalf("load registered managed endpoint: %v", err)
	}
	if storedEndpoint.IdentitySignature != identitySignature {
		t.Fatalf("expected stored identity %s, got %s", identitySignature, storedEndpoint.IdentitySignature)
	}

	importSourceResult, err := service.SelectImportDeviceRole(ctx, SelectImportDeviceRoleRequest{
		IdentitySignature: identitySignature,
		Role:              deviceRoleImportSource,
	})
	if err != nil {
		t.Fatalf("select import source role: %v", err)
	}
	if importSourceResult.Role != deviceRoleImportSource {
		t.Fatalf("expected import source role, got %s", importSourceResult.Role)
	}
	if importSourceResult.Endpoint != nil {
		t.Fatal("did not expect import source selection to create a new endpoint payload")
	}
	if importSourceResult.Device.CurrentSessionRole != deviceRoleImportSource {
		t.Fatalf("expected current session role %s, got %s", deviceRoleImportSource, importSourceResult.Device.CurrentSessionRole)
	}
}

func TestBrowseImportSourceReturnsFilteredEntries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataStore := newTestStore(t)
	service := NewService(dataStore, nil, WithAutoQueueDerivedMedia(false))

	sourceRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sourceRoot, "DCIM"), 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "DCIM", "A001.JPG"), []byte("image"), 0o644); err != nil {
		t.Fatalf("write image source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "DCIM", "A001.MOV"), []byte("video"), 0o644); err != nil {
		t.Fatalf("write video source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "DCIM", "README.txt"), []byte("notes"), 0o644); err != nil {
		t.Fatalf("write non-media source: %v", err)
	}

	device := connectors.DeviceInfo{
		MountPoint:         sourceRoot,
		VolumeLabel:        "Camera Card",
		FileSystem:         "exFAT",
		VolumeSerialNumber: "CARD-001",
		InterfaceType:      "USB",
		Model:              "Camera Reader",
		PNPDeviceID:        "USBSTOR\\CARD",
	}
	identitySignature := connectors.GenerateDeviceIdentity(device)
	service.removableEnumerator = &stubCatalogDeviceEnumerator{
		devices: []connectors.DeviceInfo{device},
	}

	result, err := service.BrowseImportSource(ctx, ImportSourceBrowseRequest{
		IdentitySignature: identitySignature,
		MediaType:         string(connectors.MediaTypeImage),
	})
	if err != nil {
		t.Fatalf("browse import source: %v", err)
	}
	if result.Device.IdentitySignature != identitySignature {
		t.Fatalf("expected identity %s, got %s", identitySignature, result.Device.IdentitySignature)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 filtered media entry, got %d", len(result.Entries))
	}
	if result.Entries[0].MediaType != string(connectors.MediaTypeImage) {
		t.Fatalf("expected image entry, got %s", result.Entries[0].MediaType)
	}
	if result.Entries[0].RelativePath != "DCIM/A001.JPG" && result.Entries[0].RelativePath != "dcim/a001.jpg" {
		// RelativePath should preserve source-relative layout and stay media-only.
		t.Fatalf("unexpected relative path: %s", result.Entries[0].RelativePath)
	}
}

func TestExecuteImportCopiesFilesToMatchedEndpointsAndUpdatesCatalog(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataStore := newTestStore(t)
	now := time.Now().UTC().Round(time.Second)

	sourceRoot := t.TempDir()
	photoPath := filepath.Join(sourceRoot, "DCIM", "Shot01.jpg")
	if err := os.MkdirAll(filepath.Dir(photoPath), 0o755); err != nil {
		t.Fatalf("create import source directory: %v", err)
	}
	if err := os.WriteFile(photoPath, []byte("image-data"), 0o644); err != nil {
		t.Fatalf("write import source file: %v", err)
	}

	device := connectors.DeviceInfo{
		MountPoint:         sourceRoot,
		VolumeLabel:        "Camera Card",
		FileSystem:         "exFAT",
		VolumeSerialNumber: "CAM-001",
		InterfaceType:      "USB",
		Model:              "SD Reader",
		PNPDeviceID:        "USBSTOR\\CAMERA",
	}
	identitySignature := connectors.GenerateDeviceIdentity(device)

	localTargetRoot := filepath.Join(t.TempDir(), "library-local")
	qnapTargetRoot := filepath.Join(t.TempDir(), "library-qnap")

	endpoints := []store.StorageEndpoint{
		{
			ID:                 "endpoint-local",
			Name:               "本地图库",
			EndpointType:       string(connectors.EndpointTypeLocal),
			RootPath:           localTargetRoot,
			RoleMode:           "MANAGED",
			IdentitySignature:  "local-library",
			AvailabilityStatus: "AVAILABLE",
			ConnectionConfig:   mustJSONText(t, map[string]string{"rootPath": localTargetRoot}),
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			ID:                 "endpoint-qnap",
			Name:               "QNAP 归档",
			EndpointType:       string(connectors.EndpointTypeQNAP),
			RootPath:           qnapTargetRoot,
			RoleMode:           "MANAGED",
			IdentitySignature:  "qnap-library",
			AvailabilityStatus: "AVAILABLE",
			ConnectionConfig:   mustJSONText(t, map[string]string{"sharePath": qnapTargetRoot}),
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}
	for _, endpoint := range endpoints {
		if err := dataStore.CreateStorageEndpoint(ctx, endpoint); err != nil {
			t.Fatalf("create endpoint %s: %v", endpoint.ID, err)
		}
	}

	service := NewService(dataStore, nil, WithAutoQueueDerivedMedia(false))
	service.removableEnumerator = &stubCatalogDeviceEnumerator{
		devices: []connectors.DeviceInfo{device},
	}

	if _, err := service.SaveImportRules(ctx, SaveImportRulesRequest{
		Rules: []ImportRuleInput{
			{
				RuleType:          importRuleTypeMediaType,
				MatchValue:        string(connectors.MediaTypeImage),
				TargetEndpointIDs: []string{"endpoint-local", "endpoint-qnap"},
			},
		},
	}); err != nil {
		t.Fatalf("save import rules: %v", err)
	}

	summary, err := service.ExecuteImport(ctx, ExecuteImportRequest{
		IdentitySignature: identitySignature,
		EntryPaths:        []string{"DCIM/Shot01.jpg"},
	})
	if err != nil {
		t.Fatalf("execute import: %v", err)
	}

	if summary.Status != taskStatusSuccess {
		t.Fatalf("expected success summary status, got %s", summary.Status)
	}
	if summary.SuccessCount != 1 || summary.FailedCount != 0 || summary.PartialCount != 0 {
		t.Fatalf("unexpected import counters: %+v", summary)
	}
	if len(summary.Items) != 1 {
		t.Fatalf("expected 1 import item, got %d", len(summary.Items))
	}
	if len(summary.Items[0].TargetResults) != 2 {
		t.Fatalf("expected 2 target results, got %d", len(summary.Items[0].TargetResults))
	}

	for _, targetRoot := range []string{localTargetRoot, qnapTargetRoot} {
		targetFile := filepath.Join(targetRoot, "DCIM", "Shot01.jpg")
		if _, err := os.Stat(targetFile); err != nil {
			t.Fatalf("expected imported file at %s: %v", targetFile, err)
		}
	}

	assets, err := service.ListAssets(ctx, 20, 0)
	if err != nil {
		t.Fatalf("list assets after import: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("expected 1 imported asset, got %d", len(assets))
	}
	if assets[0].LogicalPathKey != "dcim/shot01.jpg" {
		t.Fatalf("expected logical path key dcim/shot01.jpg, got %s", assets[0].LogicalPathKey)
	}
	if assets[0].AvailableReplicaCount != 2 {
		t.Fatalf("expected 2 available replicas, got %d", assets[0].AvailableReplicaCount)
	}

	tasks, err := dataStore.ListTasks(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 import task, got %d", len(tasks))
	}
	if tasks[0].TaskType != taskTypeImportExecute {
		t.Fatalf("expected task type %s, got %s", taskTypeImportExecute, tasks[0].TaskType)
	}
	if tasks[0].Status != taskStatusSuccess {
		t.Fatalf("expected import task success, got %s", tasks[0].Status)
	}
}

func TestExecuteImportQueuesVideoCoverTaskImmediately(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataStore := newTestStore(t)
	now := time.Now().UTC().Round(time.Second)

	sourceRoot := t.TempDir()
	videoPath := filepath.Join(sourceRoot, "DCIM", "Clip01.mp4")
	if err := os.MkdirAll(filepath.Dir(videoPath), 0o755); err != nil {
		t.Fatalf("create import source directory: %v", err)
	}
	if err := os.WriteFile(videoPath, []byte("fake-video-data"), 0o644); err != nil {
		t.Fatalf("write import source file: %v", err)
	}

	device := connectors.DeviceInfo{
		MountPoint:         sourceRoot,
		VolumeLabel:        "Camera Card",
		FileSystem:         "exFAT",
		VolumeSerialNumber: "CAM-VIDEO-001",
		InterfaceType:      "USB",
		Model:              "SD Reader",
		PNPDeviceID:        "USBSTOR\\CAMERA-VIDEO",
	}
	identitySignature := connectors.GenerateDeviceIdentity(device)

	localTargetRoot := filepath.Join(t.TempDir(), "library-local")
	endpoint := store.StorageEndpoint{
		ID:                 "endpoint-local-video",
		Name:               "本地图库",
		EndpointType:       string(connectors.EndpointTypeLocal),
		RootPath:           localTargetRoot,
		RoleMode:           "MANAGED",
		IdentitySignature:  "local-library-video",
		AvailabilityStatus: "AVAILABLE",
		ConnectionConfig:   mustJSONText(t, map[string]string{"rootPath": localTargetRoot}),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := dataStore.CreateStorageEndpoint(ctx, endpoint); err != nil {
		t.Fatalf("create endpoint %s: %v", endpoint.ID, err)
	}

	service := NewService(dataStore, nil, WithAutoQueueDerivedMedia(true))
	service.removableEnumerator = &stubCatalogDeviceEnumerator{
		devices: []connectors.DeviceInfo{device},
	}

	if _, err := service.SaveImportRules(ctx, SaveImportRulesRequest{
		Rules: []ImportRuleInput{
			{
				RuleType:          importRuleTypeMediaType,
				MatchValue:        string(connectors.MediaTypeVideo),
				TargetEndpointIDs: []string{endpoint.ID},
			},
		},
	}); err != nil {
		t.Fatalf("save import rules: %v", err)
	}

	summary, err := service.ExecuteImport(ctx, ExecuteImportRequest{
		IdentitySignature: identitySignature,
		EntryPaths:        []string{"DCIM/Clip01.mp4"},
	})
	if err != nil {
		t.Fatalf("execute import: %v", err)
	}
	if summary.Status != taskStatusSuccess {
		t.Fatalf("expected success summary status, got %s", summary.Status)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		tasks, listErr := dataStore.ListTasks(ctx, 20, 0)
		if listErr != nil {
			t.Fatalf("list tasks: %v", listErr)
		}

		for _, task := range tasks {
			if task.TaskType == mediaTaskVideoCover {
				return
			}
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("expected import to queue a video_cover task without requiring asset list access")
}

type stubCatalogDeviceEnumerator struct {
	devices []connectors.DeviceInfo
}

func (enumerator *stubCatalogDeviceEnumerator) ListDevices(context.Context) ([]connectors.DeviceInfo, error) {
	return enumerator.devices, nil
}
