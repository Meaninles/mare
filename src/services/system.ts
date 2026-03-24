import { getDefaultCatalogBackendUrl } from "./catalog";
import type { SystemLogLevel, SystemLogsResponse } from "../types/system";

function normalizeBaseUrl(baseUrl: string) {
  const trimmed = baseUrl.trim();
  return trimmed.length > 0 ? trimmed.replace(/\/+$/, "") : getDefaultCatalogBackendUrl();
}

async function getJson<TResponse>(url: string): Promise<TResponse> {
  const response = await fetch(url);
  const text = await response.text();

  try {
    return JSON.parse(text) as TResponse;
  } catch {
    const snippet = text.trim().slice(0, 160);
    throw new Error(
      snippet
        ? `后端返回了非 JSON 响应（HTTP ${response.status}）：${snippet}`
        : `后端返回了空响应（HTTP ${response.status}）。`
    );
  }
}

export function getDefaultSystemBackendUrl() {
  return getDefaultCatalogBackendUrl();
}

export async function listSystemLogs(
  baseUrl: string,
  payload: {
    limit?: number;
    level?: SystemLogLevel;
  } = {}
): Promise<SystemLogsResponse> {
  const params = new URLSearchParams();
  if (payload.limit && payload.limit > 0) {
    params.set("limit", String(payload.limit));
  }
  if (payload.level && payload.level !== "all") {
    params.set("level", payload.level);
  }

  const suffix = params.toString() ? `?${params.toString()}` : "";
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/system/logs${suffix}`);
}
