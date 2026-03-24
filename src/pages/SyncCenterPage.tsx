import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import {
  AlertTriangle,
  ArrowRight,
  AudioLines,
  CheckCircle2,
  Clapperboard,
  Images,
  LoaderCircle,
  RefreshCcw,
  ShieldAlert,
  WandSparkles
} from "lucide-react";
import { formatCatalogDate, getMediaTypeLabel, normalizeMediaType } from "../lib/catalog-view";
import {
  useCatalogBatchRestore,
  useCatalogRestoreAsset,
  useCatalogRetrySyncTask,
  useCatalogSyncOverview
} from "../hooks/useCatalog";
import type {
  CatalogSyncAsset,
  CatalogSyncEndpointRef,
  CatalogTask
} from "../types/catalog";

export function SyncCenterPage() {
  const syncOverviewQuery = useCatalogSyncOverview();
  const restoreMutation = useCatalogRestoreAsset();
  const batchRestoreMutation = useCatalogBatchRestore();
  const retryMutation = useCatalogRetrySyncTask();
  const [notice, setNotice] = useState<string | null>(null);
  const [selectedTargetId, setSelectedTargetId] = useState("");
  const [selectedAssetIds, setSelectedAssetIds] = useState<string[]>([]);

  const overview = syncOverviewQuery.data;
  const recoverableAssets = overview?.recoverableAssets ?? [];
  const conflictAssets = overview?.conflictAssets ?? [];
  const runningTasks = overview?.runningTasks ?? [];
  const failedTasks = overview?.failedTasks ?? [];

  const batchTargets = useMemo(() => {
    const groups = new Map<string, { endpoint: CatalogSyncEndpointRef; assets: CatalogSyncAsset[] }>();

    recoverableAssets.forEach((asset) => {
      asset.missingEndpoints.forEach((endpoint) => {
        const existing = groups.get(endpoint.id);
        if (existing) {
          existing.assets.push(asset);
          return;
        }

        groups.set(endpoint.id, {
          endpoint,
          assets: [asset]
        });
      });
    });

    return Array.from(groups.values()).sort((left, right) =>
      left.endpoint.name.localeCompare(right.endpoint.name)
    );
  }, [recoverableAssets]);

  const selectedBatchGroup = useMemo(
    () => batchTargets.find((group) => group.endpoint.id === selectedTargetId) ?? null,
    [batchTargets, selectedTargetId]
  );

  const selectedCount = useMemo(() => {
    if (!selectedBatchGroup) {
      return 0;
    }

    const validAssetIds = new Set(selectedBatchGroup.assets.map((asset) => asset.id));
    return selectedAssetIds.filter((assetId) => validAssetIds.has(assetId)).length;
  }, [selectedAssetIds, selectedBatchGroup]);

  useEffect(() => {
    if (!batchTargets.length) {
      setSelectedTargetId("");
      setSelectedAssetIds([]);
      return;
    }

    setSelectedTargetId((current) =>
      batchTargets.some((group) => group.endpoint.id === current) ? current : batchTargets[0].endpoint.id
    );
  }, [batchTargets]);

  useEffect(() => {
    if (!selectedBatchGroup) {
      setSelectedAssetIds([]);
      return;
    }

    const validAssetIds = new Set(selectedBatchGroup.assets.map((asset) => asset.id));
    setSelectedAssetIds((current) => {
      const next = current.filter((assetId) => validAssetIds.has(assetId));
      return next.length > 0 ? next : Array.from(validAssetIds);
    });
  }, [selectedBatchGroup?.endpoint.id, selectedBatchGroup?.assets.length]);

  async function handleRestore(asset: CatalogSyncAsset, targetEndpoint: CatalogSyncEndpointRef) {
    if (!asset.recommendedSource) {
      setNotice("这个资产当前没有可用的健康源副本。");
      return;
    }

    setNotice(null);
    try {
      const summary = await restoreMutation.mutateAsync({
        assetId: asset.id,
        sourceEndpointId: asset.recommendedSource.id,
        targetEndpointId: targetEndpoint.id
      });

      setNotice(
        summary.skipped
          ? `${asset.displayName} 在 ${targetEndpoint.name} 上已经是最新状态。`
          : `${asset.displayName} 已恢复到 ${targetEndpoint.name}。`
      );
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "恢复失败。");
    }
  }

  async function handleBatchRestore() {
    if (!selectedBatchGroup) {
      setNotice("请先选择至少一个可恢复资产。");
      return;
    }

    const validAssetIds = new Set(selectedBatchGroup.assets.map((asset) => asset.id));
    const batchAssetIds = selectedAssetIds.filter((assetId) => validAssetIds.has(assetId));

    if (batchAssetIds.length === 0) {
      setNotice("请先选择至少一个可恢复资产。");
      return;
    }

    setNotice(null);
    try {
      const summary = await batchRestoreMutation.mutateAsync({
        targetEndpointId: selectedBatchGroup.endpoint.id,
        assetIds: batchAssetIds
      });

      setNotice(
        `${selectedBatchGroup.endpoint.name}：成功恢复 ${summary.successCount} 个，跳过 ${summary.skippedCount} 个，失败 ${summary.failedCount} 个。`
      );
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "批量补齐失败。");
    }
  }

  async function handleRetry(taskId: string) {
    setNotice(null);
    try {
      const summary = await retryMutation.mutateAsync(taskId);
      setNotice(summary.message);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "重试失败。");
    }
  }

  function toggleAssetSelection(assetId: string) {
    setSelectedAssetIds((current) =>
      current.includes(assetId) ? current.filter((id) => id !== assetId) : [...current, assetId]
    );
  }

  function selectAllForTarget() {
    setSelectedAssetIds(selectedBatchGroup?.assets.map((asset) => asset.id) ?? []);
  }

  return (
    <section className="page-stack">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">同步中心</p>
          <h3>集中查看哪些资产可恢复、哪些副本缺失，以及哪些任务需要再次处理。</h3>
          <p>恢复能力始终放在核心位置，失败与冲突紧挨着展示，但界面不会变成运维后台。</p>
        </div>

        <div className="hero-metrics">
          <MetricCard label="可恢复" value={recoverableAssets.length} tone="warning" />
          <MetricCard label="冲突候选" value={conflictAssets.length} tone="neutral" />
          <MetricCard label="进行中" value={runningTasks.length} tone="success" />
          <MetricCard label="失败" value={failedTasks.length} tone="danger" />
        </div>
      </article>

      {notice ? <p className="inline-note">{notice}</p> : null}

      {syncOverviewQuery.isLoading ? (
        <article className="detail-card empty-state">
          <LoaderCircle size={24} className="spin" />
          <div>
            <h4>正在加载同步概览</h4>
            <p>客户端正在汇总可恢复资产、冲突候选与最近的同步活动。</p>
          </div>
        </article>
      ) : null}

      {syncOverviewQuery.isError ? (
        <article className="detail-card empty-state">
          <AlertTriangle size={24} />
          <div>
            <h4>同步概览暂时不可用</h4>
            <p>{syncOverviewQuery.error instanceof Error ? syncOverviewQuery.error.message : "无法读取同步数据。"}</p>
          </div>
        </article>
      ) : null}

      {!syncOverviewQuery.isLoading && !syncOverviewQuery.isError ? (
        <div className="sync-layout">
          <div className="sync-column">
            <article className="detail-card">
              <div className="section-head">
                <div>
                  <p className="eyebrow">批量补齐</p>
                  <h4>按目标端点恢复</h4>
                </div>
                <span className="status-pill warning">{batchTargets.length}</span>
              </div>

              {batchTargets.length === 0 ? (
                <div className="sync-empty-block">
                  <WandSparkles size={22} />
                  <div>
                    <strong>当前没有可恢复目标</strong>
                    <p>当某个端点缺少健康副本时，会在这里作为恢复目标出现。</p>
                  </div>
                </div>
              ) : (
                <>
                  <div className="sync-batch-toolbar">
                    <div className="segmented-group" aria-label="批量恢复目标端点">
                      {batchTargets.map((group) => (
                        <button
                          key={group.endpoint.id}
                          type="button"
                          className={`segmented-button${selectedTargetId === group.endpoint.id ? " active" : ""}`}
                          onClick={() => setSelectedTargetId(group.endpoint.id)}
                        >
                          {group.endpoint.name}
                        </button>
                      ))}
                    </div>

                    <div className="action-row">
                      <button type="button" className="ghost-button" onClick={selectAllForTarget}>
                        全选
                      </button>
                      <button type="button" className="ghost-button" onClick={() => setSelectedAssetIds([])}>
                        清空
                      </button>
                      <button
                        type="button"
                        className="primary-button"
                        onClick={() => void handleBatchRestore()}
                        disabled={batchRestoreMutation.isPending || !selectedBatchGroup || selectedCount === 0}
                      >
                        {batchRestoreMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <WandSparkles size={16} />}
                        补齐已选
                      </button>
                    </div>
                  </div>

                  {selectedBatchGroup ? (
                    <div className="catalog-result-meta">
                      <span>{selectedBatchGroup.endpoint.name} 下共有 {selectedBatchGroup.assets.length} 个可恢复资产</span>
                      <span>已选择 {selectedCount} 个</span>
                    </div>
                  ) : null}

                  <div className="sync-list">
                    {recoverableAssets.map((asset) => {
                      const selectable = asset.missingEndpoints.some((endpoint) => endpoint.id === selectedTargetId);
                      const selected = selectedAssetIds.includes(asset.id);

                      return (
                        <RecoverableAssetCard
                          key={asset.id}
                          asset={asset}
                          selected={selected}
                          selectable={selectable}
                          restorePending={restoreMutation.isPending}
                          onRestore={handleRestore}
                          onToggleSelect={toggleAssetSelection}
                        />
                      );
                    })}
                  </div>
                </>
              )}
            </article>

            <article className="detail-card">
              <div className="section-head">
                <div>
                  <p className="eyebrow">冲突候选</p>
                  <h4>需要确认</h4>
                </div>
                <span className="status-pill neutral">{conflictAssets.length}</span>
              </div>

              {conflictAssets.length === 0 ? (
                <div className="sync-empty-block">
                  <ShieldAlert size={22} />
                  <div>
                    <strong>当前没有冲突候选</strong>
                    <p>版本先后关系不明确的资产，会在执行破坏性操作前先显示在这里。</p>
                  </div>
                </div>
              ) : (
                <div className="sync-list">
                  {conflictAssets.map((asset) => (
                    <ConflictAssetCard key={asset.id} asset={asset} />
                  ))}
                </div>
              )}
            </article>
          </div>

          <div className="sync-column">
            <article className="detail-card">
              <div className="section-head">
                <div>
                  <p className="eyebrow">进行中</p>
                  <h4>活跃任务</h4>
                </div>
              </div>

              {runningTasks.length === 0 ? (
                <div className="sync-empty-block">
                  <CheckCircle2 size={22} />
                  <div>
                    <strong>当前没有同步任务</strong>
                    <p>等待中、进行中和重试中的恢复任务会显示在这里。</p>
                  </div>
                </div>
              ) : (
                <div className="task-list">
                  {runningTasks.map((task, index) => (
                    <TaskCard key={task.id || `running-${index}`} task={task} />
                  ))}
                </div>
              )}
            </article>

            <article className="detail-card">
              <div className="section-head">
                <div>
                  <p className="eyebrow">失败</p>
                  <h4>待重试</h4>
                </div>
              </div>

              {failedTasks.length === 0 ? (
                <div className="sync-empty-block">
                  <RefreshCcw size={22} />
                  <div>
                    <strong>当前没有失败任务</strong>
                    <p>最近的同步失败记录和重试入口会显示在这里。</p>
                  </div>
                </div>
              ) : (
                <div className="task-list">
                  {failedTasks.map((task, index) => (
                    <TaskCard
                      key={task.id || `failed-${index}`}
                      task={task}
                      canRetry={Boolean(task.id)}
                      retryPending={retryMutation.isPending}
                      onRetry={task.id ? () => void handleRetry(task.id) : undefined}
                    />
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

function RecoverableAssetCard({
  asset,
  restorePending,
  selected,
  selectable,
  onRestore,
  onToggleSelect
}: {
  asset: CatalogSyncAsset;
  restorePending: boolean;
  selected: boolean;
  selectable: boolean;
  onRestore: (asset: CatalogSyncAsset, targetEndpoint: CatalogSyncEndpointRef) => void;
  onToggleSelect: (assetId: string) => void;
}) {
  const MediaIcon = getMediaIcon(asset.mediaType);

  return (
    <article className="task-card sync-item-card">
      <div className="sync-item-head">
        <div className={`sync-poster tone-${getSyncAssetTone(asset)}${asset.poster?.url ? " has-poster" : ""}`}>
          {asset.poster?.url ? (
            <img src={asset.poster.url} alt={asset.displayName} className="asset-poster" loading="lazy" />
          ) : (
            <MediaIcon size={24} strokeWidth={1.8} />
          )}
        </div>

        <div className="sync-item-copy">
          <div className="asset-card-head">
            <span className="asset-badge">{getMediaTypeLabel(asset.mediaType)}</span>
            <span className={`status-pill ${getSyncAssetTone(asset)}`}>{getSyncAssetStatusLabel(asset)}</span>
          </div>

          <div className="asset-title-block">
            <h4>{asset.displayName}</h4>
            <p>{asset.logicalPathKey}</p>
          </div>

          <div className="asset-meta-row">
            <span>{formatCatalogDate(asset.primaryTimestamp)}</span>
            <span>可用副本 {asset.availableReplicaCount}</span>
            <span>缺失副本 {asset.missingReplicaCount}</span>
          </div>
        </div>
      </div>

      <div className="sync-endpoint-group">
        <strong>来源端</strong>
        <div className="replica-chip-row">
          {asset.recommendedSource ? (
            <span className="replica-chip success">{asset.recommendedSource.name}</span>
          ) : (
            <span className="replica-chip danger">不可用</span>
          )}
        </div>
      </div>

      <div className="sync-endpoint-group">
        <strong>缺失端点</strong>
        <div className="replica-chip-row">
          {asset.missingEndpoints.map((endpoint) => (
            <span key={endpoint.id} className="replica-chip danger">
              {endpoint.name}
            </span>
          ))}
        </div>
      </div>

      <div className="sync-item-footer">
        <div className="action-row">
          {selectable ? (
            <button type="button" className={`ghost-button${selected ? " is-selected" : ""}`} onClick={() => onToggleSelect(asset.id)}>
              {selected ? "已选中" : "选择"}
            </button>
          ) : null}

          <Link to={`/library?assetId=${asset.id}`} className="ghost-button">
            查看详情
            <ArrowRight size={16} />
          </Link>
        </div>

        <div className="action-row">
          {asset.missingEndpoints.map((endpoint) => (
            <button
              key={endpoint.id}
              type="button"
              className="primary-button"
              onClick={() => onRestore(asset, endpoint)}
              disabled={restorePending || !asset.recommendedSource}
            >
              <WandSparkles size={16} />
              恢复到 {endpoint.name}
            </button>
          ))}
        </div>
      </div>
    </article>
  );
}

function ConflictAssetCard({ asset }: { asset: CatalogSyncAsset }) {
  const MediaIcon = getMediaIcon(asset.mediaType);

  return (
    <article className="task-card sync-item-card">
      <div className="sync-item-head">
        <div className={`sync-poster tone-${getSyncAssetTone(asset)}${asset.poster?.url ? " has-poster" : ""}`}>
          {asset.poster?.url ? (
            <img src={asset.poster.url} alt={asset.displayName} className="asset-poster" loading="lazy" />
          ) : (
            <MediaIcon size={24} strokeWidth={1.8} />
          )}
        </div>

        <div className="sync-item-copy">
          <div className="asset-card-head">
            <span className="asset-badge">{getMediaTypeLabel(asset.mediaType)}</span>
            <span className="status-pill neutral">冲突</span>
          </div>

          <div className="asset-title-block">
            <h4>{asset.displayName}</h4>
            <p>{asset.logicalPathKey}</p>
          </div>

          <div className="asset-meta-row">
            <span>{formatCatalogDate(asset.primaryTimestamp)}</span>
            <span>可用副本 {asset.availableReplicaCount}</span>
          </div>
        </div>
      </div>

      <div className="sync-endpoint-group">
        <strong>冲突端点</strong>
        <div className="replica-chip-row">
          {asset.conflictEndpoints.map((endpoint) => (
            <span key={endpoint.id} className="replica-chip neutral">
              {endpoint.name}
            </span>
          ))}
        </div>
      </div>

      {asset.updatedEndpoints.length > 0 ? (
        <div className="sync-endpoint-group">
          <strong>已更新</strong>
          <div className="replica-chip-row">
            {asset.updatedEndpoints.map((endpoint) => (
              <span key={endpoint.id} className="replica-chip warning">
                {endpoint.name}
              </span>
            ))}
          </div>
        </div>
      ) : null}

      {asset.consistentEndpoints.length > 0 ? (
        <div className="sync-endpoint-group">
          <strong>一致</strong>
          <div className="replica-chip-row">
            {asset.consistentEndpoints.map((endpoint) => (
              <span key={endpoint.id} className="replica-chip success">
                {endpoint.name}
              </span>
            ))}
          </div>
        </div>
      ) : null}

      <div className="action-row">
        <Link to={`/library?assetId=${asset.id}`} className="ghost-button">
          查看详情
          <ArrowRight size={16} />
        </Link>
      </div>
    </article>
  );
}

function TaskCard({
  task,
  canRetry = false,
  retryPending = false,
  onRetry
}: {
  task: CatalogTask;
  canRetry?: boolean;
  retryPending?: boolean;
  onRetry?: () => void;
}) {
  const status = safeLower(task.status);
  const taskType = task.taskType || "task";

  return (
    <article className="task-card sync-task-card">
      <div className="section-head">
        <div>
          <p className="eyebrow">任务</p>
          <h4>{getTaskTypeLabel(taskType)}</h4>
        </div>
        <span className={`status-pill ${getTaskTone(status)}`}>{getTaskStatusLabel(status)}</span>
      </div>

      <div className="task-card-meta">
        <span>创建于 {formatCatalogDate(task.createdAt)}</span>
        <span>更新于 {formatCatalogDate(task.updatedAt)}</span>
      </div>

      {task.errorMessage ? <p className="error-copy">{task.errorMessage}</p> : null}
      {task.resultSummary ? <p className="muted-copy">{task.resultSummary}</p> : null}

      {canRetry && onRetry ? (
        <div className="action-row">
          <button type="button" className="ghost-button" onClick={onRetry} disabled={retryPending}>
            <RefreshCcw size={16} />
            重试
          </button>
        </div>
      ) : null}
    </article>
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

function getMediaIcon(mediaType: string) {
  switch (normalizeMediaType(mediaType)) {
    case "image":
      return Images;
    case "video":
      return Clapperboard;
    case "audio":
      return AudioLines;
    default:
      return ArrowRight;
  }
}

function getSyncAssetStatusLabel(asset: CatalogSyncAsset): string {
  switch (safeLower(asset.assetStatus)) {
    case "ready":
      return "完整可用";
    case "partial":
      return "部分缺失";
    case "processing":
      return "处理中";
    case "conflict":
      return "冲突候选";
    case "pending_delete":
      return "待删除";
    case "deleted":
      return "已删除";
    default:
      return asset.availableReplicaCount === 1 ? "仅单端存在" : "完整可用";
  }
}

function getSyncAssetTone(asset: CatalogSyncAsset): "success" | "warning" | "danger" | "neutral" {
  switch (safeLower(asset.assetStatus)) {
    case "ready":
      return "success";
    case "partial":
    case "processing":
      return "warning";
    case "conflict":
    case "pending_delete":
      return "neutral";
    case "deleted":
      return "danger";
    default:
      return asset.availableReplicaCount === 1 ? "warning" : "success";
  }
}

function getTaskTypeLabel(taskType: string): string {
  switch (safeLower(taskType)) {
    case "restore_asset":
      return "单资产恢复";
    case "restore_batch":
      return "批量补齐";
    case "scan_endpoint":
      return "端点扫描";
    default:
      return taskType;
  }
}

function getTaskStatusLabel(status: string): string {
  switch (status) {
    case "pending":
      return "等待中";
    case "running":
      return "进行中";
    case "retrying":
      return "重试中";
    case "success":
      return "成功";
    case "failed":
    case "error":
      return "失败";
    default:
      return "未知";
  }
}

function getTaskTone(status: string): "success" | "warning" | "danger" | "neutral" {
  switch (status) {
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

function safeLower(value?: string) {
  return typeof value === "string" ? value.toLowerCase() : "";
}
