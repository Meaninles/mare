import { useQuery } from "@tanstack/react-query";
import { getDefaultSystemBackendUrl, listSystemLogs } from "../services/system";
import type { SystemLogLevel } from "../types/system";

const backendUrl = getDefaultSystemBackendUrl();

export function useSystemLogs(limit = 40, level: SystemLogLevel = "all") {
  return useQuery({
    queryKey: ["system-logs", limit, level],
    queryFn: async () => {
      const response = await listSystemLogs(backendUrl, { limit, level });
      if (!response.success) {
        throw new Error(response.error ?? "加载结构化日志失败。");
      }

      return {
        logFilePath: response.logFilePath ?? "",
        limit: response.limit ?? limit,
        entries: response.entries ?? []
      };
    },
    staleTime: 5_000,
    refetchInterval: 10_000
  });
}
