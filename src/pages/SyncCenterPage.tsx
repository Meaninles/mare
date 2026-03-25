import { useEffect, useMemo, useState } from "react";
import { AlertTriangle, CheckCircle2, LoaderCircle, RefreshCcw, ShieldAlert } from "lucide-react";
import { Link } from "react-router-dom";
import { useCatalogRetrySyncTask, useCatalogSyncOverview, useCatalogTasks } from "../hooks/useCatalog";
import { formatCatalogDate, getMediaTypeLabel } from "../lib/catalog-view";
import type { CatalogSyncAsset, CatalogTask } from "../types/catalog";

type SyncTaskFilter = "all" | "running" | "failed" | "success";

type SyncTaskItemView = {
  assetId?: string;
  displayName: string;
  sourceEndpointName?: string;
  targetEndpointName?: string;
  status: string;
  skipped?: boolean;
  error?: string;
};

type SyncTaskSummaryView = {
  taskId?: string;
  status?: string;
  targetEndpointName?: string;
  sourceEndpointName?: string;
  progressLabel?: string;
  progressPercent?: number;
  totalAssets?: number;
  successCount?: number;
  failedCount?: number;
  skippedCount?: number;
  error?: string;
  items: SyncTaskItemView[];
};

type SyncTaskView = {
  task: CatalogTask;
  title: string;
  summary: SyncTaskSummaryView | null;
};

const syncFilters: Array<{ value: SyncTaskFilter; label: string }> = [
  { value: "running", label: "进行中" },
  { value: "all", label: "全部" },
  { value: "failed", label: "失败" },
  { value: "success", label: "已完成" }
];

