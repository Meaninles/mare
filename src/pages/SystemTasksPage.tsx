import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  AlertTriangle,
  ArrowUp,
  CheckCircle2,
  FolderOpen,
  LoaderCircle,
  Pause,
  Play,
  RefreshCcw,
  Search,
  Upload,
  Workflow,
  XCircle
} from "lucide-react";
import { useNavigate } from "react-router-dom";
import { useLibraryContext } from "../context/LibraryContext";
import {
  useCancelTransferTasks,
  usePauseTransferTasks,
  usePrioritizeTransferTask,
  useResumeTransferTasks,
  useTransferTaskDetail,
  useTransferTasks
} from "../hooks/useCatalog";
import { formatCatalogDate, formatFileSize, getMediaTypeLabel } from "../lib/catalog-view";
import type { CatalogTransferTaskItemRecord, CatalogTransferTaskRecord } from "../types/catalog";

type TransferStatusFilter = "all" | "active" | "queued" | "paused" | "failed" | "success" | "canceled";
type TransferDirectionFilter = "all" | "sync" | "upload" | "download";

const statusFilters: Array<{ value: TransferStatusFilter; label: string }> = [
  { value: "active", label: "进行中" },
  { value: "all", label: "全部状态" },
  { value: "queued", label: "排队中" },
  { value: "paused", label: "已暂停" },
  { value: "failed", label: "失败" },
  { value: "canceled", label: "已取消" },
  { value: "success", label: "已完成" }
];

const directionFilters: Array<{ value: TransferDirectionFilter; label: string }> = [
  { value: "all", label: "全部方向" },
  { value: "download", label: "下载" },
  { value: "upload", label: "上传" },
  { value: "sync", label: "同步" }
];

