import { getDefaultCatalogBackendUrl } from "./catalog";
import type {
  ImportBrowseMediaType,
  ImportDeviceRole,
  ImportDeviceRoleSelectionResponse,
  ImportDevicesResponse,
  ImportExecuteResponse,
  ImportRuleInput,
  ImportRulesResponse,
  ImportSourceBrowseResponse
} from "../types/import";

function normalizeBaseUrl(baseUrl: string): string {
  const trimmed = baseUrl.trim();
  return trimmed.length > 0 ? trimmed.replace(/\/+$/, "") : getDefaultCatalogBackendUrl();
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

export function getDefaultImportBackendUrl(): string {
  return getDefaultCatalogBackendUrl();
}

export async function listImportDevices(baseUrl: string): Promise<ImportDevicesResponse> {
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/import/devices`);
}

export async function selectImportDeviceRole(
  baseUrl: string,
  payload: {
    identitySignature: string;
    role: ImportDeviceRole;
    name?: string;
  }
): Promise<ImportDeviceRoleSelectionResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/import/devices/role`, payload);
}

export async function browseImportSource(
  baseUrl: string,
  payload: {
    identitySignature: string;
    mediaType: ImportBrowseMediaType;
    limit?: number;
  }
): Promise<ImportSourceBrowseResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/import/sources/browse`, payload);
}

export async function listImportRules(baseUrl: string): Promise<ImportRulesResponse> {
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/import/rules`);
}

export async function saveImportRules(
  baseUrl: string,
  rules: ImportRuleInput[]
): Promise<ImportRulesResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/import/rules`, { rules });
}

export async function executeImport(
  baseUrl: string,
  payload: {
    identitySignature: string;
    entryPaths: string[];
  }
): Promise<ImportExecuteResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/import/execute`, payload);
}
