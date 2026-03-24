package catalog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type MediaToolOverrides struct {
	FFmpegPath  string
	FFprobePath string
}

type VideoToolAnalysis struct {
	FileName        string   `json:"fileName"`
	ContentType     string   `json:"contentType"`
	FFmpegPath      string   `json:"ffmpegPath"`
	FFprobePath     string   `json:"ffprobePath"`
	DurationSeconds *float64 `json:"durationSeconds,omitempty"`
	CodecName       *string  `json:"codecName,omitempty"`
	Width           *int     `json:"width,omitempty"`
	Height          *int     `json:"height,omitempty"`
	CoverDataURL    string   `json:"coverDataUrl"`
}

type AudioToolAnalysis struct {
	FileName        string   `json:"fileName"`
	ContentType     string   `json:"contentType"`
	FFprobePath     string   `json:"ffprobePath"`
	DurationSeconds *float64 `json:"durationSeconds,omitempty"`
	CodecName       *string  `json:"codecName,omitempty"`
	SampleRateHz    *int     `json:"sampleRateHz,omitempty"`
	ChannelCount    *int     `json:"channelCount,omitempty"`
	ArtworkDataURL  *string  `json:"artworkDataUrl,omitempty"`
	ArtworkWidth    *int     `json:"artworkWidth,omitempty"`
	ArtworkHeight   *int     `json:"artworkHeight,omitempty"`
}

type videoProbeResult struct {
	DurationSeconds *float64
	CodecName       *string
	Width           *int
	Height          *int
}

type ffprobeVideoPayload struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func (service *Service) AnalyzeUploadedVideo(
	ctx context.Context,
	fileName string,
	source io.Reader,
	overrides MediaToolOverrides,
) (VideoToolAnalysis, error) {
	localPath, cleanup, err := service.writeUploadedToolFile(fileName, source)
	if err != nil {
		return VideoToolAnalysis{}, err
	}
	defer cleanup()

	ffmpegPath, err := service.resolveToolFFmpegBinary(overrides)
	if err != nil {
		return VideoToolAnalysis{}, err
	}

	probe, probeBinaryPath, err := service.probeVideoForTool(ctx, localPath, ffmpegPath, overrides)
	if err != nil {
		return VideoToolAnalysis{}, err
	}

	outputPath := filepath.Join(service.mediaConfig.CacheRoot, "tools", "video-covers", uuid.NewString()+".png")
	if err := ensureDirectory(filepath.Dir(outputPath)); err != nil {
		return VideoToolAnalysis{}, err
	}

	command := exec.CommandContext(
		ctx,
		ffmpegPath,
		"-y",
		"-i", localPath,
		"-vf", fmt.Sprintf("thumbnail,scale=%d:%d:force_original_aspect_ratio=decrease", thumbnailMaxDimension, thumbnailMaxDimension),
		"-frames:v", "1",
		outputPath,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return VideoToolAnalysis{}, fmt.Errorf("ffmpeg cover extraction failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	defer func() {
		_ = os.Remove(outputPath)
	}()

	coverDataURL, err := encodeFileToDataURL(outputPath)
	if err != nil {
		return VideoToolAnalysis{}, err
	}

	return VideoToolAnalysis{
		FileName:        fileName,
		ContentType:     detectContentType(localPath),
		FFmpegPath:      ffmpegPath,
		FFprobePath:     probeBinaryPath,
		DurationSeconds: probe.DurationSeconds,
		CodecName:       probe.CodecName,
		Width:           probe.Width,
		Height:          probe.Height,
		CoverDataURL:    coverDataURL,
	}, nil
}

func (service *Service) AnalyzeUploadedAudio(
	ctx context.Context,
	fileName string,
	source io.Reader,
	overrides MediaToolOverrides,
) (AudioToolAnalysis, error) {
	localPath, cleanup, err := service.writeUploadedToolFile(fileName, source)
	if err != nil {
		return AudioToolAnalysis{}, err
	}
	defer cleanup()

	probe, probeBinaryPath, err := service.probeAudioForTool(ctx, localPath, overrides)
	if err != nil {
		return AudioToolAnalysis{}, err
	}

	result := AudioToolAnalysis{
		FileName:        fileName,
		ContentType:     detectContentType(localPath),
		FFprobePath:     probeBinaryPath,
		DurationSeconds: probe.DurationSeconds,
		CodecName:       probe.CodecName,
		SampleRateHz:    probe.SampleRateHz,
		ChannelCount:    probe.ChannelCount,
	}

	artwork, err := readEmbeddedArtwork(localPath)
	if err != nil {
		artwork = nil
	}
	if artwork != nil {
		outputPath := filepath.Join(service.mediaConfig.CacheRoot, "tools", "audio-artwork", uuid.NewString()+previewExtensionForArtwork(artwork.MIMEType))
		if err := ensureDirectory(filepath.Dir(outputPath)); err != nil {
			return AudioToolAnalysis{}, err
		}
		if err := os.WriteFile(outputPath, artwork.Data, 0o644); err != nil {
			return AudioToolAnalysis{}, fmt.Errorf("write embedded artwork: %w", err)
		}
		defer func() {
			_ = os.Remove(outputPath)
		}()

		dataURL, err := encodeBytesToDataURL(artwork.Data, defaultString(strings.TrimSpace(artwork.MIMEType), detectContentType(outputPath)))
		if err != nil {
			return AudioToolAnalysis{}, err
		}
		result.ArtworkDataURL = &dataURL
		result.ArtworkWidth = artwork.Width
		result.ArtworkHeight = artwork.Height
	}

	return result, nil
}

func probeVideoWithFFprobe(ctx context.Context, ffprobePath, localPath string) (videoProbeResult, error) {
	command := exec.CommandContext(
		ctx,
		ffprobePath,
		"-v", "error",
		"-show_format",
		"-show_streams",
		"-print_format", "json",
		localPath,
	)
	output, err := command.Output()
	if err != nil {
		return videoProbeResult{}, err
	}

	var payload ffprobeVideoPayload
	if err := json.Unmarshal(output, &payload); err != nil {
		return videoProbeResult{}, err
	}

	result := videoProbeResult{}
	if durationText := strings.TrimSpace(payload.Format.Duration); durationText != "" {
		if duration, err := strconv.ParseFloat(durationText, 64); err == nil {
			result.DurationSeconds = &duration
		}
	}

	for _, stream := range payload.Streams {
		if stream.CodecType != "video" {
			continue
		}
		if codecName := strings.TrimSpace(stream.CodecName); codecName != "" {
			result.CodecName = stringPointer(codecName)
		}
		if stream.Width > 0 {
			width := stream.Width
			result.Width = &width
		}
		if stream.Height > 0 {
			height := stream.Height
			result.Height = &height
		}
		break
	}

	return result, nil
}

func (service *Service) writeUploadedToolFile(fileName string, source io.Reader) (string, func(), error) {
	tempDir := filepath.Join(service.mediaConfig.CacheRoot, "tools", "uploads")
	if err := ensureDirectory(tempDir); err != nil {
		return "", nil, err
	}

	safeExt := filepath.Ext(strings.TrimSpace(fileName))
	tempFile, err := os.CreateTemp(tempDir, "tool-*"+safeExt)
	if err != nil {
		return "", nil, err
	}

	if _, err := io.Copy(tempFile, source); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
		return "", nil, err
	}

	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempFile.Name())
		return "", nil, err
	}

	return tempFile.Name(), func() {
		_ = os.Remove(tempFile.Name())
	}, nil
}

