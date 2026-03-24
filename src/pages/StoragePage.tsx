import { useEffect, useMemo, useState } from "react";
import {
  CheckCircle2,
  HardDrive,
  LoaderCircle,
  Pencil,
  Plus,
  RefreshCcw,
  Save,
  Server,
  Trash2,
  Waves,
  X
} from "lucide-react";
import { useLocation } from "react-router-dom";
import {
  deleteCatalogEndpoint,
  getDefaultCatalogBackendUrl,
  runCatalogEndpointScan,
  runCatalogFullScan,
  saveCatalogEndpoint,
  updateCatalogEndpoint
} from "../services/catalog";
import { listRemovableDevices } from "../services/connector-test";
import { useCatalogEndpoints } from "../hooks/useCatalog";
import { formatCatalogDate } from "../lib/catalog-view";
import type {
  CatalogEndpoint,
  CatalogEndpointPayload,
  EndpointScanSummary,
  FullScanSummary
} from "../types/catalog";
import type { DeviceInfo } from "../types/connector-test";

const backendUrl = getDefaultCatalogBackendUrl();

const endpointTypeOptions = [
  { value: "LOCAL", label: "本地" },
  { value: "QNAP_SMB", label: "QNAP / SMB" },
  { value: "CLOUD_115", label: "115 网盘" },
  { value: "REMOVABLE", label: "可移动设备" }
] as const;

const cloud115AppOptions = [
  { value: "windows", label: "Windows" },
  { value: "android", label: "安卓" },
  { value: "ios", label: "iOS" },
  { value: "web", label: "网页" }
] as const;

type EndpointType = (typeof endpointTypeOptions)[number]["value"];

type EndpointFormState = {
  endpointType: EndpointType;
  name: string;
  note: string;
  roleMode: string;
  availabilityStatus: string;
  localRootPath: string;
  qnapSharePath: string;
  cloud115RootId: string;
  cloud115AccessToken: string;
  cloud115AppType: string;
  selectedMountPoint: string;
};

