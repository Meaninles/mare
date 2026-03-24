import { invoke } from "@tauri-apps/api/core";
import type { AppBootstrap } from "../types/bootstrap";
import type { RegisteredLibrary } from "../types/libraries";

export async function getAppBootstrap(): Promise<AppBootstrap> {
  return invoke<AppBootstrap>("get_app_bootstrap");
}

export async function listLibraries(): Promise<RegisteredLibrary[]> {
  return invoke<RegisteredLibrary[]>("list_libraries");
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
