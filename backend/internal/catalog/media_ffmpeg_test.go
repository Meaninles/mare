package catalog

import "testing"

func TestProbeVideoWithFFmpegOutputParsing(t *testing.T) {
	t.Parallel()

	output := `Input #0, mov,mp4,m4a,3gp,3g2,mj2, from 'sample.mp4':
  Metadata:
    major_brand     : isom
  Duration: 00:01:02.50, start: 0.000000, bitrate: 1240 kb/s
  Stream #0:0(und): Video: h264 (High), yuv420p(progressive), 1920x1080, 1106 kb/s, 25 fps, 25 tbr, 12800 tbn (default)
  Stream #0:1(und): Audio: aac (LC), 48000 Hz, stereo, fltp, 125 kb/s (default)`

	duration := parseFFmpegDuration(output)
	if duration == nil || *duration != 62.5 {
		t.Fatalf("expected duration 62.5, got %#v", duration)
	}

	codecName := parseCodecNameFromFFmpegLine("Stream #0:0: Video: h264 (High), yuv420p(progressive), 1920x1080", "Video:")
	if codecName != "h264" {
		t.Fatalf("expected h264 codec, got %q", codecName)
	}

	width, height := parseFFmpegDimensions(output)
	if width == nil || height == nil || *width != 1920 || *height != 1080 {
		t.Fatalf("expected 1920x1080, got %#v x %#v", width, height)
	}
}

func TestProbeAudioWithFFmpegOutputParsing(t *testing.T) {
	t.Parallel()

	line := "Stream #0:0: Audio: aac (LC), 44100 Hz, stereo, fltp, 192 kb/s"

	codecName := parseCodecNameFromFFmpegLine(line, "Audio:")
	if codecName != "aac" {
		t.Fatalf("expected aac codec, got %q", codecName)
	}

	sampleRate := parseFFmpegSampleRate(line)
	if sampleRate == nil || *sampleRate != 44100 {
		t.Fatalf("expected sample rate 44100, got %#v", sampleRate)
	}

	channelCount := parseFFmpegChannelCount(line)
	if channelCount == nil || *channelCount != 2 {
		t.Fatalf("expected stereo to resolve to 2 channels, got %#v", channelCount)
	}
}
