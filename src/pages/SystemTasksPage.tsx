import { useMemo, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  AlertTriangle,
  CheckCircle2,
  Download,
  FolderOpen,
  LoaderCircle,
  RefreshCcw,
  Search,
  Workflow
} from "lucide-react";
import { useNavigate } from "react-router-dom";
import { useLibraryContext } from "../context/LibraryContext";
import { useCatalogRetryTask } from "../hooks/useCatalog";
import { formatCatalogDate } from "../lib/catalog-view";
import {
  canRetryTask,
  getTaskStatusLabel,
  getTaskTitle,
  getTaskTone,
  safeLower
} from "../lib/task-center";
import { listLibraries, listLibraryTasks } from "../services/desktop";
import type { LibraryTaskRecord } from "../types/libraries";

type TaskLane = "sync" | "download";
type LaneFilter = "all" | TaskLane;
type StatusFilter = "all" | "running" | "failed" | "success";

type TransferTaskView = {
  task: LibraryTaskRecord;
  lane: TaskLane;
  laneLabel: string;
  subject: string;
  detail: string;
  sourceLabel: string;
  targetLabel: string;
  progressPercent: number;
  progressLabel: string;
  latestAt?: string;
};

const laneFilters: Array<{ value: LaneFilter; label: string }> = [
  { value: "all", label: "全部类别" },
  { value: "sync", label: "同步任务" },
  { value: "download", label: "下载任务" }
];

const statusFilters: Array<{ value: StatusFilter; label: string }> = [
  { value: "all", label: "全部状态" },
  { value: "running", label: "进行中" },
  { value: "failed", label: "失败" },
  { value: "success", label: "已完成" }
];

