import type {
  CatalogAssetsResponse,
  CatalogAssetQueryOptions,
  CatalogAssetInsightsResponse,
  CatalogBatchRestoreResponse,
  CatalogDeleteEndpointResponse,
  CatalogDeleteReplicaResponse,
  CatalogEndpointPayload,
  CatalogEndpointsResponse,
  CatalogRestoreAssetResponse,
  CatalogRetryResponse,
  CatalogScanResponse,
  CatalogSyncOverviewResponse,
  CatalogTasksResponse
} from "../types/catalog";

const DEFAULT_BACKEND_URL = "http://127.0.0.1:8080";

function normalizeBaseUrl(baseUrl: string): string {
  const trimmed = baseUrl.trim();
  return trimmed.length > 0 ? trimmed.replace(/\/+$/, "") : DEFAULT_BACKEND_URL;
}

async function getJson<TResponse>(url: string): Promise<TResponse> {
  const response = await fetch(url);
  return readJsonResponse<TResponse>(response);
}

async function postJson<TResponse>(url: string, payload: unknown): Promise<TResponse> {
  const response = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json"
    },
    body: JSON.stringify(payload)
  });

  return readJsonResponse<TResponse>(response);
}

async function putJson<TResponse>(url: string, payload: unknown): Promise<TResponse> {
  const response = await fetch(url, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json"
    },
    body: JSON.stringify(payload)
  });

  return readJsonResponse<TResponse>(response);
}

async function deleteJson<TResponse>(url: string): Promise<TResponse> {
  const response = await fetch(url, {
    method: "DELETE"
  });

  return readJsonResponse<TResponse>(response);
}

async function readJsonResponse<TResponse>(response: Response): Promise<TResponse> {
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

function resolveCatalogMediaUrl(baseUrl: string, value?: string): string | undefined {
  if (!value) {
    return undefined;
  }

  if (/^https?:\/\//i.test(value)) {
    return value;
  }

  const normalizedBaseUrl = normalizeBaseUrl(baseUrl);
  return `${normalizedBaseUrl}${value.startsWith("/") ? value : `/${value}`}`;
}

function normalizeCatalogTask(task: Record<string, unknown>) {
  return {
    id: String(task.id ?? task.ID ?? ""),
    taskType: String(task.taskType ?? task.TaskType ?? ""),
    status: String(task.status ?? task.Status ?? ""),
    payload: String(task.payload ?? task.Payload ?? ""),
    resultSummary:
      typeof (task.resultSummary ?? task.ResultSummary) === "string"
        ? String(task.resultSummary ?? task.ResultSummary)
        : undefined,
    errorMessage:
      typeof (task.errorMessage ?? task.ErrorMessage) === "string"
        ? String(task.errorMessage ?? task.ErrorMessage)
        : undefined,
    retryCount: Number(task.retryCount ?? task.RetryCount ?? 0),
    createdAt: String(task.createdAt ?? task.CreatedAt ?? ""),
    updatedAt: String(task.updatedAt ?? task.UpdatedAt ?? ""),
    startedAt:
      typeof (task.startedAt ?? task.StartedAt) === "string"
        ? String(task.startedAt ?? task.StartedAt)
        : undefined,
    finishedAt:
      typeof (task.finishedAt ?? task.FinishedAt) === "string"
        ? String(task.finishedAt ?? task.FinishedAt)
        : undefined
  };
}

export function getDefaultCatalogBackendUrl(): string {
  return DEFAULT_BACKEND_URL;
}

export async function listCatalogEndpoints(baseUrl: string): Promise<CatalogEndpointsResponse> {
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/endpoints`);
}

export async function saveCatalogEndpoint(baseUrl: string, payload: CatalogEndpointPayload): Promise<CatalogEndpointsResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/endpoints`, payload);
}

export async function updateCatalogEndpoint(
  baseUrl: string,
  endpointId: string,
  payload: CatalogEndpointPayload
): Promise<CatalogEndpointsResponse> {
  return putJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/endpoints/${endpointId}`, payload);
}

export async function deleteCatalogEndpoint(baseUrl: string, endpointId: string): Promise<CatalogDeleteEndpointResponse> {
  return deleteJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/endpoints/${endpointId}`);
}

