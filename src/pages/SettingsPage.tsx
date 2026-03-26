import { useMemo, useRef, useState, type ChangeEvent } from "react";
import {
  ArchiveRestore,
  Download,
  LoaderCircle,
  MoonStar,
  RefreshCcw,
  ServerCog,
  SunMedium,
  Upload
} from "lucide-react";
import { Link } from "react-router-dom";
import { useTheme, type ThemeMode } from "../components/ThemeProvider";
import { useLibraryContext } from "../context/LibraryContext";
import { useAppBootstrap } from "../hooks/useAppBootstrap";
import { useCatalogEndpoints } from "../hooks/useCatalog";
import { useExportSettingsBackup, useImportSettingsBackup } from "../hooks/useSettings";
import { formatCatalogDate } from "../lib/catalog-view";
import {
  getDefaultCatalogBackendUrl,
  runCatalogEndpointScan,
  runCatalogFullScan
} from "../services/catalog";
import type {
  CatalogEndpoint,
  EndpointScanSummary,
  FullScanSummary
} from "../types/catalog";
import type {
  BackupImportMode,
  SettingsBackupBundle,
  SettingsBackupImportSummary
} from "../types/settings";

const backendUrl = getDefaultCatalogBackendUrl();

const themeOptions: Array<{
  value: ThemeMode;
  title: string;
  icon: typeof SunMedium;
}> = [
  {
    value: "light",
    title: "浅色",
    icon: SunMedium
  },
  {
    value: "dark",
    title: "深色",
    icon: MoonStar
  }
];

