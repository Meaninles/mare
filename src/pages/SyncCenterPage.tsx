import { useMemo, useState, type ReactNode } from "react";
import { AlertTriangle, LoaderCircle, Pause, Play, RefreshCcw, Search, Square } from "lucide-react";
import { useCD2TransferAction, useCD2Transfers } from "../hooks/useCD2";
import { formatCatalogDate, formatFileSize } from "../lib/catalog-view";

type Filter = "all" | "running" | "queued" | "paused" | "failed" | "success";

export function SyncCenterPage() {
  const transfersQuery = useCD2Transfers();
  const actionMutation = useCD2TransferAction();
  const [filter, setFilter] = useState<Filter>("running");
  const [searchValue, setSearchValue] = useState("");
  const [notice, setNotice] = useState<string | null>(null);

  const result = transfersQuery.data;
  const tasks = result?.tasks ?? [];
  const filteredTasks = useMemo(() => {
    const query = searchValue.trim().toLowerCase();
    return tasks.filter((task) => {
      if (filter !== "all" && task.status !== filter) {
        return false;
      }
      if (!query) {
        return true;
      }
      return [task.title, task.sourcePath, task.targetPath, task.filePath, task.key].filter(Boolean).join(" ").toLowerCase().includes(query);
    });
  }, [filter, searchValue, tasks]);

  async function handleAction(task: (typeof tasks)[number], action: "pause" | "resume" | "cancel") {
    try {
      const summary = await actionMutation.mutateAsync({
        kind: task.kind as "upload" | "download" | "copy",
        action,
        keys: task.kind === "upload" ? [task.controlReference || task.key] : undefined,
        sourcePath: task.kind === "copy" ? task.sourcePath : undefined,
        destPath: task.kind === "copy" ? task.targetPath : undefined
      });
      setNotice(summary.message || `已执行 ${action}。`);
      await transfersQuery.refetch();
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "任务操作失败。");
    }
  }

  return (
    <section className="page-stack">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">同步中心</p>
          <h3>正式任务中心现在直接显示 CD2 上传、下载和复制任务。</h3>
          <p>这里展示的是任务 7 接入后的 CD2 实时任务与最近事件，不再优先依赖旧的 catalog 传输队列。</p>
        </div>
        <div className="hero-metrics transfer-summary-grid">
          <MetricCard label="总任务" value={String(result?.stats.totalTasks ?? 0)} />
          <MetricCard label="运行中" value={String(result?.stats.runningTasks ?? 0)} />
          <MetricCard label="上传" value={String(result?.stats.uploadTasks ?? 0)} />
          <MetricCard label="下载/复制" value={String((result?.stats.downloadTasks ?? 0) + (result?.stats.copyTasks ?? 0))} />
        </div>
      </article>

      {notice ? <p className="inline-note">{notice}</p> : null}

      <article className="detail-card">
        <div className="section-head">
          <div><p className="eyebrow">CD2 任务</p><h4>当前传输与桥接状态</h4></div>
          <div className="action-row">
            <label className="transfer-search" aria-label="搜索任务">
              <Search size={16} />
              <input value={searchValue} onChange={(event) => setSearchValue(event.target.value)} placeholder="按标题、路径或任务键搜索" />
            </label>
            <select value={filter} onChange={(event) => setFilter(event.target.value as Filter)}>
              <option value="running">运行中</option>
              <option value="all">全部</option>
              <option value="queued">排队中</option>
              <option value="paused">已暂停</option>
              <option value="failed">失败</option>
              <option value="success">成功</option>
            </select>
            <button type="button" className="ghost-button" onClick={() => void transfersQuery.refetch()} disabled={transfersQuery.isFetching}>
              {transfersQuery.isFetching ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
              刷新
            </button>
          </div>
        </div>

        {transfersQuery.isLoading ? <EmptyState icon={<LoaderCircle size={20} className="spin" />} title="正在读取 CD2 任务" copy="页面会自动轮询当前 Docker CD2 的任务状态。" /> : null}
        {transfersQuery.isError ? <EmptyState icon={<AlertTriangle size={20} />} title="暂时无法读取任务" copy={transfersQuery.error instanceof Error ? transfersQuery.error.message : "请稍后再试。"} /> : null}

        {!transfersQuery.isLoading && !transfersQuery.isError && filteredTasks.length === 0 ? (
          <EmptyState icon={<RefreshCcw size={20} />} title="当前筛选下没有任务" copy="先在存储管理或测试页执行一次上传、下载或复制，再回这里观察 CD2 任务。" />
        ) : null}

        {filteredTasks.length > 0 ? (
          <div className="endpoint-grid">
            {filteredTasks.map((task) => (
              <article key={task.key} className="endpoint-panel">
                <div className="endpoint-panel-head">
                  <div>
                    <strong>{task.title}</strong>
                    <p>{task.key}</p>
                  </div>
                  <span className={`status-pill ${task.status === "running" ? "success" : task.status === "failed" ? "warning" : "subtle"}`}>{task.status}</span>
                </div>

                <div className="scan-summary-grid">
                  <InfoCell label="类型" value={kindLabel(task.kind)} />
                  <InfoCell label="进度" value={`${task.progressPercent}%`} />
                  <InfoCell label="速度" value={task.bytesPerSecond > 0 ? `${formatFileSize(task.bytesPerSecond)}/s` : "-"} />
                  <InfoCell label="大小" value={task.totalBytes > 0 ? `${formatFileSize(task.finishedBytes)} / ${formatFileSize(task.totalBytes)}` : "-"} />
                </div>

                <p>{task.sourcePath || task.filePath || "-"}</p>
                <small>{task.targetPath || "-"}</small>
                {task.errorMessage ? <p className="error-copy">{task.errorMessage}</p> : null}
                <div className="task-card-meta">
                  <span>最近观察于 {formatCatalogDate(task.lastObservedAt)}</span>
                  {task.finishedAt ? <span>完成于 {formatCatalogDate(task.finishedAt)}</span> : null}
                </div>

                <div className="endpoint-panel-actions">
                  {task.canPause ? <button type="button" className="ghost-button" onClick={() => void handleAction(task, "pause")}><Pause size={15} />暂停</button> : null}
                  {task.canResume ? <button type="button" className="ghost-button" onClick={() => void handleAction(task, "resume")}><Play size={15} />恢复</button> : null}
                  {task.canCancel ? <button type="button" className="danger-button" onClick={() => void handleAction(task, "cancel")}><Square size={15} />取消</button> : null}
                </div>
              </article>
            ))}
          </div>
        ) : null}

        {result ? (
          <label className="field">
            <span>最近推送事件</span>
            <textarea value={JSON.stringify(result.recentEvents, null, 2)} readOnly />
          </label>
        ) : null}
      </article>
    </section>
  );
}

function MetricCard({ label, value }: { label: string; value: string }) {
  return <div className="metric-card neutral"><span>{label}</span><strong>{value}</strong></div>;
}

function InfoCell({ label, value }: { label: string; value: string }) {
  return <div className="field"><span>{label}</span><strong>{value}</strong></div>;
}

function EmptyState({ icon, title, copy }: { icon: ReactNode; title: string; copy: string }) {
  return <div className="sync-empty-block">{icon}<div><strong>{title}</strong><p>{copy}</p></div></div>;
}

function kindLabel(value: string) {
  switch (value) {
    case "upload":
      return "上传";
    case "download":
      return "下载";
    case "copy":
      return "复制";
    default:
      return value;
  }
}
