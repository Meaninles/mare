import { Expand, ImageOff, X } from "lucide-react";
import { useState } from "react";

export function ImagePreview({ src, alt }: { src?: string; alt: string }) {
  const [isOpen, setIsOpen] = useState(false);
  const [hasError, setHasError] = useState(false);

  if (!src || hasError) {
    return (
      <div className="media-preview-empty">
        <ImageOff size={28} />
        <div>
          <h4>预览暂不可用</h4>
          <p>当前没有可读取的图片副本，或图片内容暂时无法加载。</p>
        </div>
      </div>
    );
  }

  return (
    <>
      <button type="button" className="image-preview-trigger" onClick={() => setIsOpen(true)}>
        <img src={src} alt={alt} className="media-image" onError={() => setHasError(true)} />
        <span className="image-preview-hint">
          <Expand size={14} />
          放大预览
        </span>
      </button>

      {isOpen ? (
        <div className="media-lightbox" role="dialog" aria-modal="true" aria-label={`${alt} preview`} onClick={() => setIsOpen(false)}>
          <button
            type="button"
            className="media-lightbox-close"
            aria-label="Close image preview"
            onClick={() => setIsOpen(false)}
          >
            <X size={18} />
          </button>

          <div className="media-lightbox-panel" onClick={(event) => event.stopPropagation()}>
            <img src={src} alt={alt} className="media-lightbox-image" />
          </div>
        </div>
      ) : null}
    </>
  );
}
