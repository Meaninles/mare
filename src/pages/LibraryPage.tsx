import { useDeferredValue, useEffect, useMemo, useState } from "react";
import { Link, useLocation, useSearchParams } from "react-router-dom";
import {
  ArrowRight,
  AudioLines,
  ChevronLeft,
  ChevronRight,
  Clapperboard,
  Images,
  SearchX
} from "lucide-react";
import { AssetDetailPage } from "./AssetDetailPage";
import { useCatalogAssets, useCatalogEndpoints } from "../hooks/useCatalog";
import {
  formatCatalogDate,
  getAssetStatusFilterValue,
  getAssetStatusLabel,
  getAssetTone,
  getAvailableReplicaCount,
  getMediaTypeLabel,
  getMissingReplicaCount,
  getReplicaTone,
  normalizeMediaType
} from "../lib/catalog-view";
import type { AssetTone } from "../lib/catalog-view";
import type { CatalogAsset } from "../types/catalog";

const PAGE_SIZE = 12;

const mediaFilters = [
  { value: "all", label: "全部" },
  { value: "image", label: "图片" },
  { value: "video", label: "视频" },
  { value: "audio", label: "音频" }
] as const;

const statusFilters = [
  { value: "all", label: "全部状态" },
  { value: "ready", label: "完整可用" },
  { value: "partial", label: "部分缺失" },
  { value: "single", label: "仅单端存在" }
] as const;

const sortOptions = [
  { value: "latest", label: "最新优先" },
  { value: "earliest", label: "最早优先" }
] as const;

type MediaFilterValue = (typeof mediaFilters)[number]["value"];
type StatusFilterValue = (typeof statusFilters)[number]["value"];
type SortOrder = (typeof sortOptions)[number]["value"];

export function LibraryPage() {
  const [searchParams] = useSearchParams();
  const detailAssetId = searchParams.get("assetId")?.trim() ?? "";

  if (detailAssetId) {
    return <AssetDetailPage assetIdOverride={detailAssetId} />;
  }

  return <LibraryCatalogView />;
}

