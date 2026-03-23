import type { CatalogAsset, CatalogReplica } from "../types/catalog";

export type AssetTone = "success" | "warning" | "danger" | "neutral";
export type AssetStatusFilter = "all" | "ready" | "partial" | "single" | "missing";

export function getAvailableReplicas(asset: CatalogAsset): CatalogReplica[] {
  return asset.replicas.filter((replica) => replica.existsFlag);
}

export function getMissingReplicas(asset: CatalogAsset): CatalogReplica[] {
  return asset.replicas.filter((replica) => !replica.existsFlag);
}

export function getAvailableReplicaCount(asset: CatalogAsset): number {
  return asset.availableReplicaCount ?? getAvailableReplicas(asset).length;
}

export function getMissingReplicaCount(asset: CatalogAsset): number {
  return asset.missingReplicaCount ?? getMissingReplicas(asset).length;
}

export function getAssetStatusFilterValue(asset: CatalogAsset): Exclude<AssetStatusFilter, "all"> {
  if (asset.assetStatus === "missing") {
    return "missing";
  }

  if (getAvailableReplicaCount(asset) === 1) {
    return "single";
  }

  if (asset.assetStatus === "partial") {
    return "partial";
  }

  return "ready";
}

export function getAssetStatusLabel(asset: CatalogAsset): string {
  switch (getAssetStatusFilterValue(asset)) {
    case "ready":
      return "完整可用";
    case "partial":
      return "部分缺失";
    case "single":
      return "单端留存";
    case "missing":
      return "全部缺失";
  }
}

export function getAssetTone(asset: CatalogAsset): AssetTone {
  switch (getAssetStatusFilterValue(asset)) {
    case "ready":
      return "success";
    case "partial":
    case "single":
      return "warning";
    case "missing":
      return "danger";
  }
}

export function getReplicaTone(replica: CatalogReplica): AssetTone {
  if (!replica.existsFlag) {
    return "danger";
  }

  if (replica.replicaStatus.toLowerCase() === "active") {
    return "success";
  }

  return "neutral";
}

export function normalizeMediaType(mediaType: string): string {
  return mediaType.trim().toLowerCase();
}

export function getMediaTypeLabel(mediaType: string): string {
  switch (normalizeMediaType(mediaType)) {
    case "image":
      return "图片";
    case "video":
      return "视频";
    case "audio":
      return "音频";
    default:
      return "媒体";
  }
}

export function formatCatalogDate(value?: string): string {
  if (!value) {
    return "未记录";
  }

  return new Date(value).toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  });
}

export function formatFileSize(size?: number): string {
  if (!size || size <= 0) {
    return "未知大小";
  }

  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = size;
  let unitIndex = 0;

  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  const precision = value >= 100 ? 0 : value >= 10 ? 1 : 2;
  return `${value.toFixed(precision)} ${units[unitIndex]}`;
}

export function formatDurationSeconds(value?: number): string {
  if (!value || value <= 0) {
    return "未解析";
  }

  const totalSeconds = Math.round(value);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  if (hours > 0) {
    return [hours, minutes, seconds].map((item) => item.toString().padStart(2, "0")).join(":");
  }

  return [minutes, seconds].map((item) => item.toString().padStart(2, "0")).join(":");
}
