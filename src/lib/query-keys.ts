import type { Query, QueryClient } from "@tanstack/react-query";

export const APP_BOOTSTRAP_QUERY_KEY = ["app-bootstrap"] as const;
export const LIBRARY_SESSION_QUERY_KEY = ["library-session"] as const;

export function buildLibraryQueryKey(libraryId: string | undefined, ...segments: unknown[]) {
  return ["library", libraryId ?? "unloaded", ...segments] as const;
}

export function isLibraryScopedQuery(query: Pick<Query, "queryKey"> | readonly unknown[]) {
  const queryKey = hasQueryKey(query) ? query.queryKey : query;
  return Array.isArray(queryKey) && queryKey[0] === "library";
}

export function invalidateLibraryQueries(queryClient: QueryClient, libraryId: string | undefined) {
  return queryClient.invalidateQueries({
    predicate: (query) => {
      if (!isLibraryScopedQuery(query)) {
        return false;
      }

      if (!libraryId) {
        return true;
      }

      return Array.isArray(query.queryKey) && query.queryKey[1] === libraryId;
    }
  });
}

export function clearLibraryQueries(queryClient: QueryClient) {
  return queryClient.removeQueries({
    predicate: (query) => isLibraryScopedQuery(query)
  });
}

function hasQueryKey(value: Pick<Query, "queryKey"> | readonly unknown[]): value is Pick<Query, "queryKey"> {
  return !Array.isArray(value);
}
