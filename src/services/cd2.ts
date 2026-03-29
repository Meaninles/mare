import { getDefaultCatalogBackendUrl } from "./catalog";
import type {
  CD2AuthStatusResponse,
  CD2CloudAccountsResponse,
  CD2115QRCodeSessionResponse,
  CD2CopyFilesPayload,
  CD2CreateFolderPayload,
  CD2CreateFolderResponse,
  CD2DeleteFilesPayload,
  CD2DownloadURLQuery,
  CD2DownloadURLResponse,
  CD2FileDetailResponse,
  CD2FileListResponse,
  CD2FileOperationResponse,
  CD2FileStatResponse,
  CD2TransferActionPayload,
  CD2TransferActionResponse,
  CD2TransfersResponse,
  CD2MoveFilesPayload,
  CD2RenameFilePayload,
  CD2UploadFilesResponse,
  Import115CookiePayload,
  Import115CookieResponse,
  RemoveCD2CloudAccountPayload,
  RemoveCD2CloudAccountResponse,
  CD2RegisterResponse,
  Start115QRCodePayload,
  RegisterCD2AccountPayload,
  UpdateCD2AuthProfilePayload
} from "../types/cd2";

function normalizeBaseUrl(baseUrl: string) {
  const trimmed = baseUrl.trim();
  return trimmed.length > 0 ? trimmed.replace(/\/+$/, "") : getDefaultCatalogBackendUrl();
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

async function postFormData<TResponse>(url: string, payload: FormData): Promise<TResponse> {
  const response = await fetch(url, {
    method: "POST",
    body: payload
  });
  return readJsonResponse<TResponse>(response);
}

export function getDefaultCD2BackendUrl() {
  return getDefaultCatalogBackendUrl();
}

export async function getCD2AuthProfile(baseUrl: string): Promise<CD2AuthStatusResponse> {
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/auth/profile`);
}

export async function updateCD2AuthProfile(
  baseUrl: string,
  payload: UpdateCD2AuthProfilePayload
): Promise<CD2AuthStatusResponse> {
  return putJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/auth/profile`, payload);
}

export async function refreshCD2AuthProfile(baseUrl: string): Promise<CD2AuthStatusResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/auth/refresh`, {});
}

export async function clearCD2AuthProfile(baseUrl: string): Promise<CD2AuthStatusResponse> {
  return deleteJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/auth/profile`);
}

export async function registerCD2Account(
  baseUrl: string,
  payload: RegisterCD2AccountPayload
): Promise<CD2RegisterResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/auth/register`, payload);
}

export async function listCD2CloudAccounts(baseUrl: string): Promise<CD2CloudAccountsResponse> {
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/cloud-accounts`);
}

export async function import115Cookie(
  baseUrl: string,
  payload: Import115CookiePayload
): Promise<Import115CookieResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/cloud-accounts/115/cookie-import`, payload);
}

export async function start115QRCode(
  baseUrl: string,
  payload: Start115QRCodePayload
): Promise<CD2115QRCodeSessionResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/cloud-accounts/115/qrcode/start`, payload);
}

export async function start115OpenQRCode(
  baseUrl: string,
  payload: Start115QRCodePayload
): Promise<CD2115QRCodeSessionResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/cloud-accounts/115open/qrcode/start`, payload);
}

export async function get115QRCodeSession(
  baseUrl: string,
  sessionId: string
): Promise<CD2115QRCodeSessionResponse> {
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/cloud-accounts/115/qrcode/sessions/${encodeURIComponent(sessionId)}`);
}

export async function removeCD2CloudAccount(
  baseUrl: string,
  payload: RemoveCD2CloudAccountPayload
): Promise<RemoveCD2CloudAccountResponse> {
  const params = new URLSearchParams();
  if (payload.permanent) {
    params.set("permanent", "true");
  }
  const suffix = params.toString() ? `?${params.toString()}` : "";
  return deleteJson(
    `${normalizeBaseUrl(baseUrl)}/api/v1/cd2/cloud-accounts/${encodeURIComponent(payload.cloudName)}/${encodeURIComponent(payload.userName)}${suffix}`
  );
}

export async function listCD2Files(
  baseUrl: string,
  path: string,
  forceRefresh = false
): Promise<CD2FileListResponse> {
  const params = new URLSearchParams({
    path
  });
  if (forceRefresh) {
    params.set("forceRefresh", "true");
  }
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/files/list?${params.toString()}`);
}

export async function searchCD2Files(
  baseUrl: string,
  query: { path: string; query: string; forceRefresh?: boolean; fuzzyMatch?: boolean; contentSearch?: boolean }
): Promise<CD2FileListResponse> {
  const params = new URLSearchParams({
    path: query.path,
    query: query.query
  });
  if (query.forceRefresh) {
    params.set("forceRefresh", "true");
  }
  if (query.fuzzyMatch) {
    params.set("fuzzyMatch", "true");
  }
  if (query.contentSearch) {
    params.set("contentSearch", "true");
  }
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/files/search?${params.toString()}`);
}

export async function statCD2File(baseUrl: string, path: string): Promise<CD2FileStatResponse> {
  const params = new URLSearchParams({ path });
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/files/stat?${params.toString()}`);
}

export async function getCD2FileDetail(
  baseUrl: string,
  path: string,
  forceRefresh = false
): Promise<CD2FileDetailResponse> {
  const params = new URLSearchParams({ path });
  if (forceRefresh) {
    params.set("forceRefresh", "true");
  }
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/files/detail?${params.toString()}`);
}

export async function getCD2DownloadURL(
  baseUrl: string,
  query: CD2DownloadURLQuery
): Promise<CD2DownloadURLResponse> {
  const params = new URLSearchParams({ path: query.path });
  if (query.preview) {
    params.set("preview", "true");
  }
  if (query.lazyRead) {
    params.set("lazyRead", "true");
  }
  if (query.getDirect) {
    params.set("getDirect", "true");
  }
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/files/download-url?${params.toString()}`);
}

export async function createCD2Folder(
  baseUrl: string,
  payload: CD2CreateFolderPayload
): Promise<CD2CreateFolderResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/files/folders`, payload);
}

export async function renameCD2File(
  baseUrl: string,
  payload: CD2RenameFilePayload
): Promise<CD2FileOperationResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/files/rename`, payload);
}

export async function moveCD2Files(
  baseUrl: string,
  payload: CD2MoveFilesPayload
): Promise<CD2FileOperationResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/files/move`, payload);
}

export async function copyCD2Files(
  baseUrl: string,
  payload: CD2CopyFilesPayload
): Promise<CD2FileOperationResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/files/copy`, payload);
}

export async function deleteCD2Files(
  baseUrl: string,
  payload: CD2DeleteFilesPayload
): Promise<CD2FileOperationResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/files/delete`, payload);
}

export async function uploadCD2Files(
  baseUrl: string,
  payload: { parentPath: string; files: File[] }
): Promise<CD2UploadFilesResponse> {
  const formData = new FormData();
  formData.set("parentPath", payload.parentPath);
  payload.files.forEach((file) => {
    formData.append("files", file, file.name);
  });
  return postFormData(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/files/upload`, formData);
}

export async function listCD2Transfers(baseUrl: string): Promise<CD2TransfersResponse> {
  return getJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/transfers`);
}

export async function runCD2TransferAction(
  baseUrl: string,
  payload: CD2TransferActionPayload
): Promise<CD2TransferActionResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/cd2/transfers/actions`, payload);
}
