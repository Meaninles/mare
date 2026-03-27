import { useMemo, useState, type ReactNode } from "react";
import { AlertTriangle, CheckCircle2, LoaderCircle, RefreshCcw, TerminalSquare } from "lucide-react";
import { PaginationControls } from "../components/PaginationControls";
import { useCatalogRetryTask, useCatalogTasks } from "../hooks/useCatalog";
import { usePagination } from "../hooks/usePagination";
import { useSystemLogs } from "../hooks/useSystemLogs";
import { formatCatalogDate } from "../lib/catalog-view";
import {
  canRetryTask,
  getTaskDisplaySummary,
  getTaskFilterLabel,
  getTaskStatusLabel,
  getTaskSummary,
  getTaskTitle,
  getTaskTone,
  getVisibleTasks,
  matchesTaskFilter,
  type TaskFilter
} from "../lib/task-center";
import type { CatalogTask } from "../types/catalog";
import type { SystemLogLevel } from "../types/system";

const taskFilters: TaskFilter[] = ["all", "running", "failed", "completed", "scan", "sync", "import", "media"];
const logLevels: SystemLogLevel[] = ["all", "info", "warn", "error"];

export function TaskCenterPage() {
  const [taskFilter, setTaskFilter] = useState<TaskFilter>("all");
  const [logLevel, setLogLevel] = useState<SystemLogLevel>("all");
  const [showRunningTasks, setShowRunningTasks] = useState(false);
  const [showFailedTasks, setShowFailedTasks] = useState(false);
  const [showCompletedTasks, setShowCompletedTasks] = useState(false);
  const tasksQuery = useCatalogTasks(500);
  const retryMutation = useCatalogRetryTask();
  const logsQuery = useSystemLogs(50, logLevel);

  const tasks = useMemo(() => getVisibleTasks(tasksQuery.data ?? []), [tasksQuery.data]);
  const summary = getTaskSummary(tasks);
  const filteredTasks = useMemo(() => tasks.filter((task) => matchesTaskFilter(task, taskFilter)), [taskFilter, tasks]);
  const runningTasks = useMemo(() => tasks.filter((task) => matchesTaskFilter(task, "running")), [tasks]);
  const failedTasks = useMemo(() => tasks.filter((task) => matchesTaskFilter(task, "failed")), [tasks]);
  const completedTasks = useMemo(
    () => tasks.filter((task) => task.status.trim().toLowerCase() === "success"),
    [tasks]
  );
  const filteredTasksPagination = usePagination(filteredTasks, 20);
  const logsPagination = usePagination(logsQuery.data?.entries ?? [], 20);
  const usesCollapsedSections =
    taskFilter === "all" || taskFilter === "running" || taskFilter === "failed" || taskFilter === "completed";

  return (
    <section className="page-stack">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">任务与日志</p>
          <h3>在一个页面里查看扫描、恢复、导入和媒体处理任务，并直接定位失败原因。</h3>
          <p>这里集中展示后台执行状态、失败任务重试入口，以及结构化日志，方便追踪最近发生了什么。</p>
        </div>

        <div className="hero-metrics">
          <MetricCard label="进行中" value={summary.running} tone="warning" />
          <MetricCard label="失败" value={summary.failed} tone="danger" />
          <MetricCard label="已完成" value={summary.completed} tone="success" />
          <MetricCard label="当前可见" value={filteredTasks.length} tone="neutral" />
        </div>
      </article>

      <div className="page-grid task-center-layout">
        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">任务队列</p>
              <h4>后台执行状态</h4>
            </div>

            <div className="segmented-group" aria-label="任务筛选">
              {taskFilters.map((filter) => (
                <button
                  key={filter}
                  type="button"
                  className={`segmented-button${taskFilter === filter ? " active" : ""}`}
                  onClick={() => setTaskFilter(filter)}
                >
                  {getTaskFilterLabel(filter)}
                </button>
              ))}
            </div>
          </div>

          {tasksQuery.isLoading ? (
            <EmptyBlock icon={<LoaderCircle size={20} className="spin" />} title="正在读取任务列表" copy="后台最近的任务记录正在从后端拉取。" />
          ) : null}

          {tasksQuery.isError ? (
            <EmptyBlock
              icon={<AlertTriangle size={20} />}
              title="暂时无法加载任务"
              copy={tasksQuery.error instanceof Error ? tasksQuery.error.message : "请稍后再试。"}
            />
          ) : null}

          {!tasksQuery.isLoading && !tasksQuery.isError && usesCollapsedSections ? (
            <>
              {taskFilter === "all" ? (
                <>
                  <CollapsibleTaskSection
                    eyebrow="进行中"
                    title="进行中的任务"
                    emptyTitle="当前没有活动任务"
                    emptyCopy="等待中、进行中和重试中的任务会显示在这里。"
                    collapsedCopy={`当前有 ${summary.running} 个进行中的任务，点击展开查看详情。`}
                    tasks={runningTasks}
                    open={showRunningTasks}
                    onToggle={() => setShowRunningTasks((value) => !value)}
                  />

                  <CollapsibleTaskSection
                    eyebrow="失败"
                    title="失败任务"
                    emptyTitle="当前没有失败任务"
                    emptyCopy="扫描、恢复、导入或媒体任务失败后，可以直接在这里重试。"
                    collapsedCopy={`当前有 ${summary.failed} 个失败任务，点击展开查看和重试。`}
                    tasks={failedTasks}
                    open={showFailedTasks}
                    onToggle={() => setShowFailedTasks((value) => !value)}
                    renderActions={(task) =>
                      canRetryTask(task) ? (
                        <button
                          type="button"
                          className="ghost-button"
                          disabled={retryMutation.isPending}
                          onClick={() => void retryMutation.mutateAsync(task.id)}
                        >
                          <RefreshCcw size={14} />
                          重试任务
                        </button>
                      ) : null
                    }
                  />

                  <CollapsibleTaskSection
                    eyebrow="已完成"
                    title="最近完成的任务"
                    emptyTitle="暂时还没有完成任务"
                    emptyCopy="成功完成的任务会保留在这里，方便回看最近处理好的项目。"
                    collapsedCopy={`已完成 ${summary.completed} 个任务，点击展开查看最近完成的记录。`}
                    tasks={completedTasks}
                    open={showCompletedTasks}
                    onToggle={() => setShowCompletedTasks((value) => !value)}
                  />
                </>
              ) : null}

              {taskFilter === "running" ? (
                <CollapsibleTaskSection
                  eyebrow="进行中"
                  title="进行中的任务"
                  emptyTitle="当前没有活动任务"
                  emptyCopy="等待中、进行中和重试中的任务会显示在这里。"
                  collapsedCopy={`当前有 ${summary.running} 个进行中的任务，点击展开查看详情。`}
                  tasks={runningTasks}
                  open={showRunningTasks}
                  onToggle={() => setShowRunningTasks((value) => !value)}
                />
              ) : null}

              {taskFilter === "failed" ? (
                <CollapsibleTaskSection
                  eyebrow="失败"
                  title="失败任务"
                  emptyTitle="当前没有失败任务"
                  emptyCopy="扫描、恢复、导入或媒体任务失败后，可以直接在这里重试。"
                  collapsedCopy={`当前有 ${summary.failed} 个失败任务，点击展开查看和重试。`}
                  tasks={failedTasks}
                  open={showFailedTasks}
                  onToggle={() => setShowFailedTasks((value) => !value)}
                  renderActions={(task) =>
                    canRetryTask(task) ? (
                      <button
                        type="button"
                        className="ghost-button"
                        disabled={retryMutation.isPending}
                        onClick={() => void retryMutation.mutateAsync(task.id)}
                      >
                        <RefreshCcw size={14} />
                        重试任务
                      </button>
                    ) : null
                  }
                />
              ) : null}

              {taskFilter === "completed" ? (
                <CollapsibleTaskSection
                  eyebrow="已完成"
                  title="最近完成的任务"
                  emptyTitle="暂时还没有完成任务"
                  emptyCopy="成功完成的任务会保留在这里，方便回看最近处理好的项目。"
                  collapsedCopy={`已完成 ${summary.completed} 个任务，点击展开查看最近完成的记录。`}
                  tasks={completedTasks}
                  open={showCompletedTasks}
                  onToggle={() => setShowCompletedTasks((value) => !value)}
                />
              ) : null}
            </>
          ) : null}

          {!tasksQuery.isLoading && !tasksQuery.isError && !usesCollapsedSections && filteredTasks.length === 0 ? (
            <EmptyBlock icon={<CheckCircle2 size={20} />} title="这个筛选下没有任务" copy="切换筛选条件后，可以查看其他类型的后台活动。" />
          ) : null}

          {!tasksQuery.isLoading && !tasksQuery.isError && !usesCollapsedSections && filteredTasks.length > 0 ? (
            <>
              <div className="task-list">
                {filteredTasksPagination.pagedItems.map((task) => (
                  <article key={task.id} className="task-card sync-task-card">
                    <div className="task-card-head">
                      <div>
                        <strong>{getTaskTitle(task.taskType)}</strong>
                        <p>{task.id}</p>
                      </div>
                      <span className={`status-pill ${getTaskTone(task.status)}`}>{getTaskStatusLabel(task.status)}</span>
                    </div>

                    <div className="task-card-meta">
                      <span>创建于 {formatCatalogDate(task.createdAt)}</span>
                      {task.startedAt ? <span>开始于 {formatCatalogDate(task.startedAt)}</span> : null}
                      {task.finishedAt ? <span>结束于 {formatCatalogDate(task.finishedAt)}</span> : null}
                      <span>重试次数 {task.retryCount}</span>
                    </div>

                    {getTaskDisplaySummary(task) ? <p className="muted-copy">{getTaskDisplaySummary(task)}</p> : null}
                    {task.errorMessage ? <p className="error-copy">{task.errorMessage}</p> : null}

                    {canRetryTask(task) ? (
                      <div className="action-row">
                        <button
                          type="button"
                          className="ghost-button"
                          disabled={retryMutation.isPending}
                          onClick={() => void retryMutation.mutateAsync(task.id)}
                        >
                          <RefreshCcw size={14} />
                          重试任务
                        </button>
                      </div>
                    ) : null}
                  </article>
                ))}
              </div>
              <PaginationControls pagination={filteredTasksPagination} itemLabel="个任务" />
            </>
          ) : null}
        </article>

        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">结构化日志</p>
              <h4>后端最近输出</h4>
            </div>

            <div className="task-log-toolbar">
              <div className="compact-filter-control task-log-filter">
                <TerminalSquare size={16} aria-hidden="true" />
                <select aria-label="按日志级别筛选" value={logLevel} onChange={(event) => setLogLevel(event.target.value as SystemLogLevel)}>
                  {logLevels.map((level) => (
                    <option key={level} value={level}>
                      {level === "all" ? "全部级别" : level === "warn" ? "WARN" : level.toUpperCase()}
                    </option>
                  ))}
                </select>
              </div>

              <button type="button" className="ghost-button" onClick={() => void logsQuery.refetch()}>
                <RefreshCcw size={14} />
                刷新
              </button>
            </div>
          </div>

          {logsQuery.isLoading ? (
            <EmptyBlock icon={<LoaderCircle size={20} className="spin" />} title="正在读取日志" copy="最近的结构化日志正在从磁盘读取。" />
          ) : null}

          {logsQuery.isError ? (
            <EmptyBlock
              icon={<AlertTriangle size={20} />}
              title="暂时无法读取日志"
              copy={logsQuery.error instanceof Error ? logsQuery.error.message : "请稍后再试。"}
            />
          ) : null}

          {!logsQuery.isLoading && !logsQuery.isError && logsQuery.data ? (
            <>
              <div className="settings-note-card">
                <TerminalSquare size={18} />
                <div>
                  <strong>{logsQuery.data.logFilePath || "结构化日志文件"}</strong>
                  <p>当前展示最近 {logsQuery.data.entries.length} 条日志。</p>
                </div>
              </div>

              {logsQuery.data.entries.length === 0 ? (
                <EmptyBlock icon={<CheckCircle2 size={20} />} title="这个级别下没有日志" copy="可以放宽筛选条件，或者等待下一次后台事件。" />
              ) : (
                <>
                  <div className="task-log-list">
                    {logsPagination.pagedItems.map((entry, index) => (
                      <article key={`${entry.timestamp}-${index}`} className="task-log-card">
                        <div className="task-log-head">
                          <span className={`status-pill ${getLogTone(entry.level)}`}>{entry.level || "info"}</span>
                          <strong>{entry.message || "日志事件"}</strong>
                        </div>

                        <div className="task-card-meta">
                          <span>{formatCatalogDate(entry.timestamp)}</span>
                        </div>

                        {entry.attributes && Object.keys(entry.attributes).length > 0 ? (
                          <pre className="task-log-pre">{JSON.stringify(entry.attributes, null, 2)}</pre>
                        ) : null}
                      </article>
                    ))}
                  </div>
                  <PaginationControls pagination={logsPagination} itemLabel="条日志" />
                </>
              )}
            </>
          ) : null}
        </article>
      </div>
    </section>
  );
}

