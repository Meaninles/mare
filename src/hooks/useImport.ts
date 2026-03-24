import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  browseImportSource,
  executeImport,
  getDefaultImportBackendUrl,
  listImportDevices,
  listImportRules,
  saveImportRules,
  selectImportDeviceRole
} from "../services/import";
import type { ImportBrowseMediaType, ImportDeviceRole, ImportRuleInput } from "../types/import";

const backendUrl = getDefaultImportBackendUrl();

export function useImportDevices() {
  return useQuery({
    queryKey: ["import-devices"],
    queryFn: async () => {
      const response = await listImportDevices(backendUrl);
      if (!response.success) {
        throw new Error(response.error ?? "无法读取可移动设备。");
      }

      return response.devices ?? [];
    },
    staleTime: 2_000,
    refetchInterval: 4_000
  });
}

export function useSelectImportDeviceRole() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (payload: { identitySignature: string; role: ImportDeviceRole; name?: string }) => {
      const response = await selectImportDeviceRole(backendUrl, payload);
      if (!response.success || !response.result) {
        throw new Error(response.error ?? "保存设备角色失败。");
      }

      return response.result;
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["import-devices"] }),
        queryClient.invalidateQueries({ queryKey: ["catalog-endpoints"] })
      ]);
    }
  });
}

export function useImportSource(identitySignature: string, mediaType: ImportBrowseMediaType, limit = 800) {
  return useQuery({
    queryKey: ["import-source", identitySignature, mediaType, limit],
    enabled: Boolean(identitySignature),
    queryFn: async () => {
      const response = await browseImportSource(backendUrl, {
        identitySignature,
        mediaType,
        limit
      });
      if (!response.success || !response.result) {
        throw new Error(response.error ?? "无法读取导入源内容。");
      }

      return response.result;
    },
    staleTime: 2_000,
    refetchInterval: 10_000
  });
}

export function useImportRules() {
  return useQuery({
    queryKey: ["import-rules"],
    queryFn: async () => {
      const response = await listImportRules(backendUrl);
      if (!response.success) {
        throw new Error(response.error ?? "无法读取导入规则。");
      }

      return response.rules ?? [];
    },
    staleTime: 10_000
  });
}

export function useSaveImportRules() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (rules: ImportRuleInput[]) => {
      const response = await saveImportRules(backendUrl, rules);
      if (!response.success || !response.rules) {
        throw new Error(response.error ?? "保存导入规则失败。");
      }

      return response.rules;
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["import-rules"] });
    }
  });
}

export function useExecuteImport() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (payload: { identitySignature: string; entryPaths: string[] }) => {
      const response = await executeImport(backendUrl, payload);
      if (!response.summary) {
        throw new Error(response.error ?? "导入执行失败。");
      }

      return response.summary;
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["catalog-assets"] }),
        queryClient.invalidateQueries({ queryKey: ["catalog-tasks"] }),
        queryClient.invalidateQueries({ queryKey: ["import-source"] })
      ]);
    }
  });
}
