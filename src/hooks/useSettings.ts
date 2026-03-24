import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useLibraryContext } from "../context/LibraryContext";
import { APP_BOOTSTRAP_QUERY_KEY, invalidateLibraryQueries } from "../lib/query-keys";
import { getDefaultSettingsBackendUrl, exportSettingsBackup, importSettingsBackup } from "../services/settings";
import type { BackupImportMode, SettingsBackupBundle } from "../types/settings";

const backendUrl = getDefaultSettingsBackendUrl();

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
