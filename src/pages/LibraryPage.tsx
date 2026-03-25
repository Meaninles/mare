import { useDeferredValue, useEffect, useMemo, useState } from "react";
import { Link, useLocation, useSearchParams } from "react-router-dom";
import {
  AudioLines,
  ChevronRight,
  Clapperboard,
  Folder,
  FolderOpen,
  Home,
  Images,
  SearchX
} from "lucide-react";
import { AssetDetailPage } from "./AssetDetailPage";
import { useCatalogAssets, useCatalogEndpoints } from "../hooks/useCatalog";
import {
  formatCatalogDate,
  formatDurationSeconds,
  formatFileSize,
  getAssetStatusFilterValue,
  getAssetStatusLabel,
  getAssetTone,
  getAvailableReplicaCount,
  getMediaTypeLabel,
  getMissingReplicaCount,
  normalizeMediaType
} from "../lib/catalog-view";
import type { AssetTone } from "../lib/catalog-view";
import type { CatalogAsset } from "../types/catalog";

const collator = new Intl.Collator("zh-CN", {
  numeric: true,
  sensitivity: "base"
});

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
  { value: "name", label: "名称" },
  { value: "latest", label: "最新优先" },
  { value: "earliest", label: "最早优先" }
] as const;

type MediaFilterValue = (typeof mediaFilters)[number]["value"];
type StatusFilterValue = (typeof statusFilters)[number]["value"];
type SortOrder = (typeof sortOptions)[number]["value"];

type DecoratedAsset = {
  asset: CatalogAsset;
  directory: string;
  sortTimestamp: number;
  sizeLabel: string;
  locationLabel: string;
  endpointNames: string[];
};

type FolderSummary = {
  path: string;
  name: string;
  directAssetCount: number;
  assetCount: number;
  childFolderCount: number;
  latestTimestamp?: string;
  endpointCount: number;
};

type FolderIndex = {
  summaries: Map<string, FolderSummary>;
  children: Map<string, FolderSummary[]>;
};

