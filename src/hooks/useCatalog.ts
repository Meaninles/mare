import { useQuery } from "@tanstack/react-query";
import { getDefaultCatalogBackendUrl, listCatalogAssets, listCatalogEndpoints } from "../services/catalog";

const backendUrl = getDefaultCatalogBackendUrl();

export function useCatalogAssets(limit = 1000) {
  return useQuery({
    queryKey: ["catalog-assets", limit],
    queryFn: async () => {
      const response = await listCatalogAssets(backendUrl, limit);
      if (!response.success) {
        throw new Error(response.error ?? "读取资产库失败。");
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
        throw new Error(response.error ?? "读取存储端点失败。");
      }

      return response.endpoints ?? [];
    },
    staleTime: 15_000
  });
}
