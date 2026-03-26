import { getDefaultCatalogBackendUrl } from "./catalog";
import type {
  BackupImportMode,
  SettingsBackupBundle,
  SettingsBackupExportResponse,
  SettingsBackupImportResponse,
  TransferSettingsResponse
} from "../types/settings";

function normalizeBaseUrl(baseUrl: string) {
  const trimmed = baseUrl.trim();
  return trimmed.length > 0 ? trimmed.replace(/\/+$/, "") : getDefaultCatalogBackendUrl();
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

async function getJson<TResponse>(url: string): Promise<TResponse> {
  const response = await fetch(url);
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

export function getDefaultSettingsBackendUrl() {
  return getDefaultCatalogBackendUrl();
}

export async function exportSettingsBackup(
  baseUrl: string,
  payload: {
    theme: string;
    includeCatalog: boolean;
  }
): Promise<SettingsBackupExportResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/settings/backup/export`, payload);
}

export async function importSettingsBackup(
  baseUrl: string,
  payload: {
    mode: BackupImportMode;
    bundle: SettingsBackupBundle;
  }
): Promise<SettingsBackupImportResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/settings/backup/import`, payload);
}

export async function getTransferSettings(baseUrl: string): Promise<TransferSettingsResponse> {
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/settings/transfers`);
}

export async function updateTransferSettings(
  baseUrl: string,
  payload: {
    uploadConcurrency: number;
    downloadConcurrency: number;
  }
): Promise<TransferSettingsResponse> {
  return putJson(`${normalizeBaseUrl(baseUrl)}/api/v1/settings/transfers`, payload);
}