type MutableFolderNode = {
  path: string;
  directAssetCount: number;
  assetCount: number;
  latestTimestamp?: string;
  latestTimestampValue: number;
  childFolders: Set<string>;
  endpointIds: Set<string>;
};

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
  const [searchParams, setSearchParams] = useSearchParams();
  const assetsQuery = useCatalogAssets();
  const endpointsQuery = useCatalogEndpoints();
  const [mediaFilter, setMediaFilter] = useState<MediaFilterValue>("all");
  const [statusFilter, setStatusFilter] = useState<StatusFilterValue>("all");
  const [sortOrder, setSortOrder] = useState<SortOrder>("name");

  const searchQuery = searchParams.get("q")?.trim() ?? "";
  const currentDirectory = normalizeHierarchyPath(searchParams.get("dir")?.trim() ?? "");
  const deferredSearchQuery = useDeferredValue(searchQuery.toLowerCase());
  const isSearchMode = deferredSearchQuery.length > 0;

  const endpointLookup = useMemo(() => {
    return new Map((endpointsQuery.data ?? []).map((endpoint) => [endpoint.id, endpoint.name]));
  }, [endpointsQuery.data]);

  const decoratedAssets = useMemo(() => {
    return (assetsQuery.data ?? []).map((asset) => decorateAsset(asset, endpointLookup));
  }, [assetsQuery.data, endpointLookup]);

  const summary = useMemo(() => {
    const assets = decoratedAssets.map((item) => item.asset);

    return {
      totalAssets: assets.length,
      readyAssets: assets.filter((asset) => getAssetStatusFilterValue(asset) === "ready").length,
      partialAssets: assets.filter((asset) => getAssetStatusFilterValue(asset) === "partial").length,
      singleAssets: assets.filter((asset) => getAssetStatusFilterValue(asset) === "single").length
    };
  }, [decoratedAssets]);

  const filteredAssets = useMemo(() => {
    return decoratedAssets.filter((item) => {
      const { asset, directory, endpointNames } = item;

      if (mediaFilter !== "all" && normalizeMediaType(asset.mediaType) !== mediaFilter) {
        return false;
      }

      if (statusFilter !== "all" && getAssetStatusFilterValue(asset) !== statusFilter) {
        return false;
      }

      if (!deferredSearchQuery) {
        return true;
      }

      const haystack = [
        asset.displayName,
        asset.logicalPathKey,
        asset.canonicalPath,
        directory,
        ...endpointNames
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase();

      return haystack.includes(deferredSearchQuery);
    });
  }, [decoratedAssets, deferredSearchQuery, mediaFilter, statusFilter]);

  const folderIndex = useMemo(() => buildFolderIndex(filteredAssets), [filteredAssets]);

  useEffect(() => {
    if (isSearchMode || !currentDirectory || folderIndex.summaries.has(currentDirectory)) {
      return;
    }

    const nextParams = new URLSearchParams(searchParams);
    nextParams.delete("dir");
    setSearchParams(nextParams, { replace: true });
  }, [currentDirectory, folderIndex.summaries, isSearchMode, searchParams, setSearchParams]);

  const visibleFolders = useMemo(() => {
    if (isSearchMode) {
      return [] as FolderSummary[];
    }

    return folderIndex.children.get(currentDirectory) ?? [];
  }, [currentDirectory, folderIndex.children, isSearchMode]);

  const visibleAssets = useMemo(() => {
    const scopedAssets = isSearchMode
      ? filteredAssets
      : filteredAssets.filter((item) => item.directory === currentDirectory);

    return [...scopedAssets].sort((left, right) => compareAssets(left, right, sortOrder));
  }, [currentDirectory, filteredAssets, isSearchMode, sortOrder]);

  const breadcrumbs = useMemo(() => getBreadcrumbs(currentDirectory), [currentDirectory]);

  const currentFolderSummary = !isSearchMode
    ? folderIndex.summaries.get(currentDirectory) ?? folderIndex.summaries.get("")
    : undefined;

  function openDirectory(path: string) {
    const nextParams = new URLSearchParams(searchParams);
    const normalized = normalizeHierarchyPath(path);

    if (normalized) {
      nextParams.set("dir", normalized);
    } else {
      nextParams.delete("dir");
    }

    setSearchParams(nextParams);
  }

  return (
    <section className="page-stack">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">统一资产库</p>
          <h3>资产</h3>
          <p>
            目录、子目录、文件行和元数据列都放回同一个列表里。你可以像看电脑文件夹一样看资产库，同时保留副本、状态和存储位置这些跨端信息。
          </p>
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
              <p className="eyebrow">资源管理器</p>
              <h4>列表</h4>
            </div>

            <div className="toolbar-search-state">
              {searchQuery ? (
                <span>搜索 {filteredAssets.length}</span>
              ) : (
                <span>
                  {!isSearchMode ? `目录 ${visibleFolders.length}` : "目录"}
                  {" · "}
                  文件 {visibleAssets.length}
                </span>
              )}
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
        <article className="detail-card empty-state">
          <FolderOpen size={28} />
          <div>
            <h4>正在加载目录视图</h4>
            <p>正在整理资产目录、文件状态和副本位置，请稍候。</p>
          </div>
        </article>
      ) : null}

      {!assetsQuery.isLoading && !assetsQuery.isError && filteredAssets.length === 0 ? (
        <article className="detail-card empty-state">
          <SearchX size={28} />
          <div>
            <h4>当前条件下没有匹配资产</h4>
            <p>可以放宽筛选条件，或先去存储管理执行一次扫描。</p>
          </div>
        </article>
      ) : null}

      {!assetsQuery.isLoading && !assetsQuery.isError && filteredAssets.length > 0 ? (
        <article className="detail-card explorer-shell">
          <div className="explorer-header">
            <div>
              <p className="eyebrow">{isSearchMode ? "搜索结果" : "目录视图"}</p>
              <h4>{isSearchMode ? "跨目录搜索结果" : currentDirectory ? getPathBaseName(currentDirectory) : "资产库根目录"}</h4>
            </div>

            <div className="explorer-summary">
              {!isSearchMode ? <span className="explorer-summary-pill">子目录 {visibleFolders.length}</span> : null}
              <span className="explorer-summary-pill">文件 {visibleAssets.length}</span>
              <span className="explorer-summary-pill">匹配资产 {filteredAssets.length}</span>
              {!isSearchMode && currentFolderSummary ? (
                <span className="explorer-summary-pill">目录内资产 {currentFolderSummary.assetCount}</span>
              ) : null}
            </div>
          </div>

          {!isSearchMode ? (
            <div className="explorer-toolbar">
              <div className="explorer-actions">
                <button
                  type="button"
                  className="ghost-button"
                  onClick={() => openDirectory("")}
                  disabled={!currentDirectory}
                >
                  <Home size={16} />
                  回到根目录
                </button>
                <button
                  type="button"
                  className="ghost-button"
                  onClick={() => openDirectory(getParentDirectory(currentDirectory))}
                  disabled={!currentDirectory}
                >
                  <FolderOpen size={16} />
                  返回上级
                </button>
              </div>

              <nav className="explorer-breadcrumbs" aria-label="当前目录路径">
                <button
                  type="button"
                  className={`breadcrumb-button${currentDirectory ? "" : " active"}`}
                  onClick={() => openDirectory("")}
                >
                  <Home size={14} />
                  根目录
                </button>

                {breadcrumbs.map((crumb) => (
                  <div key={crumb.path} className="breadcrumb-item">
                    <ChevronRight size={14} />
                    <button
                      type="button"
                      className={`breadcrumb-button${crumb.path === currentDirectory ? " active" : ""}`}
                      onClick={() => openDirectory(crumb.path)}
                    >
                      {crumb.label}
                    </button>
                  </div>
                ))}
              </nav>
            </div>
          ) : (
            <div className="explorer-search-banner">
              <span className="status-pill subtle">搜索</span>
              <strong>{searchQuery}</strong>
              <span>{filteredAssets.length}</span>
            </div>
          )}

          <div className="explorer-table-wrap">
            <table className="explorer-table">
              <thead>
                <tr>
                  <th>名称</th>
                  <th>类型</th>
                  <th>状态</th>
                  <th>修改时间</th>
                  <th>信息</th>
                  <th>副本</th>
                  <th>位置</th>
                </tr>
              </thead>
              <tbody>
                {visibleFolders.map((folder) => (
                  <ExplorerFolderRow key={folder.path} folder={folder} onOpenDirectory={openDirectory} />
                ))}

                {visibleAssets.map((item) => (
                  <ExplorerAssetRow key={item.asset.id} item={item} detailSearch={location.search} />
                ))}
              </tbody>
            </table>
          </div>
        </article>
      ) : null}
    </section>
  );
}

