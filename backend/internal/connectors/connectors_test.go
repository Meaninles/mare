package connectors

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalConnectorListsMediaEntries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "photo.jpg"), []byte("jpg"))
	writeTestFile(t, filepath.Join(root, "clip.mp4"), []byte("mp4"))
	writeTestFile(t, filepath.Join(root, "notes.txt"), []byte("txt"))

	connector, err := NewLocalConnector(LocalConfig{Name: "Local Test", RootPath: root})
	if err != nil {
		t.Fatalf("create local connector: %v", err)
	}

	status, err := connector.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	if status != HealthStatusReady {
		t.Fatalf("expected ready, got %s", status)
	}

	entries, err := connector.ListEntries(context.Background(), ListEntriesRequest{
		Recursive: true,
		MediaOnly: true,
	})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 media entries, got %d", len(entries))
	}

	entry, err := connector.StatEntry(context.Background(), "photo.jpg")
	if err != nil {
		t.Fatalf("stat entry: %v", err)
	}
	if entry.MediaType != MediaTypeImage {
		t.Fatalf("expected image media type, got %s", entry.MediaType)
	}

	var buffer bytes.Buffer
	if err := connector.CopyOut(context.Background(), "clip.mp4", &buffer); err != nil {
		t.Fatalf("copy out: %v", err)
	}
	if buffer.String() != "mp4" {
		t.Fatalf("unexpected copy out content: %s", buffer.String())
	}

	createdDir, err := connector.MakeDirectory(context.Background(), "nested/folder")
	if err != nil {
		t.Fatalf("make directory: %v", err)
	}
	if !createdDir.IsDir {
		t.Fatal("expected created directory entry")
	}

	renamedEntry, err := connector.RenameEntry(context.Background(), "photo.jpg", "renamed.jpg")
	if err != nil {
		t.Fatalf("rename entry: %v", err)
	}
	if renamedEntry.Name != "renamed.jpg" {
		t.Fatalf("expected renamed.jpg, got %s", renamedEntry.Name)
	}

	movedEntry, err := connector.MoveEntry(context.Background(), "renamed.jpg", "nested/folder/moved.jpg")
	if err != nil {
		t.Fatalf("move entry: %v", err)
	}
	if movedEntry.RelativePath != "nested/folder/moved.jpg" {
		t.Fatalf("unexpected moved path: %s", movedEntry.RelativePath)
	}

	absoluteEntry, err := connector.StatEntry(context.Background(), filepath.Join(root, "nested", "folder", "moved.jpg"))
	if err != nil {
		t.Fatalf("stat entry with absolute path: %v", err)
	}
	if absoluteEntry.Name != "moved.jpg" {
		t.Fatalf("expected moved.jpg via absolute lookup, got %s", absoluteEntry.Name)
	}

	if err := connector.DeleteEntry(context.Background(), filepath.Join(root, "nested", "folder", "moved.jpg")); err != nil {
		t.Fatalf("delete entry with absolute path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "nested", "folder", "moved.jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected deleted file to be removed, got %v", err)
	}
}

func TestQNAPConnectorReportsMissingShare(t *testing.T) {
	t.Parallel()

	connector, err := NewQNAPConnector(QNAPConfig{Name: "QNAP", SharePath: `Z:\missing-share`})
	if err != nil {
		t.Fatalf("create qnap connector: %v", err)
	}

	status, healthErr := connector.HealthCheck(context.Background())
	if healthErr == nil {
		t.Fatal("expected health check error for missing share")
	}
	if status != HealthStatusOffline {
		t.Fatalf("expected offline, got %s", status)
	}

	var connectorErr *ConnectorError
	if !errors.As(healthErr, &connectorErr) {
		t.Fatalf("expected connector error, got %T", healthErr)
	}
	if connectorErr.Connector != EndpointTypeQNAP {
		t.Fatalf("expected qnap connector type, got %s", connectorErr.Connector)
	}
}

func TestLocalConnectorSkipsWindowsSystemArtifacts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "clips", "keep.mp4"), []byte("video"))
	writeTestFile(t, filepath.Join(root, "$RECYCLE.BIN", "$I123456.mp4"), []byte("metadata"))
	writeTestFile(t, filepath.Join(root, "System Volume Information", "ghost.mp4"), []byte("ghost"))

	connector, err := NewLocalConnector(LocalConfig{Name: "Local Test", RootPath: root})
	if err != nil {
		t.Fatalf("create local connector: %v", err)
	}

	entries, err := connector.ListEntries(context.Background(), ListEntriesRequest{
		Recursive: true,
		MediaOnly: true,
	})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected only 1 media entry after filtering system artifacts, got %d", len(entries))
	}
	if entries[0].RelativePath != "clips/keep.mp4" {
		t.Fatalf("expected clips/keep.mp4, got %s", entries[0].RelativePath)
	}
}

func TestRemovableDetectorEmitsInsertAndRemove(t *testing.T) {
	t.Parallel()

	enumerator := &fakeDeviceEnumerator{
		snapshots: [][]DeviceInfo{
			{{MountPoint: `E:\`, VolumeSerialNumber: "123", FileSystem: "exfat", VolumeLabel: "DroneCard"}},
			{},
		},
	}

	detector := NewRemovableDetector(enumerator, 20*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	events := detector.Start(ctx)
	collected := make([]DeviceEvent, 0, 2)
	for event := range events {
		collected = append(collected, event)
	}

	if len(collected) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(collected))
	}
	if collected[0].Type != DeviceEventInserted {
		t.Fatalf("expected first event inserted, got %s", collected[0].Type)
	}
	if collected[1].Type != DeviceEventRemoved {
		t.Fatalf("expected second event removed, got %s", collected[1].Type)
	}
}

func TestRemovableConnectorIdentityStable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "audio.wav"), []byte("audio"))

	device := DeviceInfo{
		MountPoint:         root,
		VolumeLabel:        "MediaBackup",
		FileSystem:         "NTFS",
		VolumeSerialNumber: "ABC123",
		Model:              "USB SSD",
		PNPDeviceID:        "USBSTOR\\DISK",
	}

	firstIdentity := GenerateDeviceIdentity(device)
	secondIdentity := GenerateDeviceIdentity(device)
	if firstIdentity != secondIdentity {
		t.Fatal("expected stable removable identity signature")
	}

	connector, err := NewRemovableConnector(RemovableConfig{Device: device})
	if err != nil {
		t.Fatalf("create removable connector: %v", err)
	}

	entries, err := connector.ListEntries(context.Background(), ListEntriesRequest{Recursive: true, MediaOnly: true})
	if err != nil {
		t.Fatalf("list removable entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 removable media entry, got %d", len(entries))
	}
}

type fakeDeviceEnumerator struct {
	snapshots [][]DeviceInfo
	index     int
}

func (enumerator *fakeDeviceEnumerator) ListDevices(context.Context) ([]DeviceInfo, error) {
	if enumerator.index >= len(enumerator.snapshots) {
		return []DeviceInfo{}, nil
	}
	devices := enumerator.snapshots[enumerator.index]
	enumerator.index++
	return devices, nil
}

func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create test directory: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}