export function SyncCenterPage() {
  const tasksQuery = useCatalogTasks(200);
  const syncOverviewQuery = useCatalogSyncOverview();
  const retryMutation = useCatalogRetrySyncTask();
  const [taskFilter, setTaskFilter] = useState<SyncTaskFilter>("running");
  const [selectedTaskId, setSelectedTaskId] = useState("");
  const [notice, setNotice] = useState<string | null>(null);

  const conflictAssets = syncOverviewQuery.data?.conflictAssets ?? [];
  const syncTasks = useMemo(() => {
    return (tasksQuery.data ?? [])
      .filter((task) => isSyncTask(task.taskType))
      .map((task) => ({
        task,
        title: getSyncTaskTitle(task.taskType),
        summary: parseSyncTaskSummary(task)
      }));
  }, [tasksQuery.data]);

  const filteredTasks = useMemo(() => {
    return syncTasks.filter((item) => matchesSyncTaskFilter(item.task, taskFilter));
  }, [syncTasks, taskFilter]);

  const summary = useMemo(() => {
    return {
      total: syncTasks.length,
      running: syncTasks.filter((item) => matchesSyncTaskFilter(item.task, "running")).length,
      failed: syncTasks.filter((item) => matchesSyncTaskFilter(item.task, "failed")).length,
      success: syncTasks.filter((item) => matchesSyncTaskFilter(item.task, "success")).length
    };
  }, [syncTasks]);

  useEffect(() => {
    setSelectedTaskId((current) => (filteredTasks.some((item) => item.task.id === current) ? current : filteredTasks[0]?.task.id ?? ""));
  }, [filteredTasks]);

  const selectedTask = useMemo(() => {
    return filteredTasks.find((item) => item.task.id === selectedTaskId) ?? null;
  }, [filteredTasks, selectedTaskId]);

  async function handleRetry(taskId: string) {
    setNotice(null);
    try {
      const summary = await retryMutation.mutateAsync(taskId);
      setNotice(summary.message);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "重试失败。");
    }
  }

  return (
    <section className="page-stack">
      <article className="detail-card sync-task-header-card">
        <div className="section-head">
          <div>
            <h4>同步任务</h4>
          </div>

          <button
            type="button"
            className="ghost-button"
            onClick={() => {
              void Promise.all([tasksQuery.refetch(), syncOverviewQuery.refetch()]);
            }}
            disabled={tasksQuery.isFetching || syncOverviewQuery.isFetching}
          >
            <RefreshCcw size={16} />
            刷新
          </button>
        </div>

        <div className="replica-chip-row">
          <span className="replica-chip neutral">任务 {summary.total}</span>
          <span className="replica-chip warning">进行中 {summary.running}</span>
          <span className="replica-chip danger">失败 {summary.failed}</span>
          <span className="replica-chip success">已完成 {summary.success}</span>
          <span className="replica-chip neutral">冲突 {conflictAssets.length}</span>
        </div>

        <div className="segmented-group sync-status-filter" aria-label="同步任务筛选">
          {syncFilters.map((filter) => (
            <button
              key={filter.value}
              type="button"
              className={`segmented-button${taskFilter === filter.value ? " active" : ""}`}
              onClick={() => setTaskFilter(filter.value)}
            >
              {filter.label}
            </button>
          ))}
        </div>
      </article>

      {notice ? <p className="inline-note">{notice}</p> : null}

      {tasksQuery.isLoading ? (
        <article className="detail-card empty-state">
          <LoaderCircle size={24} className="spin" />
          <div>
            <h4>正在读取同步任务</h4>
            <p>稍后会显示当前资产库的同步任务列表。</p>
          </div>
        </article>
      ) : null}

      {tasksQuery.isError ? (
        <article className="detail-card empty-state">
          <AlertTriangle size={24} />
          <div>
            <h4>暂时无法读取同步任务</h4>
            <p>{tasksQuery.error instanceof Error ? tasksQuery.error.message : "请稍后再试。"}</p>
          </div>
        </article>
      ) : null}

      {!tasksQuery.isLoading && !tasksQuery.isError ? (
        <div className="sync-task-layout">
          <article className="detail-card">
            <div className="section-head">
              <div>
                <h4>{taskFilter === "running" ? "进行中的任务" : "任务列表"}</h4>
              </div>
            </div>

            {filteredTasks.length === 0 ? (
              <div className="sync-empty-block">
                <CheckCircle2 size={22} />
                <div>
                  <strong>当前筛选下没有同步任务</strong>
                  <p>在资产页发起同步后，这里会按任务维度集中显示。</p>
                </div>
              </div>
            ) : (
              <div className="sync-task-list">
                {filteredTasks.map((item) => (
                  <button
                    key={item.task.id}
                    type="button"
                    className={`sync-task-row${selectedTask?.task.id === item.task.id ? " is-active" : ""}`}
                    onClick={() => setSelectedTaskId(item.task.id)}
                  >
                    <div className="sync-task-row-head">
                      <strong>{item.title}</strong>
                      <span className={`status-pill ${getTaskTone(item.task.status)}`}>{getTaskStatusLabel(item.task.status)}</span>
                    </div>

                    <div className="replica-chip-row">
                      <span className="replica-chip neutral">创建 {formatCatalogDate(item.task.createdAt)}</span>
                      <span className="replica-chip neutral">更新 {formatCatalogDate(item.task.updatedAt)}</span>
                      {item.summary?.targetEndpointName ? (
                        <span className="replica-chip neutral">目标 {item.summary.targetEndpointName}</span>
                      ) : null}
                    </div>

                    {item.summary?.progressLabel ? <p className="muted-copy clamp-2">{item.summary.progressLabel}</p> : null}
                    {!item.summary?.progressLabel && item.task.resultSummary ? (
                      <p className="muted-copy clamp-2">{item.task.resultSummary}</p>
                    ) : null}
                    {item.task.errorMessage ? <p className="error-copy">{item.task.errorMessage}</p> : null}
                  </button>
                ))}
              </div>
            )}
          </article>

          <div className="sync-task-side">
            <article className="detail-card sync-task-detail-card">
              {selectedTask ? (
                <SyncTaskDetail
                  taskView={selectedTask}
                  retryPending={retryMutation.isPending}
                  onRetry={selectedTask.task.id ? () => void handleRetry(selectedTask.task.id) : undefined}
                />
              ) : (
                <div className="sync-empty-block">
                  <CheckCircle2 size={22} />
                  <div>
                    <strong>请选择一个同步任务</strong>
                    <p>右侧会显示这次同步的目标端、状态和每个资产的处理结果。</p>
                  </div>
                </div>
              )}
            </article>

            <article className="detail-card sync-conflict-card">
              <div className="section-head">
                <div>
                  <h4>冲突资产</h4>
                </div>
                <span className="status-pill neutral">{conflictAssets.length}</span>
              </div>

              {syncOverviewQuery.isLoading ? (
                <div className="sync-empty-block">
                  <LoaderCircle size={18} className="spin" />
                  <div>
                    <strong>正在检查冲突</strong>
                  </div>
                </div>
              ) : conflictAssets.length === 0 ? (
                <div className="sync-empty-block">
                  <ShieldAlert size={20} />
                  <div>
                    <strong>当前没有冲突</strong>
                    <p>若某个资产存在版本冲突，会在这里集中提示。</p>
                  </div>
                </div>
              ) : (
                <div className="sync-task-item-list">
                  {conflictAssets.map((asset) => (
                    <article key={asset.id} className="sync-task-item-row">
                      <div className="sync-task-item-copy">
                        <strong>{asset.displayName}</strong>
                        <p>{asset.canonicalPath ?? asset.logicalPathKey}</p>
                      </div>
                      <div className="action-row">
                        <span className="asset-badge">{getMediaTypeLabel(asset.mediaType)}</span>
                        <Link to={`/assets?assetId=${asset.id}`} className="ghost-button">
                          查看
                        </Link>
                      </div>
                    </article>
                  ))}
                </div>
              )}
            </article>
          </div>
        </div>
      ) : null}
    </section>
  );
}

