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
	taskTypeImageSemantic   = "image_semantic"
	taskTypeVideoSemantic   = "video_semantic"

	searchCapabilityTranscript = "transcript"
	searchCapabilitySemantic   = "semantic"
)

func (service *Service) maybeQueueSearchFeatures(asset store.Asset, replicas []store.Replica) {
	if !service.autoQueueSearchJobs {
		return
	}
	if strings.EqualFold(strings.TrimSpace(asset.AssetStatus), string(AssetStatusDeleted)) {
		return
	}

	backgroundCtx := context.Background()

	switch strings.ToLower(strings.TrimSpace(asset.MediaType)) {
	case string(connectors.MediaTypeAudio):
		if service.isSearchCapabilityEnabled(searchCapabilityTranscript) &&
			service.needsTranscriptGeneration(backgroundCtx, asset.ID, replicas) &&
			service.shouldAutoQueueSearchTask(backgroundCtx, asset.ID, taskTypeAudioTranscript) {
			service.queueSearchJob(asset.ID, taskTypeAudioTranscript)
		}
	case string(connectors.MediaTypeVideo):
		if service.isSearchCapabilityEnabled(searchCapabilityTranscript) &&
			service.needsTranscriptGeneration(backgroundCtx, asset.ID, replicas) &&
			service.shouldAutoQueueSearchTask(backgroundCtx, asset.ID, taskTypeVideoTranscript) {
			service.queueSearchJob(asset.ID, taskTypeVideoTranscript)
		}
		if service.isSearchCapabilityEnabled(searchCapabilitySemantic) &&
			service.needsSemanticEmbeddingGeneration(backgroundCtx, asset.ID, replicas, semanticFeatureKindVideo) &&
			service.shouldAutoQueueSearchTask(backgroundCtx, asset.ID, taskTypeVideoSemantic) {
			service.queueSearchJob(asset.ID, taskTypeVideoSemantic)
		}
	case string(connectors.MediaTypeImage):
		if service.isSearchCapabilityEnabled(searchCapabilitySemantic) &&
			service.needsSemanticEmbeddingGeneration(backgroundCtx, asset.ID, replicas, semanticFeatureKindImage) &&
			service.shouldAutoQueueSearchTask(backgroundCtx, asset.ID, taskTypeImageSemantic) {
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

func (service *Service) shouldAutoQueueSearchTask(ctx context.Context, assetID, taskType string) bool {
	latestTask, err := service.store.GetLatestTaskByTypeAndAssetID(ctx, taskType, assetID)
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	if err != nil {
		return true
	}

	switch strings.ToLower(strings.TrimSpace(latestTask.Status)) {
	case taskStatusPending, taskStatusRunning, taskStatusRetrying, taskStatusFailed, "error":
		return false
	default:
		return true
	}
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

func (service *Service) isSearchCapabilityEnabled(capability string) bool {
	if strings.TrimSpace(capability) == "" {
		return true
	}

	_, disabled := service.searchCapabilityFlags.Load(strings.TrimSpace(capability))
	return !disabled
}

func (service *Service) disableSearchCapability(capability string) {
	if strings.TrimSpace(capability) == "" {
		return
	}
	service.searchCapabilityFlags.Store(strings.TrimSpace(capability), struct{}{})
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
		if skippedSummary, ok := service.searchTaskSkipSummary(taskType, runErr); ok {
			resultText = skippedSummary
			errorText := skippedSummary
			if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
				Status:        taskStatusFailed,
				ResultSummary: &resultText,
				ErrorMessage:  &errorText,
				RetryCount:    task.RetryCount,
				StartedAt:     &startedAt,
				FinishedAt:    &finishedAt,
				UpdatedAt:     finishedAt,
			}); err != nil {
				return err
			}

			slog.Warn("search feature task auto-paused", "taskId", task.ID, "assetId", assetID, "taskType", taskType, "reason", skippedSummary)
			return runErr
		}

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

func (service *Service) searchTaskSkipSummary(_ string, err error) (string, bool) {
	reason := buildSearchTaskUnavailableReason(err)
	if reason == "" {
		return "", false
	}

	return "自动任务已暂停：" + reason + "。处理完成后可在任务中心手动重试。", true
}

func buildSearchTaskUnavailableReason(err error) string {
	if err == nil {
		return ""
	}

	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "search_ai.requirements.txt"),
		strings.Contains(message, "faster-whisper"),
		strings.Contains(message, "transformers/torch"),
		strings.Contains(message, "pillow"):
		return "当前环境缺少搜索 AI 依赖，请安装 backend/tools/search_ai.requirements.txt"
	case strings.Contains(message, "no such table: asset_transcripts"),
		strings.Contains(message, "no such table: asset_search_documents"),
		strings.Contains(message, "no such table: asset_semantic_embeddings"):
		return "当前资产库尚未完成 AI 搜索迁移，请重启应用后再试"
	default:
		return ""
	}
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

	if strings.EqualFold(strings.TrimSpace(asset.MediaType), string(connectors.MediaTypeVideo)) {
		if hasAudioTrack, probeErr := service.detectAudioTrack(ctx, localPath); probeErr == nil && !hasAudioTrack {
			return service.saveTranscriptResult(ctx, asset, candidate.replica.VersionID, "", nil, "", "视频没有可转写音轨")
		}
	}

	output, err := service.searchBridge.Transcribe(ctx, localPath, asset.MediaType, ffmpegPath)
	if err != nil {
		return "", err
	}

	transcriptText := strings.TrimSpace(output.Text)
	var language *string
	if trimmedLanguage := strings.TrimSpace(output.Language); trimmedLanguage != "" {
		language = stringPointer(trimmedLanguage)
	}

	warning := ""
	if transcriptText == "" {
		warning = "未检测到可转写语音"
	}

	return service.saveTranscriptResult(ctx, asset, candidate.replica.VersionID, transcriptText, language, output.ModelName, warning)
}

func (service *Service) saveTranscriptResult(
	ctx context.Context,
	asset store.Asset,
	sourceVersionID *string,
	transcriptText string,
	language *string,
	modelName string,
	warning string,
) (string, error) {
	now := time.Now().UTC()
	transcript := store.AssetTranscript{
		AssetID:         asset.ID,
		TranscriptText:  strings.TrimSpace(transcriptText),
		Language:        language,
		SourceVersionID: sourceVersionID,
		CreatedAt:       now,
		UpdatedAt:       now,
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

	summaryPayload := map[string]any{
		"kind":      searchDocumentKindTranscript,
		"language":  transcript.Language,
		"modelName": strings.TrimSpace(modelName),
		"length":    len([]rune(transcript.TranscriptText)),
	}
	if trimmedWarning := strings.TrimSpace(warning); trimmedWarning != "" {
		summaryPayload["warning"] = trimmedWarning
	}

	summary, err := json.Marshal(summaryPayload)
	if err != nil {
		return "", err
	}

	return string(summary), nil
}

func (service *Service) detectAudioTrack(ctx context.Context, localPath string) (bool, error) {
	if ffprobePath, err := service.resolveFFprobeBinary(); err == nil {
		if result, probeErr := probeAudioWithFFprobe(ffprobePath, localPath); probeErr == nil {
			return audioProbeHasStream(result), nil
		}
	}

	if ffmpegPath, err := service.resolveFFmpegBinary(); err == nil {
		if result, probeErr := probeAudioWithFFmpeg(ctx, ffmpegPath, localPath); probeErr == nil {
			return audioProbeHasStream(result), nil
		}
	}

	return true, errors.New("audio track probe unavailable")
}

func audioProbeHasStream(result audioProbeResult) bool {
	return result.CodecName != nil || result.SampleRateHz != nil || result.ChannelCount != nil
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