export function StoragePage() {
  const location = useLocation();
  const endpointsQuery = useCatalogEndpoints();
  const endpoints = endpointsQuery.data ?? [];
  const [devices, setDevices] = useState<DeviceInfo[]>([]);
  const [form, setForm] = useState<EndpointFormState>(() => createEmptyForm(""));
  const [editingEndpointId, setEditingEndpointId] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busyAction, setBusyAction] = useState<string | null>(null);
  const [latestSummary, setLatestSummary] = useState<FullScanSummary | EndpointScanSummary | null>(null);

  useEffect(() => {
    void refreshDevices();
  }, []);

  useEffect(() => {
    if (endpointsQuery.isLoading) {
      return;
    }

    const params = new URLSearchParams(location.search);
    const editEndpointId = params.get("edit");
    if (!editEndpointId) {
      return;
    }

    const target = endpoints.find((endpoint) => endpoint.id === editEndpointId);
    if (!target) {
      return;
    }

    if (editingEndpointId === target.id) {
      return;
    }

    handleEditEndpoint(target);
    if (params.get("mode") === "rebind") {
      setNotice(`请在当前机器上重新绑定“${target.name}”，然后执行一次校验扫描。`);
    }
  }, [editingEndpointId, endpoints, endpointsQuery.isLoading, location.search]);

  useEffect(() => {
    setForm((current) => {
      if (devices.length === 0) {
        if (current.selectedMountPoint === "") {
          return current;
        }
        return {
          ...current,
          selectedMountPoint: ""
        };
      }

      if (devices.some((device) => device.mountPoint === current.selectedMountPoint)) {
        return current;
      }

      return {
        ...current,
        selectedMountPoint: devices[0]?.mountPoint ?? ""
      };
    });
  }, [devices]);

  const selectedDevice = useMemo(
    () => devices.find((device) => device.mountPoint === form.selectedMountPoint) ?? null,
    [devices, form.selectedMountPoint]
  );

  const availableEndpointCount = endpoints.filter((endpoint) => endpoint.availabilityStatus === "AVAILABLE").length;
  const managedEndpointCount = endpoints.filter((endpoint) => endpoint.roleMode === "MANAGED").length;

  async function refreshDevices() {
    try {
      const response = await listRemovableDevices(backendUrl);
      if (!response.success) {
        return;
      }
      setDevices(response.devices ?? []);
    } catch {
      // Keep the page usable even if removable detection is unavailable.
    }
  }

  function updateForm<K extends keyof EndpointFormState>(key: K, value: EndpointFormState[K]) {
    setForm((current) => ({
      ...current,
      [key]: value
    }));
  }

  function resetForm() {
    setEditingEndpointId(null);
    setForm(createEmptyForm(devices[0]?.mountPoint ?? ""));
  }

  function handleEditEndpoint(endpoint: CatalogEndpoint) {
    setNotice(null);
    setError(null);
    setEditingEndpointId(endpoint.id);
    setForm(createFormFromEndpoint(endpoint, devices));
  }

  async function handleSubmitEndpoint() {
    setBusyAction("save-endpoint");
    setNotice(null);
    setError(null);

    try {
      const payload = buildEndpointPayload(form, selectedDevice);
      const response = editingEndpointId
        ? await updateCatalogEndpoint(backendUrl, editingEndpointId, payload)
        : await saveCatalogEndpoint(backendUrl, payload);

      if (!response.success || !response.endpoint) {
        setError(response.error ?? "保存存储端点失败。");
        return;
      }

      setNotice(
        editingEndpointId
          ? `已更新存储端点：${response.endpoint.name}`
          : `已添加存储端点：${response.endpoint.name}`
      );

      resetForm();
      await Promise.all([endpointsQuery.refetch(), refreshDevices()]);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "保存存储端点失败。");
    } finally {
      setBusyAction(null);
    }
  }

  async function handleDeleteEndpoint(endpoint: CatalogEndpoint) {
    const confirmed = window.confirm(
      `确认删除“${endpoint.name}”？\n\n这会移除该端点的副本记录，并更新仍指向它的导入规则。`
    );
    if (!confirmed) {
      return;
    }

    setBusyAction(`delete-${endpoint.id}`);
    setNotice(null);
    setError(null);

    try {
      const response = await deleteCatalogEndpoint(backendUrl, endpoint.id);
      if (!response.success || !response.summary) {
        setError(response.error ?? "删除存储端点失败。");
        return;
      }

      if (editingEndpointId === endpoint.id) {
        resetForm();
      }

      setNotice(
        `已删除 ${response.summary.endpointName}，移除了 ${response.summary.removedReplicaCount} 条副本记录。`
      );
      await endpointsQuery.refetch();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "删除存储端点失败。");
    } finally {
      setBusyAction(null);
    }
  }

  async function handleFullScan() {
    setBusyAction("full-scan");
    setNotice(null);
    setError(null);

    try {
      const response = await runCatalogFullScan(backendUrl);
      if (!response.success || !response.summary) {
        setError(response.error ?? "执行全量扫描失败。");
        return;
      }

      setLatestSummary(response.summary as FullScanSummary);
      setNotice("全量扫描已完成。");
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "执行全量扫描失败。");
    } finally {
      setBusyAction(null);
    }
  }

  async function handleEndpointScan(endpointId: string) {
    setBusyAction(`scan-${endpointId}`);
    setNotice(null);
    setError(null);

    try {
      const response = await runCatalogEndpointScan(backendUrl, endpointId);
      if (!response.success || !response.summary) {
        setError(response.error ?? "扫描端点失败。");
        return;
      }

      setLatestSummary(response.summary as EndpointScanSummary);
      setNotice("端点扫描已完成。");
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "扫描端点失败。");
    } finally {
      setBusyAction(null);
    }
  }

  const scanMetrics = getScanMetrics(latestSummary);

  return (
    <section className="page-stack">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">存储管理</p>
          <h3>统一管理已连接的存储端点，并保持副本扫描状态同步。</h3>
          <p>
            你可以在这里添加新端点、修改名称或备注、临时停用、删除端点，并在同一个页面直接触发扫描。
          </p>
        </div>

        <div className="hero-metrics">
          <MetricCard label="全部端点" value={endpoints.length} tone="neutral" />
          <MetricCard label="可用" value={availableEndpointCount} tone="success" />
          <MetricCard label="纳管中" value={managedEndpointCount} tone="warning" />
          <MetricCard label="可移动设备" value={devices.length} tone="neutral" />
        </div>
      </article>

      {notice ? <p className="inline-note">{notice}</p> : null}
      {error ? <p className="error-copy">{error}</p> : null}

      <div className="page-grid storage-layout">
        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">{editingEndpointId ? "编辑端点" : "新增端点"}</p>
              <h4>{editingEndpointId ? "更新存储设置" : "登记新的存储端点"}</h4>
            </div>

            <div className="endpoint-panel-actions">
              {editingEndpointId ? (
                <button type="button" className="ghost-button" onClick={resetForm} disabled={busyAction === "save-endpoint"}>
                  <X size={16} />
                  取消
                </button>
              ) : null}

              <button
                type="button"
                className="primary-button"
                onClick={() => void handleSubmitEndpoint()}
                disabled={busyAction === "save-endpoint"}
              >
                {busyAction === "save-endpoint" ? (
                  <LoaderCircle size={16} className="spin" />
                ) : editingEndpointId ? (
                  <Save size={16} />
                ) : (
                  <Plus size={16} />
                )}
                {editingEndpointId ? "保存修改" : "添加端点"}
              </button>
            </div>
          </div>

          <div className="segmented-group storage-type-group" aria-label="端点类型">
            {endpointTypeOptions.map((option) => (
              <button
                key={option.value}
                type="button"
                className={`segmented-button${form.endpointType === option.value ? " active" : ""}`}
                onClick={() => updateForm("endpointType", option.value)}
              >
                {option.label}
              </button>
            ))}
          </div>

          <div className="field-grid">
            <label className="field">
              <span>名称</span>
              <input
                value={form.name}
                onChange={(event) => updateForm("name", event.target.value)}
                placeholder="主 NAS / 本地 SSD / 115 网盘"
              />
            </label>

            <label className="field">
              <span>角色</span>
              <select value={form.roleMode} onChange={(event) => updateForm("roleMode", event.target.value)}>
                <option value="MANAGED">管理存储</option>
                <option value="IMPORT_SOURCE">导入源</option>
              </select>
            </label>

            <label className="field">
              <span>状态</span>
              <select
                value={form.availabilityStatus}
                onChange={(event) => updateForm("availabilityStatus", event.target.value)}
              >
                <option value="AVAILABLE">可用</option>
                <option value="DISABLED">停用</option>
              </select>
            </label>

            <label className="field field-span">
              <span>备注</span>
              <textarea
                value={form.note}
                onChange={(event) => updateForm("note", event.target.value)}
                placeholder="可选备注、用途提示、路径含义或维护信息。"
                rows={3}
              />
            </label>

            {form.endpointType === "LOCAL" ? (
              <label className="field field-span">
                <span>本地根路径</span>
                <input
                  value={form.localRootPath}
                  onChange={(event) => updateForm("localRootPath", event.target.value)}
                  placeholder="D:\\Media"
                />
              </label>
            ) : null}

            {form.endpointType === "QNAP_SMB" ? (
              <label className="field field-span">
                <span>SMB 共享路径</span>
                <input
                  value={form.qnapSharePath}
                  onChange={(event) => updateForm("qnapSharePath", event.target.value)}
                  placeholder="\\\\qnap\\share\\media"
                />
              </label>
            ) : null}

            {form.endpointType === "CLOUD_115" ? (
              <>
                <label className="field">
                  <span>115 根目录 ID</span>
                  <input
                    value={form.cloud115RootId}
                    onChange={(event) => updateForm("cloud115RootId", event.target.value)}
                  />
                </label>

                <label className="field">
                  <span>115 应用类型</span>
                  <select
                    value={form.cloud115AppType}
                    onChange={(event) => updateForm("cloud115AppType", event.target.value)}
                  >
                    {cloud115AppOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </label>

                <label className="field field-span">
                  <span>凭证 / Cookies</span>
                  <input
                    value={form.cloud115AccessToken}
                    onChange={(event) => updateForm("cloud115AccessToken", event.target.value)}
                    placeholder="粘贴当前可用的 115 凭证字符串"
                  />
                </label>
              </>
            ) : null}

            {form.endpointType === "REMOVABLE" ? (
              <label className="field field-span">
                <span>设备挂载点</span>
                <select
                  value={form.selectedMountPoint}
                  onChange={(event) => updateForm("selectedMountPoint", event.target.value)}
                >
                  {devices.length === 0 ? <option value="">当前未检测到可移动设备</option> : null}
                  {devices.map((device) => (
                    <option key={device.mountPoint} value={device.mountPoint}>
                      {(device.volumeLabel || "未命名设备") + " - " + device.mountPoint}
                    </option>
                  ))}
                </select>
              </label>
            ) : null}
          </div>
        </article>

        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">检测到的设备</p>
              <h4>当前可用的可移动设备</h4>
            </div>

            <button type="button" className="ghost-button" onClick={() => void refreshDevices()}>
              <RefreshCcw size={16} />
              刷新
            </button>
          </div>

          {devices.length === 0 ? (
            <div className="sync-empty-block">
              <HardDrive size={20} />
              <div>
                <strong>当前未检测到可移动设备。</strong>
                <p>如果要把 U 盘或移动硬盘登记为管理端点，请先接入设备。</p>
              </div>
            </div>
          ) : (
            <div className="device-card-list">
              {devices.map((device) => (
                <article
                  key={device.mountPoint}
                  className={`device-card static${form.selectedMountPoint === device.mountPoint ? " active" : ""}`}
                >
                  <div className="device-card-head">
                    <strong>{device.volumeLabel || "未命名设备"}</strong>
                    <span className="status-pill subtle">{device.mountPoint}</span>
                  </div>
                  <p>{device.model || "外接存储设备"}</p>
                  <div className="replica-chip-row">
                    <span className="replica-chip neutral">{device.fileSystem || "未知文件系统"}</span>
                    <span className="replica-chip neutral">{device.interfaceType || "未知接口"}</span>
                  </div>
                </article>
              ))}
            </div>
          )}
        </article>
      </div>

      <article className="detail-card">
        <div className="section-head">
          <div>
            <p className="eyebrow">已连接端点</p>
            <h4>扫描、编辑、停用或删除现有存储条目</h4>
          </div>

          <button type="button" className="primary-button" onClick={() => void handleFullScan()} disabled={busyAction === "full-scan"}>
            {busyAction === "full-scan" ? <LoaderCircle size={16} className="spin" /> : <Waves size={16} />}
            全量扫描
          </button>
        </div>

        {endpointsQuery.isLoading ? (
          <div className="sync-empty-block">
            <LoaderCircle size={20} className="spin" />
            <div>
              <strong>正在加载存储端点...</strong>
                <p>后台服务正在拉取当前端点列表。</p>
            </div>
          </div>
        ) : endpoints.length === 0 ? (
          <div className="sync-empty-block">
            <Server size={20} />
            <div>
              <strong>还没有登记任何存储端点。</strong>
              <p>可以先在上方添加本地磁盘、NAS、115 网盘或可移动存储。</p>
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
                    <span>路径 / 根标识</span>
                    <strong>{endpoint.rootPath}</strong>
                  </div>
                  <div>
                    <span>角色</span>
                    <strong>{endpoint.roleMode === "MANAGED" ? "管理存储" : "导入源"}</strong>
                  </div>
                  <div>
                    <span>最近更新</span>
                    <strong>{formatCatalogDate(endpoint.updatedAt)}</strong>
                  </div>
                  <div>
                    <span>备注</span>
                    <strong>{endpoint.note?.trim() || "-"}</strong>
                  </div>
                </div>

                <div className="endpoint-panel-actions">
                  <button
                    type="button"
                    className="ghost-button"
                    onClick={() => void handleEndpointScan(endpoint.id)}
                    disabled={busyAction === `scan-${endpoint.id}`}
                  >
                    {busyAction === `scan-${endpoint.id}` ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
                    扫描
                  </button>

                  <button type="button" className="ghost-button" onClick={() => handleEditEndpoint(endpoint)}>
                    <Pencil size={16} />
                    编辑
                  </button>

                  <button
                    type="button"
                    className="ghost-button danger-button"
                    onClick={() => void handleDeleteEndpoint(endpoint)}
                    disabled={busyAction === `delete-${endpoint.id}`}
                  >
                    {busyAction === `delete-${endpoint.id}` ? <LoaderCircle size={16} className="spin" /> : <Trash2 size={16} />}
                    删除
                  </button>
                </div>
              </article>
            ))}
          </div>
        )}
      </article>

      {latestSummary ? (
        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">最近一次扫描</p>
              <h4>{isEndpointScanSummary(latestSummary) ? "端点扫描摘要" : "全量扫描摘要"}</h4>
            </div>

            <span className={`status-pill ${scanMetrics.statusTone}`}>{scanMetrics.statusLabel}</span>
          </div>

          <div className="scan-summary-grid">
            {scanMetrics.items.map((item) => (
              <SummaryCell key={item.label} label={item.label} value={item.value} />
            ))}
          </div>

          <div className="settings-note-card">
            <CheckCircle2 size={18} />
            <div>
              <strong>完成于 {formatCatalogDate(scanMetrics.finishedAt)}</strong>
              <p>{scanMetrics.subtitle}</p>
            </div>
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
  value: number;
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

function createEmptyForm(selectedMountPoint: string): EndpointFormState {
  return {
    endpointType: "LOCAL",
    name: "",
    note: "",
    roleMode: "MANAGED",
    availabilityStatus: "AVAILABLE",
    localRootPath: "",
    qnapSharePath: "",
    cloud115RootId: "0",
    cloud115AccessToken: "",
    cloud115AppType: "windows",
    selectedMountPoint
  };
}

function createFormFromEndpoint(endpoint: CatalogEndpoint, devices: DeviceInfo[]): EndpointFormState {
  const next = createEmptyForm(devices[0]?.mountPoint ?? "");
  const endpointType = normalizeEndpointType(endpoint.endpointType);
  const config = endpoint.connectionConfig ?? {};
  const selectedMountPoint =
    getNestedString(config, "device", "mountPoint") || endpoint.rootPath || devices[0]?.mountPoint || "";

  return {
    ...next,
    endpointType,
    name: endpoint.name,
    note: endpoint.note ?? "",
    roleMode: endpoint.roleMode || "MANAGED",
    availabilityStatus: endpoint.availabilityStatus || "AVAILABLE",
    localRootPath: endpointType === "LOCAL" ? getString(config, "rootPath") || endpoint.rootPath : "",
    qnapSharePath: endpointType === "QNAP_SMB" ? getString(config, "sharePath") || endpoint.rootPath : "",
    cloud115RootId: endpointType === "CLOUD_115" ? getString(config, "rootId") || endpoint.rootPath || "0" : "0",
    cloud115AccessToken: endpointType === "CLOUD_115" ? getString(config, "accessToken") : "",
    cloud115AppType: endpointType === "CLOUD_115" ? getString(config, "appType") || "windows" : "windows",
    selectedMountPoint
  };
}

function buildEndpointPayload(form: EndpointFormState, selectedDevice: DeviceInfo | null): CatalogEndpointPayload {
  switch (form.endpointType) {
    case "LOCAL":
      return {
        name: form.name.trim(),
        note: form.note.trim(),
        endpointType: form.endpointType,
        rootPath: form.localRootPath.trim(),
        roleMode: form.roleMode,
        availabilityStatus: form.availabilityStatus,
        connectionConfig: {
          rootPath: form.localRootPath.trim()
        }
      };
    case "QNAP_SMB":
      return {
        name: form.name.trim(),
        note: form.note.trim(),
        endpointType: form.endpointType,
        rootPath: form.qnapSharePath.trim(),
        roleMode: form.roleMode,
        availabilityStatus: form.availabilityStatus,
        connectionConfig: {
          sharePath: form.qnapSharePath.trim()
        }
      };
    case "CLOUD_115":
      return {
        name: form.name.trim(),
        note: form.note.trim(),
        endpointType: form.endpointType,
        rootPath: form.cloud115RootId.trim(),
        roleMode: form.roleMode,
        availabilityStatus: form.availabilityStatus,
        connectionConfig: {
          rootId: form.cloud115RootId.trim(),
          accessToken: form.cloud115AccessToken.trim(),
          appType: form.cloud115AppType
        }
      };
    case "REMOVABLE":
      if (!selectedDevice) {
        throw new Error("当前没有选中的可移动设备。");
      }
      return {
        name: form.name.trim(),
        note: form.note.trim(),
        endpointType: form.endpointType,
        rootPath: selectedDevice.mountPoint,
        roleMode: form.roleMode,
        availabilityStatus: form.availabilityStatus,
        connectionConfig: {
          device: selectedDevice
        }
      };
  }
}

function normalizeEndpointType(value: string): EndpointType {
  switch (value) {
    case "QNAP_SMB":
      return "QNAP_SMB";
    case "CLOUD_115":
      return "CLOUD_115";
    case "REMOVABLE":
      return "REMOVABLE";
    default:
      return "LOCAL";
  }
}

function getEndpointTypeLabel(endpointType: string) {
  switch (endpointType) {
    case "LOCAL":
      return "本地";
    case "QNAP_SMB":
      return "QNAP / SMB";
    case "CLOUD_115":
      return "115 网盘";
    case "REMOVABLE":
      return "可移动设备";
    default:
      return endpointType;
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

function getString(config: Record<string, unknown>, key: string): string {
  const value = config[key];
  return typeof value === "string" ? value : "";
}

function getNestedString(config: Record<string, unknown>, key: string, nestedKey: string): string {
  const value = config[key];
  if (!value || typeof value !== "object") {
    return "";
  }

  const nestedValue = (value as Record<string, unknown>)[nestedKey];
  return typeof nestedValue === "string" ? nestedValue : "";
}

function isEndpointScanSummary(summary: FullScanSummary | EndpointScanSummary): summary is EndpointScanSummary {
  return "endpointId" in summary;
}

function getScanMetrics(summary: FullScanSummary | EndpointScanSummary | null) {
  if (!summary) {
    return {
      statusLabel: "空闲",
      statusTone: "neutral" as const,
      finishedAt: "",
      subtitle: "",
      items: [] as Array<{ label: string; value: string }>
    };
  }

  if (isEndpointScanSummary(summary)) {
    return {
      statusLabel: summary.status === "success" ? "成功" : summary.status,
      statusTone: summary.status === "success" ? ("success" as const) : ("warning" as const),
      finishedAt: summary.finishedAt,
      subtitle: `${summary.endpointName} 共扫描 ${summary.filesScanned} 个文件，分 ${summary.batchCount} 批完成。`,
      items: [
        { label: "扫描文件", value: String(summary.filesScanned) },
        { label: "新增资产", value: String(summary.assetsCreated) },
        { label: "更新资产", value: String(summary.assetsUpdated) },
        { label: "缺失副本", value: String(summary.missingReplicas) }
      ]
    };
  }

  return {
    statusLabel: summary.failedCount === 0 ? "成功" : "完成但有错误",
    statusTone: summary.failedCount === 0 ? ("success" as const) : ("warning" as const),
    finishedAt: summary.finishedAt,
    subtitle: `${summary.successCount} 个端点成功，${summary.failedCount} 个端点失败。`,
    items: [
      { label: "端点总数", value: String(summary.endpointCount) },
      { label: "成功", value: String(summary.successCount) },
      { label: "失败", value: String(summary.failedCount) },
      { label: "开始时间", value: formatCatalogDate(summary.startedAt) }
    ]
  };
}
