import type {
  CatalogAssetsResponse,
  CatalogEndpointsResponse,
  CatalogScanResponse,
  CatalogTasksResponse
} from "../types/catalog";

const DEFAULT_BACKEND_URL = "http://127.0.0.1:8080";

function normalizeBaseUrl(baseUrl: string): string {
  const trimmed = baseUrl.trim();
  return trimmed.length > 0 ? trimmed.replace(/\/+$/, "") : DEFAULT_BACKEND_URL;
}

async function getJson<TResponse>(url: string): Promise<TResponse> {
  const response = await fetch(url);
  return response.json() as Promise<TResponse>;
}

async function postJson<TResponse>(url: string, payload: unknown): Promise<TResponse> {
  const response = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json"
    },
    body: JSON.stringify(payload)
  });

  return response.json() as Promise<TResponse>;
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

export function getDefaultCatalogBackendUrl(): string {
  return DEFAULT_BACKEND_URL;
}

export async function listCatalogEndpoints(baseUrl: string): Promise<CatalogEndpointsResponse> {
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/endpoints`);
}

export async function saveCatalogEndpoint(baseUrl: string, payload: Record<string, unknown>): Promise<CatalogEndpointsResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/endpoints`, payload);
}

export async function runCatalogFullScan(baseUrl: string): Promise<CatalogScanResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/scans/full`, {});
}

export async function runCatalogEndpointScan(baseUrl: string, endpointId: string): Promise<CatalogScanResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/scans/endpoint`, { endpointId });
}

export async function listCatalogAssets(baseUrl: string, limit = 200): Promise<CatalogAssetsResponse> {
  const response = await getJson<CatalogAssetsResponse>(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/assets?limit=${limit}`);

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

export async function listCatalogTasks(baseUrl: string, limit = 100): Promise<CatalogTasksResponse> {
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/catalog/tasks?limit=${limit}`);
}
