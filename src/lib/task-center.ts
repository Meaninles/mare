import type { CatalogTask } from "../types/catalog";

export type TaskFilter = "all" | "running" | "failed" | "completed" | "scan" | "sync" | "import" | "media";

const mediaAssetScopedTaskTypes = new Set([
  "thumbnail",
  "video_cover",
  "audio_metadata",
  "audio_transcript",
  "video_transcript",
  "image_semantic",
  "video_semantic"
]);

export function getTaskTitle(taskType: string) {
  switch (safeLower(taskType)) {
    case "scan_endpoint":
      return "端点扫描";
    case "restore_asset":
      return "恢复资产";
    case "restore_batch":
      return "批量补齐";
    case "import_execute":
      return "导入执行";
    case "thumbnail":
      return "图片缩略图";
    case "video_cover":
      return "视频封面";
    case "audio_metadata":
      return "音频元数据";
    case "audio_transcript":
      return "音频转写";
    case "video_transcript":
      return "视频转写";
    case "image_semantic":
      return "图片语义解析";
    case "video_semantic":
      return "视频语义解析";
    case "delete_replica":
      return "删除副本";
    default:
      return taskType || "后台任务";
  }
}

export function getTaskStatusLabel(status: string) {
  switch (safeLower(status)) {
    case "pending":
      return "等待中";
    case "running":
      return "进行中";
    case "retrying":
      return "重试中";
    case "failed":
    case "error":
      return "失败";
    case "success":
      return "成功";
    default:
      return status || "未知";
  }
}

export function getTaskTone(status: string) {
  if (isSuccessfulTaskStatus(status)) {
    return "success";
  }
  if (isFailedTaskStatus(status)) {
    return "danger";
  }
  return "warning";
}

export function getTaskFilterLabel(filter: TaskFilter) {
  switch (filter) {
    case "running":
      return "进行中";
    case "failed":
      return "失败";
    case "completed":
      return "已完成";
    case "scan":
      return "扫描";
    case "sync":
      return "同步";
    case "import":
      return "导入";
    case "media":
      return "媒体";
    default:
      return "全部";
  }
}

export function matchesTaskFilter(task: CatalogTask, filter: TaskFilter) {
  const taskType = safeLower(task.taskType);

  switch (filter) {
    case "running":
      return isTaskActiveStatus(task.status);
    case "failed":
      return isFailedTaskStatus(task.status);
    case "completed":
      return isSuccessfulTaskStatus(task.status);
    case "scan":
      return taskType === "scan_endpoint";
    case "sync":
      return ["restore_asset", "restore_batch"].includes(taskType);
    case "import":
      return taskType === "import_execute";
    case "media":
      return [
        "thumbnail",
        "video_cover",
        "audio_metadata",
        "audio_transcript",
        "video_transcript",
        "image_semantic",
        "video_semantic"
      ].includes(taskType);
    default:
      return true;
  }
}

export function canRetryTask(task: CatalogTask) {
  if (!isFailedTaskStatus(task.status)) {
    return false;
  }

  return [
    "scan_endpoint",
    "restore_asset",
    "restore_batch",
    "import_execute",
    "thumbnail",
    "video_cover",
    "audio_metadata",
    "audio_transcript",
    "video_transcript",
    "image_semantic",
    "video_semantic"
  ].includes(safeLower(task.taskType));
}

export function getTaskSummary(tasks: CatalogTask[]) {
  return {
    running: tasks.filter((task) => matchesTaskFilter(task, "running")).length,
    failed: tasks.filter((task) => matchesTaskFilter(task, "failed")).length,
    completed: tasks.filter((task) => isSuccessfulTaskStatus(task.status)).length
  };
}

export function getVisibleTasks(tasks: CatalogTask[]) {
  const seen = new Set<string>();
  const sortedTasks = [...tasks].sort(compareTasksForVisibility);
  const visibleTasks: CatalogTask[] = [];

  for (const task of sortedTasks) {
    const key = getTaskGroupingKey(task);
    if (seen.has(key)) {
      continue;
    }

    seen.add(key);
    visibleTasks.push(task);
  }

  return visibleTasks;
}

