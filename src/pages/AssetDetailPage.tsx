import { useMemo, useState } from "react";
import { Link, useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import {
  AlertTriangle,
  ArrowLeft,
  FolderOpen,
  LoaderCircle,
  RefreshCcw,
  Trash2,
  WandSparkles
} from "lucide-react";
import { AssetPreview } from "../components/media/AssetPreview";
import {
  useCatalogAssets,
  useCatalogDeleteReplica,
  useCatalogEndpoints,
  useCatalogRestoreAsset
} from "../hooks/useCatalog";
import {
  formatCatalogDate,
  formatDurationSeconds,
  formatFileSize,
  getAssetStatusLabel,
  getAssetTone,
  getAvailableReplicas,
  getMediaTypeLabel,
  getMissingReplicas,
  getReplicaTone,
  normalizeMediaType
} from "../lib/catalog-view";
import type { CatalogEndpoint, CatalogReplica } from "../types/catalog";

type DeleteDialogState = {
  replica: CatalogReplica;
  endpointName: string;
  isLastAvailableReplica: boolean;
};

export function AssetDetailPage({ assetIdOverride }: { assetIdOverride?: string }) {
  const { assetId: routeAssetId } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const assetsQuery = useCatalogAssets();
  const endpointsQuery = useCatalogEndpoints();
  const restoreMutation = useCatalogRestoreAsset();
  const deleteReplicaMutation = useCatalogDeleteReplica();
  const [actionNotice, setActionNotice] = useState<string | null>(null);
  const [deleteDialog, setDeleteDialog] = useState<DeleteDialogState | null>(null);
  const assetId = assetIdOverride ?? routeAssetId ?? searchParams.get("assetId") ?? "";

  const backSearch = useMemo(() => {
    const params = new URLSearchParams(location.search);
    params.delete("assetId");
    const next = params.toString();
    return next ? `?${next}` : "";
  }, [location.search]);

  const endpointLookup = useMemo(() => {
    return new Map((endpointsQuery.data ?? []).map((endpoint) => [endpoint.id, endpoint.name]));
  }, [endpointsQuery.data]);

  const asset = useMemo(() => {
    return (assetsQuery.data ?? []).find((item) => item.id === assetId) ?? null;
  }, [assetId, assetsQuery.data]);

  if (assetsQuery.isLoading) {
    return (
      <section className="page-stack">
        <article className="detail-card empty-state">
          <LoaderCircle size={24} className="spin" />
          <div>
            <h4>正在加载资产详情</h4>
            <p>客户端正在从本地 Catalog 读取副本与媒体元数据。</p>
          </div>
        </article>
      </section>
    );
  }

  if (assetsQuery.isError || !asset) {
    return (
      <section className="page-stack">
        <article className="detail-card empty-state">
          <AlertTriangle size={24} />
          <div>
            <h4>未找到这个资产</h4>
            <p>这个资产可能已经从可见资产库中移除，或最新刷新尚未完成。</p>
          </div>

          <Link to={`/assets${backSearch}`} className="ghost-button inline-button">
            <ArrowLeft size={16} />
            返回资产库
          </Link>
        </article>
      </section>
    );
  }

  const availableReplicas = getAvailableReplicas(asset);
  const missingReplicas = getMissingReplicas(asset);
  const isAudioAsset = normalizeMediaType(asset.mediaType) === "audio";
  const recommendedSource = choosePreferredRestoreSource(availableReplicas, endpointsQuery.data ?? []);
  const primaryPath = asset.canonicalPath ?? asset.logicalPathKey;
  const primaryDirectory =
    asset.canonicalDirectory ??
    availableReplicas.find((replica) => replica.logicalDirectory)?.logicalDirectory ??
    availableReplicas.find((replica) => replica.resolvedDirectory)?.resolvedDirectory ??
    "未记录";
  const preferredOpenReplica = recommendedSource ?? availableReplicas[0] ?? null;
  const preferredOpenEndpointName = preferredOpenReplica
    ? endpointLookup.get(preferredOpenReplica.endpointId) ?? preferredOpenReplica.endpointId
    : undefined;
  const endpointStatusEntries = [...asset.replicas]
    .map((replica) => ({
      replica,
      endpointName: endpointLookup.get(replica.endpointId) ?? replica.endpointId
    }))
    .sort((left, right) => left.endpointName.localeCompare(right.endpointName, "zh-CN"));
  const availableReplicaEntries = endpointStatusEntries.filter((entry) => entry.replica.existsFlag);
  const missingReplicaEntries = endpointStatusEntries.filter((entry) => !entry.replica.existsFlag);

  function handlePlaceholderAction(action: string, endpointName?: string) {
    setActionNotice(`${action}${endpointName ? `：${endpointName}` : ""} 还会继续接入更明确的客户端动作。`);
  }

  function handleDeleteIntent(replica: CatalogReplica, endpointName: string) {
    setActionNotice(null);
    setDeleteDialog({
      replica,
      endpointName,
      isLastAvailableReplica: availableReplicas.length === 1 && replica.existsFlag
    });
  }

  async function handleRestoreReplica(replica: CatalogReplica, endpointName: string) {
    if (!recommendedSource) {
      setActionNotice("当前没有可用的健康源副本。");
      return;
    }

    if (!asset) {
      return;
    }

    setActionNotice(null);
    const currentAsset = asset;

    try {
      const summary = await restoreMutation.mutateAsync({
        assetId: currentAsset.id,
        sourceEndpointId: recommendedSource.endpointId,
        targetEndpointId: replica.endpointId
      });

      setActionNotice(
        summary.skipped
          ? `${endpointName} 已经是最新状态。`
          : `${currentAsset.displayName} 已恢复到 ${endpointName}。`
      );
    } catch (error) {
      setActionNotice(error instanceof Error ? error.message : "恢复失败。");
    }
  }

  async function handleConfirmDelete() {
    if (!asset || !deleteDialog) {
      return;
    }

    const currentAsset = asset;
    const currentDialog = deleteDialog;
    setActionNotice(null);

    try {
      const summary = await deleteReplicaMutation.mutateAsync({
        assetId: currentAsset.id,
        targetEndpointId: currentDialog.replica.endpointId
      });

      setDeleteDialog(null);

      if (summary.assetRemoved) {
        navigate(`/assets${backSearch}`, { replace: true });
        return;
      }

      setActionNotice(`已删除 ${currentDialog.endpointName} 上的副本。`);
    } catch (error) {
      setActionNotice(error instanceof Error ? error.message : "删除副本失败。");
    }
  }

  async function handleRestoreMissingReplicas() {
    if (!asset || missingReplicas.length === 0) {
      return;
    }

    if (!recommendedSource) {
      setActionNotice("当前没有可用的健康源副本。");
      return;
    }

    setActionNotice(null);
    let successCount = 0;
    let skippedCount = 0;
    let failedCount = 0;

    for (const replica of missingReplicas) {
      try {
        const summary = await restoreMutation.mutateAsync({
          assetId: asset.id,
          sourceEndpointId: recommendedSource.endpointId,
          targetEndpointId: replica.endpointId
        });

        if (summary.skipped) {
          skippedCount += 1;
        } else {
          successCount += 1;
        }
      } catch {
        failedCount += 1;
      }
    }

    setActionNotice(
      `已补齐 ${successCount} 个端点${skippedCount > 0 ? `，跳过 ${skippedCount} 个` : ""}${failedCount > 0 ? `，失败 ${failedCount} 个` : ""}。`
    );
  }

  return (
    <section className="page-stack asset-detail-page">
      <div className="asset-detail-topbar">
        <Link to={`/assets${backSearch}`} className="back-link">
          <ArrowLeft size={16} />
          返回资产库
        </Link>

        <div className="replica-chip-row asset-detail-status-strip">
          <span className="asset-badge">{getMediaTypeLabel(asset.mediaType)}</span>
          <span className={`status-pill ${getAssetTone(asset)}`}>{getAssetStatusLabel(asset)}</span>
          <span className="replica-chip neutral">可用 {availableReplicas.length}</span>
          {missingReplicas.length > 0 ? <span className="replica-chip warning">待补 {missingReplicas.length}</span> : null}
        </div>
      </div>

      <div className="asset-detail-layout">
        <div className="asset-detail-main">
          <article className="detail-card asset-info-card">
            <div className="asset-file-title">
              <h3>{asset.displayName}</h3>
              <p className="asset-file-subtitle">{primaryPath}</p>
            </div>

            <div className="asset-info-grid">
              <InfoField label="标准路径" value={primaryPath} wide />
              <InfoField label="标准文件夹" value={primaryDirectory} wide />
              <InfoField label="主时间" value={formatCatalogDate(asset.primaryTimestamp)} />
              <InfoField label="创建于" value={formatCatalogDate(asset.createdAt)} />
              <InfoField label="更新于" value={formatCatalogDate(asset.updatedAt)} />
              <InfoField label="状态" value={getAssetStatusLabel(asset)} />
              {isAudioAsset ? <InfoField label="时长" value={formatDurationSeconds(asset.audioMetadata?.durationSeconds)} /> : null}
              {isAudioAsset ? <InfoField label="编码" value={asset.audioMetadata?.codecName ?? "待分析"} /> : null}
              {isAudioAsset ? <InfoField label="采样率" value={asset.audioMetadata?.sampleRateHz ? `${asset.audioMetadata.sampleRateHz} Hz` : "待分析"} /> : null}
              {isAudioAsset ? <InfoField label="声道" value={`${asset.audioMetadata?.channelCount ?? "待分析"}`} /> : null}
            </div>

            <div className="asset-presence-panel">
              <div className="section-head">
                <div>
                  <h4>同步状态</h4>
                </div>
                <div className="asset-presence-actions">
                  {missingReplicas.length > 0 ? (
                    <button
                      type="button"
                      className="primary-button"
                      onClick={() => void handleRestoreMissingReplicas()}
                      disabled={restoreMutation.isPending || !recommendedSource}
                    >
                      {restoreMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <WandSparkles size={16} />}
                      补齐缺失
                    </button>
                  ) : null}

                  <Link to="/sync" className="ghost-button">
                    <WandSparkles size={16} />
                    同步
                  </Link>
                </div>
              </div>

              <div className="asset-presence-summary">
                <InfoField label="已落位端点" value={`${availableReplicas.length}`} />
                <InfoField label="待补端点" value={`${missingReplicas.length}`} />
                <InfoField
                  label="同步来源"
                  value={recommendedSource ? endpointLookup.get(recommendedSource.endpointId) ?? recommendedSource.endpointId : "不可用"}
                />
              </div>

              <div className="asset-presence-groups">
                <div className="asset-presence-group">
                  <span>已存储</span>
                  <div className="replica-chip-row">
                    {availableReplicaEntries.length > 0 ? (
                      availableReplicaEntries.map(({ replica, endpointName }) => (
                        <button
                          key={`available-${replica.endpointId}`}
                          type="button"
                          className="replica-chip-button success"
                          onClick={() => handleDeleteIntent(replica, endpointName)}
                          disabled={deleteReplicaMutation.isPending}
                          title={`从 ${endpointName} 删除`}
                        >
                          {endpointName}
                        </button>
                      ))
                    ) : (
                      <span className="replica-chip neutral">暂无</span>
                    )}
                  </div>
                </div>

                <div className="asset-presence-group">
                  <span>未存储</span>
                  <div className="replica-chip-row">
                    {missingReplicaEntries.length > 0 ? (
                      missingReplicaEntries.map(({ replica, endpointName }) => (
                        <button
                          key={`missing-${replica.endpointId}`}
                          type="button"
                          className="replica-chip-button warning"
                          onClick={() => void handleRestoreReplica(replica, endpointName)}
                          disabled={restoreMutation.isPending || !recommendedSource}
                          title={`同步到 ${endpointName}`}
                        >
                          {endpointName}
                        </button>
                      ))
                    ) : (
                      <span className="replica-chip success">已完整</span>
                    )}
                  </div>
                </div>
              </div>
            </div>
          </article>
        </div>

        <aside className="asset-detail-side">
          <article className="detail-card asset-action-card">
            <div className="section-head">
              <div>
                <h4>操作</h4>
              </div>
            </div>

            <div className="asset-action-grid">
              <button
                type="button"
                className="primary-button"
                disabled={!preferredOpenReplica}
                onClick={() => handlePlaceholderAction("打开所在位置", preferredOpenEndpointName)}
              >
                <FolderOpen size={16} />
                打开位置
              </button>

              <button type="button" className="ghost-button" onClick={() => handlePlaceholderAction("重新扫描资产")}>
                <RefreshCcw size={16} />
                重新扫描
              </button>

              <Link to="/sync" className="ghost-button">
                <WandSparkles size={16} />
                同步
              </Link>
            </div>

            <div className="replica-chip-row">
              {preferredOpenEndpointName ? <span className="replica-chip neutral">位置 {preferredOpenEndpointName}</span> : null}
              {recommendedSource ? (
                <span className="replica-chip success">
                  来源 {endpointLookup.get(recommendedSource.endpointId) ?? recommendedSource.endpointId}
                </span>
              ) : null}
            </div>

            {actionNotice ? <p className="inline-note asset-action-note">{actionNotice}</p> : null}
          </article>

          <article className="detail-card asset-preview-card">
            <div className="section-head">
              <div>
                <h4>预览</h4>
              </div>
            </div>

            <div className={`asset-preview-visual tone-${getAssetTone(asset)}`}>
              <AssetPreview asset={asset} />
            </div>
          </article>
        </aside>
      </div>

      <ConfirmDeleteReplicaDialog
        state={deleteDialog}
        pending={deleteReplicaMutation.isPending}
        onCancel={() => {
          if (!deleteReplicaMutation.isPending) {
            setDeleteDialog(null);
          }
        }}
        onConfirm={() => void handleConfirmDelete()}
      />
    </section>
  );
}

function choosePreferredRestoreSource(replicas: CatalogReplica[], endpoints: CatalogEndpoint[]) {
  const endpointLookup = new Map(endpoints.map((endpoint) => [endpoint.id, endpoint]));

  return [...replicas].sort((left, right) => {
    const leftEndpoint = endpointLookup.get(left.endpointId);
    const rightEndpoint = endpointLookup.get(right.endpointId);
    const leftPriority = getRestoreSourcePriority(leftEndpoint?.endpointType);
    const rightPriority = getRestoreSourcePriority(rightEndpoint?.endpointType);

    if (leftPriority !== rightPriority) {
      return leftPriority - rightPriority;
    }

    const leftName = (leftEndpoint?.name ?? left.endpointId).toLowerCase();
    const rightName = (rightEndpoint?.name ?? right.endpointId).toLowerCase();
    return leftName.localeCompare(rightName, "zh-CN");
  })[0] ?? null;
}

function getRestoreSourcePriority(endpointType?: string) {
  switch ((endpointType ?? "").trim().toUpperCase()) {
    case "LOCAL":
      return 0;
    case "REMOVABLE":
      return 1;
    case "QNAP_SMB":
      return 2;
    case "CLOUD_115":
      return 3;
    default:
      return 9;
  }
}

function InfoField({
  label,
  value,
  wide = false
}: {
  label: string;
  value: string;
  wide?: boolean;
}) {
  return (
    <div className={`asset-info-field${wide ? " wide" : ""}`}>
      <span>{label}</span>
      <strong title={value}>{value}</strong>
    </div>
  );
}

function EndpointStatusRow({
  endpointName,
  replica,
  defaultDirectory,
  defaultPath,
  deletePending,
  restorePending,
  canRestore,
  onDeleteReplica,
  onRestoreReplica,
  onPlaceholderAction
}: {
  endpointName: string;
  replica: CatalogReplica;
  defaultDirectory: string;
  defaultPath: string;
  deletePending: boolean;
  restorePending: boolean;
  canRestore: boolean;
  onDeleteReplica: (replica: CatalogReplica, endpointName: string) => void;
  onRestoreReplica: (replica: CatalogReplica, endpointName: string) => void;
  onPlaceholderAction: (action: string, endpointName?: string) => void;
}) {
  const tone = getReplicaTone(replica);
  const replicaDirectory = replica.existsFlag
    ? replica.resolvedDirectory ?? replica.logicalDirectory ?? defaultDirectory
    : defaultDirectory;
  const replicaPath = replica.existsFlag ? replica.physicalPath : replica.physicalPath || defaultPath;

  return (
    <article className="replica-row-card">
      <div className="replica-row-main">
        <div className="replica-row-head">
          <div className="replica-row-title">
            <strong>{endpointName}</strong>
            <div className="replica-chip-row">
              <span className={`status-pill ${tone}`}>{replica.existsFlag ? "已落位" : "未落位"}</span>
              {replica.matchesLogicalPath === false ? <span className="status-pill warning">路径偏离</span> : null}
              {replica.existsFlag ? <span className="replica-chip neutral">{formatFileSize(replica.version?.size)}</span> : null}
              {replica.existsFlag ? <span className="replica-chip neutral">{formatCatalogDate(replica.version?.mtime)}</span> : null}
            </div>
          </div>
        </div>

        <div className="replica-row-grid">
          <InfoField label={replica.existsFlag ? "文件夹" : "目标文件夹"} value={replicaDirectory} wide />
          <InfoField label={replica.existsFlag ? "位置" : "目标位置"} value={replicaPath} wide />
        </div>
      </div>

      <div className="replica-row-actions">
        {replica.existsFlag ? (
          <>
            <button
              type="button"
              className="ghost-button icon-button"
              title="打开位置"
              aria-label={`打开 ${endpointName} 位置`}
              onClick={() => onPlaceholderAction("打开所在位置", endpointName)}
            >
              <FolderOpen size={16} />
            </button>
            <button
              type="button"
              className="ghost-button icon-button"
              title="重新扫描"
              aria-label={`重新扫描 ${endpointName}`}
              onClick={() => onPlaceholderAction("重新扫描副本", endpointName)}
            >
              <RefreshCcw size={16} />
            </button>
            <button
              type="button"
              className="danger-button icon-button"
              title="删除副本"
              aria-label={`删除 ${endpointName} 副本`}
              onClick={() => onDeleteReplica(replica, endpointName)}
              disabled={deletePending}
            >
              <Trash2 size={16} />
            </button>
          </>
        ) : (
          <button
            type="button"
            className="primary-button icon-button"
            title="恢复到这里"
            aria-label={`恢复到 ${endpointName}`}
            onClick={() => onRestoreReplica(replica, endpointName)}
            disabled={restorePending || !canRestore}
          >
            {restorePending ? <LoaderCircle size={16} className="spin" /> : <WandSparkles size={16} />}
          </button>
        )}
      </div>
    </article>
  );
}

function ConfirmDeleteReplicaDialog({
  state,
  pending,
  onCancel,
  onConfirm
}: {
  state: DeleteDialogState | null;
  pending: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  if (!state) {
    return null;
  }

  return (
    <div className="dialog-overlay" role="presentation">
      <div className="dialog-card" role="dialog" aria-modal="true" aria-labelledby="delete-replica-title">
        <div className="dialog-header">
          <span className={`status-pill ${state.isLastAvailableReplica ? "danger" : "warning"}`}>
            {state.isLastAvailableReplica ? "高风险" : "删除副本"}
          </span>
          <h4 id="delete-replica-title">
            {state.isLastAvailableReplica ? "确认删除最后一个可读副本？" : "确认删除这个副本？"}
          </h4>
          <p>
            {state.isLastAvailableReplica
              ? "删除这个副本后，这个资产将从可见资产库中消失，因为不会再剩下任何可读副本。"
              : "这个资产还有其他健康副本，所以删除后仍会继续显示在资产库中。"}
          </p>
        </div>

        <div className="dialog-meta">
          <div>
            <span>端点</span>
            <strong>{state.endpointName}</strong>
          </div>
          <div>
            <span>路径</span>
            <strong>{state.replica.physicalPath}</strong>
          </div>
        </div>

        <div className="dialog-actions">
          <button type="button" className="ghost-button" onClick={onCancel} disabled={pending}>
            取消
          </button>
          <button type="button" className="danger-button" onClick={onConfirm} disabled={pending}>
            {pending ? (
              <>
                <LoaderCircle size={16} className="spin" />
                删除中
              </>
            ) : (
              <>
                <Trash2 size={16} />
                确认删除
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
}
