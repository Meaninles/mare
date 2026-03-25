import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  AlertTriangle,
  CheckCircle2,
  Download,
  LoaderCircle,
  Pause,
  Play,
  RefreshCcw,
  Search,
  ShieldAlert,
  Trash2,
  Upload,
  Workflow
} from "lucide-react";
import { Link } from "react-router-dom";
import {
  useCatalogSyncOverview,
  useDeleteTransferTasks,
  usePauseTransferTasks,
  useResumeTransferTasks,
  useTransferTaskDetail,
  useTransferTasks
} from "../hooks/useCatalog";
import { formatCatalogDate, formatFileSize, getMediaTypeLabel } from "../lib/catalog-view";
import type { CatalogSyncAsset, CatalogTransferTaskItemRecord, CatalogTransferTaskRecord } from "../types/catalog";

type TransferStatusFilter = "all" | "active" | "queued" | "paused" | "failed" | "success";
type TransferDirectionFilter = "all" | "sync" | "upload" | "download";

const statusFilters: Array<{ value: TransferStatusFilter; label: string }> = [
  { value: "active", label: "进行中" },
  { value: "all", label: "全部状态" },
  { value: "queued", label: "排队中" },
  { value: "paused", label: "已暂停" },
  { value: "failed", label: "失败" },
  { value: "success", label: "已完成" }
];

const directionFilters: Array<{ value: TransferDirectionFilter; label: string }> = [
  { value: "all", label: "全部方向" },
  { value: "sync", label: "同步" },
  { value: "upload", label: "上传" },
  { value: "download", label: "下载" }
];

