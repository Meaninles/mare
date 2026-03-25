import { Expand, ImageOff, X } from "lucide-react";
import { useState } from "react";
import { createPortal } from "react-dom";

export function ImagePreview({ src, alt }: { src?: string; alt: string }) {
  const [isOpen, setIsOpen] = useState(false);
  const [hasError, setHasError] = useState(false);

  if (!src || hasError) {
    return (
      <div className="media-preview-empty">
        <ImageOff size={28} />
        <div>
          <h4>预览不可用</h4>
          <p>没有可显示的图像。</p>
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
          放大
        </span>
      </button>

      {isOpen && typeof document !== "undefined"
        ? createPortal(
            <div
              className="media-lightbox"
              role="dialog"
              aria-modal="true"
              aria-label={`${alt} 预览`}
              onClick={() => setIsOpen(false)}
            >
              <button
                type="button"
                className="media-lightbox-close"
                aria-label="关闭图片预览"
                onClick={() => setIsOpen(false)}
              >
                <X size={18} />
              </button>

              <div className="media-lightbox-panel" onClick={(event) => event.stopPropagation()}>
                <img src={src} alt={alt} className="media-lightbox-image" />
              </div>
            </div>,
            document.body
          )
        : null}
    </>
  );
}
