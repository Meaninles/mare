import { AudioLines, Pause, Play } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { formatDurationSeconds } from "../../lib/catalog-view";
import type { CatalogAudioMetadata } from "../../types/catalog";

export function AudioPreview({
  src,
  poster,
  title,
  metadata
}: {
  src?: string;
  poster?: string;
  title: string;
  metadata?: CatalogAudioMetadata;
}) {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(metadata?.durationSeconds ?? 0);
  const [hasError, setHasError] = useState(false);

  useEffect(() => {
    setDuration(metadata?.durationSeconds ?? 0);
  }, [metadata?.durationSeconds]);

  useEffect(() => {
    const audio = audioRef.current;
    return () => {
      audio?.pause();
    };
  }, []);

  async function handleTogglePlayback() {
    const audio = audioRef.current;
    if (!audio) {
      return;
    }

    if (audio.paused) {
      try {
        await audio.play();
        setIsPlaying(true);
      } catch {
        setHasError(true);
      }
      return;
    }

    audio.pause();
    setIsPlaying(false);
  }

  function handleSeek(nextTime: number) {
    const audio = audioRef.current;
    if (!audio) {
      return;
    }

    audio.currentTime = nextTime;
    setCurrentTime(nextTime);
  }

  if (!src || hasError) {
    return (
      <div className="media-preview-empty">
        <AudioLines size={28} />
        <div>
          <h4>音频不可用</h4>
          <p>当前没有可播放的音频源。</p>
        </div>
      </div>
    );
  }

  return (
    <div className="audio-preview-card">
      <audio
        ref={audioRef}
        preload="metadata"
        src={src}
        onLoadedMetadata={(event) => setDuration(event.currentTarget.duration || metadata?.durationSeconds || 0)}
        onTimeUpdate={(event) => setCurrentTime(event.currentTarget.currentTime)}
        onEnded={() => setIsPlaying(false)}
        onPause={() => setIsPlaying(false)}
        onPlay={() => setIsPlaying(true)}
        onError={() => setHasError(true)}
      />

      <div className="audio-preview-head">
        {poster ? (
          <img src={poster} alt={title} className="audio-artwork" />
        ) : (
          <div className="audio-artwork-placeholder">
            <AudioLines size={28} />
          </div>
        )}

        <div className="audio-preview-copy">
          <span className="eyebrow">音频</span>
          <h4>{title}</h4>
          <p>播放与基础信息</p>
        </div>
      </div>

      <div className="audio-control-row">
        <button type="button" className="audio-toggle-button" onClick={handleTogglePlayback}>
          {isPlaying ? <Pause size={18} /> : <Play size={18} />}
        </button>

        <input
          className="audio-progress"
          type="range"
          min={0}
          max={Math.max(duration, 0)}
          step={0.1}
          value={Math.min(currentTime, duration || currentTime)}
          onChange={(event) => handleSeek(Number(event.target.value))}
          aria-label="音频播放进度"
        />
      </div>

      <div className="audio-time-row">
        <span>{formatDurationSeconds(currentTime)}</span>
        <span>{formatDurationSeconds(duration)}</span>
      </div>

      <div className="audio-metadata-grid">
        <div className="audio-metadata-pill">
          <span>时长</span>
          <strong>{formatDurationSeconds(metadata?.durationSeconds ?? duration)}</strong>
        </div>
        <div className="audio-metadata-pill">
          <span>编码</span>
          <strong>{metadata?.codecName ?? "待分析"}</strong>
        </div>
        <div className="audio-metadata-pill">
          <span>采样率</span>
          <strong>{metadata?.sampleRateHz ? `${metadata.sampleRateHz} Hz` : "待分析"}</strong>
        </div>
      </div>
    </div>
  );
}
