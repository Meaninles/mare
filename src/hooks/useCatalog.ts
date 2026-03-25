import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useLibraryContext } from "../context/LibraryContext";
import { buildLibraryQueryKey, invalidateLibraryQueries } from "../lib/query-keys";
import {
  deleteCatalogTransferTasks,
  deleteCatalogReplica,
  getCatalogAssetInsights,
  getCatalogSyncOverview,
  getCatalogTransferTaskDetail,
  getDefaultCatalogBackendUrl,
  listCatalogAssets,
  listCatalogEndpoints,
  listCatalogTasks,
  listCatalogTransferTasks,
  pauseCatalogTransferTasks,
  resumeCatalogTransferTasks,
  restoreCatalogAsset,
  restoreCatalogAssetsToEndpoint,
  retryCatalogTask
} from "../services/catalog";
import type { CatalogAsset, CatalogAssetQueryOptions } from "../types/catalog";

const backendUrl = getDefaultCatalogBackendUrl();

export function useCatalogAssets(options: CatalogAssetQueryOptions = {}) {
  const { currentLibraryId, isLibraryOpen } = useLibraryContext();
  const resolvedOptions = {
    limit: options.limit ?? 1000,
    query: options.query?.trim() ?? "",
    mediaType: options.mediaType?.trim() ?? "",
    assetStatus: options.assetStatus?.trim() ?? ""
  };

  return useQuery({
    queryKey: buildLibraryQueryKey(
      currentLibraryId,
      "catalog",
      "assets",
      resolvedOptions.limit,
      resolvedOptions.query,
      resolvedOptions.mediaType,
      resolvedOptions.assetStatus
    ),
    enabled: isLibraryOpen,
    queryFn: async () => {
      const response = await listCatalogAssets(backendUrl, resolvedOptions);
      if (!response.success) {
        throw new Error(response.error ?? "无法读取资产列表。");
      }

      return response.assets ?? [];
    },
    staleTime: 5_000,
    refetchInterval: 10_000
  });
}

export function useCatalogEndpoints() {
  const { currentLibraryId, isLibraryOpen } = useLibraryContext();

  return useQuery({
    queryKey: buildLibraryQueryKey(currentLibraryId, "catalog", "endpoints"),
    enabled: isLibraryOpen,
    queryFn: async () => {
      const response = await listCatalogEndpoints(backendUrl);
      if (!response.success) {
        throw new Error(response.error ?? "无法读取存储端点。");
      }

      return response.endpoints ?? [];
    },
    staleTime: 15_000
  });
}

export function useCatalogAssetInsights(assetId: string, enabled = true) {
  const { currentLibraryId, isLibraryOpen } = useLibraryContext();
  const normalizedAssetId = assetId.trim();

  return useQuery({
    queryKey: buildLibraryQueryKey(currentLibraryId, "catalog", "asset-insights", normalizedAssetId),
    enabled: isLibraryOpen && enabled && normalizedAssetId.length > 0,
    queryFn: async () => {
      const response = await getCatalogAssetInsights(backendUrl, normalizedAssetId);
      if (!response.success || !response.insights) {
        throw new Error(response.error ?? "无法读取 AI 解析结果。");
      }

      return response.insights;
    },
    staleTime: 0,
    refetchInterval: 1_500,
    refetchIntervalInBackground: true,
    refetchOnMount: "always",
    refetchOnWindowFocus: "always"
  });
}

export function useCatalogTasks(limit = 100) {
  const { currentLibraryId, isLibraryOpen } = useLibraryContext();

  return useQuery({
    queryKey: buildLibraryQueryKey(currentLibraryId, "catalog", "tasks", limit),
    enabled: isLibraryOpen,
    queryFn: async () => {
      const response = await listCatalogTasks(backendUrl, limit);
      if (!response.success) {
        throw new Error(response.error ?? "无法读取后台任务。");
      }

      return response.tasks ?? [];
    },
    staleTime: 0,
    refetchInterval: 300,
    refetchIntervalInBackground: true,
    refetchOnMount: "always",
    refetchOnWindowFocus: "always"
  });
}

export function useTransferTasks(limit = 120) {
  const { currentLibraryId, isLibraryOpen } = useLibraryContext();

  return useQuery({
    queryKey: buildLibraryQueryKey(currentLibraryId, "catalog", "transfers", limit),
    enabled: isLibraryOpen,
    queryFn: async () => {
      const response = await listCatalogTransferTasks(backendUrl, limit);
      if (!response.success || !response.result) {
        throw new Error(response.error ?? "无法读取传输任务。");
      }

      return response.result;
    },
    staleTime: 0,
    refetchInterval: 1_500,
    refetchIntervalInBackground: true,
    refetchOnMount: "always",
    refetchOnWindowFocus: "always"
  });
}

export function useTransferTaskDetail(taskId: string) {
  const { currentLibraryId, isLibraryOpen } = useLibraryContext();
  const normalizedTaskId = taskId.trim();

  return useQuery({
    queryKey: buildLibraryQueryKey(currentLibraryId, "catalog", "transfer-detail", normalizedTaskId),
    enabled: isLibraryOpen && normalizedTaskId.length > 0,
    queryFn: async () => {
      const response = await getCatalogTransferTaskDetail(backendUrl, normalizedTaskId);
      if (!response.success || !response.result) {
        throw new Error(response.error ?? "无法读取传输任务详情。");
      }

      return response.result;
    },
    staleTime: 0,
    refetchInterval: 1_500,
    refetchIntervalInBackground: true,
    refetchOnMount: "always",
    refetchOnWindowFocus: "always"
  });
}

