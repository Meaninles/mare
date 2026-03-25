import { useMemo, useState, type ReactNode } from "react";
import { AlertTriangle, BellRing, CheckCircle2, HardDrive, LoaderCircle, RefreshCcw, Upload, X } from "lucide-react";
import { Link, useNavigate } from "react-router-dom";
import { useLibraryContext } from "../context/LibraryContext";
import { useCatalogRetryTask, useCatalogTasks } from "../hooks/useCatalog";
import { useImportDevices, useSelectImportDeviceRole } from "../hooks/useImport";
import { useRemovableNoticeState } from "../hooks/useRemovableNoticeState";
import { formatCatalogDate } from "../lib/catalog-view";
import {
  canRetryTask,
  getTaskDisplaySummary,
  getTaskStatusLabel,
  getTaskSummary,
  getTaskTitle,
  getTaskTone,
  getVisibleTasks,
  matchesTaskFilter
} from "../lib/task-center";
import type { CatalogTask } from "../types/catalog";
import type { ImportDeviceRole, ImportDeviceRecord } from "../types/import";

export function TaskCenterDrawer({ open, onClose }: { open: boolean; onClose: () => void }) {
  const navigate = useNavigate();
  const { currentLibraryId } = useLibraryContext();
  const [showRunningTasks, setShowRunningTasks] = useState(false);
  const [showFailedTasks, setShowFailedTasks] = useState(false);
  const [showCompletedTasks, setShowCompletedTasks] = useState(false);
  const tasksQuery = useCatalogTasks(500);
  const devicesQuery = useImportDevices();
  const retryMutation = useCatalogRetryTask();
  const selectRoleMutation = useSelectImportDeviceRole();
  const tasks = useMemo(() => getVisibleTasks(tasksQuery.data ?? []), [tasksQuery.data]);
  const devices = devicesQuery.data ?? [];
  const summary = getTaskSummary(tasks);
  const removableNotices = useRemovableNoticeState(devices, currentLibraryId);

  const runningTasks = useMemo(() => tasks.filter((task) => matchesTaskFilter(task, "running")), [tasks]);
  const failedTasks = useMemo(() => tasks.filter((task) => matchesTaskFilter(task, "failed")), [tasks]);
  const completedTasks = useMemo(
    () => tasks.filter((task) => task.status.trim().toLowerCase() === "success").slice(0, 12),
    [tasks]
  );

  if (!open) {
    return null;
  }

  async function handleSelectRole(device: ImportDeviceRecord, role: ImportDeviceRole) {
    const result = await selectRoleMutation.mutateAsync({
      identitySignature: device.identitySignature,
      role,
      name: device.knownEndpoint?.name ?? device.device.volumeLabel ?? "移动设备"
    });

    removableNotices.markAsRead(device.identitySignature);
    onClose();

    if (role === "import_source") {
      navigate(`/ingest?device=${encodeURIComponent(result.device.identitySignature)}`);
      return;
    }

    navigate("/storage");
  }

  return (
    <div className="task-drawer-overlay" role="presentation" onClick={onClose}>
      <aside
        className="task-drawer"
        role="dialog"
        aria-modal="true"
        aria-labelledby="task-center-title"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="task-drawer-head">
          <div>
            <p className="eyebrow">通知</p>
            <h3 id="task-center-title">任务中心</h3>
          </div>

          <button type="button" className="ghost-button icon-button" onClick={onClose} aria-label="关闭任务中心">
            <X size={18} />
          </button>
        </div>

        <div className="task-summary-grid">
          <SummaryCard label="设备提醒" value={removableNotices.unreadCount} tone="warning" />
          <SummaryCard label="失败任务" value={summary.failed} tone="danger" />
          <SummaryCard label="进行中" value={summary.running} tone="warning" />
        </div>

        <div className="task-drawer-body">
          <section className="task-drawer-section">
            <div className="section-head">
              <div>
                <p className="eyebrow">设备提醒</p>
                <h4>可移动设备</h4>
              </div>
            </div>

            {devicesQuery.isLoading ? (
              <div className="sync-empty-block">
                <LoaderCircle size={20} className="spin" />
                <div>
                  <strong>正在检查可移动设备</strong>
                  <p>已连接但尚未分配用途的设备会显示在这里。</p>
                </div>
              </div>
            ) : null}

            {devicesQuery.isError ? (
              <div className="sync-empty-block">
                <AlertTriangle size={20} />
                <div>
                  <strong>暂时无法读取设备提醒</strong>
                  <p>{devicesQuery.error instanceof Error ? devicesQuery.error.message : "请稍后再试。"}</p>
                </div>
              </div>
            ) : null}

            {!devicesQuery.isLoading && !devicesQuery.isError && removableNotices.unreadDevices.length === 0 ? (
              <div className="sync-empty-block">
                <BellRing size={20} />
                <div>
                  <strong>当前没有新的设备提醒</strong>
                  <p>已经看过的设备不会在每次刷新后反复打断你。</p>
                </div>
              </div>
            ) : null}

            {!devicesQuery.isLoading && !devicesQuery.isError && removableNotices.unreadDevices.length > 0 ? (
              <div className="task-list">
                {removableNotices.unreadDevices.map((device) => (
                  <article key={device.identitySignature} className="task-card sync-task-card">
                    <div className="task-card-head">
                      <div>
                        <strong>{device.device.volumeLabel || "未命名设备"}</strong>
                        <p>{device.device.mountPoint}</p>
                      </div>
                      <span className="status-pill warning">待处理</span>
                    </div>

                    <div className="task-card-meta">
                      <span>{[device.device.fileSystem, device.device.model || device.device.interfaceType].filter(Boolean).join(" / ") || "可移动存储"}</span>
                    </div>

                    <p className="muted-copy">
                      这台设备已经连接，但还没有在本次会话里确定用途。你可以直接标记已读，也可以指定为导入源或管理存储。
                    </p>

                    <div className="action-row">
                      <button
                        type="button"
                        className="ghost-button"
                        disabled={selectRoleMutation.isPending}
                        onClick={() => void handleSelectRole(device, "import_source")}
                      >
                        <Upload size={14} />
                        设为导入源
                      </button>

                      <button
                        type="button"
                        className="ghost-button"
                        disabled={selectRoleMutation.isPending}
                        onClick={() => void handleSelectRole(device, "managed_storage")}
                      >
                        <HardDrive size={14} />
                        设为管理存储
                      </button>

                      <button
                        type="button"
                        className="ghost-button"
                        disabled={selectRoleMutation.isPending}
                        onClick={() => removableNotices.markAsRead(device.identitySignature)}
                      >
                        已读
                      </button>
                    </div>
                  </article>
                ))}
              </div>
            ) : null}
          </section>

          <CollapsibleTaskSection
            title="进行中的任务"
            emptyTitle="当前没有活动任务"
            emptyCopy="等待中、进行中和重试中的任务会显示在这里。"
            collapsedCopy={`当前有 ${summary.running} 个进行中的任务，点击展开查看详情。`}
            tasks={runningTasks}
            open={showRunningTasks}
            onToggle={() => setShowRunningTasks((value) => !value)}
          />

          <CollapsibleTaskSection
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
                  重试
                </button>
              ) : null
            }
          />

          <CollapsibleTaskSection
            title="最近完成"
            emptyTitle="暂时还没有完成任务"
            emptyCopy="成功完成的任务会保留在这里，方便你回看刚刚已经处理好的项目。"
            collapsedCopy={`已完成 ${summary.completed} 个任务，点击展开查看最近完成的记录。`}
            tasks={completedTasks}
            open={showCompletedTasks}
            onToggle={() => setShowCompletedTasks((value) => !value)}
          />
        </div>

        <div className="task-drawer-footer">
          <Link to="/system-tasks" className="ghost-button" onClick={onClose}>
            打开系统任务
          </Link>
          <Link to="/sync" className="primary-button" onClick={onClose}>
            打开同步中心
          </Link>
        </div>
      </aside>
    </div>
  );
}

