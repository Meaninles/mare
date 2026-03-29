import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { APP_BOOTSTRAP_QUERY_KEY } from "../lib/query-keys";
import {
  clearCD2AuthProfile,
  copyCD2Files,
  createCD2Folder,
  deleteCD2Files,
  getCD2DownloadURL,
  get115QRCodeSession,
  getCD2AuthProfile,
  getCD2FileDetail,
  getDefaultCD2BackendUrl,
  import115Cookie,
  listCD2CloudAccounts,
  listCD2Files,
  listCD2Transfers,
  removeCD2CloudAccount,
  refreshCD2AuthProfile,
  registerCD2Account,
  renameCD2File,
  searchCD2Files,
  start115OpenQRCode,
  start115QRCode,
  statCD2File,
  moveCD2Files,
  runCD2TransferAction,
  uploadCD2Files,
  updateCD2AuthProfile
} from "../services/cd2";
import type {
  CD2CopyFilesPayload,
  CD2CreateFolderPayload,
  CD2DeleteFilesPayload,
  CD2DownloadURLQuery,
  CD2MoveFilesPayload,
  CD2RenameFilePayload,
  CD2TransferActionPayload,
  Import115CookiePayload,
  RemoveCD2CloudAccountPayload,
  RegisterCD2AccountPayload,
  Start115QRCodePayload,
  UpdateCD2AuthProfilePayload
} from "../types/cd2";

const backendUrl = getDefaultCD2BackendUrl();

export const CD2_AUTH_QUERY_KEY = ["cd2", "auth", "profile"] as const;
export const CD2_CLOUD_ACCOUNTS_QUERY_KEY = ["cd2", "cloud-accounts"] as const;

export function useCD2AuthProfile() {
  return useQuery({
    queryKey: CD2_AUTH_QUERY_KEY,
    queryFn: async () => {
      const response = await getCD2AuthProfile(backendUrl);
      if (!response.success || !response.auth) {
        throw new Error(response.error ?? "无法读取 CD2 认证状态。");
      }
      return response.auth;
    },
    staleTime: 5_000
  });
}

export function useUpdateCD2AuthProfile() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (payload: UpdateCD2AuthProfilePayload) => {
      const response = await updateCD2AuthProfile(backendUrl, payload);
      if (!response.success || !response.auth) {
        throw new Error(response.error ?? "保存 CD2 认证配置失败。");
      }
      return response.auth;
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: CD2_AUTH_QUERY_KEY }),
        queryClient.invalidateQueries({ queryKey: APP_BOOTSTRAP_QUERY_KEY })
      ]);
    }
  });
}

export function useRefreshCD2AuthProfile() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const response = await refreshCD2AuthProfile(backendUrl);
      if (!response.success || !response.auth) {
        throw new Error(response.error ?? "刷新 CD2 认证状态失败。");
      }
      return response.auth;
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: CD2_AUTH_QUERY_KEY }),
        queryClient.invalidateQueries({ queryKey: APP_BOOTSTRAP_QUERY_KEY })
      ]);
    }
  });
}

export function useClearCD2AuthProfile() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      const response = await clearCD2AuthProfile(backendUrl);
      if (!response.success || !response.auth) {
        throw new Error(response.error ?? "清除 CD2 认证配置失败。");
      }
      return response.auth;
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: CD2_AUTH_QUERY_KEY }),
        queryClient.invalidateQueries({ queryKey: APP_BOOTSTRAP_QUERY_KEY })
      ]);
    }
  });
}

export function useRegisterCD2Account() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (payload: RegisterCD2AccountPayload) => {
      const response = await registerCD2Account(backendUrl, payload);
      if (!response.success || !response.result) {
        throw new Error(response.error ?? response.result?.errorMessage ?? "注册 CD2 账号失败。");
      }
      return response.result;
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: CD2_AUTH_QUERY_KEY });
    }
  });
}

export function useCD2CloudAccounts() {
  return useQuery({
    queryKey: CD2_CLOUD_ACCOUNTS_QUERY_KEY,
    queryFn: async () => {
      const response = await listCD2CloudAccounts(backendUrl);
      if (!response.success || !response.accounts) {
        throw new Error(response.error ?? "无法读取 CD2 云账号列表。");
      }
      return response.accounts;
    },
    staleTime: 5_000
  });
}

export function useImport115Cookie() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (payload: Import115CookiePayload) => {
      const response = await import115Cookie(backendUrl, payload);
      if (!response.success || !response.result) {
        throw new Error(response.error ?? response.result?.message ?? "导入 115 Cookie 失败。");
      }
      return response.result;
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: CD2_CLOUD_ACCOUNTS_QUERY_KEY }),
        queryClient.invalidateQueries({ queryKey: CD2_AUTH_QUERY_KEY })
      ]);
    }
  });
}

export function useStart115QRCode() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (payload: Start115QRCodePayload) => {
      const response = await start115QRCode(backendUrl, payload);
      if (!response.success || !response.session) {
        throw new Error(response.error ?? "启动 115 二维码登录失败。");
      }
      return response.session;
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: CD2_AUTH_QUERY_KEY });
    }
  });
}

export function useStart115OpenQRCode() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (payload: Start115QRCodePayload) => {
      const response = await start115OpenQRCode(backendUrl, payload);
      if (!response.success || !response.session) {
        throw new Error(response.error ?? "启动 115open 二维码登录失败。");
      }
      return response.session;
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: CD2_AUTH_QUERY_KEY });
    }
  });
}

