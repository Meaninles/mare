import { Clapperboard } from "lucide-react";
import { useState } from "react";

export function VideoPreview({
  src,
  poster,
  title
}: {
  src?: string;
  poster?: string;
  title: string;
}) {
  const [hasError, setHasError] = useState(false);

  if (!src || hasError) {
    return (
      <div className="media-preview-empty">
        <Clapperboard size={28} />
        <div>
          <h4>视频预览暂不可用</h4>
          <p>当前没有可读取的视频副本，或浏览器暂时无法加载该视频流。</p>
        </div>
      </div>
    );
  }

  return (
    <div className="video-preview-shell">
      <video
        className="media-video"
        controls
        preload="metadata"
        playsInline
        poster={poster}
        onError={() => setHasError(true)}
      >
        <source src={src} />
        {title}
      </video>
    </div>
  );
}