function LibraryCatalogView() {
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const assetsQuery = useCatalogAssets();
  const endpointsQuery = useCatalogEndpoints();
  const [mediaFilter, setMediaFilter] = useState<MediaFilterValue>("all");
  const [statusFilter, setStatusFilter] = useState<StatusFilterValue>("all");
  const [sortOrder, setSortOrder] = useState<SortOrder>("latest");
  const [currentPage, setCurrentPage] = useState(1);

  const searchQuery = searchParams.get("q")?.trim() ?? "";
  const deferredSearchQuery = useDeferredValue(searchQuery.toLowerCase());

  useEffect(() => {
    setCurrentPage(1);
  }, [deferredSearchQuery, mediaFilter, statusFilter, sortOrder]);

  const endpointLookup = useMemo(() => {
    return new Map((endpointsQuery.data ?? []).map((endpoint) => [endpoint.id, endpoint.name]));
  }, [endpointsQuery.data]);

  const summary = useMemo(() => {
    const assets = assetsQuery.data ?? [];

    return {
      totalAssets: assets.length,
      readyAssets: assets.filter((asset) => getAssetStatusFilterValue(asset) === "ready").length,
      partialAssets: assets.filter((asset) => getAssetStatusFilterValue(asset) === "partial").length,
      singleAssets: assets.filter((asset) => getAssetStatusFilterValue(asset) === "single").length
    };
  }, [assetsQuery.data]);

  const filteredAssets = useMemo(() => {
    const assets = assetsQuery.data ?? [];

    return [...assets]
      .filter((asset) => {
        if (mediaFilter !== "all" && normalizeMediaType(asset.mediaType) !== mediaFilter) {
          return false;
        }

        if (statusFilter !== "all" && getAssetStatusFilterValue(asset) !== statusFilter) {
          return false;
        }

        if (!deferredSearchQuery) {
          return true;
        }

        const endpointNames = asset.replicas
          .map((replica) => endpointLookup.get(replica.endpointId) ?? replica.endpointId)
          .join(" ");

        const haystack = [asset.displayName, asset.logicalPathKey, endpointNames].join(" ").toLowerCase();
        return haystack.includes(deferredSearchQuery);
      })
      .sort((left, right) => {
        const leftTime = new Date(left.primaryTimestamp ?? left.updatedAt ?? left.createdAt).getTime();
        const rightTime = new Date(right.primaryTimestamp ?? right.updatedAt ?? right.createdAt).getTime();

        return sortOrder === "latest" ? rightTime - leftTime : leftTime - rightTime;
      });
  }, [assetsQuery.data, deferredSearchQuery, endpointLookup, mediaFilter, sortOrder, statusFilter]);

  const totalPages = Math.max(1, Math.ceil(filteredAssets.length / PAGE_SIZE));

  useEffect(() => {
    if (currentPage > totalPages) {
      setCurrentPage(totalPages);
    }
  }, [currentPage, totalPages]);

  const pagedAssets = filteredAssets.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE);

  return (
    <section className="page-stack">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">统一资产库</p>
          <h3>把分散在本地磁盘、NAS 和云端端点里的媒体收束进一套统一视图。</h3>
          <p>搜索放在全局入口，这里专注于浏览、筛选，以及更快地打开正确的资产。</p>
        </div>

        <div className="hero-metrics">
          <MetricCard label="资产总数" value={summary.totalAssets} tone="neutral" />
          <MetricCard label="完整可用" value={summary.readyAssets} tone="success" />
          <MetricCard label="部分缺失" value={summary.partialAssets} tone="warning" />
          <MetricCard label="仅单端存在" value={summary.singleAssets} tone="neutral" />
        </div>
      </article>

      <article className="detail-card catalog-toolbar">
        <div className="catalog-toolbar-head">
          <div>
            <p className="eyebrow">浏览</p>
            <h4>轻量筛选与时间排序</h4>
          </div>

          <div className="toolbar-search-state">
            {searchQuery ? <span>当前搜索：{searchQuery}</span> : <span>可通过顶部全局搜索进一步缩小范围。</span>}
          </div>
        </div>

        <div className="filter-stack">
          <div className="segmented-group" aria-label="媒体类型筛选">
            {mediaFilters.map((filter) => (
              <button
                key={filter.value}
                type="button"
                className={`segmented-button${mediaFilter === filter.value ? " active" : ""}`}
                onClick={() => setMediaFilter(filter.value)}
              >
                {filter.label}
              </button>
            ))}
          </div>

          <div className="segmented-group" aria-label="资产状态筛选">
            {statusFilters.map((filter) => (
              <button
                key={filter.value}
                type="button"
                className={`segmented-button${statusFilter === filter.value ? " active" : ""}`}
                onClick={() => setStatusFilter(filter.value)}
              >
                {filter.label}
              </button>
            ))}
          </div>

          <label className="field catalog-sort-field">
            <span>排序</span>
            <select value={sortOrder} onChange={(event) => setSortOrder(event.target.value as SortOrder)}>
              {sortOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </label>
        </div>
      </article>

      {assetsQuery.isError ? (
        <article className="detail-card empty-state">
          <SearchX size={28} />
          <div>
            <h4>暂时无法读取资产库</h4>
            <p>{assetsQuery.error instanceof Error ? assetsQuery.error.message : "请检查 Catalog 服务后再试。"}</p>
          </div>
        </article>
      ) : null}

      {assetsQuery.isLoading ? (
        <section className="asset-grid">
          {Array.from({ length: 6 }).map((_, index) => (
            <article key={`asset-skeleton-${index}`} className="asset-card skeleton-card">
              <div className="asset-visual skeleton-block" />
              <div className="asset-copy">
                <div className="skeleton-line short" />
                <div className="skeleton-line" />
                <div className="skeleton-line" />
              </div>
            </article>
          ))}
        </section>
      ) : null}

      {!assetsQuery.isLoading && !assetsQuery.isError && filteredAssets.length === 0 ? (
        <article className="detail-card empty-state">
          <SearchX size={28} />
          <div>
            <h4>当前条件下没有匹配资产</h4>
            <p>可以放宽筛选条件，或者先去存储管理执行一次扫描。</p>
          </div>
        </article>
      ) : null}

      {!assetsQuery.isLoading && !assetsQuery.isError && filteredAssets.length > 0 ? (
        <>
          <div className="catalog-result-meta">
            <span>共 {filteredAssets.length} 条结果</span>
            <span>
              第 {currentPage} / {totalPages} 页
            </span>
          </div>

          <section className="asset-grid">
            {pagedAssets.map((asset) => (
              <AssetCard
                key={asset.id}
                asset={asset}
                detailSearch={location.search}
                endpointLookup={endpointLookup}
              />
            ))}
          </section>

          <div className="pagination-row">
            <button
              type="button"
              className="ghost-button"
              onClick={() => setCurrentPage((page) => Math.max(1, page - 1))}
              disabled={currentPage === 1}
            >
              <ChevronLeft size={16} />
              上一页
            </button>

            <div className="pagination-indicator">
              <span>{currentPage}</span>
              <small>/ {totalPages}</small>
            </div>

            <button
              type="button"
              className="ghost-button"
              onClick={() => setCurrentPage((page) => Math.min(totalPages, page + 1))}
              disabled={currentPage === totalPages}
            >
              下一页
              <ChevronRight size={16} />
            </button>
          </div>
        </>
      ) : null}
    </section>
  );
}