export function safeLower(value?: string) {
  return typeof value === "string" ? value.trim().toLowerCase() : "";
}

export function isTaskActiveStatus(status?: string) {
  return ["pending", "running", "retrying"].includes(safeLower(status));
}

export function isFailedTaskStatus(status?: string) {
  return ["failed", "error"].includes(safeLower(status));
}

export function isSuccessfulTaskStatus(status?: string) {
  return safeLower(status) === "success";
}

export function findLatestMatchingTask(tasks: CatalogTask[], predicate: (task: CatalogTask) => boolean) {
  let selected: CatalogTask | undefined;

  for (const task of tasks) {
    if (!predicate(task)) {
      continue;
    }

    if (!selected || compareTasksForVisibility(task, selected) < 0) {
      selected = task;
    }
  }

  return selected;
}

export function taskMatchesAsset(task: CatalogTask, assetId: string, taskTypes: string[]) {
  const normalizedAssetId = assetId.trim();
  if (!normalizedAssetId) {
    return false;
  }

  const normalizedTaskTypes = new Set(taskTypes.map((taskType) => safeLower(taskType)).filter(Boolean));
  if (normalizedTaskTypes.size > 0 && !normalizedTaskTypes.has(safeLower(task.taskType))) {
    return false;
  }

  const payload = parseTaskPayload(task.payload);
  return getPayloadString(payload, "assetId", "AssetID") === normalizedAssetId;
}

function compareTasksForVisibility(left: CatalogTask, right: CatalogTask) {
  const leftActive = isTaskActiveStatus(left.status);
  const rightActive = isTaskActiveStatus(right.status);

  if (leftActive !== rightActive) {
    return leftActive ? -1 : 1;
  }

  return compareTasksByRecency(left, right);
}

function compareTasksByRecency(left: CatalogTask, right: CatalogTask) {
  return getTaskTimestamp(right) - getTaskTimestamp(left);
}

function getTaskTimestamp(task: CatalogTask) {
  const timestamp = task.updatedAt || task.finishedAt || task.startedAt || task.createdAt;
  const value = Date.parse(timestamp);
  return Number.isFinite(value) ? value : 0;
}

function getTaskGroupingKey(task: CatalogTask) {
  const taskType = safeLower(task.taskType) || "task";
  const payload = parseTaskPayload(task.payload);

  if (mediaAssetScopedTaskTypes.has(taskType)) {
    const assetId = getPayloadString(payload, "assetId", "AssetID");
    if (assetId) {
      return `${taskType}:asset:${assetId}`;
    }
  }

  if (taskType === "scan_endpoint") {
    const endpointId = getPayloadString(payload, "endpointId", "EndpointID");
    if (endpointId) {
      return `${taskType}:endpoint:${endpointId}`;
    }
  }

  if (taskType === "restore_asset" || taskType === "delete_replica") {
    const assetId = getPayloadString(payload, "assetId", "AssetID");
    const sourceEndpointId = getPayloadString(payload, "sourceEndpointId", "SourceEndpointID");
    const targetEndpointId = getPayloadString(payload, "targetEndpointId", "TargetEndpointID");
    if (assetId) {
      return `${taskType}:asset:${assetId}:source:${sourceEndpointId ?? ""}:target:${targetEndpointId ?? ""}`;
    }
  }

  return `${taskType}:task:${task.id}`;
}

function parseTaskPayload(payload: string) {
  const trimmed = payload.trim();
  if (trimmed.length === 0) {
    return null;
  }

  try {
    const parsed = JSON.parse(trimmed);
    return isRecord(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

function getPayloadString(payload: Record<string, unknown> | null, ...keys: string[]) {
  for (const key of keys) {
    const value = payload?.[key];
    if (typeof value === "string" && value.trim().length > 0) {
      return value.trim();
    }
  }

  return undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}