function SyncTaskDetail({
  taskView,
  retryPending,
  onRetry
}: {
  taskView: SyncTaskView;
  retryPending: boolean;
  onRetry?: () => void;
}) {
  const { task, title, summary } = taskView;
  const items = summary?.items ?? [];

  return (
    <div className="sync-task-detail">
      <div className="section-head">
        <div>
          <h4>{title}</h4>
        </div>

        <div className="action-row">
          <span className={`status-pill ${getTaskTone(task.status)}`}>{getTaskStatusLabel(task.status)}</span>
          {onRetry && isFailedStatus(task.status) ? (
            <button type="button" className="ghost-button" onClick={onRetry} disabled={retryPending}>
              <RefreshCcw size={16} />
              重试
            </button>
          ) : null}
        </div>
      </div>

      <div className="sync-task-detail-grid">
        <SyncDetailField label="任务 ID" value={task.id} wide />
        <SyncDetailField label="创建时间" value={formatCatalogDate(task.createdAt)} />
        <SyncDetailField label="开始时间" value={formatCatalogDate(task.startedAt)} />
        <SyncDetailField label="结束时间" value={formatCatalogDate(task.finishedAt)} />
        <SyncDetailField label="目标端" value={summary?.targetEndpointName ?? "未记录"} />
        <SyncDetailField label="来源端" value={summary?.sourceEndpointName ?? "未记录"} />
        <SyncDetailField label="总资产" value={formatNumber(summary?.totalAssets)} />
        <SyncDetailField label="成功" value={formatNumber(summary?.successCount)} />
        <SyncDetailField label="失败" value={formatNumber(summary?.failedCount)} />
        <SyncDetailField label="跳过" value={formatNumber(summary?.skippedCount)} />
      </div>

      {summary?.progressLabel ? <p className="inline-note">{summary.progressLabel}</p> : null}
      {summary?.error ? <p className="error-copy">{summary.error}</p> : null}
      {!summary?.error && task.errorMessage ? <p className="error-copy">{task.errorMessage}</p> : null}

      {items.length > 0 ? (
        <div className="sync-task-item-list">
          {items.map((item, index) => (
            <article key={`${item.assetId ?? item.displayName}-${index}`} className="sync-task-item-row">
              <div className="sync-task-item-copy">
                <div className="replica-chip-row">
                  <span className={`status-pill ${getTaskTone(item.status)}`}>{getTaskStatusLabel(item.status)}</span>
                  {item.targetEndpointName ? <span className="replica-chip neutral">目标 {item.targetEndpointName}</span> : null}
                  {item.sourceEndpointName ? <span className="replica-chip neutral">来源 {item.sourceEndpointName}</span> : null}
                </div>
                <strong>{item.displayName}</strong>
                {item.error ? <p className="error-copy">{item.error}</p> : null}
              </div>

              {item.assetId ? (
                <Link to={`/assets?assetId=${item.assetId}`} className="ghost-button">
                  查看
                </Link>
              ) : null}
            </article>
          ))}
        </div>
      ) : (
        <div className="sync-empty-block">
          <CheckCircle2 size={20} />
          <div>
            <strong>当前没有更细的资产条目</strong>
          </div>
        </div>
      )}
    </div>
  );
}

