import type { CatalogEndpoint } from "./catalog";
import type { DeviceInfo } from "./connector-test";

export type ImportDeviceRole = "managed_storage" | "import_source";
export type ImportRuleType = "media_type" | "extension";
export type ImportBrowseMediaType = "all" | "image" | "video" | "audio";

export interface ImportDeviceRecord {
  device: DeviceInfo;
  identitySignature: string;
  knownEndpoint?: CatalogEndpoint;
  suggestedRole: ImportDeviceRole;
  currentSessionRole?: ImportDeviceRole;
  selectedAt?: string;
}

export interface ImportDeviceRoleSelection {
  device: ImportDeviceRecord;
  role: ImportDeviceRole;
  endpoint?: CatalogEndpoint;
  selectedAt: string;
}

export interface ImportSourceEntry {
  path: string;
  relativePath: string;
  name: string;
  mediaType: string;
  size: number;
  modifiedAt?: string;
}

export interface ImportSourceBrowseResult {
  device: ImportDeviceRecord;
  mediaType: ImportBrowseMediaType;
  limit: number;
  entries: ImportSourceEntry[];
}

export interface ImportRuleRecord {
  id: string;
  ruleType: ImportRuleType;
  matchValue: string;
  targetEndpointIds: string[];
  createdAt: string;
  updatedAt: string;
}

export interface ImportRuleInput {
  ruleType: ImportRuleType;
  matchValue: string;
  targetEndpointIds: string[];
}

export interface ImportTargetResult {
  endpointId: string;
  endpointName: string;
  status: string;
  error?: string;
}

export interface ImportExecutionItem {
  relativePath: string;
  displayName: string;
  logicalPathKey: string;
  mediaType: string;
  status: string;
  assetId?: string;
  targetResults: ImportTargetResult[];
  error?: string;
}

export interface ImportExecutionSummary {
  taskId: string;
  identitySignature: string;
  deviceLabel: string;
  status: string;
  totalFiles: number;
  successCount: number;
  partialCount: number;
  failedCount: number;
  startedAt: string;
  finishedAt: string;
  items: ImportExecutionItem[];
  error?: string;
}

export interface ImportDevicesResponse {
  success: boolean;
  devices?: ImportDeviceRecord[];
  error?: string;
}

export interface ImportDeviceRoleSelectionResponse {
  success: boolean;
  result?: ImportDeviceRoleSelection;
  error?: string;
}

export interface ImportSourceBrowseResponse {
  success: boolean;
  result?: ImportSourceBrowseResult;
  error?: string;
}

export interface ImportRulesResponse {
  success: boolean;
  rules?: ImportRuleRecord[];
  error?: string;
}

export interface ImportExecuteResponse {
  success: boolean;
  summary?: ImportExecutionSummary;
  error?: string;
}
