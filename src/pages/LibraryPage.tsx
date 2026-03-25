import { useEffect, useMemo, useState, type FormEvent } from "react";
import { Link, useLocation, useOutletContext, useSearchParams } from "react-router-dom";
import {
  ArrowUpDown,
  AudioLines,
  BellRing,
  CheckSquare,
  Clapperboard,
  Folder,
  FolderOpen,
  Home,
  Images,
  Search,
  SearchX,
  Square,
  Trash2,
  WandSparkles
} from "lucide-react";
import { AssetDetailPage } from "./AssetDetailPage";
import type { AppShellOutletContext } from "../layouts/AppShell";
import {
  useCatalogAssets,
  useCatalogBatchRestore,
  useCatalogDeleteReplica,
  useCatalogEndpoints
} from "../hooks/useCatalog";
import {
  formatCatalogDate,
  formatDurationSeconds,
  formatFileSize,
  getAssetStatusFilterValue,
  getAssetTone,
  getAvailableReplicaCount,
  getMediaTypeLabel,
  getMissingReplicaCount,
  normalizeMediaType
} from "../lib/catalog-view";
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
  endpointStates: EndpointState[];
};

type EndpointState = {
  id: string;
  name: string;
  exists: boolean;
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
  const { openTaskCenter, notificationCount, hasNotificationAlert } =
    useOutletContext<AppShellOutletContext>();
  const [mediaFilter, setMediaFilter] = useState<MediaFilterValue>("all");
  const [statusFilter, setStatusFilter] = useState<StatusFilterValue>("all");
  const [sortOrder, setSortOrder] = useState<SortOrder>("name");
  const [selectedAssetIds, setSelectedAssetIds] = useState<string[]>([]);
  const [selectedEndpointId, setSelectedEndpointId] = useState("");
  const [bulkNotice, setBulkNotice] = useState<string | null>(null);

  const searchQuery = searchParams.get("q")?.trim() ?? "";
  const [searchValue, setSearchValue] = useState(searchQuery);
  const currentDirectory = normalizeHierarchyPath(searchParams.get("dir")?.trim() ?? "");
  const isSearchMode = searchQuery.length > 0;
  const assetsQuery = useCatalogAssets({
    limit: 1000,
    query: searchQuery,
    mediaType: mediaFilter !== "all" ? mediaFilter : undefined,
    assetStatus: statusFilter !== "all" ? statusFilter : undefined
  });
  const endpointsQuery = useCatalogEndpoints();
  const batchRestoreMutation = useCatalogBatchRestore();
  const deleteReplicaMutation = useCatalogDeleteReplica();

  const endpointLookup = useMemo(() => {
    return new Map((endpointsQuery.data ?? []).map((endpoint) => [endpoint.id, endpoint.name]));
  }, [endpointsQuery.data]);
  const managedEndpoints = useMemo(() => {
    return [...(endpointsQuery.data ?? [])]
      .filter((endpoint) => endpoint.roleMode.trim().toUpperCase() === "MANAGED")
      .sort((left, right) => left.name.localeCompare(right.name, "zh-CN"));
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

  const filteredAssets = decoratedAssets;

  const folderIndex = useMemo(() => buildFolderIndex(filteredAssets), [filteredAssets]);

  useEffect(() => {
    setSearchValue(searchQuery);
  }, [searchQuery]);

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
  const selectedAssets = useMemo(() => {
    const selectedSet = new Set(selectedAssetIds);
    return decoratedAssets.filter((item) => selectedSet.has(item.asset.id));
  }, [decoratedAssets, selectedAssetIds]);
  const visibleAssetIds = useMemo(() => visibleAssets.map((item) => item.asset.id), [visibleAssets]);
  const selectedVisibleCount = useMemo(() => {
    const selectedSet = new Set(selectedAssetIds);
    return visibleAssetIds.filter((assetId) => selectedSet.has(assetId)).length;
  }, [selectedAssetIds, visibleAssetIds]);
  const bulkTargetEndpoint = useMemo(
    () => managedEndpoints.find((endpoint) => endpoint.id === selectedEndpointId) ?? null,
    [managedEndpoints, selectedEndpointId]
  );
  const currentFolderSummary = !isSearchMode
    ? folderIndex.summaries.get(currentDirectory) ?? folderIndex.summaries.get("")
    : undefined;

  useEffect(() => {
    setSelectedEndpointId((current) =>
      managedEndpoints.some((endpoint) => endpoint.id === current) ? current : managedEndpoints[0]?.id ?? ""
    );
  }, [managedEndpoints]);

  useEffect(() => {
    const validAssetIds = new Set(decoratedAssets.map((item) => item.asset.id));
    setSelectedAssetIds((current) => current.filter((assetId) => validAssetIds.has(assetId)));
  }, [decoratedAssets]);

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

  function handleSearchSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const nextParams = new URLSearchParams(searchParams);
    const normalizedQuery = searchValue.trim();

    if (normalizedQuery) {
      nextParams.set("q", normalizedQuery);
    } else {
      nextParams.delete("q");
    }

    setSearchParams(nextParams);
  }

  function toggleAssetSelection(assetId: string) {
    setSelectedAssetIds((current) =>
      current.includes(assetId) ? current.filter((id) => id !== assetId) : [...current, assetId]
    );
  }

  function toggleVisibleSelection() {
    if (visibleAssetIds.length === 0) {
      return;
    }

    setSelectedAssetIds((current) => {
      const currentSet = new Set(current);
      const allVisibleSelected = visibleAssetIds.every((assetId) => currentSet.has(assetId));
      if (allVisibleSelected) {
        return current.filter((assetId) => !visibleAssetIds.includes(assetId));
      }

      const next = new Set(current);
      visibleAssetIds.forEach((assetId) => next.add(assetId));
      return Array.from(next);
    });
  }

  async function handleBatchSync() {
    if (!bulkTargetEndpoint) {
      setBulkNotice("请先选择一个目标端。");
      return;
    }
    if (selectedAssetIds.length === 0) {
      setBulkNotice("请先选择至少一个资产。");
      return;
    }

    setBulkNotice(null);
    try {
      const summary = await batchRestoreMutation.mutateAsync({
        targetEndpointId: bulkTargetEndpoint.id,
        assetIds: selectedAssetIds
      });
      setSelectedAssetIds([]);
      setBulkNotice(summary.progressLabel || `已将 ${selectedAssetIds.length} 个资产加入传输队列，目标为 ${bulkTargetEndpoint.name}。`);
    } catch (error) {
      setBulkNotice(error instanceof Error ? error.message : "批量同步失败。");
    }
  }

  async function handleBatchDelete() {
    if (!bulkTargetEndpoint) {
      setBulkNotice("请先选择一个删除端。");
      return;
    }
    if (selectedAssets.length === 0) {
      setBulkNotice("请先选择至少一个资产。");
      return;
    }

    const deletableAssets = selectedAssets.filter((item) =>
      item.asset.replicas.some((replica) => replica.endpointId === bulkTargetEndpoint.id && replica.existsFlag)
    );
    if (deletableAssets.length === 0) {
      setBulkNotice(`${bulkTargetEndpoint.name} 上没有可删除的已存储副本。`);
      return;
    }
    if (!window.confirm(`确认从 ${bulkTargetEndpoint.name} 删除 ${deletableAssets.length} 个已选资产的副本吗？`)) {
      return;
    }

    setBulkNotice(null);
    let successCount = 0;
    let failedCount = 0;
    const skippedCount = selectedAssets.length - deletableAssets.length;

    for (const item of deletableAssets) {
      try {
        await deleteReplicaMutation.mutateAsync({
          assetId: item.asset.id,
          targetEndpointId: bulkTargetEndpoint.id
        });
        successCount += 1;
      } catch {
        failedCount += 1;
      }
    }

    setBulkNotice(
      `${bulkTargetEndpoint.name}：删除 ${successCount}，跳过 ${skippedCount}${failedCount > 0 ? `，失败 ${failedCount}` : ""}`
    );
  }

  return (
    <section className="page-stack">
      <article className="detail-card compact-page-header library-page-header">
        <div className="compact-page-header-main">
          <div className="compact-page-header-title">
            <h3>资产</h3>
            <div className="replica-chip-row compact-page-header-metrics">
              <span className="replica-chip neutral">资产总数 {summary.totalAssets}</span>
              <span className="replica-chip success">完整可用 {summary.readyAssets}</span>
              <span className="replica-chip warning">部分缺失 {summary.partialAssets}</span>
              <span className="replica-chip danger">仅单端存在 {summary.singleAssets}</span>
              {isSearchMode ? (
                <span className="replica-chip neutral">搜索结果 {filteredAssets.length}</span>
              ) : (
                <>
                  <span className="replica-chip neutral">子目录 {visibleFolders.length}</span>
                  <span className="replica-chip neutral">文件 {visibleAssets.length}</span>
                  {currentDirectory && currentFolderSummary ? (
                    <span className="replica-chip neutral">目录内资产 {currentFolderSummary.assetCount}</span>
                  ) : null}
                </>
              )}
            </div>
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

        <div className="compact-page-header-actions">
          <form className="shell-search shell-search-minimal library-page-search" onSubmit={handleSearchSubmit}>
            <Search size={17} strokeWidth={1.9} />
            <input
              value={searchValue}
              onChange={(event) => setSearchValue(event.target.value)}
              placeholder="搜索资产"
              aria-label="搜索资产"
            />
          </form>

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

          <div className="catalog-sort-field compact-filter-control compact-sort-control">
            <ArrowUpDown size={16} aria-hidden="true" />
            <select
              aria-label="排序方式"
              value={sortOrder}
              onChange={(event) => setSortOrder(event.target.value as SortOrder)}
            >
              {sortOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </div>

          <button
            type="button"
            className={`ghost-button icon-button shell-action-button task-center-icon library-page-notice-button${
              hasNotificationAlert ? " has-alert" : ""
            }`}
            onClick={openTaskCenter}
            aria-label="打开通知中心"
            title="通知中心"
          >
            <BellRing size={18} />
            {notificationCount > 0 ? <span className="task-center-badge">{notificationCount}</span> : null}
          </button>
        </div>
      </article>

      {bulkNotice ? <p className="inline-note">{bulkNotice}</p> : null}

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
          {!isSearchMode && currentDirectory ? (
            <div className="explorer-pathbar">
              <button
                type="button"
                className="ghost-button"
                onClick={() => openDirectory(getParentDirectory(currentDirectory))}
              >
                <FolderOpen size={16} />
                返回上级
              </button>

              <button
                type="button"
                className="ghost-button"
                onClick={() => openDirectory("")}
              >
                <Home size={16} />
                根目录
              </button>

              <span className="status-pill subtle explorer-path-pill" title={currentDirectory}>
                {currentDirectory}
              </span>
            </div>
          ) : null}

          <div className="explorer-bulkbar">
            <div className="replica-chip-row explorer-bulk-summary">
              <span className="replica-chip neutral">已选 {selectedAssetIds.length}</span>
              {bulkTargetEndpoint ? <span className="replica-chip neutral">目标 {bulkTargetEndpoint.name}</span> : null}
            </div>

            <div className="explorer-bulk-actions">
              <div className="catalog-sort-field compact-filter-control compact-sort-control explorer-sort-control">
                <ArrowUpDown size={16} aria-hidden="true" />
                <select
                  aria-label="鎺掑簭鏂瑰紡"
                  value={sortOrder}
                  onChange={(event) => setSortOrder(event.target.value as SortOrder)}
                >
                  {sortOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </div>

              <button type="button" className="ghost-button" onClick={toggleVisibleSelection} disabled={visibleAssetIds.length === 0}>
                {selectedVisibleCount === visibleAssetIds.length && visibleAssetIds.length > 0 ? (
                  <CheckSquare size={16} />
                ) : (
                  <Square size={16} />
                )}
                当前可见
              </button>

              <button type="button" className="ghost-button" onClick={() => setSelectedAssetIds([])} disabled={selectedAssetIds.length === 0}>
                清空
              </button>

              <label className="field explorer-bulk-field">
                <span>目标端</span>
                <select value={selectedEndpointId} onChange={(event) => setSelectedEndpointId(event.target.value)}>
                  <option value="">请选择</option>
                  {managedEndpoints.map((endpoint) => (
                    <option key={endpoint.id} value={endpoint.id}>
                      {endpoint.name}
                    </option>
                  ))}
                </select>
              </label>

              <button
                type="button"
                className="primary-button"
                onClick={() => void handleBatchSync()}
                disabled={batchRestoreMutation.isPending || selectedAssetIds.length === 0 || !bulkTargetEndpoint}
              >
                <WandSparkles size={16} />
                同步到端
              </button>

              <button
                type="button"
                className="ghost-button danger-text"
                onClick={() => void handleBatchDelete()}
                disabled={deleteReplicaMutation.isPending || selectedAssetIds.length === 0 || !bulkTargetEndpoint}
              >
                <Trash2 size={16} />
                从端删除
              </button>
            </div>
          </div>

          <div className="explorer-table-wrap">
            <table className="explorer-table">
              <thead>
                <tr>
                  <th className="explorer-select-col">
                    <button
                      type="button"
                      className="explorer-select-toggle"
                      onClick={toggleVisibleSelection}
                      disabled={visibleAssetIds.length === 0}
                      title={selectedVisibleCount === visibleAssetIds.length && visibleAssetIds.length > 0 ? "取消当前可见" : "选择当前可见"}
                    >
                      {selectedVisibleCount === visibleAssetIds.length && visibleAssetIds.length > 0 ? (
                        <CheckSquare size={16} />
                      ) : (
                        <Square size={16} />
                      )}
                    </button>
                  </th>
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
                  <ExplorerAssetRow
                    key={item.asset.id}
                    item={item}
                    detailSearch={location.search}
                    selected={selectedAssetIds.includes(item.asset.id)}
                    onToggleSelect={toggleAssetSelection}
                  />
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
      <td className="explorer-select-cell" />
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
  detailSearch,
  selected,
  onToggleSelect
}: {
  item: DecoratedAsset;
  detailSearch: string;
  selected: boolean;
  onToggleSelect: (assetId: string) => void;
}) {
  const { asset } = item;
  const tone = getAssetTone(asset);
  const MediaIcon = getMediaIcon(asset.mediaType);
  const detailParams = new URLSearchParams(detailSearch);
  detailParams.set("assetId", asset.id);

  return (
    <tr className="explorer-row">
      <td className="explorer-select-cell">
        <button
          type="button"
          className={`explorer-selection-button${selected ? " is-selected" : ""}`}
          onClick={() => onToggleSelect(asset.id)}
          title={selected ? "取消选择" : "选择资产"}
        >
          {selected ? <CheckSquare size={16} /> : <Square size={16} />}
        </button>
      </td>
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
      <td className="explorer-source-cell">
        <div className="source-status-list" aria-label="数据源状态">
          {item.endpointStates.map((endpoint) => (
            <span
              key={endpoint.id}
              className={`source-status-chip ${endpoint.exists ? "is-online" : "is-offline"}`}
              title={`${endpoint.name} ${endpoint.exists ? "可用" : "缺失"}`}
            >
              <span className="source-status-dot" aria-hidden="true" />
              <span className="source-status-name">{endpoint.name}</span>
            </span>
          ))}
        </div>
      </td>
      <td>{formatCatalogDate(getAssetTimestamp(asset))}</td>
      <td>{item.sizeLabel}</td>
    </tr>
  );
}

function decorateAsset(asset: CatalogAsset, endpointLookup: Map<string, string>): DecoratedAsset {
  const directory = resolveAssetDirectory(asset);

  return {
    asset,
    directory,
    sortTimestamp: resolveAssetTimestampValue(asset),
    sizeLabel: getAssetInfoLabel(asset),
    endpointStates: buildEndpointStates(asset, endpointLookup)
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

function getParentDirectory(path: string) {
  const segments = splitHierarchyPath(path);
  if (segments.length <= 1) {
    return "";
  }

  return segments.slice(0, -1).join("/");
}

function buildEndpointStates(asset: CatalogAsset, endpointLookup: Map<string, string>): EndpointState[] {
  const states = new Map<string, EndpointState>();

  for (const replica of asset.replicas) {
    const existing = states.get(replica.endpointId);
    const nextState: EndpointState = {
      id: replica.endpointId,
      name: endpointLookup.get(replica.endpointId) ?? replica.endpointId,
      exists: replica.existsFlag
    };

    if (!existing || (!existing.exists && replica.existsFlag)) {
      states.set(replica.endpointId, nextState);
    }
  }

  return [...states.values()].sort((left, right) => {
    if (left.exists !== right.exists) {
      return left.exists ? -1 : 1;
    }

    return collator.compare(left.name, right.name);
  });
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
