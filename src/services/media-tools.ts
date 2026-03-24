import type { AudioToolAnalysis, MediaToolResponse, VideoToolAnalysis } from "../types/media-tools";

const DEFAULT_BACKEND_URL = "http://127.0.0.1:8080";

function normalizeBaseUrl(baseUrl: string): string {
  const trimmed = baseUrl.trim();
  return trimmed.length > 0 ? trimmed.replace(/\/+$/, "") : DEFAULT_BACKEND_URL;
}

async function postMediaTool<TAnalysis>(
  baseUrl: string,
  endpointPath: string,
  payload: {
    file: File;
    ffmpegPath?: string;
    ffprobePath?: string;
  }
): Promise<MediaToolResponse<TAnalysis>> {
  const formData = new FormData();
  formData.append("file", payload.file);
  if (payload.ffmpegPath?.trim()) {
    formData.append("ffmpegPath", payload.ffmpegPath.trim());
  }
  if (payload.ffprobePath?.trim()) {
    formData.append("ffprobePath", payload.ffprobePath.trim());
  }

  const response = await fetch(`${normalizeBaseUrl(baseUrl)}${endpointPath}`, {
    method: "POST",
    body: formData
  });

  return response.json() as Promise<MediaToolResponse<TAnalysis>>;
}

export function getDefaultMediaToolBackendUrl(): string {
  return DEFAULT_BACKEND_URL;
}

export async function analyzeVideoFile(
  baseUrl: string,
  payload: {
    file: File;
    ffmpegPath?: string;
    ffprobePath?: string;
  }
): Promise<MediaToolResponse<VideoToolAnalysis>> {
  return postMediaTool<VideoToolAnalysis>(baseUrl, "/api/v1/tools/media/video/analyze", payload);
}

export async function analyzeAudioFile(
  baseUrl: string,
  payload: {
    file: File;
    ffmpegPath?: string;
    ffprobePath?: string;
  }
): Promise<MediaToolResponse<AudioToolAnalysis>> {
  return postMediaTool<AudioToolAnalysis>(baseUrl, "/api/v1/tools/media/audio/analyze", payload);
}
