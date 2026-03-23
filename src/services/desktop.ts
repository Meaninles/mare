import { invoke } from "@tauri-apps/api/core";
import type { AppBootstrap } from "../types/bootstrap";

export async function getAppBootstrap(): Promise<AppBootstrap> {
  return invoke<AppBootstrap>("get_app_bootstrap");
}