export async function runCatalogFullScan(baseUrl: string): Promise<CatalogScanResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/scans/full`, {});
}

export async function runCatalogEndpointScan(baseUrl: string, endpointId: string): Promise<CatalogScanResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/scans/endpoint`, { endpointId });
}

export async function listCatalogAssets(
  baseUrl: string,
  options: CatalogAssetQueryOptions = {}
): Promise<CatalogAssetsResponse> {
  const params = new URLSearchParams();
  params.set("limit", String(options.limit ?? 200));

  if (options.query?.trim()) {
    params.set("q", options.query.trim());
  }
  if (options.mediaType?.trim()) {
    params.set("mediaType", options.mediaType.trim());
  }
  if (options.assetStatus?.trim()) {
    params.set("status", options.assetStatus.trim());
  }

  const response = await getJson<CatalogAssetsResponse>(
    `${normalizeBaseUrl(baseUrl)}/api/v1/catalog/assets?${params.toString()}`
  );

  if (response.assets) {
    response.assets = response.assets.map((asset) => ({
      ...asset,
      previewUrl: resolveCatalogMediaUrl(baseUrl, asset.previewUrl),
      poster: asset.poster
        ? {
            ...asset.poster,
            url: resolveCatalogMediaUrl(baseUrl, asset.poster.url) ?? asset.poster.url
          }
        : undefined
    }));
  }

  return response;
}

export async function getCatalogAssetInsights(
  baseUrl: string,
  assetId: string
): Promise<CatalogAssetInsightsResponse> {
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/assets/${encodeURIComponent(assetId)}/insights`);
}

export async function deleteCatalogReplica(
  baseUrl: string,
  payload: {
    assetId: string;
    targetEndpointId: string;
  }
): Promise<CatalogDeleteReplicaResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/replicas/delete`, payload);
}

export async function listCatalogTasks(baseUrl: string, limit = 100): Promise<CatalogTasksResponse> {
  const response = await getJson<CatalogTasksResponse>(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/tasks?limit=${limit}`);
  if (response.tasks) {
    response.tasks = response.tasks.map((task) => normalizeCatalogTask(task as unknown as Record<string, unknown>));
  }
  return response;
}

export async function getCatalogSyncOverview(baseUrl: string): Promise<CatalogSyncOverviewResponse> {
  const response = await getJson<CatalogSyncOverviewResponse>(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/sync/overview`);

  if (response.overview) {
    response.overview.recoverableAssets = response.overview.recoverableAssets.map((asset) => ({
      ...asset,
      poster: asset.poster
        ? {
            ...asset.poster,
            url: resolveCatalogMediaUrl(baseUrl, asset.poster.url) ?? asset.poster.url
          }
        : undefined
    }));

    response.overview.conflictAssets = response.overview.conflictAssets.map((asset) => ({
      ...asset,
      poster: asset.poster
        ? {
            ...asset.poster,
            url: resolveCatalogMediaUrl(baseUrl, asset.poster.url) ?? asset.poster.url
          }
        : undefined
    }));

    response.overview.runningTasks = (response.overview.runningTasks ?? []).map((task) =>
      normalizeCatalogTask(task as unknown as Record<string, unknown>)
    );
    response.overview.failedTasks = (response.overview.failedTasks ?? []).map((task) =>
      normalizeCatalogTask(task as unknown as Record<string, unknown>)
    );
  }

  return response;
}

export async function restoreCatalogAsset(
  baseUrl: string,
  payload: {
    assetId: string;
    sourceEndpointId: string;
    targetEndpointId: string;
  }
): Promise<CatalogRestoreAssetResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/sync/restore`, payload);
}

export async function restoreCatalogAssetsToEndpoint(
  baseUrl: string,
  payload: {
    targetEndpointId: string;
    assetIds: string[];
  }
): Promise<CatalogBatchRestoreResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/sync/restore/batch`, payload);
}

export async function retryCatalogSyncTask(baseUrl: string, taskId: string): Promise<CatalogRetryResponse> {
  return retryCatalogTask(baseUrl, taskId);
}

export async function retryCatalogTask(baseUrl: string, taskId: string): Promise<CatalogRetryResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/tasks/retry`, { taskId });
}