export function SettingsPage() {
  const { theme, setTheme } = useTheme();
  const { currentLibrary, isLibraryOpen } = useLibraryContext();
  const bootstrapQuery = useAppBootstrap();
  const endpointsQuery = useCatalogEndpoints();
  const exportMutation = useExportSettingsBackup();
  const importMutation = useImportSettingsBackup();
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  const [includeCatalog, setIncludeCatalog] = useState(false);
  const [importMode, setImportMode] = useState<BackupImportMode>("config_only");
  const [selectedFileName, setSelectedFileName] = useState("");
  const [selectedBundle, setSelectedBundle] = useState<SettingsBackupBundle | null>(null);
  const [validationSummary, setValidationSummary] = useState<FullScanSummary | EndpointScanSummary | null>(null);
  const [validationMessage, setValidationMessage] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [lastImportSummary, setLastImportSummary] = useState<SettingsBackupImportSummary | null>(null);
  const [busyValidationKey, setBusyValidationKey] = useState<string | null>(null);

  const endpoints = endpointsQuery.data ?? [];
  const bundleStats = useMemo(() => summarizeBundle(selectedBundle), [selectedBundle]);
  const bootstrapData = bootstrapQuery.data;

  async function handleExport() {
    if (!isLibraryOpen) {
      setError("请先在 Welcome 页面打开一个资产库，再执行库级备份导出。");
      return;
    }

    setNotice(null);
    setError(null);

    try {
      const bundle = await exportMutation.mutateAsync({
        theme,
        includeCatalog
      });

      downloadBundle(bundle, includeCatalog ? "catalog" : "config");
      setNotice(
        includeCatalog
          ? `已导出恢复包，包含 ${bundle.configuration.endpoints.length} 个端点和 ${bundle.catalog?.assets.length ?? 0} 个资产。`
          : `已导出配置备份，包含 ${bundle.configuration.endpoints.length} 个端点。`
      );
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "导出备份失败。");
    }
  }

  async function handleImport() {
    if (!isLibraryOpen) {
      setError("请先打开一个资产库，再执行库级备份导入。");
      return;
    }

    if (!selectedBundle) {
      setError("请先选择一个备份文件再导入。");
      return;
    }

    if (importMode === "config_and_catalog" && !selectedBundle.catalog) {
      setError("这个备份不包含资产快照，无法执行完整恢复。");
      return;
    }

    setNotice(null);
    setError(null);
    setValidationSummary(null);
    setValidationMessage(null);

    try {
      const summary = await importMutation.mutateAsync({
        mode: importMode,
        bundle: selectedBundle
      });

      setLastImportSummary(summary);
      applyImportedTheme(summary.appliedTheme, setTheme);
      await Promise.all([bootstrapQuery.refetch(), endpointsQuery.refetch()]);

      setNotice(
        summary.mode === "config_and_catalog"
          ? `已从恢复包导入 ${summary.importedAssets} 个资产和 ${summary.importedEndpoints} 个端点。`
          : `已导入 ${summary.importedEndpoints} 个端点和 ${summary.importedRules} 条导入规则。`
      );

      if (summary.mode === "config_and_catalog") {
        void runValidation("full");
      }
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "导入备份失败。");
    }
  }

  async function runValidation(scope: "full" | string) {
    if (!isLibraryOpen) {
      setError("当前没有已打开的资产库，无法执行库内校验。");
      return;
    }

    setBusyValidationKey(scope);
    setValidationMessage(scope === "full" ? "正在对当前资产库的全部端点执行导入后校验..." : null);
    setError(null);

    try {
      if (scope === "full") {
        const response = await runCatalogFullScan(backendUrl);
        if (!response.success || !response.summary) {
          throw new Error(response.error ?? "校验导入后的资产库失败。");
        }

        setValidationSummary(response.summary as FullScanSummary);
        setValidationMessage("后台校验已完成，当前资产库的端点状态已重新刷新。");
      } else {
        const response = await runCatalogEndpointScan(backendUrl, scope);
        if (!response.success || !response.summary) {
          throw new Error(response.error ?? "校验这个存储节点失败。");
        }

        setValidationSummary(response.summary as EndpointScanSummary);
        setValidationMessage("存储节点校验已完成。");
      }

      await Promise.all([endpointsQuery.refetch(), bootstrapQuery.refetch()]);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "校验导入后的存储节点失败。");
    } finally {
      setBusyValidationKey(null);
    }
  }

  function handleFileSelection(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (!file) {
      return;
    }

    setNotice(null);
    setError(null);
    setSelectedFileName(file.name);

    void file.text().then((text) => {
      try {
        const bundle = JSON.parse(text) as SettingsBackupBundle;
        validateBundle(bundle);
        setSelectedBundle(bundle);
        setImportMode(bundle.catalog ? "config_and_catalog" : "config_only");
        setNotice(`已载入备份文件“${file.name}”。`);
      } catch (parseError) {
        setSelectedBundle(null);
        setError(parseError instanceof Error ? parseError.message : "解析备份文件失败。");
      }
    });
  }

  return (
    <section className="page-stack">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">设置</p>
          <h3>设置</h3>
          <p>
            这里的备份与导入都作用于当前打开的资产库，不再是全局单体状态。
            应用级主题仍然在这里管理，但库级配置、端点和资产快照始终绑定到当前资产库。
          </p>
        </div>

        <div className="hero-metrics">
          <MetricCard label="主题" value={theme === "light" ? "浅色" : "深色"} tone="neutral" />
          <MetricCard
            label="应用数据库"
            value={bootstrapData?.database.ready ? "就绪" : "检查中"}
            tone={bootstrapData?.database.ready ? "success" : "warning"}
          />
          <MetricCard
            label="迁移版本"
            value={bootstrapData?.database.migrationVersion ?? "未知"}
            tone="neutral"
          />
          <MetricCard label="当前端点数" value={endpoints.length} tone="warning" />
        </div>
      </article>

      {notice ? <p className="inline-note">{notice}</p> : null}
      {error ? <p className="error-copy">{error}</p> : null}
      {validationMessage ? <p className="inline-note">{validationMessage}</p> : null}

      {!isLibraryOpen ? (
        <article className="detail-card">
          <div className="settings-note-card">
            <ServerCog size={18} />
            <div>
              <strong>当前没有已打开的资产库</strong>
              <p>应用级主题仍可在这里调整；库级备份、导入和校验，需要先在 Welcome 页面打开资产库。</p>
            </div>
          </div>
        </article>
      ) : currentLibrary ? (
        <article className="detail-card">
          <div className="settings-note-card">
            <ServerCog size={18} />
            <div>
              <strong>当前资产库：{currentLibrary.name}</strong>
              <p>本页的备份、导入、同步校验和端点重绑，都只作用于这个资产库。</p>
            </div>
          </div>
        </article>
      ) : null}

      <div className="page-grid settings-layout">
        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">外观</p>
              <h4>主题</h4>
            </div>
          </div>

          <div className="theme-option-grid">
            {themeOptions.map((option) => {
              const Icon = option.icon;
              const active = theme === option.value;

              return (
                <button
                  key={option.value}
                  type="button"
                  className={`theme-option-card${active ? " active" : ""}`}
                  onClick={() => setTheme(option.value)}
                >
                  <div className="theme-option-head">
                    <div className="theme-option-icon">
                      <Icon size={18} strokeWidth={1.9} />
                    </div>
                    <span className={`status-pill ${active ? "success" : "subtle"}`}>
                      {active ? "当前使用" : "切换"}
                    </span>
                  </div>

                  <div className={`theme-preview ${option.value}`}>
                    <div className="theme-preview-top" />
                    <div className="theme-preview-main">
                      <div className="theme-preview-sidebar" />
                      <div className="theme-preview-content">
                        <span className="theme-preview-line long" />
                        <span className="theme-preview-line" />
                        <span className="theme-preview-line short" />
                      </div>
                    </div>
                  </div>

                  <div className="theme-option-copy">
                    <strong>{option.title}</strong>
                  </div>
                </button>
              );
            })}
          </div>
        </article>

        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">备份导出</p>
              <h4>导出</h4>
            </div>
          </div>

          <div className="settings-card-list">
            <div className="settings-action-card">
              <div className="settings-action-head">
                <div className="theme-option-icon">
                  <Download size={18} />
                </div>
                <span className="status-pill subtle">{includeCatalog ? "包含资产快照" : "仅配置"}</span>
              </div>

              <strong>导出当前资产库配置</strong>
              <p>导出当前主题、存储节点、导入规则，并可选附带资产快照，用于迁移或恢复。</p>

              <label className="checkbox-field">
                <input
                  type="checkbox"
                  checked={includeCatalog}
                  onChange={(event) => setIncludeCatalog(event.target.checked)}
                />
                <span>附带资产快照，便于快速恢复资产视图</span>
              </label>

              <div className="action-row">
                <button
                  type="button"
                  className="primary-button"
                  onClick={() => void handleExport()}
                  disabled={exportMutation.isPending || !isLibraryOpen}
                >
                  {exportMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <Download size={16} />}
                  导出备份
                </button>
              </div>
            </div>
          </div>
        </article>
      </div>

      <div className="page-grid settings-layout">
        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">恢复导入</p>
              <h4>导入</h4>
            </div>
          </div>

          <div className="settings-card-list">
            <div className="settings-action-card">
              <div className="settings-action-head">
                <div className="theme-option-icon">
                  <Upload size={18} />
                </div>
                <span className="status-pill subtle">{selectedFileName || "尚未选择文件"}</span>
              </div>

              <strong>选择备份文件</strong>
              <p>可以只导入配置，也可以连同资产快照一起恢复，这样在实时校验完成前也能先看到资产视图。</p>

              <input
                ref={fileInputRef}
                type="file"
                accept="application/json,.json"
                onChange={handleFileSelection}
                disabled={!isLibraryOpen}
              />

              <label className="field">
                <span>导入模式</span>
                <select
                  value={importMode}
                  onChange={(event) => setImportMode(event.target.value as BackupImportMode)}
                  disabled={!isLibraryOpen}
                >
                  <option value="config_only">仅配置</option>
                  <option value="config_and_catalog" disabled={!selectedBundle?.catalog}>
                    配置 + 资产快照
                  </option>
                </select>
              </label>

              {selectedBundle ? (
                <div className="scan-summary-grid">
                  <SummaryCell label="格式版本" value={String(selectedBundle.formatVersion)} />
                  <SummaryCell label="端点数" value={String(bundleStats.endpointCount)} />
                  <SummaryCell label="规则数" value={String(bundleStats.ruleCount)} />
                  <SummaryCell label="资产数" value={String(bundleStats.assetCount)} />
                </div>
              ) : null}

              <div className="action-row">
                <button
                  type="button"
                  className="primary-button"
                  onClick={() => void handleImport()}
                  disabled={importMutation.isPending || !selectedBundle || !isLibraryOpen}
                >
                  {importMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <ArchiveRestore size={16} />}
                  导入备份
                </button>
              </div>
            </div>
          </div>
        </article>

        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">最近一次导入</p>
              <h4>最近导入</h4>
            </div>
          </div>

          {lastImportSummary ? (
            <>
              <div className="scan-summary-grid">
                <SummaryCell label="模式" value={getImportModeLabel(lastImportSummary.mode)} />
                <SummaryCell label="端点数" value={String(lastImportSummary.importedEndpoints)} />
                <SummaryCell label="规则数" value={String(lastImportSummary.importedRules)} />
                <SummaryCell label="资产数" value={String(lastImportSummary.importedAssets)} />
              </div>

              <div className="settings-note-card">
                <ArchiveRestore size={18} />
                <div>
                  <strong>导入于 {formatCatalogDate(lastImportSummary.importedAt)}</strong>
                  <p>
                    {lastImportSummary.mode === "config_and_catalog"
                      ? "资产快照已经先恢复到当前资产库，随后会通过实时校验来修正各端点状态。"
                      : "这次只恢复了设置、端点和导入规则，没有恢复资产快照。"}
                  </p>
                </div>
              </div>
            </>
          ) : (
            <div className="sync-empty-block">
              <ArchiveRestore size={20} />
              <div>
                <strong>当前会话还没有导入过备份</strong>
                <p>导入完成后，恢复统计和后续步骤会显示在这里。</p>
              </div>
            </div>
          )}
        </article>
      </div>

      <article className="detail-card">
        <div className="section-head">
            <div>
              <p className="eyebrow">重绑与校验</p>
              <h4>重绑与校验</h4>
          </div>

          <button
            type="button"
            className="ghost-button"
            onClick={() => void runValidation("full")}
            disabled={busyValidationKey === "full" || endpoints.length === 0 || !isLibraryOpen}
          >
            {busyValidationKey === "full" ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
            全部校验
          </button>
        </div>

        {endpointsQuery.isLoading ? (
          <div className="sync-empty-block">
            <LoaderCircle size={20} className="spin" />
            <div>
              <strong>正在加载当前资产库的存储节点</strong>
              <p>备份操作完成后，当前资产库的节点定义正在刷新。</p>
            </div>
          </div>
        ) : endpoints.length === 0 ? (
          <div className="sync-empty-block">
            <ServerCog size={20} />
            <div>
              <strong>当前资产库还没有需要重绑的端点</strong>
              <p>可以先导入备份，或者去存储节点页面登记端点。</p>
            </div>
          </div>
        ) : (
          <div className="endpoint-grid">
            {endpoints.map((endpoint) => (
              <article key={endpoint.id} className="endpoint-panel">
                <div className="endpoint-panel-head">
                  <div>
                    <strong>{endpoint.name}</strong>
                    <p>{getEndpointTypeLabel(endpoint.endpointType)}</p>
                  </div>
                  <span className={`status-pill ${endpoint.availabilityStatus === "AVAILABLE" ? "success" : "warning"}`}>
                    {getAvailabilityStatusLabel(endpoint.availabilityStatus)}
                  </span>
                </div>

                <div className="endpoint-panel-meta">
                  <div>
                    <span>根路径 / 标识</span>
                    <strong>{endpoint.rootPath}</strong>
                  </div>
                  <div>
                    <span>角色</span>
                    <strong>{getRoleModeLabel(endpoint.roleMode)}</strong>
                  </div>
                  <div>
                    <span>恢复提示</span>
                    <strong>{getRecoveryHint(endpoint)}</strong>
                  </div>
                  <div>
                    <span>最近更新</span>
                    <strong>{formatCatalogDate(endpoint.updatedAt)}</strong>
                  </div>
                </div>

                <div className="endpoint-panel-actions">
                  <Link to={`/storage?edit=${encodeURIComponent(endpoint.id)}&mode=rebind`} className="ghost-button">
                    重新绑定
                  </Link>

                  <button
                    type="button"
                    className="ghost-button"
                    onClick={() => void runValidation(endpoint.id)}
                    disabled={busyValidationKey === endpoint.id || !isLibraryOpen}
                  >
                    {busyValidationKey === endpoint.id ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
                    校验
                  </button>
                </div>
              </article>
            ))}
          </div>
        )}
      </article>

      {validationSummary ? (
        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">校验结果</p>
              <h4>{isEndpointScanSummary(validationSummary) ? "端点校验" : "全量校验"}</h4>
            </div>
          </div>

          <div className="scan-summary-grid">
            {buildValidationCells(validationSummary).map((item) => (
              <SummaryCell key={item.label} label={item.label} value={item.value} />
            ))}
          </div>
        </article>
      ) : null}

    </section>
  );
}

function MetricCard({
  label,
  value,
  tone
}: {
  label: string;
  value: string | number;
  tone: "success" | "warning" | "neutral";
}) {
  return (
    <article className={`metric-card tone-${tone}`}>
      <p>{label}</p>
      <strong>{value}</strong>
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

function summarizeBundle(bundle: SettingsBackupBundle | null) {
  if (!bundle) {
    return {
      endpointCount: 0,
      ruleCount: 0,
      assetCount: 0
    };
  }

  return {
    endpointCount: bundle.configuration.endpoints.length,
    ruleCount: bundle.configuration.importRules.length,
    assetCount: bundle.catalog?.assets.length ?? 0
  };
}

function validateBundle(bundle: SettingsBackupBundle) {
  if (!bundle || typeof bundle !== "object") {
    throw new Error("所选文件不是有效的备份包。");
  }
  if (typeof bundle.formatVersion !== "number") {
    throw new Error("所选文件缺少备份格式版本信息。");
  }
  if (
    !bundle.configuration ||
    !Array.isArray(bundle.configuration.endpoints) ||
    !Array.isArray(bundle.configuration.importRules)
  ) {
    throw new Error("所选文件不包含必需的配置快照。");
  }
}

function applyImportedTheme(theme: string | undefined, setTheme: (theme: ThemeMode) => void) {
  if (theme === "light" || theme === "dark") {
    setTheme(theme);
  }
}

function downloadBundle(bundle: SettingsBackupBundle, suffix: string) {
  const fileName = `mare-backup-${bundle.exportedAt.slice(0, 10)}-${suffix}.json`;
  const blob = new Blob([JSON.stringify(bundle, null, 2)], { type: "application/json" });
  const url = window.URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = fileName;
  anchor.click();
  window.URL.revokeObjectURL(url);
}

function getRecoveryHint(endpoint: CatalogEndpoint) {
  switch (endpoint.endpointType) {
    case "LOCAL":
      return "请确认当前机器上的本地根路径是否正确。";
    case "QNAP_SMB":
      return "请确认 SMB 共享路径和 NAS 可用性。";
    case "NETWORK_STORAGE":
      return "请检查网盘登录状态、根目录 ID 和本机保存的凭证是否有效。";
    case "CLOUD_115":
      return "请检查根目录 ID，必要时刷新 115 凭据。";
    case "ALIST":
      return "请检查 AList 挂载路径、根路径以及驱动配置。";
    case "REMOVABLE":
      return "请重新接入同一块设备，以便再次匹配身份。";
    default:
      return "校验前请先检查这个端点。";
  }
}

function getImportModeLabel(mode: BackupImportMode) {
  switch (mode) {
    case "config_and_catalog":
      return "配置 + 资产快照";
    case "config_only":
      return "仅配置";
    default:
      return mode;
  }
}

function getEndpointTypeLabel(endpointType: string) {
  switch (endpointType) {
    case "LOCAL":
      return "本地";
    case "QNAP_SMB":
      return "QNAP / SMB";
    case "NETWORK_STORAGE":
      return "网盘";
    case "CLOUD_115":
      return "115 网盘";
    case "ALIST":
      return "AList 网盘";
    case "REMOVABLE":
      return "可移动设备";
    default:
      return endpointType || "未知类型";
  }
}

function getAvailabilityStatusLabel(status: string) {
  switch (status) {
    case "AVAILABLE":
      return "可用";
    case "DISABLED":
      return "停用";
    default:
      return status || "未知";
  }
}

function getRoleModeLabel(roleMode: string) {
  switch (roleMode) {
    case "MANAGED":
      return "受管存储";
    case "IMPORT_SOURCE":
      return "导入源";
    default:
      return roleMode || "未知";
  }
}

function isEndpointScanSummary(
  summary: FullScanSummary | EndpointScanSummary
): summary is EndpointScanSummary {
  return "endpointId" in summary;
}

function buildValidationCells(summary: FullScanSummary | EndpointScanSummary) {
  if (isEndpointScanSummary(summary)) {
    return [
      { label: "端点", value: summary.endpointName },
      { label: "扫描文件", value: String(summary.filesScanned) },
      { label: "新增资产", value: String(summary.assetsCreated) },
      { label: "完成时间", value: formatCatalogDate(summary.finishedAt) }
    ];
  }

  return [
    { label: "端点总数", value: String(summary.endpointCount) },
    { label: "成功", value: String(summary.successCount) },
    { label: "失败", value: String(summary.failedCount) },
    { label: "完成时间", value: formatCatalogDate(summary.finishedAt) }
  ];
}