function ExplorerFolderRow({
  folder,
  onOpenDirectory
}: {
  folder: FolderSummary;
  onOpenDirectory: (path: string) => void;
}) {
  return (
    <tr className="explorer-row folder-row">
      <td>
        <div className="explorer-name-cell">
          <div className="explorer-icon explorer-folder-icon">
            <Folder size={18} />
          </div>

          <div className="explorer-name-copy">
            <button type="button" className="explorer-row-button" onClick={() => onOpenDirectory(folder.path)}>
              {folder.name}
            </button>
            <p className="explorer-subtitle">{folder.path}</p>
          </div>
        </div>
      </td>
      <td>文件夹</td>
      <td>{folder.assetCount} 个资产</td>
      <td>{formatCatalogDate(folder.latestTimestamp)}</td>
      <td>
        {folder.directAssetCount} 个直接文件
        {folder.childFolderCount > 0 ? ` · ${folder.childFolderCount} 个子目录` : ""}
      </td>
      <td>-</td>
      <td>{folder.endpointCount} 个位置</td>
    </tr>
  );
}

function ExplorerAssetRow({
  item,
  detailSearch
}: {
  item: DecoratedAsset;
  detailSearch: string;
}) {
  const { asset } = item;
  const tone = getAssetTone(asset);
  const MediaIcon = getMediaIcon(asset.mediaType);
  const detailParams = new URLSearchParams(detailSearch);
  detailParams.set("assetId", asset.id);

  return (
    <tr className="explorer-row">
      <td>
        <div className="explorer-name-cell">
          <div className={`explorer-icon tone-${tone}${asset.poster?.url ? " has-poster" : ""}`}>
            {asset.poster?.url ? (
              <img src={asset.poster.url} alt={asset.displayName} className="explorer-poster" loading="lazy" />
            ) : (
              <MediaIcon size={18} strokeWidth={1.8} />
            )}
          </div>

          <div className="explorer-name-copy">
            <Link to={`/assets?${detailParams.toString()}`} className="explorer-link">
              {asset.displayName}
            </Link>
            <p className="explorer-subtitle">{asset.logicalPathKey}</p>
          </div>
        </div>
      </td>
      <td>{getMediaTypeLabel(asset.mediaType)}</td>
      <td className="explorer-status-cell">
        <span className={`status-pill ${tone}`}>{getAssetStatusLabel(asset)}</span>
      </td>
      <td>{formatCatalogDate(getAssetTimestamp(asset))}</td>
      <td>{item.sizeLabel}</td>
      <td>{formatReplicaSummary(asset)}</td>
      <td>{item.locationLabel}</td>
    </tr>
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

function decorateAsset(asset: CatalogAsset, endpointLookup: Map<string, string>): DecoratedAsset {
  const directory = resolveAssetDirectory(asset);
  const endpointNames = uniqueText(
    asset.replicas.map((replica) => endpointLookup.get(replica.endpointId) ?? replica.endpointId)
  );

  return {
    asset,
    directory,
    sortTimestamp: resolveAssetTimestampValue(asset),
    sizeLabel: getAssetInfoLabel(asset),
    locationLabel: formatEndpointSummary(endpointNames),
    endpointNames
  };
}

function buildFolderIndex(assets: DecoratedAsset[]): FolderIndex {
  const nodes = new Map<string, MutableFolderNode>();

  function ensureNode(path: string) {
    const normalized = normalizeHierarchyPath(path);
    const existing = nodes.get(normalized);
    if (existing) {
      return existing;
    }

    const created: MutableFolderNode = {
      path: normalized,
      directAssetCount: 0,
      assetCount: 0,
      latestTimestampValue: 0,
      childFolders: new Set<string>(),
      endpointIds: new Set<string>()
    };

    nodes.set(normalized, created);
    return created;
  }

  ensureNode("");

  for (const item of assets) {
    const lineage = [""];
    let parentPath = "";

    for (const segment of splitHierarchyPath(item.directory)) {
      const nextPath = parentPath ? `${parentPath}/${segment}` : segment;
      ensureNode(parentPath).childFolders.add(nextPath);
      parentPath = nextPath;
      lineage.push(nextPath);
    }

    ensureNode(item.directory).directAssetCount += 1;

    for (const path of lineage) {
      const node = ensureNode(path);
      node.assetCount += 1;

      if (item.sortTimestamp > node.latestTimestampValue) {
        node.latestTimestampValue = item.sortTimestamp;
        node.latestTimestamp = getAssetTimestamp(item.asset);
      }

      for (const replica of item.asset.replicas) {
        node.endpointIds.add(replica.endpointId);
      }
    }
  }

  const summaries = new Map<string, FolderSummary>();

  for (const node of nodes.values()) {
    summaries.set(node.path, {
      path: node.path,
      name: node.path ? getPathBaseName(node.path) : "根目录",
      directAssetCount: node.directAssetCount,
      assetCount: node.assetCount,
      childFolderCount: node.childFolders.size,
      latestTimestamp: node.latestTimestamp,
      endpointCount: node.endpointIds.size
    });
  }

  const children = new Map<string, FolderSummary[]>();

  for (const node of nodes.values()) {
    const folderSummaries = [...node.childFolders]
      .map((path) => summaries.get(path))
      .filter((folder): folder is FolderSummary => Boolean(folder))
      .sort((left, right) => collator.compare(left.name, right.name));

    children.set(node.path, folderSummaries);
  }

  return { summaries, children };
}

function compareAssets(left: DecoratedAsset, right: DecoratedAsset, sortOrder: SortOrder) {
  if (sortOrder === "name") {
    const byName = collator.compare(left.asset.displayName, right.asset.displayName);
    return byName !== 0
      ? byName
      : collator.compare(left.asset.logicalPathKey, right.asset.logicalPathKey);
  }

  const delta =
    sortOrder === "latest"
      ? right.sortTimestamp - left.sortTimestamp
      : left.sortTimestamp - right.sortTimestamp;

  if (delta !== 0) {
    return delta;
  }

  return collator.compare(left.asset.displayName, right.asset.displayName);
}

function formatReplicaSummary(asset: CatalogAsset) {
  const available = getAvailableReplicaCount(asset);
  const missing = getMissingReplicaCount(asset);
  return `${available} 可用 / ${missing} 缺失`;
}

function getAssetInfoLabel(asset: CatalogAsset) {
  const size = getPrimaryReplicaSize(asset);
  const duration = asset.audioMetadata?.durationSeconds;

  if (size && duration) {
    return `${formatFileSize(size)} · ${formatDurationSeconds(duration)}`;
  }

  if (size) {
    return formatFileSize(size);
  }

  if (duration) {
    return formatDurationSeconds(duration);
  }

  return "待补充";
}

function getPrimaryReplicaSize(asset: CatalogAsset) {
  const sizes = asset.replicas
    .map((replica) => replica.version?.size)
    .filter((size): size is number => typeof size === "number" && size > 0);

  if (sizes.length === 0) {
    return undefined;
  }

  return Math.max(...sizes);
}

function resolveAssetDirectory(asset: CatalogAsset) {
  const candidates = [
    asset.canonicalDirectory,
    ...asset.replicas.map((replica) => replica.logicalDirectory),
    ...asset.replicas.map((replica) => replica.resolvedDirectory),
    getPathDirectory(asset.canonicalPath),
    getPathDirectory(asset.logicalPathKey)
  ];

  for (const candidate of candidates) {
    const normalized = normalizeHierarchyPath(candidate);
    if (normalized || candidate === "") {
      return normalized;
    }
  }

  return "";
}

function getAssetTimestamp(asset: CatalogAsset) {
  return asset.primaryTimestamp ?? asset.updatedAt ?? asset.createdAt;
}

function resolveAssetTimestampValue(asset: CatalogAsset) {
  const raw = getAssetTimestamp(asset);
  if (!raw) {
    return 0;
  }

  const parsed = new Date(raw).getTime();
  return Number.isNaN(parsed) ? 0 : parsed;
}

function formatEndpointSummary(endpointNames: string[]) {
  if (endpointNames.length === 0) {
    return "未记录位置";
  }

  if (endpointNames.length <= 2) {
    return endpointNames.join(" / ");
  }

  return `${endpointNames.slice(0, 2).join(" / ")} +${endpointNames.length - 2}`;
}

function getBreadcrumbs(path: string) {
  const segments = splitHierarchyPath(path);
  const breadcrumbs: Array<{ path: string; label: string }> = [];
  let currentPath = "";

  for (const segment of segments) {
    currentPath = currentPath ? `${currentPath}/${segment}` : segment;
    breadcrumbs.push({ path: currentPath, label: segment });
  }

  return breadcrumbs;
}

function getParentDirectory(path: string) {
  const segments = splitHierarchyPath(path);
  if (segments.length <= 1) {
    return "";
  }

  return segments.slice(0, -1).join("/");
}

function getPathDirectory(path?: string) {
  const normalized = normalizeHierarchyPath(path);
  if (!normalized) {
    return "";
  }

  const segments = splitHierarchyPath(normalized);
  if (segments.length <= 1) {
    return "";
  }

  return segments.slice(0, -1).join("/");
}

function getPathBaseName(path: string) {
  const segments = splitHierarchyPath(path);
  return segments[segments.length - 1] ?? path;
}

function splitHierarchyPath(path?: string) {
  const normalized = normalizeHierarchyPath(path);
  return normalized ? normalized.split("/") : [];
}

function normalizeHierarchyPath(path?: string) {
  if (!path) {
    return "";
  }

  return path
    .replace(/\\/g, "/")
    .replace(/\/+/g, "/")
    .replace(/^\/+|\/+$/g, "")
    .trim();
}

function uniqueText(values: Array<string | undefined>) {
  return [...new Set(values.map((value) => value?.trim()).filter((value): value is string => Boolean(value)))];
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
