import { useQuery } from "@tanstack/react-query";
import { getAppBootstrap } from "../services/desktop";

export function useAppBootstrap() {
  return useQuery({
    queryKey: ["app-bootstrap"],
    queryFn: getAppBootstrap,
    staleTime: 30_000
  });
}
