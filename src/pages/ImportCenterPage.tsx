import { useEffect, useMemo, useState, type ReactNode } from "react";
import { CheckCircle2, FolderOpen, HardDrive, Image as ImageIcon, LoaderCircle, Music4, RefreshCcw, Route, Settings2, Upload, Video } from "lucide-react";
import { Link, useSearchParams } from "react-router-dom";
import { useCatalogEndpoints, useCatalogTasks } from "../hooks/useCatalog";
import { useExecuteImport, useImportDevices, useImportRules, useImportSource, useSaveImportRules, useSelectImportDeviceRole } from "../hooks/useImport";
import { formatCatalogDate, formatFileSize } from "../lib/catalog-view";
import { getTaskDisplaySummary } from "../lib/task-center";
import type { CatalogEndpoint, CatalogTask } from "../types/catalog";
import type { ImportBrowseMediaType, ImportExecutionSummary, ImportRuleInput, ImportSourceEntry } from "../types/import";

type ManagedMediaType = "image" | "video" | "audio";
type ExtensionRuleDraft = { id: string; extension: string; targetEndpointIds: string[] };

const MEDIA_FILTERS: Array<{ value: ImportBrowseMediaType; label: string }> = [
  { value: "all", label: "全部" },
  { value: "image", label: "图片" },
  { value: "video", label: "视频" },
  { value: "audio", label: "音频" }
];

const MEDIA_RULES: Array<{ value: ManagedMediaType; label: string }> = [
  { value: "image", label: "图片" },
  { value: "video", label: "视频" },
  { value: "audio", label: "音频" }
];