func (service *Service) resolveToolFFmpegBinary(overrides MediaToolOverrides) (string, error) {
	if strings.TrimSpace(overrides.FFmpegPath) != "" {
		return resolveBinary(overrides.FFmpegPath)
	}
	return service.resolveFFmpegBinary()
}

func (service *Service) resolveToolFFprobeBinary(overrides MediaToolOverrides) (string, error) {
	switch {
	case strings.TrimSpace(overrides.FFprobePath) != "":
		return resolveBinary(overrides.FFprobePath)
	case strings.TrimSpace(overrides.FFmpegPath) != "":
		return resolveBinary(deriveFFprobePath(overrides.FFmpegPath))
	default:
		return service.resolveFFprobeBinary()
	}
}

func (service *Service) probeVideoForTool(
	ctx context.Context,
	localPath string,
	ffmpegPath string,
	overrides MediaToolOverrides,
) (videoProbeResult, string, error) {
	if ffprobePath, err := service.resolveToolFFprobeBinary(overrides); err == nil {
		probe, probeErr := probeVideoWithFFprobe(ctx, ffprobePath, localPath)
		if probeErr == nil {
			return probe, ffprobePath, nil
		}
	}

	probe, err := probeVideoWithFFmpeg(ctx, ffmpegPath, localPath)
	if err != nil {
		return videoProbeResult{}, "", err
	}

	return probe, ffmpegPath, nil
}

func (service *Service) probeAudioForTool(
	ctx context.Context,
	localPath string,
	overrides MediaToolOverrides,
) (audioProbeResult, string, error) {
	if ffprobePath, err := service.resolveToolFFprobeBinary(overrides); err == nil {
		probe, probeErr := probeAudioWithFFprobe(ffprobePath, localPath)
		if probeErr == nil {
			return probe, ffprobePath, nil
		}
	}

	if ffmpegPath, err := service.resolveToolFFmpegBinary(overrides); err == nil {
		probe, probeErr := probeAudioWithFFmpeg(ctx, ffmpegPath, localPath)
		if probeErr == nil {
			return probe, ffmpegPath, nil
		}
	}

	probe, err := probeAudioWithFallback(localPath)
	if err != nil {
		return audioProbeResult{}, "", err
	}

	return probe, "", nil
}

func encodeFileToDataURL(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return encodeBytesToDataURL(data, detectContentType(path))
}

func encodeBytesToDataURL(data []byte, mimeType string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("media payload is empty")
	}
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "application/octet-stream"
	}

	return fmt.Sprintf(
		"data:%s;base64,%s",
		mimeType,
		base64.StdEncoding.EncodeToString(data),
	), nil
}
