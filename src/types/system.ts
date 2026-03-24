export type SystemLogLevel = "all" | "debug" | "info" | "warn" | "error";

export interface SystemLogEntry {
  timestamp: string;
  level: string;
  message: string;
  attributes?: Record<string, unknown>;
}

export interface SystemLogsResponse {
  success: boolean;
  logFilePath?: string;
  limit?: number;
  entries?: SystemLogEntry[];
  error?: string;
}