function CollapsibleTaskSection({
  title,
  emptyTitle,
  emptyCopy,
  collapsedCopy,
  tasks,
  open,
  onToggle,
  renderActions
}: {
  title: string;
  emptyTitle: string;
  emptyCopy: string;
  collapsedCopy: string;
  tasks: CatalogTask[];
  open: boolean;
  onToggle: () => void;
  renderActions?: (task: CatalogTask) => ReactNode;
}) {
  return (
    <section className="task-drawer-section">
      <div className="section-head task-section-collapsible-head">
        <div>
          <p className="eyebrow">任务</p>
          <h4>{title}</h4>
        </div>

        <button type="button" className="ghost-button" aria-expanded={open} onClick={onToggle}>
          {open ? "收起" : "展开"}
        </button>
      </div>

      {tasks.length === 0 ? (
        <div className="sync-empty-block">
          <CheckCircle2 size={20} />
          <div>
            <strong>{emptyTitle}</strong>
            <p>{emptyCopy}</p>
          </div>
        </div>
      ) : open ? (
        <div className="task-list">
          {tasks.map((task) => (
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

              {task.errorMessage ? <p className="error-copy">{task.errorMessage}</p> : null}
              {getTaskDisplaySummary(task) ? <p className="muted-copy clamp-2">{getTaskDisplaySummary(task)}</p> : null}
              {renderActions ? <div className="action-row">{renderActions(task)}</div> : null}
            </article>
          ))}
        </div>
      ) : (
        <div className="task-section-collapsed-note">
          <p>{collapsedCopy}</p>
        </div>
      )}
    </section>
  );
}

function SummaryCard({
  label,
  value,
  tone
}: {
  label: string;
  value: number;
  tone: "success" | "warning" | "danger";
}) {
  return (
    <article className={`metric-card tone-${tone}`}>
      <p>{label}</p>
      <strong>{value}</strong>
    </article>
  );
}
