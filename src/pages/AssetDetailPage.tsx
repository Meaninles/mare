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
import type { CatalogReplica } from "../types/catalog";

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

          <Link to={`/library${backSearch}`} className="ghost-button inline-button">
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
  const recommendedSource = availableReplicas[0] ?? null;

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
        navigate(`/library${backSearch}`, { replace: true });
        return;
      }

      setActionNotice(`已删除 ${currentDialog.endpointName} 上的副本。`);
    } catch (error) {
      setActionNotice(error instanceof Error ? error.message : "删除副本失败。");
    }
  }

  return (
    <section className="page-stack">
      <Link to={`/library${backSearch}`} className="back-link">
        <ArrowLeft size={16} />
        返回资产库
      </Link>

      <article className="hero-card asset-detail-hero">
        <div className="asset-detail-copy">
          <div className="asset-detail-title-row">
            <span className="asset-badge">{getMediaTypeLabel(asset.mediaType)}</span>
            <span className={`status-pill ${getAssetTone(asset)}`}>{getAssetStatusLabel(asset)}</span>
          </div>

          <h3>{asset.displayName}</h3>
          <p>{asset.logicalPathKey}</p>

          <div className="detail-stat-row">
            <div className="detail-stat">
              <span>主时间</span>
              <strong>{formatCatalogDate(asset.primaryTimestamp)}</strong>
            </div>
            <div className="detail-stat">
              <span>可用副本</span>
              <strong>{availableReplicas.length}</strong>
            </div>
            <div className="detail-stat">
              <span>缺失记录</span>
              <strong>{missingReplicas.length}</strong>
            </div>
            <div className="detail-stat">
              <span>资产状态</span>
              <strong>{getAssetStatusLabel(asset)}</strong>
            </div>
          </div>
        </div>

        <div className="asset-preview-panel">
          <div className={`asset-preview-visual tone-${getAssetTone(asset)}`}>
            <AssetPreview asset={asset} />
          </div>

          <div className="detail-actions">
            <Link to="/sync" className="primary-button">
              <WandSparkles size={16} />
              打开同步中心
            </Link>
            <button type="button" className="ghost-button" onClick={() => handlePlaceholderAction("重新扫描资产")}>
              <RefreshCcw size={16} />
              重新扫描
            </button>
            <button type="button" className="ghost-button" onClick={() => handlePlaceholderAction("打开所在位置")}>
              <FolderOpen size={16} />
              打开位置
            </button>
          </div>
        </div>
      </article>

      {actionNotice ? <p className="inline-note">{actionNotice}</p> : null}

      <article className="detail-card">
        <div className="section-head">
          <div>
            <p className="eyebrow">概览</p>
            <h4>资产信息</h4>
          </div>
        </div>

        <div className="field-grid">
          <div className="field">
            <span>名称</span>
            <strong>{asset.displayName}</strong>
          </div>
          <div className="field">
            <span>媒体类型</span>
            <strong>{getMediaTypeLabel(asset.mediaType)}</strong>
          </div>
          <div className="field field-span">
            <span>逻辑路径</span>
            <strong>{asset.logicalPathKey}</strong>
          </div>
          <div className="field">
            <span>创建时间</span>
            <strong>{formatCatalogDate(asset.createdAt)}</strong>
          </div>
          <div className="field">
            <span>更新时间</span>
            <strong>{formatCatalogDate(asset.updatedAt)}</strong>
          </div>

          {isAudioAsset ? (
            <>
              <div className="field">
                <span>时长</span>
                <strong>{formatDurationSeconds(asset.audioMetadata?.durationSeconds)}</strong>
              </div>
              <div className="field">
                <span>编码</span>
                <strong>{asset.audioMetadata?.codecName ?? "待分析"}</strong>
              </div>
              <div className="field">
                <span>采样率</span>
                <strong>{asset.audioMetadata?.sampleRateHz ? `${asset.audioMetadata.sampleRateHz} Hz` : "待分析"}</strong>
              </div>
              <div className="field">
                <span>声道</span>
                <strong>{asset.audioMetadata?.channelCount ?? "待分析"}</strong>
              </div>
            </>
          ) : null}
        </div>
      </article>

      <section className="replica-section">
        <div className="section-head">
          <div>
            <p className="eyebrow">副本</p>
            <h4>可用副本</h4>
          </div>
        </div>

        {availableReplicas.length === 0 ? (
          <article className="detail-card empty-state">
            <div>
              <h4>当前没有健康副本</h4>
              <p>当其他端点重新可用后，恢复与重建动作会在这里接上。</p>
            </div>
          </article>
        ) : (
          <div className="replica-card-grid">
            {availableReplicas.map((replica) => (
              <ReplicaCard
                key={replica.id}
                replica={replica}
                endpointName={endpointLookup.get(replica.endpointId) ?? replica.endpointId}
                deletePending={deleteReplicaMutation.isPending}
                restorePending={restoreMutation.isPending}
                canRestore={false}
                onDeleteReplica={handleDeleteIntent}
                onRestoreReplica={handleRestoreReplica}
                onPlaceholderAction={handlePlaceholderAction}
              />
            ))}
          </div>
        )}
      </section>

      {missingReplicas.length > 0 ? (
        <section className="replica-section">
          <div className="section-head">
            <div>
              <p className="eyebrow">副本记录</p>
              <h4>缺失副本</h4>
            </div>
          </div>

          <div className="replica-card-grid">
            {missingReplicas.map((replica) => (
              <ReplicaCard
                key={replica.id}
                replica={replica}
                endpointName={endpointLookup.get(replica.endpointId) ?? replica.endpointId}
                deletePending={false}
                restorePending={restoreMutation.isPending}
                canRestore={Boolean(recommendedSource)}
                onDeleteReplica={handleDeleteIntent}
                onRestoreReplica={handleRestoreReplica}
                onPlaceholderAction={handlePlaceholderAction}
              />
            ))}
          </div>
        </section>
      ) : null}

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

function ReplicaCard({
  endpointName,
  replica,
  deletePending,
  restorePending,
  canRestore,
  onDeleteReplica,
  onRestoreReplica,
  onPlaceholderAction
}: {
  endpointName: string;
  replica: CatalogReplica;
  deletePending: boolean;
  restorePending: boolean;
  canRestore: boolean;
  onDeleteReplica: (replica: CatalogReplica, endpointName: string) => void;
  onRestoreReplica: (replica: CatalogReplica, endpointName: string) => void;
  onPlaceholderAction: (action: string, endpointName?: string) => void;
}) {
  const tone = getReplicaTone(replica);

  return (
    <article className="detail-card replica-card">
      <div className="replica-card-head">
        <div>
          <p className="eyebrow">端点</p>
          <h4>{endpointName}</h4>
        </div>
        <span className={`status-pill ${tone}`}>{replica.existsFlag ? "可用" : "缺失"}</span>
      </div>

      <div className="replica-card-meta">
        <div>
          <span>物理路径</span>
          <strong>{replica.physicalPath}</strong>
        </div>
        <div>
          <span>副本状态</span>
          <strong>{replica.replicaStatus}</strong>
        </div>
        <div>
          <span>文件大小</span>
          <strong>{formatFileSize(replica.version?.size)}</strong>
        </div>
        <div>
          <span>修改时间</span>
          <strong>{formatCatalogDate(replica.version?.mtime)}</strong>
        </div>
      </div>

      <div className="action-row">
        {replica.existsFlag ? (
          <>
            <button
              type="button"
              className="ghost-button"
              onClick={() => onPlaceholderAction("打开所在位置", endpointName)}
            >
              <FolderOpen size={16} />
              打开位置
            </button>
            <button
              type="button"
              className="ghost-button danger-button"
              onClick={() => onDeleteReplica(replica, endpointName)}
              disabled={deletePending}
            >
              <Trash2 size={16} />
              删除副本
            </button>
          </>
        ) : (
          <button
            type="button"
            className="primary-button"
            onClick={() => onRestoreReplica(replica, endpointName)}
            disabled={restorePending || !canRestore}
          >
            {restorePending ? <LoaderCircle size={16} className="spin" /> : <WandSparkles size={16} />}
            恢复到这里
          </button>
        )}

        <button type="button" className="ghost-button" onClick={() => onPlaceholderAction("重新扫描副本", endpointName)}>
          <RefreshCcw size={16} />
          重新扫描
        </button>
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
