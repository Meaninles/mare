import { AudioLines, Clapperboard, Images } from "lucide-react";
import { normalizeMediaType } from "../../lib/catalog-view";
import type { CatalogAsset } from "../../types/catalog";
import { AudioPreview } from "./AudioPreview";
import { ImagePreview } from "./ImagePreview";
import { VideoPreview } from "./VideoPreview";

export function AssetPreview({ asset }: { asset: CatalogAsset }) {
  const mediaType = normalizeMediaType(asset.mediaType);

  switch (mediaType) {
    case "image":
      return <ImagePreview src={asset.previewUrl} alt={asset.displayName} />;
    case "video":
      return <VideoPreview src={asset.previewUrl} poster={asset.poster?.url} title={asset.displayName} />;
    case "audio":
      return (
        <AudioPreview
          src={asset.previewUrl}
          poster={asset.poster?.url}
          title={asset.displayName}
          metadata={asset.audioMetadata}
        />
      );
    default:
      return (
        <div className="media-preview-empty">
          <MediaIcon mediaType={asset.mediaType} />
          <div>
            <h4>暂不支持预览</h4>
            <p>这个媒体类型目前还没有对应的预览组件。</p>
          </div>
        </div>
      );
  }
}

function MediaIcon({ mediaType }: { mediaType: string }) {
  switch (normalizeMediaType(mediaType)) {
    case "image":
      return <Images size={28} />;
    case "video":
      return <Clapperboard size={28} />;
    case "audio":
      return <AudioLines size={28} />;
    default:
      return <Images size={28} />;
  }
}
