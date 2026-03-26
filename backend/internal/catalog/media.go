package catalog

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dhowden/tag"
	"github.com/google/uuid"

	"mam/backend/internal/connectors"
	"mam/backend/internal/store"
)

import (
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

const (
	previewKindImageThumbnail = "image_thumbnail"
	previewKindVideoCover     = "video_cover"
	previewKindAudioArtwork   = "audio_artwork"

	mediaTaskThumbnail     = "thumbnail"
	mediaTaskVideoCover    = "video_cover"
	mediaTaskAudioMetadata = "audio_metadata"

	thumbnailMaxDimension = 640
)

var (
	ffmpegDurationPattern     = regexp.MustCompile(`Duration:\s*(\d{2}):(\d{2}):(\d{2}(?:\.\d+)?)`)
	ffmpegDimensionPattern    = regexp.MustCompile(`(\d{2,5})x(\d{2,5})`)
	ffmpegSampleRatePattern   = regexp.MustCompile(`(\d{4,6})\s*Hz`)
	ffmpegChannelCountPattern = regexp.MustCompile(`(\d+)\s*channels?`)
)

type MediaConfig struct {
	CacheRoot  string
	FFmpegPath string
}

type assetReadableReplica struct {
	endpoint store.StorageEndpoint
	replica  store.Replica
	version  *store.ReplicaVersion
	priority int
}

type AssetMediaResource struct {
	FilePath    string
	ContentType string
	FileName    string
	ModTime     time.Time
	Cleanup     func()
}

type audioProbeResult struct {
	DurationSeconds *float64
	CodecName       *string
	SampleRateHz    *int
	ChannelCount    *int
}

type embeddedArtwork struct {
	Data     []byte
	MIMEType string
	Width    *int
	Height   *int
}

type ffprobePayload struct {
	Streams []struct {
		CodecType  string `json:"codec_type"`
		CodecName  string `json:"codec_name"`
		SampleRate string `json:"sample_rate"`
		Channels   int    `json:"channels"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func normalizeMediaConfig(config MediaConfig) MediaConfig {
	cacheRoot := strings.TrimSpace(config.CacheRoot)
	if cacheRoot == "" {
		cacheRoot = filepath.Join(".", "data", "cache", "media")
	}

	ffmpegPath := strings.TrimSpace(config.FFmpegPath)
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}

	return MediaConfig{
		CacheRoot:  cacheRoot,
		FFmpegPath: ffmpegPath,
	}
}

func (service *Service) buildPosterRecord(ctx context.Context, asset store.Asset) (*AssetPreviewRecord, error) {
	if asset.PrimaryThumbnailID == nil {
		return nil, nil
	}

	preview, err := service.store.GetAssetPreviewByID(ctx, *asset.PrimaryThumbnailID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if _, err := os.Stat(preview.FilePath); err != nil {
		return nil, nil
	}

	url := service.posterURL(asset.ID)
	return &AssetPreviewRecord{
		ID:        preview.ID,
		Kind:      preview.Kind,
		URL:       url,
		MIMEType:  preview.MIMEType,
		Width:     preview.Width,
		Height:    preview.Height,
		CreatedAt: preview.CreatedAt,
		UpdatedAt: preview.UpdatedAt,
	}, nil
}

func (service *Service) buildAudioMetadataRecord(ctx context.Context, assetID string) (*AssetAudioMetadataRecord, error) {
	metadata, err := service.store.GetAssetMediaMetadataByAssetID(ctx, assetID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &AssetAudioMetadataRecord{
		DurationSeconds: metadata.DurationSeconds,
		CodecName:       metadata.CodecName,
		SampleRateHz:    metadata.SampleRateHz,
		ChannelCount:    metadata.ChannelCount,
	}, nil
}

func (service *Service) ResolvePosterResource(ctx context.Context, assetID string) (AssetMediaResource, error) {
	asset, err := service.store.GetAssetByID(ctx, assetID)
	if err != nil {
		return AssetMediaResource{}, err
	}

	if asset.PrimaryThumbnailID == nil {
		return AssetMediaResource{}, os.ErrNotExist
	}

	preview, err := service.store.GetAssetPreviewByID(ctx, *asset.PrimaryThumbnailID)
	if err != nil {
		return AssetMediaResource{}, err
	}

	if _, err := os.Stat(preview.FilePath); err != nil {
		return AssetMediaResource{}, err
	}

	return AssetMediaResource{
		FilePath:    preview.FilePath,
		ContentType: defaultString(nullableStringValue(preview.MIMEType), detectContentType(preview.FilePath)),
		FileName:    filepath.Base(preview.FilePath),
		ModTime:     preview.UpdatedAt,
	}, nil
}

func (service *Service) ResolvePreviewResource(ctx context.Context, assetID string) (AssetMediaResource, error) {
	replicas, err := service.store.ListReplicasByAssetID(ctx, assetID)
	if err != nil {
		return AssetMediaResource{}, err
	}

	candidate, err := service.selectReadableReplica(ctx, replicas)
	if err != nil {
		return AssetMediaResource{}, err
	}
	if candidate == nil {
		return AssetMediaResource{}, os.ErrNotExist
	}

	filePath, cleanup, err := service.materializeReplicaToLocalFile(ctx, candidate.endpoint, candidate.replica)
	if err != nil {
		return AssetMediaResource{}, err
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return AssetMediaResource{}, err
	}

	return AssetMediaResource{
		FilePath:    filePath,
		ContentType: detectContentType(filePath),
		FileName:    filepath.Base(filePath),
		ModTime:     info.ModTime().UTC(),
		Cleanup:     cleanup,
	}, nil
}

func (service *Service) maybeQueueDerivedMedia(asset store.Asset, replicas []store.Replica) {
	if !service.autoQueueDerivedMedia {
		return
	}

	backgroundCtx := context.Background()

	switch strings.ToLower(strings.TrimSpace(asset.MediaType)) {
	case string(connectors.MediaTypeImage):
		if service.needsPreviewGeneration(backgroundCtx, asset, replicas, previewKindImageThumbnail) {
			service.queueMediaJob(asset.ID, mediaTaskThumbnail)
		}
	case string(connectors.MediaTypeVideo):
		if service.needsPreviewGeneration(backgroundCtx, asset, replicas, previewKindVideoCover) {
			service.queueMediaJob(asset.ID, mediaTaskVideoCover)
		}
	case string(connectors.MediaTypeAudio):
		if service.needsAudioMetadataGeneration(backgroundCtx, asset, replicas) {
			service.queueMediaJob(asset.ID, mediaTaskAudioMetadata)
		}
	}
}

func (service *Service) needsPreviewGeneration(
	ctx context.Context,
	asset store.Asset,
	replicas []store.Replica,
	previewKind string,
) bool {
	candidate, err := service.selectReadableReplica(ctx, replicas)
	if err != nil || candidate == nil {
		return false
	}

	preview, err := service.store.GetAssetPreviewByAssetAndKind(ctx, asset.ID, previewKind)
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	if err != nil {
		return false
	}

	if _, err := os.Stat(preview.FilePath); err != nil {
		return true
	}

	return !sameNullableString(preview.SourceVersionID, candidate.replica.VersionID)
}

func (service *Service) needsAudioMetadataGeneration(ctx context.Context, asset store.Asset, replicas []store.Replica) bool {
	candidate, err := service.selectReadableReplica(ctx, replicas)
	if err != nil || candidate == nil {
		return false
	}

	metadata, err := service.store.GetAssetMediaMetadataByAssetID(ctx, asset.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	if err != nil {
		return false
	}

	return !sameNullableString(metadata.SourceVersionID, candidate.replica.VersionID)
}

func (service *Service) queueMediaJob(assetID, taskType string) {
	jobKey := taskType + ":" + assetID
	if _, loaded := service.mediaJobKeys.LoadOrStore(jobKey, struct{}{}); loaded {
		return
	}

	go func() {
		defer service.mediaJobKeys.Delete(jobKey)

		if _, err := service.startMediaTask(context.Background(), assetID, taskType); err != nil {
			slog.Warn("media job failed", "assetId", assetID, "taskType", taskType, "error", err)
		}
	}()
}

func (service *Service) startMediaTask(ctx context.Context, assetID, taskType string) (store.Task, error) {
	task, err := service.createMediaTask(ctx, assetID, taskType)
	if err != nil {
		return store.Task{}, err
	}

	return task, service.runMediaTask(ctx, task, assetID, taskType)
}

func (service *Service) runMediaTask(ctx context.Context, task store.Task, assetID, taskType string) error {
	startedAt := time.Now().UTC()
	if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
		Status:     "running",
		RetryCount: task.RetryCount,
		StartedAt:  &startedAt,
		UpdatedAt:  startedAt,
	}); err != nil {
		return err
	}

	slog.Info("media task started", "taskId", task.ID, "assetId", assetID, "taskType", taskType)

	resultSummary, runErr := service.executeMediaTask(ctx, assetID, taskType)
	finishedAt := time.Now().UTC()
	resultText := resultSummary

	if runErr != nil {
		errorText := runErr.Error()
		if updateErr := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
			Status:        "failed",
			ResultSummary: &resultText,
			ErrorMessage:  &errorText,
			RetryCount:    task.RetryCount,
			StartedAt:     &startedAt,
			FinishedAt:    &finishedAt,
			UpdatedAt:     finishedAt,
		}); updateErr != nil {
			return updateErr
		}

		slog.Error("media task failed", "taskId", task.ID, "assetId", assetID, "taskType", taskType, "error", runErr)
		return runErr
	}

	if err := service.store.UpdateTaskStatus(ctx, task.ID, store.TaskStatusUpdate{
		Status:        "success",
		ResultSummary: &resultText,
		RetryCount:    task.RetryCount,
		StartedAt:     &startedAt,
		FinishedAt:    &finishedAt,
		UpdatedAt:     finishedAt,
	}); err != nil {
		return err
	}

	slog.Info("media task completed", "taskId", task.ID, "assetId", assetID, "taskType", taskType)
	return nil
}

func (service *Service) createMediaTask(ctx context.Context, assetID, taskType string) (store.Task, error) {
	now := time.Now().UTC()
	payload, err := json.Marshal(map[string]string{
		"assetId":  assetID,
		"taskType": taskType,
	})
	if err != nil {
		return store.Task{}, err
	}

	task := store.Task{
		ID:        uuid.NewString(),
		TaskType:  taskType,
		Status:    "pending",
		Payload:   string(payload),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := service.store.CreateTask(ctx, task); err != nil {
		return store.Task{}, err
	}
	return task, nil
}

func (service *Service) executeMediaTask(ctx context.Context, assetID, taskType string) (string, error) {
	asset, err := service.store.GetAssetByID(ctx, assetID)
	if err != nil {
		return "", err
	}

	replicas, err := service.store.ListReplicasByAssetID(ctx, assetID)
	if err != nil {
		return "", err
	}

	switch taskType {
	case mediaTaskThumbnail:
		summary, err := service.generateImageThumbnail(ctx, asset, replicas)
		return summary, err
	case mediaTaskVideoCover:
		summary, err := service.generateVideoCover(ctx, asset, replicas)
		return summary, err
	case mediaTaskAudioMetadata:
		summary, err := service.extractAudioMetadata(ctx, asset, replicas)
		return summary, err
	default:
		return "", fmt.Errorf("unsupported media task type: %s", taskType)
	}
}

func (service *Service) generateImageThumbnail(ctx context.Context, asset store.Asset, replicas []store.Replica) (string, error) {
	candidate, err := service.selectReadableReplica(ctx, replicas)
	if err != nil {
		return "", err
	}
	if candidate == nil {
		return "", errors.New("no readable replica available for thumbnail generation")
	}

	localPath, cleanup, err := service.materializeReplicaToLocalFile(ctx, candidate.endpoint, candidate.replica)
	if err != nil {
		return "", err
	}
	if cleanup != nil {
		defer cleanup()
	}

	file, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	sourceImage, _, err := image.Decode(file)
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}

	thumbnail, width, height := createThumbnail(sourceImage, thumbnailMaxDimension, thumbnailMaxDimension)
	outputPath := filepath.Join(service.mediaConfig.CacheRoot, "images", asset.ID+".png")
	if err := writePNG(outputPath, thumbnail); err != nil {
		return "", err
	}

	mimeType := "image/png"
	preview, err := service.saveOrUpdatePreview(ctx, asset, previewKindImageThumbnail, outputPath, &mimeType, &width, &height, candidate.replica.VersionID)
	if err != nil {
		return "", err
	}

	summary, err := json.Marshal(map[string]any{
		"kind":       preview.Kind,
		"cachedPath": preview.FilePath,
		"width":      width,
		"height":     height,
	})
	if err != nil {
		return "", err
	}

	return string(summary), nil
}

func (service *Service) generateVideoCover(ctx context.Context, asset store.Asset, replicas []store.Replica) (string, error) {
	candidate, err := service.selectReadableReplica(ctx, replicas)
	if err != nil {
		return "", err
	}
	if candidate == nil {
		return "", errors.New("no readable replica available for video cover extraction")
	}

	ffmpegPath, err := service.resolveFFmpegBinary()
	if err != nil {
		return "", err
	}

	localPath, cleanup, err := service.materializeReplicaToLocalFile(ctx, candidate.endpoint, candidate.replica)
	if err != nil {
		return "", err
	}
	if cleanup != nil {
		defer cleanup()
	}

	outputPath := filepath.Join(service.mediaConfig.CacheRoot, "videos", asset.ID+".png")
	if err := ensureDirectory(filepath.Dir(outputPath)); err != nil {
		return "", err
	}

	command := exec.Command(
		ffmpegPath,
		"-y",
		"-i", localPath,
		"-vf", fmt.Sprintf("thumbnail,scale=%d:%d:force_original_aspect_ratio=decrease", thumbnailMaxDimension, thumbnailMaxDimension),
		"-frames:v", "1",
		outputPath,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg cover extraction failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}

	width, height, err := readImageDimensions(outputPath)
	if err != nil {
		return "", err
	}

	mimeType := "image/png"
	preview, err := service.saveOrUpdatePreview(ctx, asset, previewKindVideoCover, outputPath, &mimeType, &width, &height, candidate.replica.VersionID)
	if err != nil {
		return "", err
	}

	summary, err := json.Marshal(map[string]any{
		"kind":       preview.Kind,
		"cachedPath": preview.FilePath,
		"width":      width,
		"height":     height,
	})
	if err != nil {
		return "", err
	}

	return string(summary), nil
}

func (service *Service) extractAudioMetadata(ctx context.Context, asset store.Asset, replicas []store.Replica) (string, error) {
	candidate, err := service.selectReadableReplica(ctx, replicas)
	if err != nil {
		return "", err
	}
	if candidate == nil {
		return "", errors.New("no readable replica available for audio metadata extraction")
	}

	localPath, cleanup, err := service.materializeReplicaToLocalFile(ctx, candidate.endpoint, candidate.replica)
	if err != nil {
		return "", err
	}
	if cleanup != nil {
		defer cleanup()
	}

	probeResult, probeErr := service.probeAudioMetadata(ctx, localPath)
	if probeErr != nil {
		slog.Warn("audio metadata probe degraded", "assetId", asset.ID, "error", probeErr)
	}

	artwork, artworkErr := readEmbeddedArtwork(localPath)
	if artworkErr != nil {
		slog.Warn("audio artwork extraction skipped", "assetId", asset.ID, "error", artworkErr)
	}

	now := time.Now().UTC()
	metadata := store.AssetMediaMetadata{
		AssetID:         asset.ID,
		DurationSeconds: probeResult.DurationSeconds,
		CodecName:       probeResult.CodecName,
		SampleRateHz:    probeResult.SampleRateHz,
		ChannelCount:    probeResult.ChannelCount,
		SourceVersionID: candidate.replica.VersionID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := service.store.SaveAssetMediaMetadata(ctx, metadata); err != nil {
		return "", err
	}

	resultPayload := map[string]any{
		"durationSeconds": nullableFloatValue(metadata.DurationSeconds),
		"codecName":       nullableStringValue(metadata.CodecName),
		"sampleRateHz":    nullableIntValue(metadata.SampleRateHz),
		"channelCount":    nullableIntValue(metadata.ChannelCount),
	}

	if artwork != nil {
		outputPath := filepath.Join(service.mediaConfig.CacheRoot, "audio", asset.ID+previewExtensionForArtwork(artwork.MIMEType))
		mimeType := defaultString(strings.TrimSpace(artwork.MIMEType), detectContentType(outputPath))
		width := artwork.Width
		height := artwork.Height

		if err := ensureDirectory(filepath.Dir(outputPath)); err != nil {
			return "", err
		}
		if err := os.WriteFile(outputPath, artwork.Data, 0o644); err != nil {
			return "", fmt.Errorf("write audio artwork: %w", err)
		}

		preview, err := service.saveOrUpdatePreview(ctx, asset, previewKindAudioArtwork, outputPath, &mimeType, width, height, candidate.replica.VersionID)
		if err != nil {
			return "", err
		}
		resultPayload["artworkPath"] = preview.FilePath
	}

	summary, err := json.Marshal(resultPayload)
	if err != nil {
		return "", err
	}

	return string(summary), nil
}

func (service *Service) saveOrUpdatePreview(
	ctx context.Context,
	asset store.Asset,
	kind string,
	filePath string,
	mimeType *string,
	width *int,
	height *int,
	sourceVersionID *string,
) (store.AssetPreview, error) {
	now := time.Now().UTC()
	preview, err := service.store.GetAssetPreviewByAssetAndKind(ctx, asset.ID, kind)
	if errors.Is(err, sql.ErrNoRows) {
		preview = store.AssetPreview{
			ID:              uuid.NewString(),
			AssetID:         asset.ID,
			Kind:            kind,
			FilePath:        filePath,
			MIMEType:        mimeType,
			Width:           width,
			Height:          height,
			SourceVersionID: sourceVersionID,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := service.store.CreateAssetPreview(ctx, preview); err != nil {
			return store.AssetPreview{}, err
		}
	} else if err != nil {
		return store.AssetPreview{}, err
	} else {
		preview.FilePath = filePath
		preview.MIMEType = mimeType
		preview.Width = width
		preview.Height = height
		preview.SourceVersionID = sourceVersionID
		preview.UpdatedAt = now
		if err := service.store.UpdateAssetPreview(ctx, preview); err != nil {
			return store.AssetPreview{}, err
		}
	}

	if asset.PrimaryThumbnailID == nil || *asset.PrimaryThumbnailID != preview.ID {
		asset.PrimaryThumbnailID = &preview.ID
		asset.UpdatedAt = now
		if err := service.store.UpdateAsset(ctx, asset); err != nil {
			return store.AssetPreview{}, err
		}
	}

	return preview, nil
}

func (service *Service) selectReadableReplica(ctx context.Context, replicas []store.Replica) (*assetReadableReplica, error) {
	candidates := make([]assetReadableReplica, 0, len(replicas))

	for _, replica := range replicas {
		if !replica.ExistsFlag {
			continue
		}
		if connectors.ShouldIgnoreAssetPath(replica.PhysicalPath) {
			continue
		}

		endpoint, err := service.store.GetStorageEndpointByID(ctx, replica.EndpointID)
		if err != nil {
			return nil, err
		}

		connector, err := service.buildConnector(endpoint)
		if err != nil {
			continue
		}
		if !connector.Descriptor().Capabilities.CanReadStream {
			continue
		}

		var version *store.ReplicaVersion
		if replica.VersionID != nil {
			resolvedVersion, err := service.store.GetReplicaVersionByID(ctx, *replica.VersionID)
			if err == nil {
				version = &resolvedVersion
			}
		}

		candidates = append(candidates, assetReadableReplica{
			endpoint: endpoint,
			replica:  replica,
			version:  version,
			priority: readableReplicaPriority(endpoint.EndpointType),
		})
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].priority != candidates[right].priority {
			return candidates[left].priority < candidates[right].priority
		}
		return strings.ToLower(candidates[left].endpoint.Name) < strings.ToLower(candidates[right].endpoint.Name)
	})

	return &candidates[0], nil
}

func (service *Service) materializeReplicaToLocalFile(
	ctx context.Context,
	endpoint store.StorageEndpoint,
	replica store.Replica,
) (string, func(), error) {
	physicalPath := strings.TrimSpace(replica.PhysicalPath)
	if physicalPath != "" {
		if info, err := os.Stat(physicalPath); err == nil && !info.IsDir() {
			return physicalPath, nil, nil
		}
	}

	connector, err := service.buildConnector(endpoint)
	if err != nil {
		return "", nil, err
	}

	reader, err := connector.ReadStream(ctx, replica.PhysicalPath)
	if err != nil {
		return "", nil, err
	}
	defer reader.Close()

	tempDir := filepath.Join(service.mediaConfig.CacheRoot, "temp")
	if err := ensureDirectory(tempDir); err != nil {
		return "", nil, err
	}

	tempFile, err := os.CreateTemp(tempDir, "asset-*"+filepath.Ext(replica.PhysicalPath))
	if err != nil {
		return "", nil, err
	}
	defer tempFile.Close()

	if _, err := io.Copy(tempFile, reader); err != nil {
		_ = os.Remove(tempFile.Name())
		return "", nil, err
	}

	return tempFile.Name(), func() {
		_ = os.Remove(tempFile.Name())
	}, nil
}

func (service *Service) probeAudioMetadata(ctx context.Context, localPath string) (audioProbeResult, error) {
	if ffprobePath, err := service.resolveFFprobeBinary(); err == nil {
		result, probeErr := probeAudioWithFFprobe(ffprobePath, localPath)
		if probeErr == nil {
			return result, nil
		}
	}

	if ffmpegPath, err := service.resolveFFmpegBinary(); err == nil {
		result, probeErr := probeAudioWithFFmpeg(ctx, ffmpegPath, localPath)
		if probeErr == nil {
			return result, nil
		}
	}

	return probeAudioWithFallback(localPath)
}

func probeAudioWithFFprobe(ffprobePath, localPath string) (audioProbeResult, error) {
	command := exec.Command(
		ffprobePath,
		"-v", "error",
		"-show_format",
		"-show_streams",
		"-print_format", "json",
		localPath,
	)
	output, err := command.Output()
	if err != nil {
		return audioProbeResult{}, err
	}

	var payload ffprobePayload
	if err := json.Unmarshal(output, &payload); err != nil {
		return audioProbeResult{}, err
	}

	result := audioProbeResult{}
	if durationText := strings.TrimSpace(payload.Format.Duration); durationText != "" {
		if duration, err := strconv.ParseFloat(durationText, 64); err == nil {
			result.DurationSeconds = &duration
		}
	}

	for _, stream := range payload.Streams {
		if stream.CodecType != "audio" {
			continue
		}

		if codecName := strings.TrimSpace(stream.CodecName); codecName != "" {
			result.CodecName = stringPointer(codecName)
		}
		if sampleRateText := strings.TrimSpace(stream.SampleRate); sampleRateText != "" {
			if sampleRate, err := strconv.Atoi(sampleRateText); err == nil {
				result.SampleRateHz = &sampleRate
			}
		}
		if stream.Channels > 0 {
			channelCount := stream.Channels
			result.ChannelCount = &channelCount
		}
		break
	}

	return result, nil
}

func probeAudioWithFallback(localPath string) (audioProbeResult, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return audioProbeResult{}, err
	}
	defer file.Close()

	metadata, err := tag.ReadFrom(file)
	if err != nil {
		codecName := codecNameFromPath(localPath)
		if codecName == nil {
			return audioProbeResult{}, err
		}
		return audioProbeResult{CodecName: codecName}, nil
	}

	format := strings.TrimSpace(string(metadata.Format()))
	if format == "" {
		return audioProbeResult{CodecName: codecNameFromPath(localPath)}, nil
	}

	return audioProbeResult{
		CodecName: stringPointer(strings.ToLower(format)),
	}, nil
}

func readEmbeddedArtwork(localPath string) (*embeddedArtwork, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	metadata, err := tag.ReadFrom(file)
	if err != nil {
		return nil, err
	}

	picture := metadata.Picture()
	if picture == nil || len(picture.Data) == 0 {
		return nil, nil
	}

	artwork := &embeddedArtwork{
		Data:     picture.Data,
		MIMEType: strings.TrimSpace(picture.MIMEType),
	}

	if config, _, err := image.DecodeConfig(bytes.NewReader(picture.Data)); err == nil {
		width := config.Width
		height := config.Height
		artwork.Width = &width
		artwork.Height = &height
	}

	return artwork, nil
}

func (service *Service) resolveFFmpegBinary() (string, error) {
	return resolveBinaryCandidates(ffmpegBinaryCandidates(service.mediaConfig.FFmpegPath), "ffmpeg")
}

func (service *Service) resolveFFprobeBinary() (string, error) {
	return resolveBinaryCandidates(ffprobeBinaryCandidates(service.mediaConfig.FFmpegPath), "ffprobe")
}

func resolveBinary(candidate string) (string, error) {
	binaryPath := strings.TrimSpace(candidate)
	if binaryPath == "" {
		return "", errors.New("binary path is not configured")
	}

	resolved, err := exec.LookPath(binaryPath)
	if err != nil {
		return "", fmt.Errorf("executable %q is not available: %w", binaryPath, err)
	}
	return resolved, nil
}

func resolveBinaryCandidates(candidates []string, binaryName string) (string, error) {
	checked := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))

	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			continue
		}

		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		checked = append(checked, trimmed)

		resolved, err := exec.LookPath(trimmed)
		if err != nil {
			continue
		}
		return resolved, nil
	}

	if len(checked) == 0 {
		return "", fmt.Errorf("executable %q is not available", binaryName)
	}

	return "", fmt.Errorf(
		"executable %q is not available; searched: %s",
		binaryName,
		strings.Join(checked, "; "),
	)
}

func deriveFFprobePath(ffmpegPath string) string {
	if strings.TrimSpace(ffmpegPath) == "" {
		return "ffprobe"
	}

	base := filepath.Base(ffmpegPath)
	lowered := strings.ToLower(base)
	switch {
	case lowered == "ffmpeg":
		return filepath.Join(filepath.Dir(ffmpegPath), "ffprobe")
	case lowered == "ffmpeg.exe":
		return filepath.Join(filepath.Dir(ffmpegPath), "ffprobe.exe")
	default:
		return "ffprobe"
	}
}

func ffmpegBinaryCandidates(configured string) []string {
	candidates := []string{configured, "ffmpeg", "ffmpeg.exe"}
	return append(candidates, discoverBinaryCandidates("ffmpeg")...)
}

func ffprobeBinaryCandidates(configuredFFmpeg string) []string {
	candidates := []string{
		deriveFFprobePath(configuredFFmpeg),
		"ffprobe",
		"ffprobe.exe",
	}
	for _, ffmpegCandidate := range ffmpegBinaryCandidates(configuredFFmpeg) {
		trimmed := strings.TrimSpace(ffmpegCandidate)
		if trimmed == "" {
			continue
		}
		if strings.ContainsAny(trimmed, `\/`) {
			candidates = append(candidates, deriveFFprobePath(trimmed))
		}
	}
	return append(candidates, discoverBinaryCandidates("ffprobe")...)
}

func discoverBinaryCandidates(binaryName string) []string {
	executableName := binaryName
	if filepath.Ext(executableName) == "" {
		executableName += ".exe"
	}

	candidates := make([]string, 0, 32)
	candidates = append(candidates, projectBinaryCandidates(executableName)...)
	candidates = append(candidates, windowsBinaryCandidates(executableName)...)
	return candidates
}

func projectBinaryCandidates(executableName string) []string {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}

	roots := []string{cwd, filepath.Dir(cwd)}
	relativeDirs := []string{
		filepath.Join(".tools", "ffmpeg", "bin"),
		filepath.Join("tools", "ffmpeg", "bin"),
		filepath.Join("ffmpeg", "bin"),
		filepath.Join("vendor", "ffmpeg", "bin"),
		filepath.Join("backend", ".tools", "ffmpeg", "bin"),
		filepath.Join("backend", "tools", "ffmpeg", "bin"),
		filepath.Join("backend", "ffmpeg", "bin"),
	}

	candidates := make([]string, 0, len(roots)*len(relativeDirs))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		for _, relativeDir := range relativeDirs {
			candidates = append(candidates, filepath.Join(root, relativeDir, executableName))
		}
	}

	return candidates
}

func windowsBinaryCandidates(executableName string) []string {
	paths := make([]string, 0, 24)
	directories := make([]string, 0, 9)
	if programFiles := strings.TrimSpace(os.Getenv("ProgramFiles")); programFiles != "" {
		directories = append(directories,
			filepath.Join(programFiles, "ffmpeg", "bin"),
			filepath.Join(programFiles, "FFmpeg", "bin"),
		)
	}
	if programFilesX86 := strings.TrimSpace(os.Getenv("ProgramFiles(x86)")); programFilesX86 != "" {
		directories = append(directories,
			filepath.Join(programFilesX86, "ffmpeg", "bin"),
			filepath.Join(programFilesX86, "FFmpeg", "bin"),
		)
	}
	if chocolateyInstall := strings.TrimSpace(os.Getenv("ChocolateyInstall")); chocolateyInstall != "" {
		directories = append(directories, filepath.Join(chocolateyInstall, "bin"))
	}
	if userProfile := strings.TrimSpace(os.Getenv("USERPROFILE")); userProfile != "" {
		directories = append(directories, filepath.Join(userProfile, "scoop", "apps", "ffmpeg", "current", "bin"))
	}
	if localAppData := strings.TrimSpace(os.Getenv("LocalAppData")); localAppData != "" {
		directories = append(directories, filepath.Join(localAppData, "Microsoft", "WinGet", "Links"))
	}
	directories = append(directories,
		filepath.Join("C:\\", "ffmpeg", "bin"),
		filepath.Join("C:\\", "ffmpeg"),
	)
	for _, directory := range directories {
		directory = strings.TrimSpace(directory)
		if directory == "" {
			continue
		}
		paths = append(paths, filepath.Join(directory, executableName))
	}

	roots := []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
	}
	patterns := []string{
		filepath.Join("*", "ffmpeg", executableName),
		filepath.Join("*", "*", "ffmpeg", executableName),
		filepath.Join("*", "bin", executableName),
		filepath.Join("*", "*", "bin", executableName),
	}

	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		for _, pattern := range patterns {
			matches, err := filepath.Glob(filepath.Join(root, pattern))
			if err != nil {
				continue
			}
			paths = append(paths, matches...)
		}
	}

	return paths
}

func probeVideoWithFFmpeg(ctx context.Context, ffmpegPath, localPath string) (videoProbeResult, error) {
	output, err := readFFmpegProbeOutput(ctx, ffmpegPath, localPath)
	if err != nil {
		return videoProbeResult{}, err
	}

	result := videoProbeResult{
		DurationSeconds: parseFFmpegDuration(output),
	}
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "Video:") {
			continue
		}
		videoLine := strings.TrimSpace(line)
		if codecName := parseCodecNameFromFFmpegLine(videoLine, "Video:"); codecName != "" {
			result.CodecName = stringPointer(codecName)
		}
		if width, height := parseFFmpegDimensions(videoLine); width != nil && height != nil {
			result.Width = width
			result.Height = height
		}
		break
	}

	if result.DurationSeconds == nil && result.CodecName == nil && result.Width == nil && result.Height == nil {
		return videoProbeResult{}, fmt.Errorf("ffmpeg did not return usable video metadata")
	}

	return result, nil
}

func probeAudioWithFFmpeg(ctx context.Context, ffmpegPath, localPath string) (audioProbeResult, error) {
	output, err := readFFmpegProbeOutput(ctx, ffmpegPath, localPath)
	if err != nil {
		return audioProbeResult{}, err
	}

	result := audioProbeResult{
		DurationSeconds: parseFFmpegDuration(output),
	}
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "Audio:") {
			continue
		}
		audioLine := strings.TrimSpace(line)
		if codecName := parseCodecNameFromFFmpegLine(audioLine, "Audio:"); codecName != "" {
			result.CodecName = stringPointer(codecName)
		}
		if sampleRate := parseFFmpegSampleRate(audioLine); sampleRate != nil {
			result.SampleRateHz = sampleRate
		}
		if channelCount := parseFFmpegChannelCount(audioLine); channelCount != nil {
			result.ChannelCount = channelCount
		}
		break
	}

	if result.DurationSeconds == nil && result.CodecName == nil && result.SampleRateHz == nil && result.ChannelCount == nil {
		return audioProbeResult{}, fmt.Errorf("ffmpeg did not return usable audio metadata")
	}

	return result, nil
}

func readFFmpegProbeOutput(ctx context.Context, ffmpegPath, localPath string) (string, error) {
	command := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-i", localPath)
	output, err := command.CombinedOutput()
	if len(output) == 0 {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("ffmpeg returned empty probe output")
	}

	return string(output), nil
}

func parseFFmpegDuration(output string) *float64 {
	match := ffmpegDurationPattern.FindStringSubmatch(output)
	if len(match) != 4 {
		return nil
	}

	hours, err := strconv.Atoi(match[1])
	if err != nil {
		return nil
	}
	minutes, err := strconv.Atoi(match[2])
	if err != nil {
		return nil
	}
	seconds, err := strconv.ParseFloat(match[3], 64)
	if err != nil {
		return nil
	}

	duration := float64(hours*3600+minutes*60) + seconds
	return &duration
}

func parseCodecNameFromFFmpegLine(line, marker string) string {
	parts := strings.SplitN(line, marker, 2)
	if len(parts) != 2 {
		return ""
	}

	codecField := strings.TrimSpace(strings.SplitN(parts[1], ",", 2)[0])
	codecField = strings.TrimSpace(strings.SplitN(codecField, " ", 2)[0])
	codecField = strings.TrimSpace(strings.SplitN(codecField, "(", 2)[0])
	return strings.ToLower(codecField)
}

func parseFFmpegDimensions(line string) (*int, *int) {
	match := ffmpegDimensionPattern.FindStringSubmatch(line)
	if len(match) != 3 {
		return nil, nil
	}

	width, err := strconv.Atoi(match[1])
	if err != nil {
		return nil, nil
	}
	height, err := strconv.Atoi(match[2])
	if err != nil {
		return nil, nil
	}

	return &width, &height
}

func parseFFmpegSampleRate(line string) *int {
	match := ffmpegSampleRatePattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return nil
	}

	sampleRate, err := strconv.Atoi(match[1])
	if err != nil {
		return nil
	}

	return &sampleRate
}

func parseFFmpegChannelCount(line string) *int {
	match := ffmpegChannelCountPattern.FindStringSubmatch(line)
	if len(match) == 2 {
		channelCount, err := strconv.Atoi(match[1])
		if err == nil && channelCount > 0 {
			return &channelCount
		}
	}

	lowered := strings.ToLower(line)
	switch {
	case strings.Contains(lowered, "7.1"):
		return intPointer(8)
	case strings.Contains(lowered, "6.1"):
		return intPointer(7)
	case strings.Contains(lowered, "5.1"):
		return intPointer(6)
	case strings.Contains(lowered, "4.1"):
		return intPointer(5)
	case strings.Contains(lowered, "4.0"):
		return intPointer(4)
	case strings.Contains(lowered, "3.1"):
		return intPointer(4)
	case strings.Contains(lowered, "3.0"):
		return intPointer(3)
	case strings.Contains(lowered, "2.1"):
		return intPointer(3)
	case strings.Contains(lowered, "stereo"):
		return intPointer(2)
	case strings.Contains(lowered, "mono"):
		return intPointer(1)
	default:
		return nil
	}
}

func intPointer(value int) *int {
	return &value
}

func createThumbnail(source image.Image, maxWidth, maxHeight int) (image.Image, int, int) {
	bounds := source.Bounds()
	sourceWidth := bounds.Dx()
	sourceHeight := bounds.Dy()

	if sourceWidth <= 0 || sourceHeight <= 0 {
		placeholder := image.NewNRGBA(image.Rect(0, 0, maxWidth, maxHeight))
		fillImage(placeholder, color.NRGBA{R: 18, G: 23, B: 34, A: 255})
		return placeholder, maxWidth, maxHeight
	}

	scale := math.Min(float64(maxWidth)/float64(sourceWidth), float64(maxHeight)/float64(sourceHeight))
	if scale > 1 {
		scale = 1
	}

	targetWidth := maxInt(1, int(math.Round(float64(sourceWidth)*scale)))
	targetHeight := maxInt(1, int(math.Round(float64(sourceHeight)*scale)))
	target := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))

	for y := 0; y < targetHeight; y++ {
		sourceY := bounds.Min.Y + int(float64(y)*float64(sourceHeight)/float64(targetHeight))
		for x := 0; x < targetWidth; x++ {
			sourceX := bounds.Min.X + int(float64(x)*float64(sourceWidth)/float64(targetWidth))
			target.Set(x, y, source.At(sourceX, sourceY))
		}
	}

	return target, targetWidth, targetHeight
}

func fillImage(target *image.NRGBA, fill color.NRGBA) {
	for y := 0; y < target.Bounds().Dy(); y++ {
		for x := 0; x < target.Bounds().Dx(); x++ {
			target.SetNRGBA(x, y, fill)
		}
	}
}

func writePNG(outputPath string, source image.Image) error {
	if err := ensureDirectory(filepath.Dir(outputPath)); err != nil {
		return err
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create png file: %w", err)
	}
	defer file.Close()

	if err := png.Encode(file, source); err != nil {
		return fmt.Errorf("encode png: %w", err)
	}
	return nil
}

func readImageDimensions(path string) (int, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, err
	}

	return config.Width, config.Height, nil
}

func ensureDirectory(path string) error {
	return os.MkdirAll(path, 0o755)
}

func detectContentType(path string) string {
	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); contentType != "" {
		return contentType
	}

	file, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer file.Close()

	buffer := make([]byte, 512)
	readBytes, _ := file.Read(buffer)
	return http.DetectContentType(buffer[:readBytes])
}

func previewExtensionForArtwork(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}

func readableReplicaPriority(endpointType string) int {
	switch strings.ToUpper(strings.TrimSpace(endpointType)) {
	case string(connectors.EndpointTypeLocal):
		return 0
	case string(connectors.EndpointTypeRemovable):
		return 1
	case string(connectors.EndpointTypeQNAP):
		return 2
	case string(connectors.EndpointTypeNetwork):
		return 3
	default:
		return 9
	}
}

func sameNullableString(left, right *string) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return strings.TrimSpace(*left) == strings.TrimSpace(*right)
	}
}

func posterPreviewKindForMediaType(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case string(connectors.MediaTypeImage):
		return previewKindImageThumbnail
	case string(connectors.MediaTypeVideo):
		return previewKindVideoCover
	case string(connectors.MediaTypeAudio):
		return previewKindAudioArtwork
	default:
		return ""
	}
}

func (service *Service) posterURL(assetID string) string {
	return fmt.Sprintf("/api/v1/catalog/assets/%s/poster", assetID)
}

func (service *Service) previewURL(assetID string) string {
	return fmt.Sprintf("/api/v1/catalog/assets/%s/preview", assetID)
}

func nullableStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func nullableIntValue(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableFloatValue(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func codecNameFromPath(path string) *string {
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if extension == "" {
		return nil
	}
	return &extension
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