export function SystemTasksPage() {
  const navigate = useNavigate();
  const { currentLibrary, isLibraryOpen } = useLibraryContext();
  const retryMutation = useCatalogRetryTask();
  const [libraryFilter, setLibraryFilter] = useState("all");
  const [laneFilter, setLaneFilter] = useState<LaneFilter>("sync");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("running");
  const [searchValue, setSearchValue] = useState("");

  const librariesQuery = useQuery({
    queryKey: ["registered-libraries"],
    queryFn: listLibraries,
    staleTime: 30_000
  });

  const tasksQuery = useQuery({
    queryKey: ["library-task-feed", "all"],
    queryFn: () => listLibraryTasks(),
    staleTime: 5_000,
    refetchInterval: 8_000
  });

  const libraryLookup = useMemo(() => {
    return new Map((librariesQuery.data ?? []).map((library) => [library.id, library]));
  }, [librariesQuery.data]);

  const taskViews = useMemo(() => {
    return (tasksQuery.data ?? [])
      .map((task) => buildTransferTaskView(task))
      .filter((task): task is TransferTaskView => task !== null);
  }, [tasksQuery.data]);

  const filteredTasks = useMemo(() => {
    const query = searchValue.trim().toLowerCase();

    return taskViews.filter((task) => {
      if (libraryFilter !== "all" && task.task.libraryId !== libraryFilter) {
        return false;
      }

      if (laneFilter !== "all" && task.lane !== laneFilter) {
        return false;
      }

      if (statusFilter !== "all" && !matchesStatusFilter(task.task.status, statusFilter)) {
        return false;
      }

      if (!query) {
        return true;
      }

      const haystack = [
        task.task.libraryName,
        task.subject,
        task.detail,
        task.sourceLabel,
        task.targetLabel,
        task.task.id
      ]
        .join(" ")
        .toLowerCase();

      return haystack.includes(query);
    });
  }, [laneFilter, libraryFilter, searchValue, statusFilter, taskViews]);

  const summary = useMemo(() => {
    const syncTasks = taskViews.filter((task) => task.lane === "sync");
    const downloadTasks = taskViews.filter((task) => task.lane === "download");
    const libraryIds = new Set(taskViews.map((task) => task.task.libraryId));

    return {
      syncTotal: syncTasks.length,
      syncRunning: syncTasks.filter((task) => matchesStatusFilter(task.task.status, "running")).length,
      downloadTotal: downloadTasks.length,
      downloadRunning: downloadTasks.filter((task) => matchesStatusFilter(task.task.status, "running")).length,
      failedTotal: taskViews.filter((task) => matchesStatusFilter(task.task.status, "failed")).length,
      libraryCount: libraryIds.size
    };
  }, [taskViews]);

  const selectedLibrary = libraryFilter === "all" ? undefined : libraryLookup.get(libraryFilter);

  return (
    <section className="page-stack">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">传输任务</p>
          <h3>统一查看所有资产库里的文件传输任务，并按资产库筛选。</h3>
          <p>
            资产库内部流转一律归到同步任务；只有从某个位置下载到资产库外文件夹的任务，才会归到下载任务。当前页面默认聚焦同步任务。
          </p>
        </div>

        <div className="hero-metrics transfer-summary-grid">
          <SummaryCard
            icon={<Workflow size={18} />}
            label="同步任务"
            value={summary.syncTotal}
            meta={summary.syncRunning > 0 ? `${summary.syncRunning} 个进行中` : "当前空闲"}
            tone="warning"
          />
          <SummaryCard
            icon={<Download size={18} />}
            label="下载任务"
            value={summary.downloadTotal}
            meta={summary.downloadRunning > 0 ? `${summary.downloadRunning} 个进行中` : "暂时没有"}
            tone="neutral"
          />
          <SummaryCard
            icon={<AlertTriangle size={18} />}
            label="失败任务"
            value={summary.failedTotal}
            meta="可按资产库筛选并查看错误"
            tone="danger"
          />
          <SummaryCard
            icon={<FolderOpen size={18} />}
            label="涉及资产库"
            value={summary.libraryCount}
            meta={librariesQuery.data?.length ? `共登记 ${librariesQuery.data.length} 个资产库` : "还没有登记资产库"}
            tone="success"
          />
        </div>
      </article>

      <article className="detail-card">
        <div className="section-head">
          <div>
            <p className="eyebrow">当前上下文</p>
            <h4>{selectedLibrary ? selectedLibrary.name : "所有资产库"}</h4>
          </div>

          {isLibraryOpen && currentLibrary ? (
            <button type="button" className="ghost-button" onClick={() => navigate("/assets")}>
              <FolderOpen size={16} />
              返回当前资产库
            </button>
          ) : (
            <button type="button" className="ghost-button" onClick={() => navigate("/welcome")}>
              <FolderOpen size={16} />
              打开资产库
            </button>
          )}
        </div>

        <div className="field-grid">
          <div className="field">
            <span>筛选范围</span>
            <strong>{selectedLibrary ? selectedLibrary.path : "所有已登记资产库"}</strong>
          </div>
          <div className="field">
            <span>当前会话</span>
            <strong>{currentLibrary ? `当前打开：${currentLibrary.name}` : "当前没有已打开的资产库"}</strong>
          </div>
          <div className="field field-span">
            <span>说明</span>
            <strong>
              这个页面属于应用层全局入口，不在库内导航里。你可以先看所有资产库的传输任务，再切换到某个具体资产库细看。
            </strong>
          </div>
        </div>
      </article>

      <article className="detail-card">
        <div className="section-head">
          <div>
            <p className="eyebrow">任务面板</p>
            <h4>跨资产库传输列表</h4>
          </div>

          <div className="action-row">
            <button
              type="button"
              className="ghost-button"
              onClick={() => {
                void Promise.all([tasksQuery.refetch(), librariesQuery.refetch()]);
              }}
              disabled={tasksQuery.isFetching || librariesQuery.isFetching}
            >
              <RefreshCcw size={14} className={tasksQuery.isFetching || librariesQuery.isFetching ? "spin" : ""} />
              刷新
            </button>
          </div>
        </div>

        <div className="transfer-toolbar">
          <div className="transfer-toolbar-main">
            <label className="transfer-search" aria-label="筛选传输任务">
              <Search size={16} />
              <input
                value={searchValue}
                onChange={(event) => setSearchValue(event.target.value)}
                placeholder="筛选任务名称、来源、目标、任务 ID 或资产库"
              />
            </label>

            <div className="transfer-filter-group">
              <label className="field transfer-filter-field">
                <span>资产库</span>
                <select value={libraryFilter} onChange={(event) => setLibraryFilter(event.target.value)}>
                  <option value="all">所有资产库</option>
                  {(librariesQuery.data ?? []).map((library) => (
                    <option key={library.id} value={library.id}>
                      {library.name}
                    </option>
                  ))}
                </select>
              </label>

              <label className="field transfer-filter-field">
                <span>任务类别</span>
                <select value={laneFilter} onChange={(event) => setLaneFilter(event.target.value as LaneFilter)}>
                  {laneFilters.map((filter) => (
                    <option key={filter.value} value={filter.value}>
                      {filter.label}
                    </option>
                  ))}
                </select>
              </label>

              <label className="field transfer-filter-field">
                <span>状态</span>
                <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value as StatusFilter)}>
                  {statusFilters.map((filter) => (
                    <option key={filter.value} value={filter.value}>
                      {filter.label}
                    </option>
                  ))}
                </select>
              </label>
            </div>
          </div>
        </div>

        {tasksQuery.isLoading || librariesQuery.isLoading ? (
          <div className="sync-empty-block">
            <LoaderCircle size={20} className="spin" />
            <div>
              <strong>正在汇总传输任务</strong>
              <p>正在读取所有已登记资产库里的同步和下载任务。</p>
            </div>
          </div>
        ) : null}

        {tasksQuery.isError || librariesQuery.isError ? (
          <div className="sync-empty-block">
            <AlertTriangle size={20} />
            <div>
              <strong>暂时无法读取传输任务</strong>
              <p>
                {tasksQuery.error instanceof Error
                  ? tasksQuery.error.message
                  : librariesQuery.error instanceof Error
                    ? librariesQuery.error.message
                    : "请稍后再试。"}
              </p>
            </div>
          </div>
        ) : null}

        {!tasksQuery.isLoading && !librariesQuery.isLoading && !tasksQuery.isError && !librariesQuery.isError && filteredTasks.length === 0 ? (
          <div className="sync-empty-block">
            <CheckCircle2 size={20} />
            <div>
              <strong>当前筛选下没有传输任务</strong>
              <p>可以切回所有资产库、放宽筛选条件，或者等待下一次文件流转开始。</p>
            </div>
          </div>
        ) : null}

        {!tasksQuery.isLoading && !librariesQuery.isLoading && !tasksQuery.isError && !librariesQuery.isError && filteredTasks.length > 0 ? (
          <div className="transfer-table-wrap">
            <table className="transfer-task-table">
              <thead>
                <tr>
                  <th>资产库</th>
                  <th>任务对象</th>
                  <th>类别</th>
                  <th>来源</th>
                  <th>目标</th>
                  <th>进度</th>
                  <th>状态</th>
                  <th>任务时间</th>
                  <th>处理说明</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {filteredTasks.map((item) => (
                  <tr key={`${item.task.libraryId}-${item.task.id}`} className="transfer-task-row">
                    <td>
                      <div className="transfer-task-library">
                        <strong>{item.task.libraryName}</strong>
                        <p>{item.task.libraryIsActive ? "当前激活" : "已登记资产库"}</p>
                      </div>
                    </td>
                    <td>
                      <div className="transfer-task-title">
                        <strong>{item.subject}</strong>
                        <p>{getTaskTitle(item.task.taskType)}</p>
                        <span>{item.task.id}</span>
                      </div>
                    </td>
                    <td>
                      <span className="status-pill subtle">{item.laneLabel}</span>
                    </td>
                    <td>
                      <div className="transfer-endpoint-stack">
                        <strong>{item.sourceLabel}</strong>
                      </div>
                    </td>
                    <td>
                      <div className="transfer-endpoint-stack">
                        <strong>{item.targetLabel}</strong>
                      </div>
                    </td>
                    <td>
                      <div className="transfer-progress-cell">
                        <div className="transfer-progress-track" aria-hidden="true">
                          <div className="transfer-progress-fill" style={{ width: `${item.progressPercent}%` }} />
                        </div>
                        <div className="transfer-progress-meta">
                          <strong>{item.progressPercent}%</strong>
                          <span>{item.progressLabel}</span>
                        </div>
                      </div>
                    </td>
                    <td>
                      <span className={`status-pill ${getTaskTone(item.task.status)}`}>
                        {getTaskStatusLabel(item.task.status)}
                      </span>
                    </td>
                    <td>
                      <div className="transfer-time-stack">
                        <span>创建：{formatCatalogDate(item.task.createdAt)}</span>
                        {item.task.startedAt ? <span>开始：{formatCatalogDate(item.task.startedAt)}</span> : null}
                        {item.task.finishedAt ? <span>完成：{formatCatalogDate(item.task.finishedAt)}</span> : null}
                        {!item.task.finishedAt ? <span>更新：{formatCatalogDate(item.latestAt)}</span> : null}
                      </div>
                    </td>
                    <td>
                      <p className="transfer-task-detail">{item.detail}</p>
                    </td>
                    <td>
                      {canRetryTask(item.task) && item.task.libraryIsActive ? (
                        <button
                          type="button"
                          className="ghost-button"
                          onClick={() => void retryMutation.mutateAsync(item.task.id)}
                          disabled={retryMutation.isPending}
                        >
                          <RefreshCcw size={14} className={retryMutation.isPending ? "spin" : ""} />
                          重试
                        </button>
                      ) : canRetryTask(item.task) ? (
                        <span className="status-pill subtle">先打开该资产库</span>
                      ) : (
                        <span className="status-pill subtle">无可执行操作</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
      </article>
    </section>
  );
}

function SummaryCard({
  icon,
  label,
  value,
  meta,
  tone
}: {
  icon: ReactNode;
  label: string;
  value: number;
  meta: string;
  tone: "success" | "warning" | "danger" | "neutral";
}) {
  return (
    <article className={`metric-card transfer-summary-card tone-${tone}`}>
      <div className="transfer-summary-card-head">
        <span className="transfer-summary-card-icon">{icon}</span>
        <p>{label}</p>
      </div>
      <strong>{value}</strong>
      <small>{meta}</small>
    </article>
  );
}

function buildTransferTaskView(task: LibraryTaskRecord): TransferTaskView | null {
  const payload = parseJsonRecord(task.payload);
  const result = parseJsonRecord(task.resultSummary);
  const lane = classifyTaskLane(task, payload, result);

  if (!lane) {
    return null;
  }

  return {
    task,
    lane,
    laneLabel: lane === "sync" ? "同步任务" : "下载任务",
    subject: resolveTaskSubject(task, payload, result),
    detail: resolveTaskDetail(task, payload, result),
    sourceLabel: task.sourceEndpointName ?? resolveTaskSource(task, result),
    targetLabel: task.targetEndpointName ?? resolveTaskTarget(task, result),
    progressPercent: resolveProgressPercent(task, payload, result),
    progressLabel: resolveProgressLabel(task, payload, result),
    latestAt: task.finishedAt ?? task.updatedAt ?? task.startedAt ?? task.createdAt
  };
}

function classifyTaskLane(
  task: LibraryTaskRecord,
  payload: Record<string, unknown> | null,
  result: Record<string, unknown> | null
): TaskLane | null {
  const taskType = safeLower(task.taskType);

  if (taskType === "import_execute" || taskType === "restore_asset" || taskType === "restore_batch") {
    return "sync";
  }

  if (taskType.startsWith("download")) {
    return "download";
  }

  const destinationFolder = readStringField(payload, "destinationFolder") ?? readStringField(result, "destinationFolder");
  if (destinationFolder) {
    return "download";
  }

  return null;
}

function resolveTaskSubject(
  task: LibraryTaskRecord,
  payload: Record<string, unknown> | null,
  result: Record<string, unknown> | null
) {
  const displayName = readStringField(result, "displayName");
  if (displayName) {
    return displayName;
  }

  const deviceLabel = readStringField(result, "deviceLabel");
  if (deviceLabel) {
    return deviceLabel;
  }

  const totalAssets = readNumberField(result, "totalAssets");
  if (typeof totalAssets === "number" && totalAssets > 1) {
    return `${totalAssets} 个资产`;
  }

  const totalFiles = readNumberField(result, "totalFiles");
  if (typeof totalFiles === "number" && totalFiles > 0) {
    return `${totalFiles} 个文件`;
  }

  const assetIds = readStringArrayField(payload, "assetIds");
  if (assetIds.length > 1) {
    return `${assetIds.length} 个资产`;
  }

  const entryPaths = readStringArrayField(payload, "entryPaths");
  if (entryPaths.length > 0) {
    return entryPaths.length === 1 ? entryPaths[0] : `${entryPaths.length} 个导入文件`;
  }

  return task.id;
}

function resolveTaskDetail(
  task: LibraryTaskRecord,
  payload: Record<string, unknown> | null,
  result: Record<string, unknown> | null
) {
  if (task.errorMessage) {
    return task.errorMessage;
  }

  const targetPhysicalPath = readStringField(result, "targetPhysicalPath");
  if (targetPhysicalPath) {
    return targetPhysicalPath;
  }

  const logicalPathKey = readStringField(result, "logicalPathKey");
  if (logicalPathKey) {
    return logicalPathKey;
  }

  const totalFiles = readNumberField(result, "totalFiles");
  const successCount = readNumberField(result, "successCount");
  const partialCount = readNumberField(result, "partialCount");
  const failedCount = readNumberField(result, "failedCount");
  if (typeof totalFiles === "number") {
    return `共 ${totalFiles} 个文件，成功 ${successCount ?? 0}，部分完成 ${partialCount ?? 0}，失败 ${failedCount ?? 0}`;
  }

  const totalAssets = readNumberField(result, "totalAssets");
  if (typeof totalAssets === "number") {
    return `批量同步 ${totalAssets} 个资产`;
  }

  return task.resultSummary || task.payload || "等待后端返回更详细的处理说明。";
}

function resolveTaskSource(task: LibraryTaskRecord, result: Record<string, unknown> | null) {
  const deviceLabel = readStringField(result, "deviceLabel");
  if (deviceLabel) {
    return deviceLabel;
  }

  return task.sourceEndpointId ?? "-";
}

function resolveTaskTarget(task: LibraryTaskRecord, result: Record<string, unknown> | null) {
  const targetPhysicalPath = readStringField(result, "targetPhysicalPath");
  if (targetPhysicalPath) {
    return targetPhysicalPath;
  }

  return task.targetEndpointId ?? "-";
}

function matchesStatusFilter(status: string, filter: StatusFilter) {
  const normalized = safeLower(status);

  switch (filter) {
    case "running":
      return ["pending", "running", "retrying"].includes(normalized);
    case "failed":
      return ["failed", "error"].includes(normalized);
    case "success":
      return normalized === "success";
    default:
      return true;
  }
}

function resolveProgressPercent(
  task: LibraryTaskRecord,
  payload: Record<string, unknown> | null,
  result: Record<string, unknown> | null
) {
  const jsonPercent = readNumberField(result, "progressPercent");
  if (typeof jsonPercent === "number") {
    return clampPercent(jsonPercent);
  }

  const summaryPercent = extractPercentFromText(task.resultSummary);
  if (typeof summaryPercent === "number") {
    return clampPercent(summaryPercent);
  }

  const status = safeLower(task.status);
  switch (status) {
    case "success":
      return 100;
    case "failed":
    case "error":
      return 100;
    case "retrying":
      return 10;
    case "running":
      return 5;
    default:
      return 0;
  }
}

function resolveProgressLabel(
  task: LibraryTaskRecord,
  payload: Record<string, unknown> | null,
  result: Record<string, unknown> | null
) {
  const jsonLabel = readStringField(result, "progressLabel");
  if (jsonLabel) {
    return jsonLabel;
  }

  const summary = task.resultSummary?.trim();
  if (summary) {
    return summary;
  }

  const status = safeLower(task.status);
  if (status === "success") {
    return "已完成";
  }
  if (status === "failed" || status === "error") {
    return task.errorMessage?.trim() || "执行失败";
  }

  const totalFiles = readNumberField(result, "totalFiles") ?? readNumberField(payload, "totalFiles");
  if (typeof totalFiles === "number" && totalFiles > 0) {
    return `等待处理，共 ${totalFiles} 个文件`;
  }

  return "等待后端上报进度";
}

function parseJsonRecord(value?: string) {
  if (!value) {
    return null;
  }

  try {
    const parsed = JSON.parse(value) as unknown;
    return parsed && typeof parsed === "object" && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}

function readStringField(record: Record<string, unknown> | null, key: string) {
  const value = record?.[key];
  return typeof value === "string" && value.trim().length > 0 ? value.trim() : undefined;
}

function readNumberField(record: Record<string, unknown> | null, key: string) {
  const value = record?.[key];
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function readStringArrayField(record: Record<string, unknown> | null, key: string) {
  const value = record?.[key];
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

function extractPercentFromText(value?: string) {
  if (!value) {
    return undefined;
  }

  const matched = value.match(/(\d{1,3})\s*%/);
  if (!matched) {
    return undefined;
  }

  return clampPercent(Number(matched[1]));
}

function clampPercent(value: number) {
  if (!Number.isFinite(value)) {
    return 0;
  }

  if (value < 0) {
    return 0;
  }
  if (value > 100) {
    return 100;
  }
  return Math.round(value);
}
