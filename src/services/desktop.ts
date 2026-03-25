import { invoke } from "@tauri-apps/api/core";
import type { AppBootstrap } from "../types/bootstrap";
import type { LibraryTaskRecord, RegisteredLibrary } from "../types/libraries";

export async function getAppBootstrap(): Promise<AppBootstrap> {
  return invoke<AppBootstrap>("get_app_bootstrap");
}

export async function listLibraries(): Promise<RegisteredLibrary[]> {
  return invoke<RegisteredLibrary[]>("list_libraries");
}

export async function listLibraryTasks(limitPerLibrary?: number): Promise<LibraryTaskRecord[]> {
  return typeof limitPerLibrary === "number"
    ? invoke<LibraryTaskRecord[]>("list_library_tasks", { limitPerLibrary })
    : invoke<LibraryTaskRecord[]>("list_library_tasks");
}

export async function createLibraryRecord(payload: {
  path: string;
  name?: string;
}): Promise<RegisteredLibrary> {
  return invoke<RegisteredLibrary>("create_library_record", payload);
}

export async function registerExistingLibrary(payload: {
  path: string;
  name?: string;
}): Promise<RegisteredLibrary> {
  return invoke<RegisteredLibrary>("register_existing_library", payload);
}

export async function setActiveLibrary(id: string): Promise<RegisteredLibrary> {
  return invoke<RegisteredLibrary>("set_active_library", { id });
}

export async function clearActiveLibrary(): Promise<void> {
  return invoke<void>("clear_active_library");
}

export async function updateLibraryRecord(payload: {
  id: string;
  path: string;
  name?: string;
}): Promise<RegisteredLibrary> {
  return invoke<RegisteredLibrary>("update_library_record", payload);
}

export async function setLibraryPinned(id: string, pinned: boolean): Promise<RegisteredLibrary> {
  return invoke<RegisteredLibrary>("set_library_pinned", { id, pinned });
}

export async function deleteLibraryRecord(id: string): Promise<void> {
  return invoke<void>("delete_library_record", { id });
}
