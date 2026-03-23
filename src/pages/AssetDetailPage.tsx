import { useMemo, useState } from "react";
import { Link, useLocation, useParams, useSearchParams } from "react-router-dom";
import { ArrowLeft, FolderOpen, RefreshCcw, Trash2, WandSparkles } from "lucide-react";
import { AssetPreview } from "../components/media/AssetPreview";
import { useCatalogAssets, useCatalogEndpoints } from "../hooks/useCatalog";
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

export function AssetDetailPage({ assetIdOverride }: { assetIdOverride?: string }) {
  const { assetId: routeAssetId } = useParams();
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const assetsQuery = useCatalogAssets();
  const endpointsQuery = useCatalogEndpoints();
  const [actionNotice, setActionNotice] = useState<string | null>(null);
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
          <div>
            <h4>正在读取资产详情</h4>
            <p>正在从本地 Catalog 获取当前资产及其全部副本信息。</p>
          </div>
        </article>
      </section>
    );
  }

  if (assetsQuery.isError || !asset) {
    return (
      <section className="page-stack">
        <article className="detail-card empty-state">
          <div>
            <h4>没有找到对应资产</h4>
            <p>这条资产可能尚未被扫描入库，或者已经在后续重扫中被移除。</p>
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

  function handlePlaceholderAction(action: string, endpointName?: string) {
    setActionNotice(`${action}${endpointName ? ` · ${endpointName}` : ""} 已预留入口，后续会接入真实后端操作。`);
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
              <span>媒体类型</span>
              <strong>{getMediaTypeLabel(asset.mediaType)}</strong>
            </div>
            <div className="detail-stat">
              <span>主时间</span>
              <strong>{formatCatalogDate(asset.primaryTimestamp)}</strong>
            </div>
            <div className="detail-stat">
              <span>可用副本</span>
              <strong>{availableReplicas.length}</strong>
            </div>
            <div className="detail-stat">
              <span>缺失副本</span>
              <strong>{missingReplicas.length}</strong>
            </div>
          </div>
        </div>

        <div className="asset-preview-panel">
          <div className={`asset-preview-visual tone-${getAssetTone(asset)}`}>
            <AssetPreview asset={asset} />
          </div>

          <div className="detail-actions">
            <button type="button" className="primary-button" onClick={() => handlePlaceholderAction("恢复到某端")}>
              <WandSparkles size={16} />
              恢复到某端
            </button>
            <button type="button" className="ghost-button" onClick={() => handlePlaceholderAction("重新扫描")}>
              <RefreshCcw size={16} />
              重新扫描
            </button>
            <button type="button" className="ghost-button" onClick={() => handlePlaceholderAction("打开所在位置")}>
              <FolderOpen size={16} />
              打开所在位置
            </button>
          </div>
        </div>
      </article>

      {actionNotice ? <p className="success-copy">{actionNotice}</p> : null}

      <article className="detail-card">
        <div className="section-head">
          <div>
            <p className="eyebrow">Asset Overview</p>
            <h4>基础信息</h4>
          </div>
        </div>

        <div className="field-grid">
          <div className="field">
            <span>名称</span>
            <strong>{asset.displayName}</strong>
          </div>
          <div className="field">
            <span>状态</span>
            <strong>{getAssetStatusLabel(asset)}</strong>
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
                <span>音频时长</span>
                <strong>{formatDurationSeconds(asset.audioMetadata?.durationSeconds)}</strong>
              </div>
              <div className="field">
                <span>音频编码</span>
                <strong>{asset.audioMetadata?.codecName ?? "待解析"}</strong>
              </div>
              <div className="field">
                <span>采样率</span>
                <strong>{asset.audioMetadata?.sampleRateHz ? `${asset.audioMetadata.sampleRateHz} Hz` : "待解析"}</strong>
              </div>
              <div className="field">
                <span>声道数</span>
                <strong>{asset.audioMetadata?.channelCount ?? "待解析"}</strong>
              </div>
            </>
          ) : null}
        </div>
      </article>

      <section className="replica-section">
        <div className="section-head">
          <div>
            <p className="eyebrow">Replicas</p>
            <h4>可用副本</h4>
          </div>
        </div>

        {availableReplicas.length === 0 ? (
          <article className="detail-card empty-state">
            <div>
              <h4>当前没有可用副本</h4>
              <p>这条资产暂时没有健康副本。后续可在同步中心承接恢复与补齐能力。</p>
            </div>
          </article>
        ) : (
          <div className="replica-card-grid">
            {availableReplicas.map((replica) => (
              <ReplicaCard
                key={replica.id}
                replica={replica}
                endpointName={endpointLookup.get(replica.endpointId) ?? replica.endpointId}
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
              <p className="eyebrow">Missing Records</p>
              <h4>缺失副本记录</h4>
            </div>
          </div>

          <div className="replica-card-grid">
            {missingReplicas.map((replica) => (
              <ReplicaCard
                key={replica.id}
                replica={replica}
                endpointName={endpointLookup.get(replica.endpointId) ?? replica.endpointId}
                onPlaceholderAction={handlePlaceholderAction}
              />
            ))}
          </div>
        </section>
      ) : null}
    </section>
  );
}

function ReplicaCard({
  endpointName,
  replica,
  onPlaceholderAction
}: {
  endpointName: string;
  replica: CatalogReplica;
  onPlaceholderAction: (action: string, endpointName?: string) => void;
}) {
  const tone = getReplicaTone(replica);

  return (
    <article className="detail-card replica-card">
      <div className="replica-card-head">
        <div>
          <p className="eyebrow">Endpoint</p>
          <h4>{endpointName}</h4>
        </div>
        <span className={`status-pill ${tone}`}>{replica.existsFlag ? "可用副本" : "缺失记录"}</span>
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
              打开所在位置
            </button>
            <button
              type="button"
              className="ghost-button danger-button"
              onClick={() => onPlaceholderAction("删除该端副本", endpointName)}
            >
              <Trash2 size={16} />
              删除该端副本
            </button>
          </>
        ) : (
          <button type="button" className="primary-button" onClick={() => onPlaceholderAction("恢复到该端", endpointName)}>
            <WandSparkles size={16} />
            恢复到该端
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
