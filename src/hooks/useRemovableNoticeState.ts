import { useCallback, useEffect, useMemo, useState } from "react";
import type { ImportDeviceRecord } from "../types/import";

const STORAGE_KEY_PREFIX = "mare.removable_notice.dismissed";
const UPDATE_EVENT = "mare-removable-notice-updated";

function readDismissedIds(scopeKey: string) {
  if (typeof window === "undefined") {
    return [] as string[];
  }

  try {
    const raw = window.localStorage.getItem(buildStorageKey(scopeKey));
    if (!raw) {
      return [];
    }

    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      return [];
    }

    return parsed.filter((value): value is string => typeof value === "string" && value.trim().length > 0);
  } catch {
    return [];
  }
}

function writeDismissedIds(scopeKey: string, ids: string[]) {
  if (typeof window === "undefined") {
    return;
  }

  const next = Array.from(new Set(ids.filter((value) => value.trim().length > 0)));
  window.localStorage.setItem(buildStorageKey(scopeKey), JSON.stringify(next));
  window.dispatchEvent(new CustomEvent(UPDATE_EVENT, { detail: scopeKey }));
}

export function useRemovableNoticeState(devices: ImportDeviceRecord[], scopeKey = "global") {
  const [dismissedIds, setDismissedIds] = useState<string[]>(() => readDismissedIds(scopeKey));

  useEffect(() => {
    if (typeof window === "undefined") {
      return undefined;
    }

    const sync = (event?: Event) => {
      const detailScope =
        event instanceof CustomEvent && typeof event.detail === "string" ? event.detail : undefined;
      if (detailScope && detailScope !== scopeKey) {
        return;
      }

      setDismissedIds(readDismissedIds(scopeKey));
    };
    window.addEventListener("storage", sync);
    window.addEventListener(UPDATE_EVENT, sync as EventListener);

    return () => {
      window.removeEventListener("storage", sync);
      window.removeEventListener(UPDATE_EVENT, sync as EventListener);
    };
  }, [scopeKey]);

  const unreadDevices = useMemo(() => {
    const dismissed = new Set(dismissedIds);
    return devices.filter((device) => !device.currentSessionRole && !dismissed.has(device.identitySignature));
  }, [devices, dismissedIds]);

  const markAsRead = useCallback((identitySignature: string) => {
    if (!identitySignature.trim()) {
      return;
    }

    const next = Array.from(new Set([...dismissedIds, identitySignature]));
    setDismissedIds(next);
    writeDismissedIds(scopeKey, next);
  }, [dismissedIds, scopeKey]);

  return {
    unreadDevices,
    unreadCount: unreadDevices.length,
    markAsRead
  };
}

function buildStorageKey(scopeKey: string) {
  return `${STORAGE_KEY_PREFIX}.${scopeKey.trim() || "global"}`;
}
