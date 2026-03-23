import type {
  Cloud115QRCodeSession,
  ConnectorTestResponse,
  DeviceInfo,
  RemovableDevicesResponse
} from "../types/connector-test";

const DEFAULT_BACKEND_URL = "http://127.0.0.1:8080";

function normalizeBaseUrl(baseUrl: string): string {
  const trimmed = baseUrl.trim();
  return trimmed.length > 0 ? trimmed.replace(/\/+$/, "") : DEFAULT_BACKEND_URL;
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

export function getDefaultBackendUrl(): string {
  return DEFAULT_BACKEND_URL;
}

export async function testQNAPConnector(baseUrl: string, payload: Record<string, unknown>): Promise<ConnectorTestResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/tools/connectors/qnap/test`, payload);
}

export async function testCloud115Connector(baseUrl: string, payload: Record<string, unknown>): Promise<ConnectorTestResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/tools/connectors/cloud115/test`, payload);
}

export async function startCloud115QRCodeLogin(
  baseUrl: string,
  appType: string
): Promise<ConnectorTestResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/tools/connectors/cloud115/qrcode/start`, { appType });
}

export async function pollCloud115QRCodeLogin(
  baseUrl: string,
  session: Cloud115QRCodeSession
): Promise<ConnectorTestResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/tools/connectors/cloud115/qrcode/poll`, {
    appType: session.appType,
    qrUid: session.uid,
    qrTime: session.time,
    qrSign: session.sign
  });
}

export async function listRemovableDevices(baseUrl: string): Promise<RemovableDevicesResponse> {
  const response = await fetch(`${normalizeBaseUrl(baseUrl)}/api/v1/tools/connectors/removable/devices`);
  return response.json() as Promise<RemovableDevicesResponse>;
}

export async function testRemovableConnector(
  baseUrl: string,
  device: DeviceInfo,
  payload: Record<string, unknown>
): Promise<ConnectorTestResponse> {
  return postJson(`${normalizeBaseUrl(baseUrl)}/api/v1/tools/connectors/removable/test`, {
    ...payload,
    device
  });
}
