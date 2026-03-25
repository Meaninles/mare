package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"mam/backend/internal/connectors"
	"mam/backend/internal/store"
)

const (
	searchDocumentKindTranscript = "transcript"

	semanticFeatureKindImage = "image"
	semanticFeatureKindVideo = "video"

	taskTypeAudioTranscript = "audio_transcript"
	taskTypeVideoTranscript = "video_transcript"
	taskTypeImageSemantic  = "image_semantic"
	taskTypeVideoSemantic  = "video_semantic"
)

func (service *Service) maybeQueueSearchFeatures(asset store.Asset, replicas []store.Replica) {
	if !service.autoQueueSearchJobs {
		return
	}

	backgroundCtx := context.Background()

	switch strings.ToLower(strings.TrimSpace(asset.MediaType)) {
	case string(connectors.MediaTypeAudio):
		if service.needsTranscriptGeneration(backgroundCtx, asset.ID, replicas) {
			service.queueSearchJob(asset.ID, taskTypeAudioTranscript)
		}
	case string(connectors.MediaTypeVideo):
		if service.needsTranscriptGeneration(backgroundCtx, asset.ID, replicas) {
			service.queueSearchJob(asset.ID, taskTypeVideoTranscript)
		}
		if service.needsSemanticEmbeddingGeneration(backgroundCtx, asset.ID, replicas, semanticFeatureKindVideo) {
			service.queueSearchJob(asset.ID, taskTypeVideoSemantic)
		}
	case string(connectors.MediaTypeImage):
		if service.needsSemanticEmbeddingGeneration(backgroundCtx, asset.ID, replicas, semanticFeatureKindImage) {
			service.queueSearchJob(asset.ID, taskTypeImageSemantic)
		}
	}
}

func (service *Service) needsTranscriptGeneration(ctx context.Context, assetID string, replicas []store.Replica) bool {
	candidate, err := service.selectReadableReplica(ctx, replicas)
	if err != nil || candidate == nil {
		return false
	}

	transcript, err := service.store.GetAssetTranscriptByAssetID(ctx, assetID)
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	if err != nil {
		return false
	}

	return !sameNullableString(transcript.SourceVersionID, candidate.replica.VersionID)
}

func (service *Service) needsSemanticEmbeddingGeneration(
	ctx context.Context,
	assetID string,
	replicas []store.Replica,
	featureKind string,
) bool {
	candidate, err := service.selectReadableReplica(ctx, replicas)
	if err != nil || candidate == nil {
		return false
	}

	embedding, err := service.store.GetAssetSemanticEmbeddingByAssetAndKind(ctx, assetID, featureKind)
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	if err != nil {
		return false
	}

	return !sameNullableString(embedding.SourceVersionID, candidate.replica.VersionID)
}

func (service *Service) queueSearchJob(assetID string, taskType string) {
	jobKey := taskType + ":" + assetID
	if _, loaded := service.mediaJobKeys.LoadOrStore(jobKey, struct{}{}); loaded {
		return
	}

	go func() {
		defer service.mediaJobKeys.Delete(jobKey)

		if _, err := service.startSearchTask(context.Background(), assetID, taskType); err != nil {
			slog.Warn("search feature job failed", "assetId", assetID, "taskType", taskType, "error", err)
		}
	}()
}

func (service *Service) startSearchTask(ctx context.Context, assetID, taskType string) (store.Task, error) {
	task, err := service.createCatalogTask(ctx, taskType, map[string]string{
		"assetId":  assetID,
		"taskType": taskType,
	})
	if err != nil {
		return store.Task{}, err
	}

	return task, service.runSearchTask(ctx, task, assetID, taskType)
}

func (service *Service) runSearchTask(ctx context.Context, task store.Task, assetID, taskType string) error {
	startedAt := time.Now().UTC()
	if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
		Status:     taskStatusRunning,
		RetryCount: task.RetryCount,
		StartedAt:  &startedAt,
		UpdatedAt:  startedAt,
	}); err != nil {
		return err
	}

	slog.Info("search feature task started", "taskId", task.ID, "assetId", assetID, "taskType", taskType)

	resultSummary, runErr := service.executeSearchTask(ctx, assetID, taskType)
	finishedAt := time.Now().UTC()
	resultText := resultSummary

	if runErr != nil {
		errorText := runErr.Error()
		if updateErr := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
			Status:        taskStatusFailed,
			ResultSummary: &resultText,
			ErrorMessage:  &errorText,
			RetryCount:    task.RetryCount,
			StartedAt:     &startedAt,
			FinishedAt:    &finishedAt,
			UpdatedAt:     finishedAt,
		}); updateErr != nil {
			return updateErr
		}

		slog.Error("search feature task failed", "taskId", task.ID, "assetId", assetID, "taskType", taskType, "error", runErr)
		return runErr
	}

	if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
		Status:        taskStatusSuccess,
		ResultSummary: &resultText,
		RetryCount:    task.RetryCount,
		StartedAt:     &startedAt,
		FinishedAt:    &finishedAt,
		UpdatedAt:     finishedAt,
	}); err != nil {
		return err
	}

	slog.Info("search feature task completed", "taskId", task.ID, "assetId", assetID, "taskType", taskType)
	return nil
}

