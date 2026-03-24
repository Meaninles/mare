import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  type ReactNode
} from "react";
import { useQuery, useQueryClient, type UseQueryResult } from "@tanstack/react-query";
import { useAppBootstrap } from "../hooks/useAppBootstrap";
import { APP_BOOTSTRAP_QUERY_KEY, clearLibraryQueries, LIBRARY_SESSION_QUERY_KEY } from "../lib/query-keys";
import {
  clearActiveLibrary,
  createLibraryRecord,
  registerExistingLibrary,
  setActiveLibrary
} from "../services/desktop";
import {
  closeBackendLibrary,
  createBackendLibrary,
  getCurrentLibrarySession,
  getDefaultLibrarySessionBackendUrl,
  openBackendLibrary
} from "../services/library-session";
import type {
  BackendLibrarySession,
  BackendLibrarySessionResponse,
  RegisteredLibrary
} from "../types/libraries";

const DEFAULT_LIBRARY_EXTENSION = ".maredb";
const backendUrl = getDefaultLibrarySessionBackendUrl();

type LibraryMutationInput = {
  path: string;
  name?: string;
};

interface LibraryContextValue {
  bootstrapQuery: ReturnType<typeof useAppBootstrap>;
  sessionQuery: UseQueryResult<BackendLibrarySessionResponse, Error>;
  recentLibraries: RegisteredLibrary[];
  activeLibrary?: RegisteredLibrary;
  currentLibrary?: RegisteredLibrary;
  currentLibrarySession?: BackendLibrarySession;
  currentLibraryId?: string;
  isInitializing: boolean;
  isLibraryOpen: boolean;
  refreshState: () => Promise<void>;
  createLibrary: (input: LibraryMutationInput) => Promise<RegisteredLibrary>;
  openLibraryPath: (input: LibraryMutationInput) => Promise<RegisteredLibrary>;
  openRegisteredLibrary: (library: RegisteredLibrary) => Promise<RegisteredLibrary>;
  closeLibrary: () => Promise<void>;
}

const LibraryContext = createContext<LibraryContextValue | null>(null);

export function LibraryProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const bootstrapQuery = useAppBootstrap();
  const sessionQuery = useQuery({
    queryKey: LIBRARY_SESSION_QUERY_KEY,
    queryFn: () => getCurrentLibrarySession(backendUrl),
    staleTime: 10_000
  });

  const recentLibraries = bootstrapQuery.data?.recentLibraries ?? [];
  const activeLibrary = bootstrapQuery.data?.activeLibrary;
  const currentLibrarySession = sessionQuery.data?.library;
  const isLibraryOpen = Boolean(currentLibrarySession?.ready);
  const currentLibraryId = currentLibrarySession?.ready ? currentLibrarySession.libraryId : undefined;

  const currentLibrary = useMemo(() => {
    if (!currentLibrarySession?.ready) {
      return activeLibrary;
    }

    return (
      recentLibraries.find(
        (library) =>
          library.id === currentLibrarySession.libraryId ||
          sameLibraryPath(library.path, currentLibrarySession.path)
      ) ?? activeLibrary
    );
  }, [activeLibrary, currentLibrarySession, recentLibraries]);

  const refreshState = useCallback(async () => {
    await Promise.all([bootstrapQuery.refetch(), sessionQuery.refetch()]);
  }, [bootstrapQuery, sessionQuery]);

  const finalizeLibrarySwitch = useCallback(async () => {
    clearLibraryQueries(queryClient);
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: APP_BOOTSTRAP_QUERY_KEY }),
      queryClient.invalidateQueries({ queryKey: LIBRARY_SESSION_QUERY_KEY })
    ]);
    await refreshState();
  }, [queryClient, refreshState]);

  const createLibrary = useCallback(async (input: LibraryMutationInput) => {
    const path = normalizeLibraryPath(input.path);
    const name = normalizeOptionalText(input.name);

    const response = await createBackendLibrary(backendUrl, path);
    unwrapLibraryResponse(response, "无法创建资产库。");
    const library = await createLibraryRecord({ path, name });
    await finalizeLibrarySwitch();
    return library;
  }, [finalizeLibrarySwitch]);

  const openLibraryPath = useCallback(async (input: LibraryMutationInput) => {
    const path = normalizeLibraryPath(input.path);
    const name = normalizeOptionalText(input.name);

    const response = await openBackendLibrary(backendUrl, path);
    unwrapLibraryResponse(response, "无法打开资产库。");
    const library = await registerExistingLibrary({ path, name });
    await finalizeLibrarySwitch();
    return library;
  }, [finalizeLibrarySwitch]);

  const openRegisteredLibrary = useCallback(async (library: RegisteredLibrary) => {
    if (currentLibrarySession?.ready && sameLibraryPath(currentLibrarySession.path, library.path)) {
      await setActiveLibrary(library.id);
      await refreshState();
      return library;
    }

    const response = await openBackendLibrary(backendUrl, normalizeLibraryPath(library.path));
    unwrapLibraryResponse(response, "无法打开所选资产库。");
    await setActiveLibrary(library.id);
    await finalizeLibrarySwitch();
    return library;
  }, [currentLibrarySession, finalizeLibrarySwitch, refreshState]);

  const closeLibrary = useCallback(async () => {
    const response = await closeBackendLibrary(backendUrl);
    if (!response.success) {
      throw new Error(response.error ?? "无法关闭当前资产库。");
    }

    await clearActiveLibrary();
    clearLibraryQueries(queryClient);
    await refreshState();
  }, [queryClient, refreshState]);

  const value = useMemo<LibraryContextValue>(() => ({
    bootstrapQuery,
    sessionQuery,
    recentLibraries,
    activeLibrary,
    currentLibrary,
    currentLibrarySession,
    currentLibraryId,
    isInitializing: bootstrapQuery.isLoading || sessionQuery.isLoading,
    isLibraryOpen,
    refreshState,
    createLibrary,
    openLibraryPath,
    openRegisteredLibrary,
    closeLibrary
  }), [
    activeLibrary,
    bootstrapQuery,
    closeLibrary,
    createLibrary,
    currentLibrary,
    currentLibraryId,
    currentLibrarySession,
    isLibraryOpen,
    openLibraryPath,
    openRegisteredLibrary,
    recentLibraries,
    refreshState,
    sessionQuery
  ]);

  return <LibraryContext.Provider value={value}>{children}</LibraryContext.Provider>;
}

export function useLibraryContext() {
  const context = useContext(LibraryContext);
  if (!context) {
    throw new Error("useLibraryContext must be used within a LibraryProvider");
  }

  return context;
}

function unwrapLibraryResponse(response: BackendLibrarySessionResponse, fallbackMessage: string) {
  if (!response.success || !response.library) {
    throw new Error(response.error ?? fallbackMessage);
  }

  return response.library;
}

function normalizeLibraryPath(path: string) {
  const trimmed = path.trim();
  if (!trimmed) {
    throw new Error("请输入资产库文件路径。");
  }

  const lastSegment = trimmed.split(/[\\/]/).pop() ?? trimmed;
  if (/\.[^./\\]+$/.test(lastSegment)) {
    return trimmed;
  }

  return `${trimmed}${DEFAULT_LIBRARY_EXTENSION}`;
}

function normalizeOptionalText(value?: string) {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
}

function sameLibraryPath(left?: string, right?: string) {
  if (!left || !right) {
    return false;
  }

  return left.trim().toLowerCase() === right.trim().toLowerCase();
}
