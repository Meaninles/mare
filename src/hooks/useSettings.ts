import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useLibraryContext } from "../context/LibraryContext";
import { APP_BOOTSTRAP_QUERY_KEY, invalidateLibraryQueries } from "../lib/query-keys";
import {
  getDefaultSettingsBackendUrl,
  exportSettingsBackup,
  getTransferSettings,
  importSettingsBackup,
  updateTransferSettings
} from "../services/settings";
import type { BackupImportMode, SettingsBackupBundle } from "../types/settings";

const backendUrl = getDefaultSettingsBackendUrl();

export function useTransferSettings() {
  const { currentLibraryId, isLibraryOpen } = useLibraryContext();

  return useQuery({
    queryKey: ["library", currentLibraryId ?? "unbound", "settings", "transfers"],
    enabled: isLibraryOpen,
    queryFn: async () => {
      const response = await getTransferSettings(backendUrl);
      if (!response.success || !response.preferences) {
        throw new Error(response.error ?? "无法读取传输设置。");
      }

      return response.preferences;
    },
    staleTime: 5_000
  });
}

export function useExportSettingsBackup() {
  return useMutation({
    mutationFn: async (payload: { theme: string; includeCatalog: boolean }) => {
      const response = await exportSettingsBackup(backendUrl, payload);
      if (!response.success || !response.bundle) {
        throw new Error(response.error ?? "导出备份包失败。");
      }

      return response.bundle;
    }
  });
}

export function useImportSettingsBackup() {
  const queryClient = useQueryClient();
  const { currentLibraryId } = useLibraryContext();

  return useMutation({
    mutationFn: async (payload: { mode: BackupImportMode; bundle: SettingsBackupBundle }) => {
      const response = await importSettingsBackup(backendUrl, payload);
      if (!response.success || !response.summary) {
        throw new Error(response.error ?? "导入备份包失败。");
      }

      return response.summary;
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: APP_BOOTSTRAP_QUERY_KEY }),
        invalidateLibraryQueries(queryClient, currentLibraryId)
      ]);
    }
  });
}

export function useUpdateTransferSettings() {
  const queryClient = useQueryClient();
  const { currentLibraryId } = useLibraryContext();

  return useMutation({
    mutationFn: async (payload: { uploadConcurrency: number; downloadConcurrency: number }) => {
      const response = await updateTransferSettings(backendUrl, payload);
      if (!response.success || !response.preferences) {
        throw new Error(response.error ?? "保存传输设置失败。");
      }

      return response.preferences;
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["library", currentLibraryId ?? "unbound", "settings", "transfers"] }),
        invalidateLibraryQueries(queryClient, currentLibraryId)
      ]);
    }
  });
}