function SyncDetailField({
  label,
  value,
  wide = false
}: {
  label: string;
  value: string;
  wide?: boolean;
}) {
  return (
    <div className={`sync-info-field${wide ? " wide" : ""}`}>
      <span>{label}</span>
      <strong title={value}>{value}</strong>
    </div>
  );
}

function parseSyncTaskSummary(task: CatalogTask): SyncTaskSummaryView | null {
  const parsed = safeParseJson(task.resultSummary);
  if (!parsed) {
    return null;
  }

  if (task.taskType === "restore_asset") {
    return {
      taskId: getString(parsed.taskId),
      status: getString(parsed.status),
      targetEndpointName: getString(parsed.targetEndpointName),
      sourceEndpointName: getString(parsed.sourceEndpointName),
      progressLabel: getString(parsed.progressLabel),
      progressPercent: getNumber(parsed.progressPercent),
      error: getString(parsed.error),
      items: [
        {
          assetId: getString(parsed.assetId),
          displayName: getString(parsed.displayName) ?? "未命名资产",
          sourceEndpointName: getString(parsed.sourceEndpointName),
          targetEndpointName: getString(parsed.targetEndpointName),
          status: getString(parsed.status) ?? task.status,
          skipped: getBoolean(parsed.skipped),
          error: getString(parsed.error)
        }
      ]
    };
  }

  const rawItems = Array.isArray(parsed.items) ? parsed.items : [];
  return {
    taskId: getString(parsed.taskId),
    status: getString(parsed.status),
    targetEndpointName: getString(parsed.targetEndpointName),
    sourceEndpointName: getString(parsed.sourceEndpointName),
    progressLabel: getString(parsed.progressLabel),
    progressPercent: getNumber(parsed.progressPercent),
    totalAssets: getNumber(parsed.totalAssets),
    successCount: getNumber(parsed.successCount),
    failedCount: getNumber(parsed.failedCount),
    skippedCount: getNumber(parsed.skippedCount),
    error: getString(parsed.error),
    items: rawItems.map((item) => {
      const record = isRecord(item) ? item : {};
      return {
        assetId: getString(record.assetId),
        displayName: getString(record.displayName) ?? "未命名资产",
        sourceEndpointName: getString(record.sourceEndpointName),
        targetEndpointName: getString(record.targetEndpointName),
        status: getString(record.status) ?? task.status,
        skipped: getBoolean(record.skipped),
        error: getString(record.error)
      };
    })
  };
}

function safeParseJson(value?: string) {
  if (!value) {
    return null;
  }

  try {
    const parsed = JSON.parse(value);
    return isRecord(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

function isSyncTask(taskType: string) {
  return ["restore_asset", "restore_batch"].includes(safeLower(taskType));
}

function matchesSyncTaskFilter(task: CatalogTask, filter: SyncTaskFilter) {
  const status = safeLower(task.status);

  switch (filter) {
    case "running":
      return ["pending", "running", "retrying"].includes(status);
    case "failed":
      return ["failed", "error"].includes(status);
    case "success":
      return status === "success";
    default:
      return true;
  }
}

function getSyncTaskTitle(taskType: string) {
  switch (safeLower(taskType)) {
    case "restore_asset":
      return "单资产同步";
    case "restore_batch":
      return "批量同步";
    default:
      return taskType;
  }
}

function getTaskStatusLabel(status: string) {
  switch (safeLower(status)) {
    case "pending":
      return "等待中";
    case "running":
      return "进行中";
    case "retrying":
      return "重试中";
    case "success":
      return "已完成";
    case "failed":
    case "error":
      return "失败";
    default:
      return "未知";
  }
}

function getTaskTone(status: string): "success" | "warning" | "danger" | "neutral" {
  switch (safeLower(status)) {
    case "success":
      return "success";
    case "pending":
    case "running":
    case "retrying":
      return "warning";
    case "failed":
    case "error":
      return "danger";
    default:
      return "neutral";
  }
}

function isFailedStatus(status: string) {
  return ["failed", "error"].includes(safeLower(status));
}

function safeLower(value?: string) {
  return typeof value === "string" ? value.trim().toLowerCase() : "";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function getString(value: unknown) {
  return typeof value === "string" && value.trim().length > 0 ? value : undefined;
}

function getNumber(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function getBoolean(value: unknown) {
  return typeof value === "boolean" ? value : undefined;
}

function formatNumber(value?: number) {
  return typeof value === "number" ? `${value}` : "未记录";
}