export function use115QRCodeSession(sessionId: string | null) {
  return useQuery({
    queryKey: ["cd2", "115-qrcode-session", sessionId ?? "idle"],
    enabled: Boolean(sessionId),
    queryFn: async () => {
      const response = await get115QRCodeSession(backendUrl, sessionId ?? "");
      if (!response.success || !response.session) {
        throw new Error(response.error ?? "无法读取 115 二维码登录会话。");
      }
      return response.session;
    },
    refetchInterval: (query) => {
      const session = query.state.data;
      if (!sessionId || !session || session.finishedAt) {
        return false;
      }
      return 1_500;
    }
  });
}

export function useRemoveCD2CloudAccount() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (payload: RemoveCD2CloudAccountPayload) => {
      const response = await removeCD2CloudAccount(backendUrl, payload);
      if (!response.success) {
        throw new Error(response.error ?? "删除 CD2 云账号失败。");
      }
      return response;
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: CD2_CLOUD_ACCOUNTS_QUERY_KEY });
    }
  });
}

export function useListCD2Files() {
  return useMutation({
    mutationFn: async (payload: { path: string; forceRefresh?: boolean }) => {
      const response = await listCD2Files(backendUrl, payload.path, payload.forceRefresh);
      if (!response.success || !response.entries) {
        throw new Error(response.error ?? "无法读取 CD2 文件列表。");
      }
      return response;
    }
  });
}

export function useSearchCD2Files() {
  return useMutation({
    mutationFn: async (payload: { path: string; query: string; forceRefresh?: boolean; fuzzyMatch?: boolean; contentSearch?: boolean }) => {
      const response = await searchCD2Files(backendUrl, payload);
      if (!response.success || !response.entries) {
        throw new Error(response.error ?? "无法搜索 CD2 文件。");
      }
      return response;
    }
  });
}

export function useStatCD2File() {
  return useMutation({
    mutationFn: async (path: string) => {
      const response = await statCD2File(backendUrl, path);
      if (!response.success || !response.entry) {
        throw new Error(response.error ?? "无法读取 CD2 文件信息。");
      }
      return response.entry;
    }
  });
}

export function useCD2FileDetail() {
  return useMutation({
    mutationFn: async (payload: { path: string; forceRefresh?: boolean }) => {
      const response = await getCD2FileDetail(backendUrl, payload.path, payload.forceRefresh);
      if (!response.success || !response.detail) {
        throw new Error(response.error ?? "无法读取 CD2 文件详情。");
      }
      return response.detail;
    }
  });
}

export function useCD2DownloadURL() {
  return useMutation({
    mutationFn: async (payload: CD2DownloadURLQuery) => {
      const response = await getCD2DownloadURL(backendUrl, payload);
      if (!response.success || !response.info) {
        throw new Error(response.error ?? "无法获取 CD2 下载链接。");
      }
      return response.info;
    }
  });
}

export function useCreateCD2Folder() {
  return useMutation({
    mutationFn: async (payload: CD2CreateFolderPayload) => {
      const response = await createCD2Folder(backendUrl, payload);
      if (!response.success || !response.entry) {
        throw new Error(response.error ?? response.result?.errorMessage ?? "创建目录失败。");
      }
      return response;
    }
  });
}

export function useRenameCD2File() {
  return useMutation({
    mutationFn: async (payload: CD2RenameFilePayload) => {
      const response = await renameCD2File(backendUrl, payload);
      if (!response.success || !response.result) {
        throw new Error(response.error ?? response.result?.errorMessage ?? "重命名失败。");
      }
      return response.result;
    }
  });
}

export function useMoveCD2Files() {
  return useMutation({
    mutationFn: async (payload: CD2MoveFilesPayload) => {
      const response = await moveCD2Files(backendUrl, payload);
      if (!response.success || !response.result) {
        throw new Error(response.error ?? response.result?.errorMessage ?? "移动失败。");
      }
      return response.result;
    }
  });
}

export function useCopyCD2Files() {
  return useMutation({
    mutationFn: async (payload: CD2CopyFilesPayload) => {
      const response = await copyCD2Files(backendUrl, payload);
      if (!response.success || !response.result) {
        throw new Error(response.error ?? response.result?.errorMessage ?? "复制失败。");
      }
      return response.result;
    }
  });
}

export function useDeleteCD2Files() {
  return useMutation({
    mutationFn: async (payload: CD2DeleteFilesPayload) => {
      const response = await deleteCD2Files(backendUrl, payload);
      if (!response.success || !response.result) {
        throw new Error(response.error ?? response.result?.errorMessage ?? "删除失败。");
      }
      return response.result;
    }
  });
}

export function useUploadCD2Files() {
  return useMutation({
    mutationFn: async (payload: { parentPath: string; files: File[] }) => {
      const response = await uploadCD2Files(backendUrl, payload);
      if (!response.success || !response.uploaded) {
        throw new Error(response.error ?? "上传失败。");
      }
      return response;
    }
  });
}

export function useCD2Transfers() {
  return useQuery({
    queryKey: ["cd2", "transfers"],
    queryFn: async () => {
      const response = await listCD2Transfers(backendUrl);
      if (!response.success || !response.result) {
        throw new Error(response.error ?? "无法读取 CD2 传输任务。");
      }
      return response.result;
    },
    staleTime: 2_000,
    refetchInterval: 2_000
  });
}

export function useCD2TransferAction() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (payload: CD2TransferActionPayload) => {
      const response = await runCD2TransferAction(backendUrl, payload);
      if (!response.success || !response.summary) {
        throw new Error(response.error ?? "执行 CD2 传输任务操作失败。");
      }
      return response.summary;
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["cd2", "transfers"] });
    }
  });
}