function CollapsibleTaskSection({
  eyebrow,
  title,
  tasks,
  emptyTitle,
  emptyCopy,
  collapsedCopy,
  open,
  onToggle,
  renderActions
}: {
  eyebrow: string;
  title: string;
  tasks: CatalogTask[];
  emptyTitle: string;
  emptyCopy: string;
  collapsedCopy: string;
  open: boolean;
  onToggle: () => void;
  renderActions?: (task: CatalogTask) => ReactNode;
}) {
  const tasksPagination = usePagination(tasks, 10);

  return (
    <section className="task-drawer-section">
      <div className="section-head task-section-collapsible-head">
        <div>
          <p className="eyebrow">{eyebrow}</p>
          <h4>{title}</h4>
        </div>

        <button type="button" className="ghost-button" aria-expanded={open} onClick={onToggle}>
          {open ? "收起" : "展开"}
        </button>
      </div>

      {tasks.length === 0 ? (
        <EmptyBlock icon={<CheckCircle2 size={20} />} title={emptyTitle} copy={emptyCopy} />
      ) : open ? (
        <>
          <div className="task-list">
            {tasksPagination.pagedItems.map((task) => (
              <article key={task.id} className="task-card sync-task-card">
                <div className="task-card-head">
                  <div>
                    <strong>{getTaskTitle(task.taskType)}</strong>
                    <p>{task.id}</p>
                  </div>
                  <span className={`status-pill ${getTaskTone(task.status)}`}>{getTaskStatusLabel(task.status)}</span>
                </div>

                <div className="task-card-meta">
                  <span>创建于 {formatCatalogDate(task.createdAt)}</span>
                  {task.startedAt ? <span>开始于 {formatCatalogDate(task.startedAt)}</span> : null}
                  {task.finishedAt ? <span>结束于 {formatCatalogDate(task.finishedAt)}</span> : null}
                </div>

                {getTaskDisplaySummary(task) ? <p className="muted-copy">{getTaskDisplaySummary(task)}</p> : null}
                {task.errorMessage ? <p className="error-copy">{task.errorMessage}</p> : null}
                {renderActions ? <div className="action-row">{renderActions(task)}</div> : null}
              </article>
            ))}
          </div>
          <PaginationControls pagination={tasksPagination} itemLabel="个任务" />
        </>
      ) : (
        <div className="task-section-collapsed-note">
          <p>{collapsedCopy}</p>
        </div>
      )}
    </section>
  );
}

function MetricCard({
  label,
  value,
  tone
}: {
  label: string;
  value: number;
  tone: "success" | "warning" | "danger" | "neutral";
}) {
  return (
    <article className={`metric-card tone-${tone}`}>
      <p>{label}</p>
      <strong>{value}</strong>
    </article>
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

function getLogTone(level: string) {
  switch (level.trim().toLowerCase()) {
    case "error":
      return "danger";
    case "warn":
    case "warning":
      return "warning";
    case "info":
      return "success";
    default:
      return "subtle";
  }
}
