import type { ReactNode } from "react";
import { AlertTriangle, BellRing, CheckCircle2, HardDrive, LoaderCircle, RefreshCcw, Upload, X } from "lucide-react";
import { Link, useNavigate } from "react-router-dom";
import { useLibraryContext } from "../context/LibraryContext";
import { useCatalogRetryTask, useCatalogTasks } from "../hooks/useCatalog";
import { useImportDevices, useSelectImportDeviceRole } from "../hooks/useImport";
import { useRemovableNoticeState } from "../hooks/useRemovableNoticeState";
import { formatCatalogDate } from "../lib/catalog-view";
import { canRetryTask, getTaskStatusLabel, getTaskSummary, getTaskTitle, getTaskTone, matchesTaskFilter } from "../lib/task-center";
import type { CatalogTask } from "../types/catalog";
import type { ImportDeviceRole, ImportDeviceRecord } from "../types/import";

export function TaskCenterDrawer({ open, onClose }: { open: boolean; onClose: () => void }) {
  const navigate = useNavigate();
  const { currentLibraryId } = useLibraryContext();
  const tasksQuery = useCatalogTasks(24);
  const devicesQuery = useImportDevices();
  const retryMutation = useCatalogRetryTask();
  const selectRoleMutation = useSelectImportDeviceRole();
  const tasks = tasksQuery.data ?? [];
  const devices = devicesQuery.data ?? [];
  const summary = getTaskSummary(tasks);
  const removableNotices = useRemovableNoticeState(devices, currentLibraryId);

  const runningTasks = tasks.filter((task) => matchesTaskFilter(task, "running"));
  const failedTasks = tasks.filter((task) => matchesTaskFilter(task, "failed"));
  const completedTasks = tasks.filter((task) => task.status.trim().toLowerCase() === "success").slice(0, 6);

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
            <h3 id="task-center-title">通知中心</h3>
          </div>

          <button type="button" className="ghost-button icon-button" onClick={onClose} aria-label="关闭通知中心">
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
                  <p>已连接但尚未分配用途的设备会出现在这里。</p>
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
                  <p>已阅读过的设备不会再在每次刷新后弹出打断你。</p>
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
                      这台设备已经连接，但还没有在本次会话中确定用途。你可以直接标记已读，也可以指定为导入源或管理存储。
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

          <TaskSection
            title="进行中的任务"
            emptyTitle="当前没有活动任务"
            emptyCopy="等待中、进行中和重试中的任务会显示在这里。"
            tasks={runningTasks}
          />

          <TaskSection
            title="失败任务"
            emptyTitle="当前没有失败任务"
            emptyCopy="扫描、恢复、导入或媒体任务失败后，可以直接在这里重试。"
            tasks={failedTasks}
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

          <TaskSection
            title="最近完成"
            emptyTitle="暂时还没有完成任务"
            emptyCopy="成功完成的任务会短暂保留在这里，方便快速确认。"
            tasks={completedTasks}
          />
        </div>

        <div className="task-drawer-footer">
          <Link to="/system-tasks" className="ghost-button" onClick={onClose}>
            打开传输任务
          </Link>
          <Link to="/sync" className="primary-button" onClick={onClose}>
            打开同步中心
          </Link>
        </div>
      </aside>
    </div>
  );
}

function TaskSection({
  title,
  emptyTitle,
  emptyCopy,
  tasks,
  renderActions
}: {
  title: string;
  emptyTitle: string;
  emptyCopy: string;
  tasks: CatalogTask[];
  renderActions?: (task: CatalogTask) => ReactNode;
}) {
  return (
    <section className="task-drawer-section">
      <div className="section-head">
        <div>
          <p className="eyebrow">任务</p>
          <h4>{title}</h4>
        </div>
      </div>

      {tasks.length === 0 ? (
        <div className="sync-empty-block">
          <CheckCircle2 size={20} />
          <div>
            <strong>{emptyTitle}</strong>
            <p>{emptyCopy}</p>
          </div>
        </div>
      ) : (
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
              {task.resultSummary ? <p className="muted-copy clamp-2">{task.resultSummary}</p> : null}

              {renderActions ? <div className="action-row">{renderActions(task)}</div> : null}
            </article>
          ))}
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
