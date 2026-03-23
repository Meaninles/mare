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
            <h4>当前媒体类型暂无预览</h4>
            <p>这条资产还没有接入可视化预览组件，后续会继续扩展支持范围。</p>
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
