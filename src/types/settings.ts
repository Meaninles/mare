import type { CatalogAsset, CatalogEndpoint } from "./catalog";
import type { ImportRuleRecord } from "./import";

export type BackupImportMode = "config_only" | "config_and_catalog";

export interface SettingsBackupAppInfo {
  name: string;
  env: string;
}

export interface SettingsBackupPreferences {
  theme: string;
  uploadConcurrency?: number;
  downloadConcurrency?: number;
}

export interface TransferPreferences {
  uploadConcurrency: number;
  downloadConcurrency: number;
  updatedAt: string;
}

export interface TransferSettingsResponse {
  success: boolean;
  preferences?: TransferPreferences;
  error?: string;
}

export interface SettingsBackupConfigurationSnapshot {
  endpoints: CatalogEndpoint[];
  importRules: ImportRuleRecord[];
}

export interface SettingsBackupCatalogSnapshot {
  assets: CatalogAsset[];
}

export interface SettingsBackupBundle {
  formatVersion: number;
  exportedAt: string;
  app: SettingsBackupAppInfo;
  preferences: SettingsBackupPreferences;
  configuration: SettingsBackupConfigurationSnapshot;
  catalog?: SettingsBackupCatalogSnapshot;
}

export interface SettingsBackupExportResponse {
  success: boolean;
  bundle?: SettingsBackupBundle;
  error?: string;
}

export interface SettingsBackupImportSummary {
  mode: BackupImportMode;
  importedEndpoints: number;
  importedRules: number;
  importedAssets: number;
  importedReplicas: number;
  importedVersions: number;
  appliedTheme?: string;
  importedAt: string;
}

export interface SettingsBackupImportResponse {
  success: boolean;
  summary?: SettingsBackupImportSummary;
  error?: string;
}
