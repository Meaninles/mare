import { FormEvent, useEffect, useState } from "react";
import { CheckCircle2, LoaderCircle, Music4, Sparkles, SquarePlay } from "lucide-react";
import { AudioPreview } from "../components/media/AudioPreview";
import { VideoPreview } from "../components/media/VideoPreview";
import { formatDurationSeconds } from "../lib/catalog-view";
import {
  analyzeAudioFile,
  analyzeVideoFile,
  getDefaultMediaToolBackendUrl
} from "../services/media-tools";
import type { AudioToolAnalysis, VideoToolAnalysis } from "../types/media-tools";

export function MediaLabPage() {
  const [backendUrl, setBackendUrl] = useState(getDefaultMediaToolBackendUrl());
  const [ffmpegPath, setFFmpegPath] = useState("");
  const [ffprobePath, setFFprobePath] = useState("");

  const [videoFile, setVideoFile] = useState<File | null>(null);
  const [videoPreviewUrl, setVideoPreviewUrl] = useState<string>();
  const [videoResult, setVideoResult] = useState<VideoToolAnalysis | null>(null);
  const [videoError, setVideoError] = useState<string | null>(null);
  const [videoBusy, setVideoBusy] = useState(false);

  const [audioFile, setAudioFile] = useState<File | null>(null);
  const [audioPreviewUrl, setAudioPreviewUrl] = useState<string>();
  const [audioResult, setAudioResult] = useState<AudioToolAnalysis | null>(null);
  const [audioError, setAudioError] = useState<string | null>(null);
  const [audioBusy, setAudioBusy] = useState(false);

  useEffect(() => {
    if (!videoFile) {
      setVideoPreviewUrl(undefined);
      return;
    }

    const url = URL.createObjectURL(videoFile);
    setVideoPreviewUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [videoFile]);

  useEffect(() => {
    if (!audioFile) {
      setAudioPreviewUrl(undefined);
      return;
    }

    const url = URL.createObjectURL(audioFile);
    setAudioPreviewUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [audioFile]);

  async function handleVideoAnalyze() {
    if (!videoFile) {
      setVideoError("请先选择一个视频文件。");
      return;
    }

    setVideoBusy(true);
    setVideoError(null);
    try {
      const response = await analyzeVideoFile(backendUrl, {
        file: videoFile,
        ffmpegPath,
        ffprobePath
      });
      if (!response.success || !response.analysis) {
        setVideoError(response.error ?? "视频分析失败。");
        setVideoResult(null);
        return;
      }
      setVideoResult(response.analysis);
    } catch (error) {
      setVideoError(error instanceof Error ? error.message : "视频分析失败。");
      setVideoResult(null);
    } finally {
      setVideoBusy(false);
    }
  }

  async function handleAudioAnalyze() {
    if (!audioFile) {
      setAudioError("请先选择一个音频文件。");
      return;
    }

    setAudioBusy(true);
    setAudioError(null);
    try {
      const response = await analyzeAudioFile(backendUrl, {
        file: audioFile,
        ffmpegPath,
        ffprobePath
      });
      if (!response.success || !response.analysis) {
        setAudioError(response.error ?? "音频分析失败。");
        setAudioResult(null);
        return;
      }
      setAudioResult(response.analysis);
    } catch (error) {
      setAudioError(error instanceof Error ? error.message : "音频分析失败。");
      setAudioResult(null);
    } finally {
      setAudioBusy(false);
    }
  }

  function preventSubmit(event: FormEvent) {
    event.preventDefault();
  }

  return (
    <section className="page-stack">
      <article className="hero-card media-lab-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">媒体实验室</p>
          <h3>上传样本文件，验证封面提取、元数据读取与预览播放能力。</h3>
          <p>默认会优先自动发现 FFmpeg。只有你想手动覆盖路径时，才需要填写下面的绝对路径。</p>
        </div>

        <form className="media-tool-config" onSubmit={preventSubmit}>
          <label className="field">
            <span>后端地址</span>
            <input value={backendUrl} onChange={(event) => setBackendUrl(event.target.value)} />
          </label>
          <label className="field">
            <span>FFmpeg</span>
            <input
              placeholder="可选，填写绝对路径"
              value={ffmpegPath}
              onChange={(event) => setFFmpegPath(event.target.value)}
            />
          </label>
          <label className="field">
            <span>FFprobe</span>
            <input
              placeholder="可选，填写绝对路径"
              value={ffprobePath}
              onChange={(event) => setFFprobePath(event.target.value)}
            />
          </label>
        </form>
      </article>

      <div className="media-lab-grid">
        <article className="detail-card media-lab-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">视频</p>
              <h4>封面与播放</h4>
            </div>
            <span className="status-pill subtle">FFmpeg</span>
          </div>

          <label className="upload-field">
            <span className="upload-field-label">视频文件</span>
            <input
              type="file"
              accept="video/*,.mp4,.mov,.mkv,.avi,.m4v,.webm"
              onChange={(event) => {
                setVideoFile(event.target.files?.[0] ?? null);
                setVideoResult(null);
                setVideoError(null);
              }}
            />
          </label>

          <div className="action-row">
            <button
              type="button"
              className="primary-button"
              onClick={() => void handleVideoAnalyze()}
              disabled={videoBusy}
            >
              {videoBusy ? <LoaderCircle size={16} className="spin" /> : <Sparkles size={16} />}
              提取封面
            </button>
          </div>

          {videoError ? <p className="error-copy">{videoError}</p> : null}

          <div className="media-result-stack">
            <div className="media-cover-panel">
              <div className="section-head media-cover-head">
                <div>
                  <p className="eyebrow">封面</p>
                  <h4>提取结果</h4>
                </div>
                {videoResult?.coverDataUrl ? (
                  <span className="status-pill success">
                    <CheckCircle2 size={14} />
                    已生成
                  </span>
                ) : null}
              </div>

              {videoResult?.coverDataUrl ? (
                <div className="media-cover-frame">
                  <img src={videoResult.coverDataUrl} alt="提取出的视频封面" className="media-cover-image" />
                </div>
              ) : (
                <div className="media-cover-empty">
                  <SquarePlay size={24} />
                  <p>提取出的封面会显示在这里。</p>
                </div>
              )}
            </div>

            <div className="asset-preview-visual tone-neutral">
              <VideoPreview
                src={videoPreviewUrl}
                poster={videoResult?.coverDataUrl}
                title={videoFile?.name ?? "视频样本"}
              />
            </div>
          </div>

          <div className="media-stat-grid">
            <Stat label="文件" value={videoFile?.name ?? "未选择"} />
            <Stat label="时长" value={formatDurationSeconds(videoResult?.durationSeconds)} />
            <Stat label="编码" value={videoResult?.codecName ?? "待分析"} />
            <Stat
              label="分辨率"
              value={
                videoResult?.width && videoResult?.height
                  ? `${videoResult.width} × ${videoResult.height}`
                  : "待分析"
              }
            />
          </div>

          {videoResult?.ffmpegPath ? (
            <div className="media-tool-paths">
              <ToolPath label="FFmpeg" value={videoResult.ffmpegPath} />
              <ToolPath label="FFprobe" value={videoResult.ffprobePath || videoResult.ffmpegPath} />
            </div>
          ) : null}
        </article>

        <article className="detail-card media-lab-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">音频</p>
              <h4>元数据与封面</h4>
            </div>
            <span className="status-pill subtle">探测</span>
          </div>

          <label className="upload-field">
            <span className="upload-field-label">音频文件</span>
            <input
              type="file"
              accept="audio/*,.mp3,.wav,.flac,.m4a,.aac,.ogg"
              onChange={(event) => {
                setAudioFile(event.target.files?.[0] ?? null);
                setAudioResult(null);
                setAudioError(null);
              }}
            />
          </label>

          <div className="action-row">
            <button
              type="button"
              className="primary-button"
              onClick={() => void handleAudioAnalyze()}
              disabled={audioBusy}
            >
              {audioBusy ? <LoaderCircle size={16} className="spin" /> : <Music4 size={16} />}
              读取信息
            </button>
          </div>

          {audioError ? <p className="error-copy">{audioError}</p> : null}

          <div className="asset-preview-visual tone-neutral">
            <AudioPreview
              src={audioPreviewUrl}
              poster={audioResult?.artworkDataUrl}
              title={audioFile?.name ?? "音频样本"}
              metadata={{
                durationSeconds: audioResult?.durationSeconds,
                codecName: audioResult?.codecName,
                sampleRateHz: audioResult?.sampleRateHz,
                channelCount: audioResult?.channelCount
              }}
            />
          </div>

          <div className="media-stat-grid">
            <Stat label="文件" value={audioFile?.name ?? "未选择"} />
            <Stat label="时长" value={formatDurationSeconds(audioResult?.durationSeconds)} />
            <Stat label="编码" value={audioResult?.codecName ?? "待分析"} />
            <Stat label="采样率" value={audioResult?.sampleRateHz ? `${audioResult.sampleRateHz} Hz` : "待分析"} />
            <Stat label="声道" value={audioResult?.channelCount ? `${audioResult.channelCount}` : "待分析"} />
            <Stat label="封面" value={audioResult?.artworkDataUrl ? "已内嵌" : "无"} />
          </div>

          {audioResult?.ffprobePath ? (
            <div className="media-tool-paths">
              <ToolPath label="FFprobe" value={audioResult.ffprobePath} />
            </div>
          ) : null}
        </article>
      </div>
    </section>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="audio-metadata-pill">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function ToolPath({ label, value }: { label: string; value: string }) {
  return (
    <div className="media-tool-path">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}