export function ImportCenterPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const devicesQuery = useImportDevices();
  const endpointsQuery = useCatalogEndpoints();
  const tasksQuery = useCatalogTasks(24);
  const rulesQuery = useImportRules();
  const selectRoleMutation = useSelectImportDeviceRole();
  const saveRulesMutation = useSaveImportRules();
  const executeImportMutation = useExecuteImport();

  const [mediaFilter, setMediaFilter] = useState<ImportBrowseMediaType>("all");
  const [selectedPaths, setSelectedPaths] = useState<string[]>([]);
  const [mediaRuleTargets, setMediaRuleTargets] = useState<Record<ManagedMediaType, string[]>>({ image: [], video: [], audio: [] });
  const [extensionRules, setExtensionRules] = useState<ExtensionRuleDraft[]>([{ id: `draft-${crypto.randomUUID()}`, extension: "", targetEndpointIds: [] }]);
  const [lastSummary, setLastSummary] = useState<ImportExecutionSummary | null>(null);

  const devices = devicesQuery.data ?? [];
  const activeManagedEndpoints = useMemo(() => {
    return (endpointsQuery.data ?? []).filter(
      (endpoint) => endpoint.roleMode.toUpperCase() === "MANAGED" && endpoint.availabilityStatus.toUpperCase() !== "DISABLED"
    );
  }, [endpointsQuery.data]);

  const selectedIdentity = searchParams.get("device") ?? "";
  const selectedDevice =
    devices.find((device) => device.identitySignature === selectedIdentity) ??
    devices.find((device) => device.currentSessionRole === "import_source") ??
    devices[0] ??
    null;

  useEffect(() => {
    const nextIdentity = selectedDevice?.identitySignature ?? "";
    if (!nextIdentity || nextIdentity === selectedIdentity) {
      return;
    }
    const next = new URLSearchParams(searchParams);
    next.set("device", nextIdentity);
    setSearchParams(next, { replace: true });
  }, [searchParams, selectedDevice?.identitySignature, selectedIdentity, setSearchParams]);

  const sourceQuery = useImportSource(selectedDevice?.identitySignature ?? "", mediaFilter, 1000);
  const sourceEntries = sourceQuery.data?.entries ?? [];
  const groupedSourceEntries = useMemo(() => {
    const groups = new Map<string, ImportSourceEntry[]>();

    sourceEntries.forEach((entry) => {
      const directory = getImportDirectory(entry.relativePath);
      const items = groups.get(directory);
      if (items) {
        items.push(entry);
        return;
      }
      groups.set(directory, [entry]);
    });

    return Array.from(groups.entries())
      .map(([path, entries]) => ({
        path,
        name: getImportFolderName(path),
        entries: [...entries].sort((left, right) => left.name.localeCompare(right.name, "zh-CN"))
      }))
      .sort((left, right) => {
        if (!left.path && right.path) {
          return -1;
        }
        if (left.path && !right.path) {
          return 1;
        }
        return left.path.localeCompare(right.path, "zh-CN");
      });
  }, [sourceEntries]);
  const importTasks = (tasksQuery.data ?? []).filter((task) => task.taskType.toLowerCase().includes("import"));
  const selectedEntries = sourceEntries.filter((entry) => selectedPaths.includes(entry.relativePath));
  const configuredRuleCount =
    Object.values(mediaRuleTargets).filter((targetIds) => targetIds.length > 0).length +
    extensionRules.filter((rule) => rule.extension.trim() && rule.targetEndpointIds.length > 0).length;

  useEffect(() => {
    setSelectedPaths((current) => current.filter((path) => sourceEntries.some((entry) => entry.relativePath === path)));
  }, [sourceEntries]);

  useEffect(() => {
    if (!rulesQuery.data) {
      return;
    }
    const nextMediaTargets: Record<ManagedMediaType, string[]> = { image: [], video: [], audio: [] };
    const nextExtensionRules: ExtensionRuleDraft[] = [];
    for (const rule of rulesQuery.data) {
      if (rule.ruleType === "media_type" && (rule.matchValue === "image" || rule.matchValue === "video" || rule.matchValue === "audio")) {
        nextMediaTargets[rule.matchValue] = [...rule.targetEndpointIds];
      }
      if (rule.ruleType === "extension") {
        nextExtensionRules.push({ id: rule.id, extension: rule.matchValue, targetEndpointIds: [...rule.targetEndpointIds] });
      }
    }
    setMediaRuleTargets(nextMediaTargets);
    setExtensionRules(nextExtensionRules.length > 0 ? nextExtensionRules : [{ id: `draft-${crypto.randomUUID()}`, extension: "", targetEndpointIds: [] }]);
  }, [rulesQuery.data]);

  async function handleChooseRole(identitySignature: string, role: "managed_storage" | "import_source", name?: string) {
    const result = await selectRoleMutation.mutateAsync({ identitySignature, role, name });
    if (role === "import_source") {
      const next = new URLSearchParams(searchParams);
      next.set("device", result.device.identitySignature);
      setSearchParams(next);
    }
  }

  async function handleSaveRules() {
    const rules: ImportRuleInput[] = [
      ...MEDIA_RULES.map((rule) => ({ ruleType: "media_type" as const, matchValue: rule.value, targetEndpointIds: mediaRuleTargets[rule.value] })),
      ...extensionRules.map((rule) => ({ ruleType: "extension" as const, matchValue: rule.extension, targetEndpointIds: rule.targetEndpointIds }))
    ];
    await saveRulesMutation.mutateAsync(rules);
  }

  async function handleImport() {
    if (!selectedDevice || selectedEntries.length === 0) {
      return;
    }
    const summary = await executeImportMutation.mutateAsync({
      identitySignature: selectedDevice.identitySignature,
      entryPaths: selectedEntries.map((entry) => entry.relativePath)
    });
    setLastSummary(summary);
    setSelectedPaths([]);
  }

  function toggleEntrySelection(relativePath: string) {
    setSelectedPaths((current) =>
      current.includes(relativePath)
        ? current.filter((item) => item !== relativePath)
        : [...current, relativePath]
    );
  }

  function toggleFolderSelection(entries: ImportSourceEntry[]) {
    const folderPaths = entries.map((entry) => entry.relativePath);
    const folderPathSet = new Set(folderPaths);
    const isAllSelected = folderPaths.every((path) => selectedPaths.includes(path));

    setSelectedPaths((current) => {
      if (isAllSelected) {
        return current.filter((path) => !folderPathSet.has(path));
      }

      return Array.from(new Set([...current, ...folderPaths]));
    });
  }

  return (
    <section className="page-stack import-page-shell">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">导入中心</p>
          <h3>导入</h3>
          <p>左侧专注设备和文件，右侧负责目标、规则和结果，导入链路会更直观。</p>
        </div>
        <div className="hero-metrics">
          <MetricCard label="已连接设备" value={devices.length} tone="neutral" />
          <MetricCard label="管理端点" value={activeManagedEndpoints.length} tone="success" />
          <MetricCard label="已勾选条目" value={selectedEntries.length} tone="warning" />
        </div>
      </article>

      <div className="page-grid import-layout import-page-grid">
        <article className="detail-card import-source-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">来源设备</p>
              <h4>设备</h4>
            </div>
            <button type="button" className="ghost-button" onClick={() => void devicesQuery.refetch()} disabled={devicesQuery.isFetching}>
              {devicesQuery.isFetching ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
              刷新
            </button>
          </div>

          {devicesQuery.isLoading ? <EmptyBlock icon={<LoaderCircle size={20} className="spin" />} title="正在检测设备" copy="Mare 会自动识别已连接的 U 盘、移动硬盘和存储卡。" /> : null}
          {devicesQuery.isError ? <p className="error-copy">{devicesQuery.error instanceof Error ? devicesQuery.error.message : "无法读取设备列表。"}</p> : null}
          {selectRoleMutation.isError ? <p className="error-copy">{selectRoleMutation.error instanceof Error ? selectRoleMutation.error.message : "保存设备角色失败。"}</p> : null}
          {!devicesQuery.isLoading && !devicesQuery.isError && devices.length === 0 ? <EmptyBlock icon={<HardDrive size={20} />} title="当前没有可用设备" copy="插入可移动设备后，这里会显示它们并允许你选择用途。" /> : null}

          {devices.length > 0 ? (
            <div className="device-card-list">
              {devices.map((device) => (
                <article key={device.identitySignature} className={`device-card${selectedDevice?.identitySignature === device.identitySignature ? " active" : ""}`}>
                  <button
                    type="button"
                    className="device-card-trigger"
                    onClick={() => {
                      const next = new URLSearchParams(searchParams);
                      next.set("device", device.identitySignature);
                      setSearchParams(next);
                    }}
                  >
                    <div className="device-card-head">
                      <strong>{device.device.volumeLabel || "未命名设备"}</strong>
                      <span className="status-pill subtle">{device.device.mountPoint}</span>
                    </div>
                    <p>{[device.device.fileSystem, device.device.model || device.device.interfaceType].filter(Boolean).join(" / ") || "可移动设备"}</p>
                    <div className="replica-chip-row">
                      <span className="replica-chip neutral">
                        {device.currentSessionRole === "import_source"
                          ? "当前作为导入源"
                          : device.currentSessionRole === "managed_storage"
                            ? "当前作为管理存储"
                            : device.knownEndpoint
                              ? `已纳管：${device.knownEndpoint.name}`
                              : "尚未选择角色"}
                      </span>
                    </div>
                  </button>
                  <div className="device-role-actions">
                    <button type="button" className={`ghost-button${device.currentSessionRole === "import_source" ? " is-selected" : ""}`} disabled={selectRoleMutation.isPending} onClick={() => void handleChooseRole(device.identitySignature, "import_source", device.knownEndpoint?.name ?? device.device.volumeLabel)}>
                      <Upload size={15} />
                      导入源
                    </button>
                    <button type="button" className={`ghost-button${device.currentSessionRole === "managed_storage" ? " is-selected" : ""}`} disabled={selectRoleMutation.isPending} onClick={() => void handleChooseRole(device.identitySignature, "managed_storage", device.knownEndpoint?.name ?? device.device.volumeLabel)}>
                      <Route size={15} />
                      管理存储
                    </button>
                  </div>
                </article>
              ))}
            </div>
          ) : null}
        </article>

        <article className="detail-card import-target-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">目标端点</p>
              <h4>目标</h4>
            </div>
          </div>
          {activeManagedEndpoints.length === 0 ? (
            <EmptyBlock icon={<Route size={20} />} title="当前没有可用目标端点" copy="请先在存储管理中接入本地、QNAP、115 或可移动存储端点。" />
          ) : (
            <div className="endpoint-grid compact-grid">
              {activeManagedEndpoints.map((endpoint) => (
                <article key={endpoint.id} className="endpoint-panel compact-panel">
                  <div className="endpoint-panel-head">
                    <strong>{endpoint.name}</strong>
                    <span className={`status-pill ${endpoint.availabilityStatus === "AVAILABLE" ? "success" : "warning"}`}>
                      {getAvailabilityStatusLabel(endpoint.availabilityStatus)}
                    </span>
                  </div>
                  <p>{getEndpointTypeLabel(endpoint)}</p>
                  <small>{endpoint.rootPath}</small>
                </article>
              ))}
            </div>
          )}
          <Link to="/storage" className="ghost-button inline-button">
            打开存储管理
          </Link>
        </article>
      </div>

      <article className="detail-card import-browser-card">
        <div className="section-head">
          <div>
              <p className="eyebrow">导入源浏览</p>
              <h4>文件</h4>
          </div>
          <div className="action-row">
            <div className="segmented-group">
              {MEDIA_FILTERS.map((filter) => (
                <button key={filter.value} type="button" className={`segmented-button${mediaFilter === filter.value ? " active" : ""}`} onClick={() => setMediaFilter(filter.value)}>
                  {filter.label}
                </button>
              ))}
            </div>
            <button type="button" className="ghost-button" onClick={() => void sourceQuery.refetch()} disabled={!selectedDevice || sourceQuery.isFetching}>
              {sourceQuery.isFetching ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
              重扫来源
            </button>
          </div>
        </div>

        {!selectedDevice ? <EmptyBlock icon={<HardDrive size={20} />} title="还没有选中导入设备" copy="先从上方选择一个设备，并把它设为导入源。" /> : null}
        {selectedDevice && sourceQuery.isLoading ? <EmptyBlock icon={<LoaderCircle size={20} className="spin" />} title="正在读取设备内容" copy="正在扫描图片、视频和音频文件，请稍候。" /> : null}
        {selectedDevice && sourceQuery.isError ? <p className="error-copy">{sourceQuery.error instanceof Error ? sourceQuery.error.message : "无法读取导入源内容。"}</p> : null}

        {selectedDevice && !sourceQuery.isLoading && !sourceQuery.isError ? (
          <>
            <div className="import-browser-toolbar">
              <div className="status-strip import-browser-meta">
                <span className="status-pill subtle">目录 {groupedSourceEntries.length}</span>
                <span className="status-pill subtle">可见文件 {sourceEntries.length}</span>
                <span className="status-pill subtle">已勾选 {selectedEntries.length}</span>
              </div>
              <div className="action-row">
                <button type="button" className="ghost-button" onClick={() => setSelectedPaths(selectedEntries.length === sourceEntries.length ? [] : sourceEntries.map((entry) => entry.relativePath))} disabled={sourceEntries.length === 0}>
                  {selectedEntries.length === sourceEntries.length && sourceEntries.length > 0 ? "取消全选" : "全选当前列表"}
                </button>
                <button type="button" className="primary-button" onClick={() => void handleImport()} disabled={selectedEntries.length === 0 || executeImportMutation.isPending}>
                  {executeImportMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <Upload size={16} />}
                  开始导入
                </button>
              </div>
            </div>

            {sourceEntries.length === 0 ? (
              <EmptyBlock icon={<CheckCircle2 size={20} />} title="当前筛选下没有可导入文件" copy="你可以切换媒体类型筛选，或重新扫描来源设备。" />
            ) : (
              <div className="import-folder-list">
                {groupedSourceEntries.map((group) => {
                  const folderSelectedCount = group.entries.filter((entry) => selectedPaths.includes(entry.relativePath)).length;
                  const allSelected = folderSelectedCount === group.entries.length && group.entries.length > 0;

                  return (
                    <section key={group.path || "__root__"} className="import-folder-card">
                      <div className="import-folder-head">
                        <div className="import-folder-copy">
                          <div className="replica-chip-row">
                            <span className="asset-badge">
                              <FolderOpen size={14} />
                              {group.name}
                            </span>
                            <span className="replica-chip neutral">{group.entries.length} 个文件</span>
                            <span className="replica-chip warning">已选 {folderSelectedCount}</span>
                          </div>
                          <small>{group.path || "根目录"}</small>
                        </div>

                        <button type="button" className={`ghost-button${allSelected ? " is-selected" : ""}`} onClick={() => toggleFolderSelection(group.entries)}>
                          {allSelected ? "取消目录" : "选择目录"}
                        </button>
                      </div>

                      <div className="import-entry-list import-folder-entry-list">
                        {group.entries.map((entry) => {
                          const Icon = getEntryIcon(entry.mediaType);
                          const checked = selectedPaths.includes(entry.relativePath);
                          return (
                            <label key={entry.relativePath} className={`import-entry-card${checked ? " active" : ""}`}>
                              <input type="checkbox" checked={checked} onChange={() => toggleEntrySelection(entry.relativePath)} />
                              <div className="import-entry-icon">
                                <Icon size={18} />
                              </div>
                              <div className="import-entry-copy">
                                <strong>{entry.name}</strong>
                                <div className="replica-chip-row">
                                  <span className="replica-chip neutral">{getMediaLabel(entry.mediaType)}</span>
                                  <span className="replica-chip neutral">{formatFileSize(entry.size)}</span>
                                  <span className="replica-chip neutral">{formatCatalogDate(entry.modifiedAt)}</span>
                                </div>
                              </div>
                            </label>
                          );
                        })}
                      </div>
                    </section>
                  );
                })}
              </div>
            )}
          </>
        ) : null}
      </article>

      <div className="page-grid import-layout import-rules-layout">
        <article className="detail-card import-rules-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">导入规则</p>
              <h4>规则</h4>
            </div>
            <button type="button" className="primary-button" onClick={() => void handleSaveRules()} disabled={saveRulesMutation.isPending}>
              {saveRulesMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <Settings2 size={16} />}
              保存规则
            </button>
          </div>
          {rulesQuery.isError ? <p className="error-copy">{rulesQuery.error instanceof Error ? rulesQuery.error.message : "无法读取导入规则。"}</p> : null}
          {saveRulesMutation.isError ? <p className="error-copy">{saveRulesMutation.error instanceof Error ? saveRulesMutation.error.message : "保存导入规则失败。"}</p> : null}

          <div className="rule-card-grid">
            {MEDIA_RULES.map((rule) => (
              <article key={rule.value} className="rule-card">
                <div className="rule-card-head">
                  <strong>{rule.label}</strong>
                  <span className="status-pill subtle">{mediaRuleTargets[rule.value].length} 个目标</span>
                </div>
                <div className="endpoint-toggle-row">
                  {activeManagedEndpoints.map((endpoint) => (
                    <button key={`${rule.value}-${endpoint.id}`} type="button" className={`ghost-button${mediaRuleTargets[rule.value].includes(endpoint.id) ? " is-selected" : ""}`} onClick={() => setMediaRuleTargets((current) => ({ ...current, [rule.value]: toggleString(current[rule.value], endpoint.id) }))}>
                      {endpoint.name}
                    </button>
                  ))}
                </div>
              </article>
            ))}
          </div>

          <div className="section-head">
            <div>
              <p className="eyebrow">扩展名</p>
              <h4>扩展名</h4>
            </div>
            <button type="button" className="ghost-button" onClick={() => setExtensionRules((current) => [...current, { id: `draft-${crypto.randomUUID()}`, extension: "", targetEndpointIds: [] }])}>
              添加规则
            </button>
          </div>

          <div className="extension-rule-list">
            {extensionRules.map((rule, index) => (
              <article key={rule.id} className="extension-rule-card">
                <div className="section-head">
                  <div>
                    <p className="eyebrow">规则 {index + 1}</p>
                    <h4>规则 {index + 1}</h4>
                  </div>
                  <button type="button" className="ghost-button" onClick={() => setExtensionRules((current) => {
                    const next = current.filter((item) => item.id !== rule.id);
                    return next.length > 0 ? next : [{ id: `draft-${crypto.randomUUID()}`, extension: "", targetEndpointIds: [] }];
                  })}>
                    删除
                  </button>
                </div>
                <label className="field">
                  <span>扩展名</span>
                  <input value={rule.extension} placeholder=".jpg / .mov / .wav" onChange={(event) => setExtensionRules((current) => current.map((item) => item.id === rule.id ? { ...item, extension: event.target.value } : item))} />
                </label>
                <div className="endpoint-toggle-row">
                  {activeManagedEndpoints.map((endpoint) => (
                    <button key={`${rule.id}-${endpoint.id}`} type="button" className={`ghost-button${rule.targetEndpointIds.includes(endpoint.id) ? " is-selected" : ""}`} onClick={() => setExtensionRules((current) => current.map((item) => item.id === rule.id ? { ...item, targetEndpointIds: toggleString(item.targetEndpointIds, endpoint.id) } : item))}>
                      {endpoint.name}
                    </button>
                  ))}
                </div>
              </article>
            ))}
          </div>
        </article>

        <article className="detail-card import-results-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">结果与任务</p>
              <h4>结果</h4>
            </div>
          </div>

          {lastSummary ? (
            <div className="import-summary-stack">
              <article className="rule-card">
                <div className="rule-card-head">
                  <strong>{lastSummary.deviceLabel || "导入执行"}</strong>
                  <span className={`status-pill ${getRunTone(lastSummary.status)}`}>{getRunLabel(lastSummary.status)}</span>
                </div>
                <p>{lastSummary.progressLabel || `已将 ${lastSummary.totalFiles} 个文件加入可靠传输队列，可在同步中心继续管理。`}</p>
                {lastSummary.error ? <p className="error-copy">{lastSummary.error}</p> : null}
                {!lastSummary.error ? (
                  <div className="action-row">
                    <Link to="/sync" className="ghost-button">
                      打开同步中心
                    </Link>
                  </div>
                ) : null}
              </article>
              {lastSummary.items.length > 0 ? (
                <div className="import-summary-list">
                  {lastSummary.items.slice(0, 8).map((item) => (
                    <article key={`${item.relativePath}-${item.logicalPathKey || item.displayName}`} className="import-summary-item">
                      <div className="rule-card-head">
                        <div>
                          <strong>{item.displayName}</strong>
                          <p>{item.relativePath}</p>
                        </div>
                        <span className={`status-pill ${getRunTone(item.status)}`}>{getRunLabel(item.status)}</span>
                      </div>
                      <div className="replica-chip-row">
                        <span className="replica-chip neutral">{getMediaLabel(item.mediaType)}</span>
                        {item.assetId ? <span className="replica-chip neutral">资产 {item.assetId.slice(0, 8)}</span> : null}
                      </div>
                      {item.targetResults.length > 0 ? (
                        <div className="import-target-list">
                          {item.targetResults.map((target) => (
                            <span key={`${item.relativePath}-${target.endpointId}`} className={`replica-chip ${getRunTone(target.status)}`}>
                              {target.endpointName}
                            </span>
                          ))}
                        </div>
                      ) : null}
                      {item.error ? <p className="error-copy">{item.error}</p> : null}
                    </article>
                  ))}
                </div>
              ) : null}
            </div>
          ) : (
            <EmptyBlock icon={<Upload size={20} />} title="还没有导入结果" copy="执行一次导入后，这里会显示摘要与失败提示。" />
          )}
          {executeImportMutation.isError ? <p className="error-copy">{executeImportMutation.error instanceof Error ? executeImportMutation.error.message : "导入执行失败。"}</p> : null}

          {importTasks.length === 0 ? (
            <EmptyBlock icon={<Upload size={20} />} title="还没有导入任务" copy="任务执行后，这里会显示最近的导入记录。" />
          ) : (
            <div className="task-list">
              {importTasks.slice(0, 6).map((task) => (
                <ImportTaskCard key={task.id} task={task} />
              ))}
            </div>
          )}
        </article>
      </div>
    </section>
  );
}

function MetricCard({ label, value, tone }: { label: string; value: number; tone: "success" | "warning" | "neutral" }) {
  return <article className={`metric-card tone-${tone}`}><p>{label}</p><strong>{value}</strong></article>;
}

function EmptyBlock({ icon, title, copy }: { icon: ReactNode; title: string; copy: string }) {
  return <div className="sync-empty-block">{icon}<div><strong>{title}</strong><p>{copy}</p></div></div>;
}

function ImportTaskCard({ task }: { task: CatalogTask }) {
  return (
    <article className="task-card sync-task-card">
      <div className="task-card-head"><div><strong>{task.taskType === "import_execute" ? "导入执行" : task.taskType}</strong><p>{task.id}</p></div><span className={`status-pill ${getTaskTone(task.status)}`}>{task.status.trim().toLowerCase() === "queued" ? "排队中" : getTaskLabel(task.status)}</span></div>
      <div className="task-card-meta"><span>创建于 {formatCatalogDate(task.createdAt)}</span><span>更新于 {formatCatalogDate(task.updatedAt)}</span></div>
      {getTaskDisplaySummary(task) ? <p className="muted-copy clamp-2">{getTaskDisplaySummary(task)}</p> : null}
      {task.errorMessage ? <p className="error-copy">{task.errorMessage}</p> : null}
    </article>
  );
}

function toggleString(values: string[], nextValue: string) { return values.includes(nextValue) ? values.filter((value) => value !== nextValue) : [...values, nextValue]; }
function getImportDirectory(path: string) {
  const normalized = path.replace(/\\/g, "/");
  const lastSlashIndex = normalized.lastIndexOf("/");
  return lastSlashIndex >= 0 ? normalized.slice(0, lastSlashIndex) : "";
}
function getImportFolderName(path: string) {
  if (!path) {
    return "根目录";
  }
  const normalized = path.replace(/\\/g, "/");
  const parts = normalized.split("/").filter(Boolean);
  return parts[parts.length - 1] ?? normalized;
}
function getMediaLabel(mediaType: string) { return mediaType === "image" ? "图片" : mediaType === "video" ? "视频" : mediaType === "audio" ? "音频" : "媒体"; }
function getTaskLabel(status: string) { return getRunLabel(status); }
function getTaskTone(status: string) { return getRunTone(status); }
function getEndpointTypeLabel(endpoint: CatalogEndpoint) { return endpoint.endpointType === "LOCAL" ? "本地" : endpoint.endpointType === "QNAP_SMB" ? "QNAP / SMB" : endpoint.endpointType === "CLOUD_115" ? "115 网盘" : endpoint.endpointType === "ALIST" ? "AList 网盘" : endpoint.endpointType === "REMOVABLE" ? "可移动设备" : endpoint.endpointType; }
function getAvailabilityStatusLabel(status: string) { return status === "AVAILABLE" ? "可用" : status === "DISABLED" ? "停用" : status || "未知"; }
function getEntryIcon(mediaType: string) { return mediaType === "image" ? ImageIcon : mediaType === "video" ? Video : mediaType === "audio" ? Music4 : HardDrive; }
function getRunLabel(status: string) {
  switch (status.toLowerCase()) {
    case "success":
      return "成功";
    case "queued":
      return "排队中";
    case "running":
      return "进行中";
    case "pending":
      return "等待中";
    case "paused":
      return "已暂停";
    case "canceled":
      return "已取消";
    case "partial":
      return "部分完成";
    default:
      return "失败";
  }
}
function getRunTone(status: string) {
  switch (status.toLowerCase()) {
    case "success":
      return "success";
    case "failed":
    case "error":
      return "danger";
    case "paused":
    case "canceled":
      return "neutral";
    default:
      return "warning";
  }
}
