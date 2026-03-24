import type { CatalogTask } from "../types/catalog";

export type TaskFilter = "all" | "running" | "failed" | "scan" | "sync" | "import" | "media";

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
  switch (safeLower(status)) {
    case "success":
      return "success";
    case "failed":
    case "error":
      return "danger";
    default:
      return "warning";
  }
}

export function getTaskFilterLabel(filter: TaskFilter) {
  switch (filter) {
    case "running":
      return "进行中";
    case "failed":
      return "失败";
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
  const status = safeLower(task.status);

  switch (filter) {
    case "running":
      return ["pending", "running", "retrying"].includes(status);
    case "failed":
      return ["failed", "error"].includes(status);
    case "scan":
      return taskType === "scan_endpoint";
    case "sync":
      return ["restore_asset", "restore_batch"].includes(taskType);
    case "import":
      return taskType === "import_execute";
    case "media":
      return ["thumbnail", "video_cover", "audio_metadata"].includes(taskType);
    default:
      return true;
  }
}

export function canRetryTask(task: CatalogTask) {
  if (!["failed", "error"].includes(safeLower(task.status))) {
    return false;
  }

  return [
    "scan_endpoint",
    "restore_asset",
    "restore_batch",
    "import_execute",
    "thumbnail",
    "video_cover",
    "audio_metadata"
  ].includes(safeLower(task.taskType));
}

export function getTaskSummary(tasks: CatalogTask[]) {
  return {
    running: tasks.filter((task) => matchesTaskFilter(task, "running")).length,
    failed: tasks.filter((task) => matchesTaskFilter(task, "failed")).length,
    completed: tasks.filter((task) => safeLower(task.status) === "success").length
  };
}

export function safeLower(value?: string) {
  return typeof value === "string" ? value.trim().toLowerCase() : "";
}