export function SyncCenterPage() {
  const transferTasksQuery = useTransferTasks(240);
  const syncOverviewQuery = useCatalogSyncOverview();
  const pauseMutation = usePauseTransferTasks();
  const resumeMutation = useResumeTransferTasks();
  const deleteMutation = useDeleteTransferTasks();

  const [statusFilter, setStatusFilter] = useState<TransferStatusFilter>("active");
  const [directionFilter, setDirectionFilter] = useState<TransferDirectionFilter>("all");
  const [searchValue, setSearchValue] = useState("");
  const [selectedTaskIds, setSelectedTaskIds] = useState<string[]>([]);
  const [focusedTaskId, setFocusedTaskId] = useState("");
  const [notice, setNotice] = useState<string | null>(null);

  const transferResult = transferTasksQuery.data;
  const tasks = transferResult?.tasks ?? [];
  const stats = transferResult?.stats;
  const recoverableAssets = syncOverviewQuery.data?.recoverableAssets ?? [];
  const conflictAssets = syncOverviewQuery.data?.conflictAssets ?? [];

  const filteredTasks = useMemo(() => {
    const query = searchValue.trim().toLowerCase();

    return tasks.filter((task) => {
      if (!matchesTransferStatus(task.status, statusFilter)) {
        return false;
      }
      if (!matchesTransferDirection(task.direction, directionFilter)) {
        return false;
      }
      if (!query) {
        return true;
      }

      const haystack = [
        task.id,
        task.title,
        task.sourceLabel,
        task.targetLabel,
        task.currentItemName,
        task.progressLabel
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase();

      return haystack.includes(query);
    });
  }, [directionFilter, searchValue, statusFilter, tasks]);

  const focusedTask = useMemo(
    () => filteredTasks.find((task) => task.id === focusedTaskId) ?? tasks.find((task) => task.id === focusedTaskId) ?? null,
    [filteredTasks, focusedTaskId, tasks]
  );
  const taskDetailQuery = useTransferTaskDetail(focusedTaskId);
  const selectedTaskIdSet = useMemo(() => new Set(selectedTaskIds), [selectedTaskIds]);
  const selectedTasks = useMemo(() => tasks.filter((task) => selectedTaskIdSet.has(task.id)), [selectedTaskIdSet, tasks]);
  const visibleTaskIds = useMemo(() => filteredTasks.map((task) => task.id), [filteredTasks]);
  const allVisibleSelected = visibleTaskIds.length > 0 && visibleTaskIds.every((taskId) => selectedTaskIdSet.has(taskId));
  const hasBusyAction = pauseMutation.isPending || resumeMutation.isPending || deleteMutation.isPending;

  useEffect(() => {
    const visibleTaskIdSet = new Set(tasks.map((task) => task.id));
    setSelectedTaskIds((current) => current.filter((taskId) => visibleTaskIdSet.has(taskId)));
    setFocusedTaskId((current) => (visibleTaskIdSet.has(current) ? current : tasks[0]?.id ?? ""));
  }, [tasks]);

  async function handlePause(taskIds: string[]) {
    if (taskIds.length === 0) {
      return;
    }

    setNotice(null);
    try {
      const summary = await pauseMutation.mutateAsync(taskIds);
      setNotice(summary.message || `已暂停 ${summary.updated} 个任务。`);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "暂停传输任务失败。");
    }
  }

  async function handleResume(taskIds: string[]) {
    if (taskIds.length === 0) {
      return;
    }

    setNotice(null);
    try {
      const summary = await resumeMutation.mutateAsync(taskIds);
      setNotice(summary.message || `已恢复 ${summary.updated} 个任务。`);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "恢复传输任务失败。");
    }
  }

  async function handleDelete(taskIds: string[]) {
    if (taskIds.length === 0) {
      return;
    }
    if (!window.confirm(`确认删除选中的 ${taskIds.length} 个传输任务吗？已完成和断点文件也会一并清理。`)) {
      return;
    }

    setNotice(null);
    try {
      const summary = await deleteMutation.mutateAsync(taskIds);
      setSelectedTaskIds((current) => current.filter((taskId) => !taskIds.includes(taskId)));
      if (taskIds.includes(focusedTaskId)) {
        setFocusedTaskId("");
      }
      setNotice(summary.message || `已删除 ${summary.updated} 个任务。`);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "删除传输任务失败。");
    }
  }

  function toggleTaskSelection(taskId: string) {
    setSelectedTaskIds((current) =>
      current.includes(taskId) ? current.filter((item) => item !== taskId) : [...current, taskId]
    );
  }

  function toggleVisibleSelection() {
    if (visibleTaskIds.length === 0) {
      return;
    }

    setSelectedTaskIds((current) => {
      if (allVisibleSelected) {
        return current.filter((taskId) => !visibleTaskIds.includes(taskId));
      }

      return Array.from(new Set([...current, ...visibleTaskIds]));
    });
  }

  return (
    <section className="page-stack">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">传输中心</p>
          <h3>统一托管同步、上传、下载任务，支持暂停、恢复、删除和断点续传。</h3>
          <p>
            所有恢复与导入动作都会先进入可靠队列，再由后台调度执行。这里可以批量管理任务、查看每个文件的传输阶段，并结合待补资产与冲突资产做统一处理。
          </p>
        </div>

        <div className="hero-metrics transfer-summary-grid">
          <SummaryCard
            icon={<Workflow size={18} />}
            label="同步任务"
            value={stats?.syncTasks ?? 0}
            meta={`${stats?.runningTasks ?? 0} 个进行中`}
            tone="warning"
          />
          <SummaryCard
            icon={<Upload size={18} />}
            label="上传任务"
            value={stats?.uploadTasks ?? 0}
            meta={`${stats?.queuedTasks ?? 0} 个排队中`}
            tone="neutral"
          />
          <SummaryCard
            icon={<Download size={18} />}
            label="下载任务"
            value={stats?.downloadTasks ?? 0}
            meta={`${stats?.pausedTasks ?? 0} 个已暂停`}
            tone="success"
          />
          <SummaryCard
            icon={<ShieldAlert size={18} />}
            label="异常提醒"
            value={(stats?.failedTasks ?? 0) + conflictAssets.length}
            meta={`待补 ${recoverableAssets.length} / 冲突 ${conflictAssets.length}`}
            tone="danger"
          />
        </div>
      </article>

      {notice ? <p className="inline-note">{notice}</p> : null}

      <article className="detail-card">
        <div className="section-head">
          <div>
            <p className="eyebrow">任务面板</p>
            <h4>可靠传输队列</h4>
          </div>

          <div className="action-row">
            <button
              type="button"
              className="ghost-button"
              onClick={() => {
                void Promise.all([transferTasksQuery.refetch(), syncOverviewQuery.refetch()]);
              }}
              disabled={transferTasksQuery.isFetching || syncOverviewQuery.isFetching}
            >
              <RefreshCcw size={16} className={transferTasksQuery.isFetching || syncOverviewQuery.isFetching ? "spin" : ""} />
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
                placeholder="筛选任务标题、来源、目标、当前文件或任务 ID"
              />
            </label>

            <div className="transfer-filter-group">
              <div className="transfer-filter-field compact-filter-control">
                <Workflow size={16} aria-hidden="true" />
                <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value as TransferStatusFilter)}>
                  {statusFilters.map((filter) => (
                    <option key={filter.value} value={filter.value}>
                      {filter.label}
                    </option>
                  ))}
                </select>
              </div>

              <div className="transfer-filter-field compact-filter-control">
                <Upload size={16} aria-hidden="true" />
                <select value={directionFilter} onChange={(event) => setDirectionFilter(event.target.value as TransferDirectionFilter)}>
                  {directionFilters.map((filter) => (
                    <option key={filter.value} value={filter.value}>
                      {filter.label}
                    </option>
                  ))}
                </select>
              </div>

              <div className="action-row">
                <button type="button" className="ghost-button" onClick={toggleVisibleSelection} disabled={visibleTaskIds.length === 0}>
                  {allVisibleSelected ? "取消当前筛选" : "选择当前筛选"}
                </button>
                <button
                  type="button"
                  className="ghost-button"
                  onClick={() => void handlePause(selectedTaskIds)}
                  disabled={hasBusyAction || selectedTaskIds.length === 0}
                >
                  <Pause size={15} />
                  暂停
                </button>
                <button
                  type="button"
                  className="ghost-button"
                  onClick={() => void handleResume(selectedTaskIds)}
                  disabled={hasBusyAction || selectedTaskIds.length === 0}
                >
                  <Play size={15} />
                  恢复
                </button>
                <button
                  type="button"
                  className="ghost-button danger-text"
                  onClick={() => void handleDelete(selectedTaskIds)}
                  disabled={hasBusyAction || selectedTaskIds.length === 0}
                >
                  <Trash2 size={15} />
                  删除
                </button>
              </div>
            </div>
          </div>
        </div>

        {transferTasksQuery.isLoading ? (
          <EmptyBlock
            icon={<LoaderCircle size={20} className="spin" />}
            title="正在读取传输任务"
            copy="后台正在汇总当前资产库里的可靠传输队列。"
          />
        ) : null}

        {transferTasksQuery.isError ? (
          <EmptyBlock
            icon={<AlertTriangle size={20} />}
            title="暂时无法读取传输中心"
            copy={transferTasksQuery.error instanceof Error ? transferTasksQuery.error.message : "请稍后再试。"}
          />
        ) : null}

        {!transferTasksQuery.isLoading && !transferTasksQuery.isError && filteredTasks.length === 0 ? (
          <EmptyBlock
            icon={<CheckCircle2 size={20} />}
            title="当前筛选下没有传输任务"
            copy="在资产详情页、资产页或导入中心发起恢复与导入后，这里会集中显示。"
          />
        ) : null}

        {!transferTasksQuery.isLoading && !transferTasksQuery.isError && filteredTasks.length > 0 ? (
          <div className="transfer-table-wrap">
            <table className="transfer-task-table">
              <colgroup>
                <col style={{ width: "5%" }} />
                <col style={{ width: "24%" }} />
                <col style={{ width: "26%" }} />
                <col style={{ width: "16%" }} />
                <col style={{ width: "11%" }} />
                <col style={{ width: "10%" }} />
                <col style={{ width: "8%" }} />
              </colgroup>
              <thead>
                <tr>
                  <th>选择</th>
                  <th>任务</th>
                  <th>来源 / 目标</th>
                  <th>进度</th>
                  <th>状态</th>
                  <th>最近更新</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {filteredTasks.map((task) => {
                  const isSelected = selectedTaskIdSet.has(task.id);
                  const isFocused = focusedTaskId === task.id;
                  return (
                    <tr
                      key={task.id}
                      className={`transfer-task-row${isFocused ? " is-active" : ""}${isSelected ? " is-selected" : ""}`}
                      onClick={() => setFocusedTaskId(task.id)}
                    >
                      <td className="transfer-selection-cell" onClick={(event) => event.stopPropagation()}>
                        <input
                          type="checkbox"
                          checked={isSelected}
                          onChange={() => toggleTaskSelection(task.id)}
                          aria-label={`选择任务 ${task.title}`}
                        />
                      </td>
                      <td>
                        <div className="transfer-task-title compact">
                          <strong>{task.title}</strong>
                          <div className="transfer-inline-meta">
                            <span className="status-pill subtle">{getDirectionLabel(task.direction)}</span>
                            <span>{task.totalItems} 项</span>
                            {task.currentItemName ? <span>{task.currentItemName}</span> : null}
                          </div>
                        </div>
                      </td>
                      <td>
                        <div className="transfer-route-cell">
                          <span className="transfer-endpoint-chip">{task.sourceLabel || "未记录来源"}</span>
                          <span className="transfer-route-arrow" aria-hidden="true">
                            →
                          </span>
                          <span className="transfer-endpoint-chip">{task.targetLabel || "未记录目标"}</span>
                        </div>
                      </td>
                      <td>
                        <TransferProgress
                          percent={task.progressPercent}
                          primary={task.progressLabel || `${task.progressPercent}%`}
                          secondary={`${formatFileSize(task.completedBytes)} / ${formatFileSize(task.totalBytes)}`}
                          compact
                        />
                      </td>
                      <td>
                        <span className={`status-pill ${getTransferTone(task.status)}`}>{getTransferStatusLabel(task.status)}</span>
                      </td>
                      <td>
                        <div className="transfer-time-compact">
                          <strong>{formatCatalogDate(task.updatedAt)}</strong>
                          <span>{task.startedAt ? `开始 ${formatCatalogDate(task.startedAt)}` : "等待开始"}</span>
                        </div>
                      </td>
                      <td onClick={(event) => event.stopPropagation()}>
                        <div className="action-row">
                          {canPause(task.status) ? (
                            <button
                              type="button"
                              className="ghost-button transfer-action-button"
                              onClick={() => void handlePause([task.id])}
                              disabled={hasBusyAction}
                            >
                              <Pause size={14} />
                              暂停
                            </button>
                          ) : null}
                          {canResume(task.status) ? (
                            <button
                              type="button"
                              className="ghost-button transfer-action-button"
                              onClick={() => void handleResume([task.id])}
                              disabled={hasBusyAction}
                            >
                              <Play size={14} />
                              恢复
                            </button>
                          ) : null}
                          {!canPause(task.status) && !canResume(task.status) ? (
                            <button
                              type="button"
                              className="ghost-button transfer-action-button"
                              onClick={() => void handleDelete([task.id])}
                              disabled={hasBusyAction}
                            >
                              <Trash2 size={14} />
                              删除
                            </button>
                          ) : null}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        ) : null}
      </article>

      <div className="sync-task-layout">
        <article className="detail-card sync-task-detail-card">
          <TransferTaskDetailPanel
            task={taskDetailQuery.data?.task ?? focusedTask}
            items={taskDetailQuery.data?.items ?? []}
            isLoading={Boolean(focusedTaskId) && taskDetailQuery.isLoading}
            error={taskDetailQuery.error instanceof Error ? taskDetailQuery.error.message : undefined}
          />
        </article>

        <article className="detail-card sync-overview-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">项目侧重点</p>
              <h4>待补与冲突资产</h4>
            </div>
            <Link to="/assets" className="ghost-button">
              打开资产页
            </Link>
          </div>

          <div className="sync-overview-section">
            <div className="section-head">
              <div>
                <h4>待补资产</h4>
              </div>
              <span className="status-pill warning">{recoverableAssets.length}</span>
            </div>
            {recoverableAssets.length === 0 ? (
              <EmptyBlock icon={<CheckCircle2 size={18} />} title="当前没有待补资产" copy="同步缺失副本时，这里会列出可恢复的资产。" />
            ) : (
              <div className="sync-task-item-list">
                {recoverableAssets.slice(0, 6).map((asset) => (
                  <SyncAssetCard key={asset.id} asset={asset} kind="recoverable" />
                ))}
              </div>
            )}
          </div>

          <div className="sync-overview-section">
            <div className="section-head">
              <div>
                <h4>冲突资产</h4>
              </div>
              <span className="status-pill danger">{conflictAssets.length}</span>
            </div>
            {conflictAssets.length === 0 ? (
              <EmptyBlock icon={<ShieldAlert size={18} />} title="当前没有冲突资产" copy="当多个副本出现版本冲突时，这里会提醒人工处理。" />
            ) : (
              <div className="sync-task-item-list">
                {conflictAssets.slice(0, 6).map((asset) => (
                  <SyncAssetCard key={asset.id} asset={asset} kind="conflict" />
                ))}
              </div>
            )}
          </div>
        </article>
      </div>
    </section>
  );
}

function TransferTaskDetailPanel({
  task,
  items,
  isLoading,
  error
}: {
  task: CatalogTransferTaskRecord | null | undefined;
  items: CatalogTransferTaskItemRecord[];
  isLoading: boolean;
  error?: string;
}) {
  if (!task) {
    return <EmptyBlock icon={<Workflow size={20} />} title="请选择一个任务" copy="右侧会显示文件级进度、来源目标、错误与断点恢复状态。" />;
  }

  return (
    <div className="sync-task-detail">
      <div className="section-head">
        <div>
          <p className="eyebrow">任务详情</p>
          <h4>{task.title}</h4>
        </div>
        <span className={`status-pill ${getTransferTone(task.status)}`}>{getTransferStatusLabel(task.status)}</span>
      </div>

      <div className="sync-task-detail-grid">
        <DetailField label="任务 ID" value={task.id} wide />
        <DetailField label="方向" value={getDirectionLabel(task.direction)} />
        <DetailField label="总项目" value={`${task.totalItems}`} />
        <DetailField label="已完成" value={`${task.successItems + task.skippedItems}`} />
        <DetailField label="失败" value={`${task.failedItems}`} />
        <DetailField label="排队 / 暂停" value={`${task.queuedItems} / ${task.pausedItems}`} />
        <DetailField label="来源" value={task.sourceLabel || "未记录"} wide />
        <DetailField label="目标" value={task.targetLabel || "未记录"} wide />
        <DetailField label="创建时间" value={formatCatalogDate(task.createdAt)} />
        <DetailField label="开始时间" value={formatCatalogDate(task.startedAt)} />
        <DetailField label="结束时间" value={formatCatalogDate(task.finishedAt)} />
        <DetailField label="已传输" value={`${formatFileSize(task.completedBytes)} / ${formatFileSize(task.totalBytes)}`} />
      </div>

      <TransferProgress
        percent={task.progressPercent}
        primary={task.progressLabel || `${task.progressPercent}%`}
        secondary={task.currentItemName ? `当前文件：${task.currentItemName}` : "等待处理"}
      />

      {task.errorMessage ? <p className="error-copy">{task.errorMessage}</p> : null}
      {error ? <p className="error-copy">{error}</p> : null}

      {isLoading ? (
        <EmptyBlock icon={<LoaderCircle size={18} className="spin" />} title="正在读取文件项详情" copy="传输中心正在拉取这个任务里的每个文件状态。" />
      ) : items.length === 0 ? (
        <EmptyBlock icon={<CheckCircle2 size={18} />} title="当前没有文件项" copy="如果这是刚入队的任务，稍后会显示到文件级的传输项。" />
      ) : (
        <div className="sync-task-item-list">
          {items.map((item) => (
            <article key={item.id} className="sync-task-item-row">
              <div className="sync-task-item-copy">
                <div className="replica-chip-row">
                  <span className={`status-pill ${getTransferTone(item.status)}`}>{getTransferStatusLabel(item.status)}</span>
                  <span className="replica-chip neutral">{getMediaTypeLabel(item.mediaType)}</span>
                  <span className="replica-chip neutral">{getTransferPhaseLabel(item.phase)}</span>
                </div>
                <strong>{item.displayName}</strong>
                <p>{item.sourceLabel ? `${item.sourceLabel}：${item.sourcePath}` : item.sourcePath}</p>
                <p>{item.targetLabel ? `${item.targetLabel}：${item.targetPath}` : item.targetPath}</p>
                {item.errorMessage ? <p className="error-copy">{item.errorMessage}</p> : null}
              </div>

              <div className="transfer-progress-cell compact">
                <div className="transfer-progress-track compact" aria-hidden="true">
                  <div className="transfer-progress-fill" style={{ width: `${Math.max(0, Math.min(100, item.progressPercent))}%` }} />
                </div>
                <div className="transfer-progress-meta compact">
                  <strong>{item.progressPercent}%</strong>
                  <span>
                    {formatFileSize(item.transferredBytes)} / {formatFileSize(item.totalBytes)}
                  </span>
                </div>
              </div>
            </article>
          ))}
        </div>
      )}
    </div>
  );
}

function SyncAssetCard({ asset, kind }: { asset: CatalogSyncAsset; kind: "recoverable" | "conflict" }) {
  return (
    <article className="sync-task-item-row">
      <div className="sync-task-item-copy">
        <div className="replica-chip-row">
          <span className={`status-pill ${kind === "recoverable" ? "warning" : "danger"}`}>
            {kind === "recoverable" ? "待补" : "冲突"}
          </span>
          <span className="replica-chip neutral">{getMediaTypeLabel(asset.mediaType)}</span>
          <span className="replica-chip neutral">缺失 {asset.missingReplicaCount}</span>
        </div>
        <strong>{asset.displayName}</strong>
        <p>{asset.canonicalPath || asset.logicalPathKey}</p>
      </div>

      <Link to={`/assets?assetId=${asset.id}`} className="ghost-button">
        查看
      </Link>
    </article>
  );
}

function TransferProgress({
  percent,
  primary,
  secondary,
  compact = false
}: {
  percent: number;
  primary: string;
  secondary?: string;
  compact?: boolean;
}) {
  return (
    <div className={`transfer-progress-cell${compact ? " compact" : ""}`}>
      <div className={`transfer-progress-track${compact ? " compact" : ""}`} aria-hidden="true">
        <div className="transfer-progress-fill" style={{ width: `${Math.max(0, Math.min(100, percent))}%` }} />
      </div>
      <div className={`transfer-progress-meta${compact ? " compact" : ""}`}>
        <strong>{Math.max(0, Math.min(100, percent))}%</strong>
        <span>{primary}</span>
        {secondary ? <span>{secondary}</span> : null}
      </div>
    </div>
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

function DetailField({ label, value, wide = false }: { label: string; value: string; wide?: boolean }) {
  return (
    <div className={`sync-info-field${wide ? " wide" : ""}`}>
      <span>{label}</span>
      <strong title={value}>{value}</strong>
    </div>
  );
}

function EmptyBlock({ icon, title, copy }: { icon: ReactNode; title: string; copy: string }) {
  return (
    <div className="sync-empty-block">
      {icon}
      <div>
        <strong>{title}</strong>
        <p>{copy}</p>
      </div>
    </div>
  );
}

function matchesTransferStatus(status: string, filter: TransferStatusFilter) {
  const normalizedStatus = status.trim().toLowerCase();
  switch (filter) {
    case "active":
      return ["queued", "pending", "running", "retrying"].includes(normalizedStatus);
    case "queued":
      return normalizedStatus === "queued";
    case "paused":
      return normalizedStatus === "paused";
    case "failed":
      return ["failed", "error"].includes(normalizedStatus);
    case "success":
      return normalizedStatus === "success";
    default:
      return true;
  }
}

function matchesTransferDirection(direction: string, filter: TransferDirectionFilter) {
  if (filter === "all") {
    return true;
  }
  return direction.trim().toLowerCase() === filter;
}

function getDirectionLabel(direction: string) {
  switch (direction.trim().toLowerCase()) {
    case "upload":
      return "上传";
    case "download":
      return "下载";
    default:
      return "同步";
  }
}

function getTransferStatusLabel(status: string) {
  switch (status.trim().toLowerCase()) {
    case "queued":
      return "排队中";
    case "pending":
      return "等待中";
    case "running":
      return "进行中";
    case "paused":
      return "已暂停";
    case "failed":
    case "error":
      return "失败";
    case "success":
      return "成功";
    case "canceled":
      return "已取消";
    case "skipped":
      return "已跳过";
    default:
      return status || "未知";
  }
}

function getTransferTone(status: string) {
  switch (status.trim().toLowerCase()) {
    case "success":
    case "skipped":
      return "success";
    case "failed":
    case "error":
      return "danger";
    case "paused":
    case "canceled":
      return "neutral";
    default:
      return "warning";
  }
}

function getTransferPhaseLabel(phase: string) {
  switch (phase.trim().toLowerCase()) {
    case "pending":
      return "待处理";
    case "staging":
      return "缓存中";
    case "committing":
      return "提交中";
    case "finalizing":
      return "收尾中";
    case "completed":
      return "已完成";
    case "paused":
      return "已暂停";
    case "canceled":
      return "已取消";
    case "failed":
      return "失败";
    default:
      return phase || "未记录";
  }
}

function canPause(status: string) {
  return ["queued", "pending", "running", "retrying"].includes(status.trim().toLowerCase());
}

function canResume(status: string) {
  return ["paused", "failed", "canceled"].includes(status.trim().toLowerCase());
}
