export interface VideoToolAnalysis {
  fileName: string;
  contentType: string;
  ffmpegPath: string;
  ffprobePath: string;
  durationSeconds?: number;
  codecName?: string;
  width?: number;
  height?: number;
  coverDataUrl: string;
}

export interface AudioToolAnalysis {
  fileName: string;
  contentType: string;
  ffprobePath: string;
  durationSeconds?: number;
  codecName?: string;
  sampleRateHz?: number;
  channelCount?: number;
  artworkDataUrl?: string;
  artworkWidth?: number;
  artworkHeight?: number;
}

export interface MediaToolResponse<TAnalysis> {
  success: boolean;
  analysis?: TAnalysis;
  error?: string;
}