func (service *Service) executeSearchTask(ctx context.Context, assetID, taskType string) (string, error) {
	asset, err := service.store.GetAssetByID(ctx, assetID)
	if err != nil {
		return "", err
	}

	replicas, err := service.store.ListReplicasByAssetID(ctx, assetID)
	if err != nil {
		return "", err
	}

	switch taskType {
	case taskTypeAudioTranscript, taskTypeVideoTranscript:
		return service.generateTranscript(ctx, asset, replicas)
	case taskTypeImageSemantic:
		return service.generateSemanticEmbedding(ctx, asset, replicas, semanticFeatureKindImage)
	case taskTypeVideoSemantic:
		return service.generateSemanticEmbedding(ctx, asset, replicas, semanticFeatureKindVideo)
	default:
		return "", fmt.Errorf("unsupported search task type: %s", taskType)
	}
}

func (service *Service) generateTranscript(
	ctx context.Context,
	asset store.Asset,
	replicas []store.Replica,
) (string, error) {
	candidate, err := service.selectReadableReplica(ctx, replicas)
	if err != nil {
		return "", err
	}
	if candidate == nil {
		return "", errors.New("no readable replica available for transcription")
	}

	localPath, cleanup, err := service.materializeReplicaToLocalFile(ctx, candidate.endpoint, candidate.replica)
	if err != nil {
		return "", err
	}
	if cleanup != nil {
		defer cleanup()
	}

	ffmpegPath := ""
	if resolvedFFmpegPath, resolveErr := service.resolveFFmpegBinary(); resolveErr == nil {
		ffmpegPath = resolvedFFmpegPath
	}

	output, err := service.searchBridge.Transcribe(ctx, localPath, asset.MediaType, ffmpegPath)
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	language := stringPointer(strings.TrimSpace(output.Language))
	transcript := store.AssetTranscript{
		AssetID:         asset.ID,
		TranscriptText:  strings.TrimSpace(output.Text),
		Language:        language,
		SourceVersionID: candidate.replica.VersionID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if transcript.TranscriptText == "" {
		return "", errors.New("transcription result is empty")
	}
	if err := service.store.SaveAssetTranscript(ctx, transcript); err != nil {
		return "", err
	}
	if err := service.store.SaveAssetSearchDocument(ctx, store.AssetSearchDocument{
		ID:         uuid.NewString(),
		AssetID:    asset.ID,
		SourceKind: searchDocumentKindTranscript,
		Content:    transcript.TranscriptText,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		return "", err
	}

	summary, err := json.Marshal(map[string]any{
		"kind":      searchDocumentKindTranscript,
		"language":  transcript.Language,
		"modelName": output.ModelName,
		"length":    len([]rune(transcript.TranscriptText)),
	})
	if err != nil {
		return "", err
	}

	return string(summary), nil
}

func (service *Service) generateSemanticEmbedding(
	ctx context.Context,
	asset store.Asset,
	replicas []store.Replica,
	featureKind string,
) (string, error) {
	candidate, err := service.selectReadableReplica(ctx, replicas)
	if err != nil {
		return "", err
	}
	if candidate == nil {
		return "", errors.New("no readable replica available for semantic feature generation")
	}

	localPath, cleanup, err := service.materializeReplicaToLocalFile(ctx, candidate.endpoint, candidate.replica)
	if err != nil {
		return "", err
	}
	if cleanup != nil {
		defer cleanup()
	}

	ffmpegPath := ""
	if resolvedFFmpegPath, resolveErr := service.resolveFFmpegBinary(); resolveErr == nil {
		ffmpegPath = resolvedFFmpegPath
	}

	var output SearchEmbeddingOutput
	switch featureKind {
	case semanticFeatureKindImage:
		output, err = service.searchBridge.EmbedImage(ctx, localPath)
	case semanticFeatureKindVideo:
		output, err = service.searchBridge.EmbedVideo(ctx, localPath, ffmpegPath)
	default:
		return "", fmt.Errorf("unsupported semantic feature kind: %s", featureKind)
	}
	if err != nil {
		return "", err
	}

	embeddingJSON, err := json.Marshal(output.Vector)
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	if err := service.store.SaveAssetSemanticEmbedding(ctx, store.AssetSemanticEmbedding{
		ID:              uuid.NewString(),
		AssetID:         asset.ID,
		FeatureKind:     featureKind,
		ModelName:       defaultString(output.ModelName, "clip"),
		EmbeddingJSON:   string(embeddingJSON),
		SourceVersionID: candidate.replica.VersionID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		return "", err
	}

	summary, err := json.Marshal(map[string]any{
		"kind":       featureKind,
		"modelName":  output.ModelName,
		"dimensions": len(output.Vector),
	})
	if err != nil {
		return "", err
	}

	return string(summary), nil
}