function AssetCard({
  asset,
  detailSearch,
  endpointLookup
}: {
  asset: CatalogAsset;
  detailSearch: string;
  endpointLookup: Map<string, string>;
}) {
  const availableReplicaCount = getAvailableReplicaCount(asset);
  const missingReplicaCount = getMissingReplicaCount(asset);
  const statusTone = getAssetTone(asset);
  const MediaIcon = getMediaIcon(asset.mediaType);

  const detailParams = new URLSearchParams(detailSearch);
  detailParams.set("assetId", asset.id);

  return (
    <Link to={`/library?${detailParams.toString()}`} className="asset-card">
      <div className={`asset-visual tone-${statusTone}${asset.poster?.url ? " has-poster" : ""}`}>
        {asset.poster?.url ? (
          <img src={asset.poster.url} alt={asset.displayName} className="asset-poster" loading="lazy" />
        ) : (
          <MediaIcon size={28} strokeWidth={1.8} />
        )}
      </div>

      <div className="asset-copy">
        <div className="asset-card-head">
          <span className="asset-badge">{getMediaTypeLabel(asset.mediaType)}</span>
          <span className={`status-pill ${statusTone}`}>{getAssetStatusLabel(asset)}</span>
        </div>

        <div className="asset-title-block">
          <h4>{asset.displayName}</h4>
          <p>{asset.logicalPathKey}</p>
        </div>

        <div className="asset-meta-row">
          <span>{formatCatalogDate(asset.primaryTimestamp)}</span>
          <span>可用副本 {availableReplicaCount}</span>
          <span>缺失副本 {missingReplicaCount}</span>
        </div>

        <div className="replica-chip-row">
          {asset.replicas.slice(0, 4).map((replica) => (
            <span key={replica.id} className={`replica-chip ${getReplicaTone(replica)}`}>
              {endpointLookup.get(replica.endpointId) ?? replica.endpointId}
            </span>
          ))}
        </div>
      </div>

      <div className="asset-card-footer">
        <span>查看详情</span>
        <ArrowRight size={16} />
      </div>
    </Link>
  );
}

function MetricCard({ label, value, tone }: { label: string; value: number; tone: AssetTone }) {
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
      return Images;
  }
}