export function useCatalogSyncOverview() {
  const { currentLibraryId, isLibraryOpen } = useLibraryContext();

  return useQuery({
    queryKey: buildLibraryQueryKey(currentLibraryId, "catalog", "sync-overview"),
    enabled: isLibraryOpen,
    queryFn: async () => {
      const response = await getCatalogSyncOverview(backendUrl);
      if (!response.success) {
        throw new Error(response.error ?? "无法读取同步概览。");
      }

      return (
        response.overview ?? {
          generatedAt: new Date().toISOString(),
          recoverableAssets: [],
          conflictAssets: [],
          runningTasks: [],
          failedTasks: []
        }
      );
    },
    staleTime: 5_000,
    refetchInterval: 10_000
  });
}

export function useCatalogRestoreAsset() {
  const queryClient = useQueryClient();
  const { currentLibraryId } = useLibraryContext();

  return useMutation({
    mutationFn: async (payload: { assetId: string; sourceEndpointId: string; targetEndpointId: string }) => {
      const response = await restoreCatalogAsset(backendUrl, payload);
      if (!response.success || !response.summary) {
        throw new Error(response.error ?? "恢复资产失败。");
      }

      return response.summary;
    },
    onSuccess: () => {
      void invalidateCatalogQueries(queryClient, currentLibraryId);
    }
  });
}

export function useCatalogBatchRestore() {
  const queryClient = useQueryClient();
  const { currentLibraryId } = useLibraryContext();

  return useMutation({
    mutationFn: async (payload: { targetEndpointId: string; assetIds: string[] }) => {
      const response = await restoreCatalogAssetsToEndpoint(backendUrl, payload);
      if (!response.summary) {
        throw new Error(response.error ?? "批量补齐失败。");
      }

      return {
        ...response.summary,
        items: response.summary.items ?? []
      };
    },
    onSettled: () => {
      void invalidateCatalogQueries(queryClient, currentLibraryId);
    }
  });
}

export function useCatalogDeleteReplica() {
  const queryClient = useQueryClient();
  const { currentLibraryId } = useLibraryContext();

  return useMutation({
    mutationFn: async (payload: { assetId: string; targetEndpointId: string }) => {
      const response = await deleteCatalogReplica(backendUrl, payload);
      if (!response.success || !response.summary) {
        throw new Error(response.error ?? "删除副本失败。");
      }

      return response.summary;
    },
    onSuccess: (summary) => {
      if (summary.assetRemoved) {
        queryClient.setQueriesData(
          { queryKey: buildLibraryQueryKey(currentLibraryId, "catalog", "assets") },
          (current: CatalogAsset[] | undefined) => {
            if (!current) {
              return current;
            }

            return current.filter((asset) => asset.id !== summary.assetId);
          }
        );
      }

      void invalidateCatalogQueries(queryClient, currentLibraryId);
    }
  });
}

export function useCatalogRetryTask() {
  const queryClient = useQueryClient();
  const { currentLibraryId } = useLibraryContext();

  return useMutation({
    mutationFn: async (taskId: string) => {
      const response = await retryCatalogTask(backendUrl, taskId);
      if (!response.success || !response.summary) {
        throw new Error(response.error ?? "重试后台任务失败。");
      }

      return response.summary;
    },
    onSuccess: () => {
      void invalidateCatalogQueries(queryClient, currentLibraryId);
    }
  });
}

export function useCatalogRetrySyncTask() {
  return useCatalogRetryTask();
}

export function usePauseTransferTasks() {
  const queryClient = useQueryClient();
  const { currentLibraryId } = useLibraryContext();

  return useMutation({
    mutationFn: async (taskIds: string[]) => {
      const response = await pauseCatalogTransferTasks(backendUrl, taskIds);
      if (!response.summary) {
        throw new Error(response.error ?? "暂停传输任务失败。");
      }

      return response.summary;
    },
    onSuccess: async () => {
      await invalidateCatalogQueries(queryClient, currentLibraryId);
    }
  });
}

export function useResumeTransferTasks() {
  const queryClient = useQueryClient();
  const { currentLibraryId } = useLibraryContext();

  return useMutation({
    mutationFn: async (taskIds: string[]) => {
      const response = await resumeCatalogTransferTasks(backendUrl, taskIds);
      if (!response.summary) {
        throw new Error(response.error ?? "恢复传输任务失败。");
      }

      return response.summary;
    },
    onSuccess: async () => {
      await invalidateCatalogQueries(queryClient, currentLibraryId);
    }
  });
}

export function useDeleteTransferTasks() {
  const queryClient = useQueryClient();
  const { currentLibraryId } = useLibraryContext();

  return useMutation({
    mutationFn: async (taskIds: string[]) => {
      const response = await deleteCatalogTransferTasks(backendUrl, taskIds);
      if (!response.summary) {
        throw new Error(response.error ?? "删除传输任务失败。");
      }

      return response.summary;
    },
    onSuccess: async () => {
      await invalidateCatalogQueries(queryClient, currentLibraryId);
    }
  });
}

async function invalidateCatalogQueries(
  queryClient: ReturnType<typeof useQueryClient>,
  libraryId: string | undefined
) {
  await invalidateLibraryQueries(queryClient, libraryId);
}
