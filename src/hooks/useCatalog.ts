import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  deleteCatalogReplica,
  getCatalogSyncOverview,
  getDefaultCatalogBackendUrl,
  listCatalogAssets,
  listCatalogEndpoints,
  listCatalogTasks,
  restoreCatalogAsset,
  restoreCatalogAssetsToEndpoint,
  retryCatalogTask
} from "../services/catalog";
import type { CatalogAsset } from "../types/catalog";

const backendUrl = getDefaultCatalogBackendUrl();

export function useCatalogAssets(limit = 1000) {
  return useQuery({
    queryKey: ["catalog-assets", limit],
    queryFn: async () => {
      const response = await listCatalogAssets(backendUrl, limit);
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
  return useQuery({
    queryKey: ["catalog-endpoints"],
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

export function useCatalogTasks(limit = 100) {
  return useQuery({
    queryKey: ["catalog-tasks", limit],
    queryFn: async () => {
      const response = await listCatalogTasks(backendUrl, limit);
      if (!response.success) {
        throw new Error(response.error ?? "无法读取后台任务。");
      }

      return response.tasks ?? [];
    },
    staleTime: 4_000,
    refetchInterval: 8_000
  });
}

export function useCatalogSyncOverview() {
  return useQuery({
    queryKey: ["catalog-sync-overview"],
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

  return useMutation({
    mutationFn: async (payload: { assetId: string; sourceEndpointId: string; targetEndpointId: string }) => {
      const response = await restoreCatalogAsset(backendUrl, payload);
      if (!response.success || !response.summary) {
        throw new Error(response.error ?? "恢复资产失败。");
      }

      return response.summary;
    },
    onSuccess: async () => {
      await invalidateCatalogQueries(queryClient);
    }
  });
}

export function useCatalogBatchRestore() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (payload: { targetEndpointId: string; assetIds: string[] }) => {
      const response = await restoreCatalogAssetsToEndpoint(backendUrl, payload);
      if (!response.summary) {
        throw new Error(response.error ?? "批量补齐失败。");
      }

      return response.summary;
    },
    onSettled: async () => {
      await invalidateCatalogQueries(queryClient);
    }
  });
}

export function useCatalogDeleteReplica() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (payload: { assetId: string; targetEndpointId: string }) => {
      const response = await deleteCatalogReplica(backendUrl, payload);
      if (!response.success || !response.summary) {
        throw new Error(response.error ?? "删除副本失败。");
      }

      return response.summary;
    },
    onSuccess: async (summary) => {
      if (summary.assetRemoved) {
        queryClient.setQueriesData({ queryKey: ["catalog-assets"] }, (current: CatalogAsset[] | undefined) => {
          if (!current) {
            return current;
          }

          return current.filter((asset) => asset.id !== summary.assetId);
        });
      }

      await invalidateCatalogQueries(queryClient);
    }
  });
}

export function useCatalogRetryTask() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (taskId: string) => {
      const response = await retryCatalogTask(backendUrl, taskId);
      if (!response.success || !response.summary) {
        throw new Error(response.error ?? "重试后台任务失败。");
      }

      return response.summary;
    },
    onSuccess: async () => {
      await invalidateCatalogQueries(queryClient);
    }
  });
}

export function useCatalogRetrySyncTask() {
  return useCatalogRetryTask();
}

async function invalidateCatalogQueries(queryClient: ReturnType<typeof useQueryClient>) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["catalog-assets"] }),
    queryClient.invalidateQueries({ queryKey: ["catalog-endpoints"] }),
    queryClient.invalidateQueries({ queryKey: ["catalog-sync-overview"] }),
    queryClient.invalidateQueries({ queryKey: ["catalog-tasks"] })
  ]);
}
