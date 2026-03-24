import { useCallback, useEffect, useMemo, useState } from "react";
import type { ImportDeviceRecord } from "../types/import";

const STORAGE_KEY = "mare.removable_notice.dismissed";
const UPDATE_EVENT = "mare-removable-notice-updated";

function readDismissedIds() {
  if (typeof window === "undefined") {
    return [] as string[];
  }

  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
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

function writeDismissedIds(ids: string[]) {
  if (typeof window === "undefined") {
    return;
  }

  const next = Array.from(new Set(ids.filter((value) => value.trim().length > 0)));
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  window.dispatchEvent(new CustomEvent(UPDATE_EVENT));
}

export function useRemovableNoticeState(devices: ImportDeviceRecord[]) {
  const [dismissedIds, setDismissedIds] = useState<string[]>(() => readDismissedIds());

  useEffect(() => {
    if (typeof window === "undefined") {
      return undefined;
    }

    const sync = () => setDismissedIds(readDismissedIds());
    window.addEventListener("storage", sync);
    window.addEventListener(UPDATE_EVENT, sync as EventListener);

    return () => {
      window.removeEventListener("storage", sync);
      window.removeEventListener(UPDATE_EVENT, sync as EventListener);
    };
  }, []);

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
    writeDismissedIds(next);
  }, [dismissedIds]);

  return {
    unreadDevices,
    unreadCount: unreadDevices.length,
    markAsRead
  };
}
