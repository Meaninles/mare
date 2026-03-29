import { LoaderCircle, Pause, Play, RefreshCcw, Square, Wifi } from "lucide-react";
import { useMemo, useState } from "react";
import { useCD2TransferAction, useCD2Transfers } from "../../hooks/useCD2";
import { formatCatalogDate } from "../../lib/catalog-view";
import type { CD2TransferTask } from "../../types/cd2";

export function CD2TransfersTestPanel() {
  const transfersQuery = useCD2Transfers();
  const actionMutation = useCD2TransferAction();
  const [notice, setNotice] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selectedTaskKey, setSelectedTaskKey] = useState<string | null>(null);

  const result = transfersQuery.data;
  const tasks = result?.tasks ?? [];
  const selectedTask = useMemo(
    () => tasks.find((item) => item.key === selectedTaskKey) ?? null,
    [selectedTaskKey, tasks]
  );

  async function handleAction(task: CD2TransferTask, action: "pause" | "resume" | "cancel") {
    setNotice(null);
    setError(null);

    try {
      if (task.kind === "copy") {
        await actionMutation.mutateAsync({
          kind: "copy",
          action,
          sourcePath: task.sourcePath,
          destPath: task.targetPath
        });
      } else if (task.kind === "upload") {
        await actionMutation.mutateAsync({
          kind: "upload",
          action,
          keys: [task.controlReference || task.key]
        });
      } else {
        setError("下载任务当前只支持观察，不支持控制。");
        return;
      }
      setNotice(`已对任务 ${task.title} 执行${actionLabel(action)}。`);
      await transfersQuery.refetch();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "执行任务控制失败。");
    }
  }

  return (
    <article className="detail-card">
      <div className="section-head">
        <div>
          <p className="eyebrow">任务 7 / 传输任务桥接测试</p>
          <h4>CD2 传输任务与实时事件</h4>
        </div>

        <button type="button" className="ghost-button" onClick={() => void transfersQuery.refetch()} disabled={transfersQuery.isFetching}>
          {transfersQuery.isFetching ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
          刷新任务
        </button>
      </div>

      {notice ? <p className="inline-note">{notice}</p> : null}
      {error ? <p className="error-copy">{error}</p> : null}
      {transfersQuery.error instanceof Error ? <p className="error-copy">{transfersQuery.error.message}</p> : null}

      <div className="settings-note-card">
        <Wifi size={18} />
        <div>
          <strong>这块会显示 CD2 当前上传、下载、复制任务，以及 PushTaskChange / PushMessage 的最近事件。</strong>
          <p>最容易验证的是“复制任务”：先在上面的文件操作面板执行一次复制，这里通常就会出现一条 copy 任务。</p>
        </div>
      </div>

      {result ? (
        <>
          <div className="scan-summary-grid">
            <SummaryCell label="总任务数" value={String(result.stats.totalTasks)} />
            <SummaryCell label="运行中" value={String(result.stats.runningTasks)} />
            <SummaryCell label="已暂停" value={String(result.stats.pausedTasks)} />
            <SummaryCell label="失败数" value={String(result.stats.failedTasks)} />
            <SummaryCell label="上传任务" value={String(result.stats.uploadTasks)} />
            <SummaryCell label="下载任务" value={String(result.stats.downloadTasks)} />
            <SummaryCell label="复制任务" value={String(result.stats.copyTasks)} />
            <SummaryCell label="监听事件数" value={String(result.watcher.eventCount)} />
          </div>

          <div className="scan-summary-grid">
            <SummaryCell label="PushTaskChange" value={result.watcher.pushTaskChangeActive ? "已连接" : "未连接"} />
            <SummaryCell label="PushMessage" value={result.watcher.pushMessageActive ? "已连接" : "未连接"} />
            <SummaryCell label="最近连接" value={result.watcher.lastConnectedAt ? formatCatalogDate(result.watcher.lastConnectedAt) : "-"} />
            <SummaryCell label="最近事件" value={result.watcher.lastEventAt ? formatCatalogDate(result.watcher.lastEventAt) : "-"} />
          </div>

          {result.watcher.lastError ? <p className="error-copy">监听错误：{result.watcher.lastError}</p> : null}

          {tasks.length === 0 ? (
            <div className="sync-empty-block">
              <RefreshCcw size={18} />
              <div>
                <strong>当前没有传输任务</strong>
                <p>可以先在任务 6 的面板里执行一次复制，或者等后续上传能力接入后在这里观察上传任务。</p>
              </div>
            </div>
          ) : (
            <div className="endpoint-grid">
              {tasks.map((task) => (
                <article
                  key={task.key}
                  className="endpoint-panel"
                  onClick={() => setSelectedTaskKey(task.key)}
                  style={{ cursor: "pointer", outline: selectedTaskKey === task.key ? "1px solid rgba(91, 141, 239, 0.6)" : "none" }}
                >
                  <div className="endpoint-panel-head">
                    <div>
                      <strong>{task.title}</strong>
                      <p>{task.kind} / {task.key}</p>
                    </div>
                    <span className={`status-pill ${statusTone(task.status)}`}>{statusLabel(task.status)}</span>
                  </div>

                  <div className="endpoint-panel-meta">
                    <div>
                      <span>进度</span>
                      <strong>{task.progressPercent}%</strong>
                    </div>
                    <div>
                      <span>速率</span>
                      <strong>{task.bytesPerSecond || 0}</strong>
                    </div>
                    <div>
                      <span>源路径</span>
                      <strong>{task.sourcePath || "-"}</strong>
                    </div>
                    <div>
                      <span>目标路径</span>
                      <strong>{task.targetPath || task.filePath || "-"}</strong>
                    </div>
                  </div>

                  <div className="endpoint-panel-actions">
                    {task.canPause ? (
                      <button type="button" className="ghost-button" onClick={(event) => {
                        event.stopPropagation();
                        void handleAction(task, "pause");
                      }}>
                        <Pause size={16} />
                        暂停
                      </button>
                    ) : null}

                    {task.canResume ? (
                      <button type="button" className="ghost-button" onClick={(event) => {
                        event.stopPropagation();
                        void handleAction(task, "resume");
                      }}>
                        <Play size={16} />
                        恢复
                      </button>
                    ) : null}

                    {task.canCancel ? (
                      <button type="button" className="danger-button" onClick={(event) => {
                        event.stopPropagation();
                        void handleAction(task, "cancel");
                      }}>
                        <Square size={16} />
                        取消
                      </button>
                    ) : null}
                  </div>
                </article>
              ))}
            </div>
          )}

          {selectedTask ? (
            <label className="field">
              <span>当前选中任务</span>
              <textarea value={JSON.stringify(selectedTask, null, 2)} readOnly />
            </label>
          ) : null}

          <label className="field">
            <span>最近推送事件</span>
            <textarea value={JSON.stringify(result.recentEvents, null, 2)} readOnly />
          </label>
        </>
      ) : (
        <div className="sync-empty-block">
          <LoaderCircle size={18} className="spin" />
          <div>
            <strong>正在连接 CD2 传输任务服务</strong>
            <p>页面会自动轮询任务列表，并在后台持续监听 CD2 的推送消息。</p>
          </div>
        </div>
      )}
    </article>
  );
}

function SummaryCell({ label, value }: { label: string; value: string }) {
  return (
    <div className="field">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function actionLabel(action: "pause" | "resume" | "cancel") {
  switch (action) {
    case "pause":
      return "暂停";
    case "resume":
      return "恢复";
    case "cancel":
      return "取消";
    default:
      return action;
  }
}

function statusLabel(status: string) {
  switch (status) {
    case "queued":
      return "排队中";
    case "running":
      return "运行中";
    case "paused":
      return "已暂停";
    case "failed":
      return "失败";
    case "success":
      return "成功";
    case "canceled":
      return "已取消";
    case "skipped":
      return "已跳过";
    default:
      return status;
  }
}

function statusTone(status: string) {
  switch (status) {
    case "running":
      return "success";
    case "paused":
      return "warning";
    case "failed":
    case "canceled":
      return "warning";
    default:
      return "subtle";
  }
}