export function SystemTasksPage() {
  const navigate = useNavigate();
  const { currentLibrary, isLibraryOpen } = useLibraryContext();
  const transferTasksQuery = useTransferTasks(240);
  const pauseMutation = usePauseTransferTasks();
  const resumeMutation = useResumeTransferTasks();
  const cancelMutation = useCancelTransferTasks();
  const prioritizeMutation = usePrioritizeTransferTask();

  const [statusFilter, setStatusFilter] = useState<TransferStatusFilter>("active");
  const [directionFilter, setDirectionFilter] = useState<TransferDirectionFilter>("all");
  const [searchValue, setSearchValue] = useState("");
  const [selectedTaskIds, setSelectedTaskIds] = useState<string[]>([]);
  const [focusedTaskId, setFocusedTaskId] = useState("");
  const [notice, setNotice] = useState<string | null>(null);

  const transferResult = transferTasksQuery.data;
  const tasks = transferResult?.tasks ?? [];
  const stats = transferResult?.stats;
  const selectedTaskIdSet = useMemo(() => new Set(selectedTaskIds), [selectedTaskIds]);

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
        task.fileName,
        task.filePath,
        task.sourceLabel,
        task.targetLabel,
        task.engineSummary,
        task.currentItemName,
        task.progressLabel
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase();

      return haystack.includes(query);
    });
  }, [directionFilter, searchValue, statusFilter, tasks]);

  const visibleTaskIds = useMemo(() => filteredTasks.map((task) => task.id), [filteredTasks]);
  const selectedTasks = useMemo(
    () => tasks.filter((task) => selectedTaskIdSet.has(task.id)),
    [selectedTaskIdSet, tasks]
  );
  const prioritizedTaskIds = useMemo(
    () => selectedTasks.filter((task) => canPrioritizeDownload(task)).map((task) => task.id),
    [selectedTasks]
  );
  const allVisibleSelected = visibleTaskIds.length > 0 && visibleTaskIds.every((taskId) => selectedTaskIdSet.has(taskId));
  const focusedTask =
    filteredTasks.find((task) => task.id === focusedTaskId) ?? tasks.find((task) => task.id === focusedTaskId) ?? null;
  const taskDetailQuery = useTransferTaskDetail(focusedTaskId);
  const hasBusyAction =
    pauseMutation.isPending || resumeMutation.isPending || cancelMutation.isPending || prioritizeMutation.isPending;

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
      setNotice(summary.message || `已暂停 ${summary.updated} 个任务`);
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
      setNotice(summary.message || `已恢复 ${summary.updated} 个任务`);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "恢复传输任务失败。");
    }
  }

  async function handleCancel(taskIds: string[]) {
    if (taskIds.length === 0) {
      return;
    }
    if (!window.confirm(`确认取消选中的 ${taskIds.length} 个任务吗？已完成的文件不会删除，后续仍可恢复继续。`)) {
      return;
    }

    setNotice(null);
    try {
      const summary = await cancelMutation.mutateAsync(taskIds);
      setNotice(summary.message || `已取消 ${summary.updated} 个任务`);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "取消传输任务失败。");
    }
  }

  async function handlePrioritize(taskIds: string[]) {
    const candidates = taskIds
      .map((taskId) => tasks.find((task) => task.id === taskId))
      .filter((task): task is CatalogTransferTaskRecord => {
        if (!task) {
          return false;
        }

        return canPrioritizeDownload(task);
      });

    if (candidates.length === 0) {
      return;
    }

    setNotice(null);

    let updated = 0;
    for (const task of candidates) {
      try {
        const summary = await prioritizeMutation.mutateAsync(task.id);
        updated += summary.updated;
      } catch (error) {
        const message = error instanceof Error ? error.message : "优先下载失败。";
        setNotice(updated > 0 ? `已处理 ${updated} 个任务，后续操作失败：${message}` : message);
        return;
      }
    }

    setNotice(candidates.length === 1 ? "任务已设为优先下载" : `已将 ${updated} 个任务设为优先下载`);
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
    <section className="page-stack system-tasks-page">
      <article className="detail-card compact-page-header system-tasks-header">
        <div className="compact-page-header-main">
          <div className="compact-page-header-title">
            <h3>任务列表</h3>
            <div className="replica-chip-row compact-page-header-metrics">
              <span className="replica-chip success">{currentLibrary ? currentLibrary.name : "未打开资产库"}</span>
              <span className="replica-chip warning">进行中 {stats?.runningTasks ?? 0}</span>
              <span className="replica-chip neutral">上传 {stats?.uploadTasks ?? 0}</span>
              <span className="replica-chip neutral">下载 {stats?.downloadTasks ?? 0}</span>
              <span className="replica-chip danger">失败 {stats?.failedTasks ?? 0}</span>
              {selectedTaskIds.length > 0 ? <span className="replica-chip neutral">已选 {selectedTaskIds.length}</span> : null}
            </div>
          </div>
        </div>

        <div className="compact-page-header-actions">
          <button
            type="button"
            className="ghost-button"
            onClick={() => void transferTasksQuery.refetch()}
            disabled={!isLibraryOpen || transferTasksQuery.isFetching}
          >
            <RefreshCcw size={14} className={transferTasksQuery.isFetching ? "spin" : ""} />
            刷新
          </button>

          <button
            type="button"
            className="ghost-button"
            onClick={() => navigate(isLibraryOpen ? "/assets" : "/welcome")}
          >
            <FolderOpen size={14} />
            {isLibraryOpen ? "返回当前资产库" : "打开资产库"}
          </button>
        </div>
      </article>

      {notice ? <p className="inline-note">{notice}</p> : null}

      {!isLibraryOpen ? (
        <article className="detail-card empty-state">
          <FolderOpen size={22} />
          <div>
            <h4>请先打开一个资产库</h4>
            <p>任务页现在支持直接暂停、恢复、取消和优先下载，需要先进入当前资产库后才能操作传输队列。</p>
          </div>
        </article>
      ) : (
        <>
          <article className="detail-card">
            <div className="section-head">
              <div>
                <p className="eyebrow">任务面板</p>
                <h4>当前资产库传输队列</h4>
              </div>
            </div>

            <div className="transfer-toolbar">
              <div className="transfer-toolbar-main">
                <label className="transfer-search" aria-label="筛选传输任务">
                  <Search size={16} />
                  <input
                    value={searchValue}
                    onChange={(event) => setSearchValue(event.target.value)}
                    placeholder="筛选文件名、路径、来源、目标、任务 ID"
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
                    <select
                      value={directionFilter}
                      onChange={(event) => setDirectionFilter(event.target.value as TransferDirectionFilter)}
                    >
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
                      onClick={() => void handleCancel(selectedTaskIds)}
                      disabled={hasBusyAction || selectedTaskIds.length === 0}
                    >
                      <XCircle size={15} />
                      取消
                    </button>
                    <button
                      type="button"
                      className="ghost-button"
                      onClick={() => void handlePrioritize(prioritizedTaskIds)}
                      disabled={hasBusyAction || prioritizedTaskIds.length === 0}
                    >
                      <ArrowUp size={15} />
                      优先下载
                    </button>
                  </div>
                </div>
              </div>
            </div>

            <p className="transfer-bulk-summary">
              当前筛选下共 {filteredTasks.length} 个任务，已选 {selectedTaskIds.length} 个，可优先下载 {prioritizedTaskIds.length} 个。
            </p>

            {transferTasksQuery.isLoading ? (
              <EmptyBlock
                icon={<LoaderCircle size={20} className="spin" />}
                title="正在读取传输任务"
                copy="后台正在汇总当前资产库里的上传、下载和同步任务。"
              />
            ) : null}

            {transferTasksQuery.isError ? (
              <EmptyBlock
                icon={<AlertTriangle size={20} />}
                title="暂时无法读取传输任务"
                copy={transferTasksQuery.error instanceof Error ? transferTasksQuery.error.message : "请稍后再试。"}
              />
            ) : null}

            {!transferTasksQuery.isLoading && !transferTasksQuery.isError && filteredTasks.length === 0 ? (
              <EmptyBlock
                icon={<CheckCircle2 size={20} />}
                title="当前筛选下没有传输任务"
                copy="发起导入、恢复或下载后，这里会显示可操作的任务列表。"
              />
            ) : null}

            {!transferTasksQuery.isLoading && !transferTasksQuery.isError && filteredTasks.length > 0 ? (
              <div className="transfer-table-wrap">
                <table className="transfer-task-table">
                  <colgroup>
                    <col style={{ width: "5%" }} />
                    <col style={{ width: "24%" }} />
                    <col style={{ width: "17%" }} />
                    <col style={{ width: "14%" }} />
                    <col style={{ width: "10%" }} />
                    <col style={{ width: "10%" }} />
                    <col style={{ width: "8%" }} />
                    <col style={{ width: "10%" }} />
                    <col style={{ width: "12%" }} />
                  </colgroup>
                  <thead>
                    <tr>
                      <th>选择</th>
                      <th>文件</th>
                      <th>来源 / 目标</th>
                      <th>进度</th>
                      <th>文件大小</th>
                      <th>传输速度</th>
                      <th>状态</th>
                      <th>最近更新</th>
                      <th>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredTasks.map((task) => {
                      const isSelected = selectedTaskIdSet.has(task.id);
                      const isFocused = focusedTaskId === task.id;
                      const fileName = getTransferFileName(task);
                      const filePath = getTransferFilePath(task);
                      const fileSize = getTransferFileSize(task);
                      const fileTransferred = getTransferFileTransferred(task);

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
                              aria-label={`选择任务 ${fileName}`}
                            />
                          </td>
                          <td>
                            <div className="transfer-task-title compact">
                              <strong title={fileName}>{fileName}</strong>
                              <p className="transfer-task-path" title={filePath}>
                                {filePath || "未记录路径"}
                              </p>
                              <div className="transfer-inline-meta">
                                <span className="status-pill subtle">{getDirectionLabel(task.direction)}</span>
                                {task.totalItems > 1 ? <span>共 {task.totalItems} 项</span> : null}
                                {task.priority > 0 ? <span>优先级 {task.priority}</span> : null}
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
                            <div className="transfer-time-compact">
                              <strong>{formatFileSize(fileSize)}</strong>
                              <span>
                                {task.totalItems > 1
                                  ? `任务总量 ${formatFileSize(task.totalBytes)}`
                                  : `${formatFileSize(fileTransferred)} 已传输`}
                              </span>
                            </div>
                          </td>
                          <td>
                            <div className="transfer-time-compact">
                              <strong>{formatTransferSpeed(task.currentSpeed)}</strong>
                              <span>{task.engineSummary || "等待调度"}</span>
                            </div>
                          </td>
                          <td>
                            <span className={`status-pill ${getTransferTone(task.status)}`}>{getTransferStatusLabel(task.status)}</span>
                          </td>
                          <td>
                            <div className="transfer-time-compact">
                              <strong>{formatCatalogDate(task.updatedAt)}</strong>
                              <span>{task.startedAt ? `开始于 ${formatCatalogDate(task.startedAt)}` : "等待开始"}</span>
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
                              {canCancel(task.status) ? (
                                <button
                                  type="button"
                                  className="ghost-button transfer-action-button danger-text"
                                  onClick={() => void handleCancel([task.id])}
                                  disabled={hasBusyAction}
                                >
                                  <XCircle size={14} />
                                  取消
                                </button>
                              ) : null}
                              {canPrioritizeDownload(task) ? (
                                <button
                                  type="button"
                                  className="ghost-button transfer-action-button"
                                  onClick={() => void handlePrioritize([task.id])}
                                  disabled={hasBusyAction}
                                >
                                  <ArrowUp size={14} />
                                  优先下载
                                </button>
                              ) : null}
                              {!canPause(task.status) &&
                              !canResume(task.status) &&
                              !canCancel(task.status) &&
                              !canPrioritizeDownload(task) ? (
                                <span className="status-pill subtle">只读</span>
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

          <article className="detail-card sync-task-detail-card">
            <TransferTaskDetailPanel
              task={taskDetailQuery.data?.task ?? focusedTask}
              items={taskDetailQuery.data?.items ?? []}
              isLoading={Boolean(focusedTaskId) && taskDetailQuery.isLoading}
              error={taskDetailQuery.error instanceof Error ? taskDetailQuery.error.message : undefined}
            />
          </article>
        </>
      )}
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
    return <EmptyBlock icon={<Workflow size={20} />} title="请选择一个任务" copy="下方会显示文件级路径、大小、速度和每个传输项的详细状态。" />;
  }

  return (
    <div className="sync-task-detail">
      <div className="section-head">
        <div>
          <p className="eyebrow">任务详情</p>
          <h4>{getTransferFileName(task)}</h4>
        </div>
        <span className={`status-pill ${getTransferTone(task.status)}`}>{getTransferStatusLabel(task.status)}</span>
      </div>

      <div className="sync-task-detail-grid">
        <DetailField label="文件路径" value={getTransferFilePath(task) || "-"} wide />
        <DetailField label="任务 ID" value={task.id} wide />
        <DetailField label="方向" value={getDirectionLabel(task.direction)} />
        <DetailField label="实时速度" value={formatTransferSpeed(task.currentSpeed)} />
        <DetailField label="文件大小" value={formatFileSize(getTransferFileSize(task))} />
        <DetailField label="已传输" value={formatFileSize(getTransferFileTransferred(task))} />
        <DetailField label="来源" value={task.sourceLabel || "未记录"} wide />
        <DetailField label="目标" value={task.targetLabel || "未记录"} wide />
        <DetailField label="总任务项" value={`${task.totalItems}`} />
        <DetailField label="优先级" value={`${task.priority}`} />
        <DetailField label="创建时间" value={formatCatalogDate(task.createdAt)} />
        <DetailField label="开始时间" value={formatCatalogDate(task.startedAt)} />
        <DetailField label="结束时间" value={formatCatalogDate(task.finishedAt)} />
        <DetailField label="任务总量" value={`${formatFileSize(task.completedBytes)} / ${formatFileSize(task.totalBytes)}`} />
      </div>

      <TransferProgress
        percent={task.progressPercent}
        primary={task.progressLabel || `${task.progressPercent}%`}
        secondary={task.currentItemName ? `当前文件：${task.currentItemName}` : "等待处理"}
      />

      {task.errorMessage ? <p className="error-copy">{task.errorMessage}</p> : null}
      {error ? <p className="error-copy">{error}</p> : null}

      {isLoading ? (
        <EmptyBlock icon={<LoaderCircle size={18} className="spin" />} title="正在读取文件项详情" copy="任务中的每个文件项状态正在同步中。" />
      ) : items.length === 0 ? (
        <EmptyBlock icon={<CheckCircle2 size={18} />} title="当前没有文件项详情" copy="如果任务刚入队，稍后会显示更完整的文件级进度。" />
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
                <div className="replica-chip-row">
                  <span className="replica-chip neutral">{formatFileSize(item.totalBytes)}</span>
                  <span className="replica-chip neutral">{formatFileSize(item.transferredBytes)} 已传输</span>
                  {item.currentSpeed ? <span className="replica-chip neutral">{formatTransferSpeed(item.currentSpeed)}</span> : null}
                </div>
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
    case "canceled":
      return normalizedStatus === "canceled";
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

function canPrioritizeDownload(task: CatalogTransferTaskRecord) {
  if (task.direction.trim().toLowerCase() !== "download") {
    return false;
  }

  return ["queued", "pending", "retrying", "paused", "failed", "canceled"].includes(task.status.trim().toLowerCase());
}

function getTransferFileName(task: CatalogTransferTaskRecord) {
  return task.fileName?.trim() || task.currentItemName?.trim() || task.title || task.id;
}

function getTransferFilePath(task: CatalogTransferTaskRecord) {
  return task.filePath?.trim() || "";
}

function getTransferFileSize(task: CatalogTransferTaskRecord) {
  return task.fileSize && task.fileSize > 0 ? task.fileSize : task.totalBytes;
}

function getTransferFileTransferred(task: CatalogTransferTaskRecord) {
  return task.fileTransferredBytes && task.fileTransferredBytes > 0 ? task.fileTransferredBytes : task.completedBytes;
}

function formatTransferSpeed(value?: number) {
  if (!value || value <= 0) {
    return "-";
  }
  return `${formatFileSize(value)}/s`;
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

function canCancel(status: string) {
  return ["queued", "pending", "running", "retrying", "paused"].includes(status.trim().toLowerCase());
}
