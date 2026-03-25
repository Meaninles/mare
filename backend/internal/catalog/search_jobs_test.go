package catalog

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mam/backend/internal/store"
)

func TestShouldAutoQueueSearchTaskStopsAfterFailedTask(t *testing.T) {
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

	task := store.Task{
		ID:         "search-task-1",
		TaskType:   taskTypeVideoSemantic,
		Status:     taskStatusFailed,
		Payload:    `{"assetId":"asset-1","taskType":"video_semantic"}`,
		CreatedAt:  now,
		UpdatedAt:  now,
		FinishedAt: &now,
	}
	if err := dataStore.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	service := NewService(dataStore, nil)
	if service.shouldAutoQueueSearchTask(ctx, "asset-1", taskTypeVideoSemantic) {
		t.Fatal("expected failed search task to block automatic requeue")
	}
}

func TestSaveTranscriptResultAllowsEmptyTranscript(t *testing.T) {
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

	asset := store.Asset{
		ID:             "asset-empty-transcript",
		LogicalPathKey: "demo/clip.mp4",
		DisplayName:    "clip.mp4",
		MediaType:      "video",
		AssetStatus:    "ready",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := dataStore.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("create asset: %v", err)
	}

	service := NewService(dataStore, nil, WithAutoQueueSearchJobs(false))
	summary, err := service.saveTranscriptResult(ctx, asset, nil, "", nil, "tiny", "视频没有可转写音轨")
	if err != nil {
		t.Fatalf("save transcript result: %v", err)
	}
	if !strings.Contains(summary, "视频没有可转写音轨") {
		t.Fatalf("expected summary to include warning, got %s", summary)
	}

	transcript, err := dataStore.GetAssetTranscriptByAssetID(ctx, asset.ID)
	if err != nil {
		t.Fatalf("get asset transcript: %v", err)
	}
	if transcript.TranscriptText != "" {
		t.Fatalf("expected empty transcript text, got %q", transcript.TranscriptText)
	}

	document, err := dataStore.GetAssetSearchDocumentByAssetAndKind(ctx, asset.ID, searchDocumentKindTranscript)
	if err != nil {
		t.Fatalf("get transcript search document: %v", err)
	}
	if document.Content != "" {
		t.Fatalf("expected empty transcript search document, got %q", document.Content)
	}
}
