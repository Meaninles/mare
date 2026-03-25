import type { CatalogAsset, CatalogReplica } from "../types/catalog";

export type AssetTone = "success" | "warning" | "danger" | "neutral";
export type AssetStatusFilter =
  | "all"
  | "ready"
  | "partial"
  | "processing"
  | "conflict"
  | "pending_delete"
  | "single"
  | "deleted";

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
  switch (safeLower(asset.assetStatus)) {
    case "processing":
      return "processing";
    case "conflict":
      return "conflict";
    case "pending_delete":
      return "pending_delete";
    case "deleted":
      return "deleted";
    case "partial":
      return "partial";
    default:
      break;
  }

  if (getAvailableReplicaCount(asset) === 1) {
    return "single";
  }

  return "ready";
}

export function getAssetStatusLabel(asset: CatalogAsset): string {
  switch (getAssetStatusFilterValue(asset)) {
    case "ready":
      return "完整可用";
    case "partial":
      return "部分缺失";
    case "processing":
      return "处理中";
    case "conflict":
      return "冲突候选";
    case "pending_delete":
      return "待删除";
    case "single":
      return "仅单端存在";
    case "deleted":
      return "已删除";
  }
}

export function getAssetTone(asset: CatalogAsset): AssetTone {
  switch (getAssetStatusFilterValue(asset)) {
    case "ready":
      return "success";
    case "partial":
    case "processing":
      return "warning";
    case "single":
      return "danger";
    case "conflict":
    case "pending_delete":
      return "neutral";
    case "deleted":
      return "danger";
  }
}

export function getReplicaTone(replica: CatalogReplica): AssetTone {
  if (!replica.existsFlag) {
    return "danger";
  }

  switch (safeLower(replica.replicaStatus)) {
    case "active":
      return "success";
    case "processing":
    case "restoring":
    case "pending_delete":
      return "warning";
    case "conflict":
      return "neutral";
    default:
      return "neutral";
  }
}

export function normalizeMediaType(mediaType: string): string {
  return safeLower(mediaType).trim();
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

  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return "未记录";
  }

  return parsed.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  });
}

export function formatFileSize(size?: number): string {
  if (!size || size <= 0) {
    return "大小未知";
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
    return "待分析";
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

function safeLower(value?: string) {
  return typeof value === "string" ? value.toLowerCase() : "";
}
