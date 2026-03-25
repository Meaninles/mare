package catalog

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"mam/backend/internal/store"
)

type fakeInsightsSearchBridge struct {
	descriptionOutput SearchSemanticDescriptionOutput
}

func (bridge *fakeInsightsSearchBridge) Transcribe(context.Context, string, string, string) (SearchTranscriptOutput, error) {
	return SearchTranscriptOutput{}, nil
}

func (bridge *fakeInsightsSearchBridge) EmbedImage(context.Context, string) (SearchEmbeddingOutput, error) {
	return SearchEmbeddingOutput{}, nil
}

func (bridge *fakeInsightsSearchBridge) EmbedVideo(context.Context, string, string) (SearchEmbeddingOutput, error) {
	return SearchEmbeddingOutput{}, nil
}

func (bridge *fakeInsightsSearchBridge) EmbedText(context.Context, string) (SearchEmbeddingOutput, error) {
	return SearchEmbeddingOutput{}, nil
}

func (bridge *fakeInsightsSearchBridge) DescribeVector(
	context.Context,
	[]float64,
	[]SearchSemanticPrompt,
	int,
) (SearchSemanticDescriptionOutput, error) {
	return bridge.descriptionOutput, nil
}

func TestGetAssetInsightsReturnsTranscriptAndSemanticLabels(t *testing.T) {
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
		ID:             "asset-insights-1",
		LogicalPathKey: "projects/demo/clip.mp4",
		DisplayName:    "clip.mp4",
		MediaType:      "video",
		AssetStatus:    "ready",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := dataStore.CreateAsset(ctx, asset); err != nil {
		t.Fatalf("create asset: %v", err)
	}

	if err := dataStore.SaveAssetTranscript(ctx, store.AssetTranscript{
		AssetID:        asset.ID,
		TranscriptText: "这是一次产品演示录屏。",
		Language:       stringPointer("zh"),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("save asset transcript: %v", err)
	}

	vectorJSON, err := json.Marshal([]float64{0.3, 0.4, 0.5})
	if err != nil {
		t.Fatalf("marshal embedding json: %v", err)
	}
	if err := dataStore.SaveAssetSemanticEmbedding(ctx, store.AssetSemanticEmbedding{
		ID:            "embedding-1",
		AssetID:       asset.ID,
		FeatureKind:   semanticFeatureKindVideo,
		ModelName:     "openai/clip-vit-base-patch32",
		EmbeddingJSON: string(vectorJSON),
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("save asset semantic embedding: %v", err)
	}

	service := NewService(
		dataStore,
		nil,
		WithAutoQueueSearchJobs(false),
		WithSearchBridge(&fakeInsightsSearchBridge{
			descriptionOutput: SearchSemanticDescriptionOutput{
				ModelName: "openai/clip-vit-base-patch32",
				Labels: []SearchSemanticLabel{
					{Label: "会议演讲", Score: 0.8123},
					{Label: "屏幕截图", Score: 0.5531},
				},
			},
		}),
	)

	insights, err := service.GetAssetInsights(ctx, asset.ID)
	if err != nil {
		t.Fatalf("get asset insights: %v", err)
	}
	if insights.Transcript == nil {
		t.Fatal("expected transcript insights to be available")
	}
	if insights.Transcript.Language == nil || *insights.Transcript.Language != "zh" {
		t.Fatalf("expected transcript language zh, got %+v", insights.Transcript.Language)
	}
	if insights.Semantic == nil {
		t.Fatal("expected semantic insights to be available")
	}
	if insights.Semantic.FeatureKind != semanticFeatureKindVideo {
		t.Fatalf("expected semantic feature kind %s, got %s", semanticFeatureKindVideo, insights.Semantic.FeatureKind)
	}
	if insights.Semantic.Dimensions != 3 {
		t.Fatalf("expected semantic dimensions 3, got %d", insights.Semantic.Dimensions)
	}
	if len(insights.Semantic.Labels) != 2 {
		t.Fatalf("expected 2 semantic labels, got %d", len(insights.Semantic.Labels))
	}
	if insights.Semantic.Labels[0].Label != "会议演讲" {
		t.Fatalf("expected top semantic label 会议演讲, got %s", insights.Semantic.Labels[0].Label)
	}
}
