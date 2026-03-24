import { useQuery } from "@tanstack/react-query";
import { APP_BOOTSTRAP_QUERY_KEY } from "../lib/query-keys";
import { getAppBootstrap } from "../services/desktop";

export function useAppBootstrap() {
  return useQuery({
    queryKey: APP_BOOTSTRAP_QUERY_KEY,
    queryFn: getAppBootstrap,
    staleTime: 30_000
  });
}
