import type {
  BackendLibrarySessionResponse,
  LegacyCatalogMigrationResponse,
  LegacyCatalogStatusResponse
} from "../types/libraries";

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

export function getDefaultLibrarySessionBackendUrl(): string {
  return DEFAULT_BACKEND_URL;
}

export async function getCurrentLibrarySession(baseUrl: string): Promise<BackendLibrarySessionResponse> {
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/libraries/current`);
}

export async function createBackendLibrary(baseUrl: string, path: string): Promise<BackendLibrarySessionResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/libraries/create`, { path });
}

export async function openBackendLibrary(baseUrl: string, path: string): Promise<BackendLibrarySessionResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/libraries/open`, { path });
}

export async function closeBackendLibrary(baseUrl: string): Promise<BackendLibrarySessionResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/libraries/close`, {});
}

export async function getLegacyCatalogStatus(baseUrl: string): Promise<LegacyCatalogStatusResponse> {
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/libraries/legacy/status`);
}

export async function migrateLegacyCatalog(
  baseUrl: string,
  payload: {
    targetPath?: string;
    libraryName?: string;
  }
): Promise<LegacyCatalogMigrationResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/libraries/legacy/migrate`, payload);
}
