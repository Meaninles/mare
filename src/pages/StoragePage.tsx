import { useEffect, useMemo, useRef, useState } from "react";
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
import {
  listRemovableDevices,
  pollCloud115QRCodeLogin,
  startCloud115QRCodeLogin
} from "../services/connector-test";
import { useCatalogEndpoints } from "../hooks/useCatalog";
import { formatCatalogDate } from "../lib/catalog-view";
import type {
  CatalogEndpoint,
  CatalogEndpointPayload,
  EndpointScanSummary,
  FullScanSummary
} from "../types/catalog";
import type { Cloud115QRCodeSession, DeviceInfo } from "../types/connector-test";

const backendUrl = getDefaultCatalogBackendUrl();

const endpointTypeOptions = [
  { value: "LOCAL", label: "本地" },
  { value: "QNAP_SMB", label: "QNAP / SMB" },
  { value: "NETWORK_STORAGE", label: "网盘" },
  { value: "REMOVABLE", label: "可移动设备" }
] as const;

const networkProviderOptions = [{ value: "115", label: "115 网盘" }] as const;

const networkLoginMethodOptions = [
  { value: "qrcode", label: "扫码登录" },
  { value: "manual", label: "手动填写凭证" }
] as const;

const cloud115AvailableAppOptions = [
  { value: "wechatmini", label: "微信小程序" },
  { value: "android", label: "安卓" },
  { value: "alipaymini", label: "支付宝小程序" },
  { value: "qandroid", label: "115 生活" },
  { value: "tv", label: "电视" },
  { value: "ios", label: "iOS" },
  { value: "web", label: "网页" }
] as const;

type EndpointType = (typeof endpointTypeOptions)[number]["value"];
type NetworkProvider = (typeof networkProviderOptions)[number]["value"];
type NetworkLoginMethod = (typeof networkLoginMethodOptions)[number]["value"];

type EndpointFormState = {
  endpointType: EndpointType;
  name: string;
  note: string;
  roleMode: string;
  availabilityStatus: string;
  localRootPath: string;
  qnapSharePath: string;
  networkProvider: NetworkProvider;
  networkStorageKey: string;
  networkMountPath: string;
  networkDriver: string;
  networkRootFolderId: string;
  networkLoginMethod: NetworkLoginMethod;
  networkAppType: string;
  networkCredential: string;
  networkHasStoredCredential: boolean;
  networkPageSize: number;
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
  const [networkQRCodeSession, setNetworkQRCodeSession] = useState<Cloud115QRCodeSession | null>(null);
  const qrPollTimerRef = useRef<number | null>(null);
  const qrSessionRef = useRef<Cloud115QRCodeSession | null>(null);

  useEffect(() => {
    void refreshDevices();
  }, []);

  useEffect(() => {
    qrSessionRef.current = networkQRCodeSession;
  }, [networkQRCodeSession]);

  useEffect(() => {
    return () => {
      clearQRCodePolling();
    };
  }, []);

  useEffect(() => {
    if (form.endpointType !== "NETWORK_STORAGE" || form.networkLoginMethod !== "qrcode") {
      clearQRCodePolling();
      qrSessionRef.current = null;
      setNetworkQRCodeSession(null);
    }
  }, [form.endpointType, form.networkLoginMethod]);

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
    if (!target || editingEndpointId === target.id) {
      return;
    }

    handleEditEndpoint(target);
    if (params.get("mode") === "rebind") {
      setNotice(`请在当前机器上重新绑定“${target.name}”，然后执行一次校验扫描。`);
    }
  }, [editingEndpointId, endpoints, endpointsQuery.isLoading, location.search]);

  useEffect(() => {
    const params = new URLSearchParams(location.search);
    if (params.get("setup") !== "first-endpoint" || endpointsQuery.isLoading) {
      return;
    }

    if (endpoints.length === 0) {
      setNotice("这是一个新的资产库，请先配置至少一个存储端点。");
    }
  }, [endpoints.length, endpointsQuery.isLoading, location.search]);

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
  const currentTypeLabel =
    endpointTypeOptions.find((option) => option.value === form.endpointType)?.label ?? form.endpointType;
  const isSavingEndpoint = busyAction === "save-endpoint";
  const isStartingQRCode = busyAction === "network-qrcode-start";
  const isPollingQRCode = busyAction === "network-qrcode-poll";

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

  function clearQRCodePolling() {
    if (qrPollTimerRef.current !== null) {
      window.clearTimeout(qrPollTimerRef.current);
      qrPollTimerRef.current = null;
    }
  }

  function resetQRCodeSession() {
    clearQRCodePolling();
    qrSessionRef.current = null;
    setNetworkQRCodeSession(null);
  }

  function scheduleQRCodePoll(delayMs = 2500) {
    clearQRCodePolling();
    qrPollTimerRef.current = window.setTimeout(() => {
      void pollNetworkStorageQRCode(false);
    }, delayMs);
  }

  function updateForm<K extends keyof EndpointFormState>(key: K, value: EndpointFormState[K]) {
    setForm((current) => ({
      ...current,
      [key]: value
    }));
  }

  function resetForm() {
    setEditingEndpointId(null);
    resetQRCodeSession();
    setForm(createEmptyForm(devices[0]?.mountPoint ?? ""));
  }

  function handleEditEndpoint(endpoint: CatalogEndpoint) {
    setNotice(null);
    setError(null);
    setEditingEndpointId(endpoint.id);
    resetQRCodeSession();
    setForm(createFormFromEndpoint(endpoint, devices));
  }

  async function handleStartNetworkQRCode() {
    if (form.endpointType !== "NETWORK_STORAGE" || form.networkProvider !== "115") {
      return;
    }

    resetQRCodeSession();
    setBusyAction("network-qrcode-start");
    setNotice(null);
    setError(null);

    try {
      const response = await startCloud115QRCodeLogin(backendUrl, form.networkAppType);
      if (!response.success || !response.qrCodeSession) {
        setError(response.error ?? "生成 115 登录二维码失败。");
        return;
      }

      qrSessionRef.current = response.qrCodeSession;
      setNetworkQRCodeSession(response.qrCodeSession);
      setNotice("二维码已生成，请使用 115 客户端扫码登录。");

      if (!response.qrCodeSession.credential) {
        scheduleQRCodePoll();
      }
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "生成 115 登录二维码失败。");
    } finally {
      setBusyAction(null);
    }
  }

  async function pollNetworkStorageQRCode(manual: boolean) {
    const session = qrSessionRef.current;
    if (!session) {
      return;
    }

    if (manual) {
      setBusyAction("network-qrcode-poll");
    }
    setError(null);

    try {
      const response = await pollCloud115QRCodeLogin(backendUrl, session);
      if (!response.success || !response.qrCodeSession) {
        setError(response.error ?? "轮询二维码登录状态失败。");
        return;
      }

      qrSessionRef.current = response.qrCodeSession;
      setNetworkQRCodeSession(response.qrCodeSession);

      if (response.qrCodeSession.credential) {
        clearQRCodePolling();
        setForm((current) => ({
          ...current,
          networkCredential: response.qrCodeSession?.credential?.trim() ?? current.networkCredential,
          networkAppType: response.qrCodeSession?.appType ?? current.networkAppType,
          networkLoginMethod: "qrcode",
          networkHasStoredCredential: true
        }));
        setNotice("扫码登录成功，凭证已写入表单。保存后会存到本机凭证库。");
        return;
      }

      const statusCode = response.qrCodeSession.statusCode;
      if ((statusCode === 0 || statusCode === 1) && !manual) {
        scheduleQRCodePoll();
      } else if (statusCode !== 0 && statusCode !== 1) {
        clearQRCodePolling();
      }
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "轮询二维码登录状态失败。");
    } finally {
      if (manual) {
        setBusyAction(null);
      }
    }
  }

  async function handleSubmitEndpoint() {
    if (
      form.endpointType === "NETWORK_STORAGE" &&
      form.networkProvider === "115" &&
      !form.networkCredential.trim() &&
      !form.networkHasStoredCredential
    ) {
      setError(form.networkLoginMethod === "qrcode" ? "请先扫码登录，或手动填写 115 凭证。" : "请填写 115 凭证。");
      return;
    }

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

      setNotice(editingEndpointId ? `已更新存储端点：${response.endpoint.name}` : `已添加存储端点：${response.endpoint.name}`);
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

      setNotice(`已删除 ${response.summary.endpointName}，移除了 ${response.summary.removedReplicaCount} 条副本记录。`);
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
    <section className="page-stack storage-page-shell">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">存储管理</p>
          <h3>端点</h3>
          <p>现在的网盘端点统一走一个入口配置。当前只开放 115 网盘，底层由打包的 AList 负责云盘交互。</p>
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
        <article className="detail-card storage-editor-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">{editingEndpointId ? "编辑端点" : "新增端点"}</p>
              <h4>{editingEndpointId ? "端点编辑" : "新建端点"}</h4>
            </div>

            <div className="endpoint-panel-actions">
              {editingEndpointId ? (
                <button type="button" className="ghost-button" onClick={resetForm} disabled={isSavingEndpoint}>
                  <X size={16} />
                  取消
                </button>
              ) : null}

              <button type="button" className="primary-button" onClick={() => void handleSubmitEndpoint()} disabled={isSavingEndpoint}>
                {isSavingEndpoint ? (
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
                placeholder={`例如：家用 NAS / 本地 SSD / ${currentTypeLabel}`}
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
              <select value={form.availabilityStatus} onChange={(event) => updateForm("availabilityStatus", event.target.value)}>
                <option value="AVAILABLE">可用</option>
                <option value="DISABLED">停用</option>
              </select>
            </label>

            <label className="field field-span">
              <span>备注</span>
              <textarea
                value={form.note}
                onChange={(event) => updateForm("note", event.target.value)}
                placeholder="可选备注、用途提示、路径含义或维护说明。"
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

            {form.endpointType === "NETWORK_STORAGE" ? (
              <>
                <label className="field">
                  <span>网盘类型</span>
                  <select
                    value={form.networkProvider}
                    onChange={(event) => updateForm("networkProvider", normalizeNetworkProvider(event.target.value))}
                  >
                    {networkProviderOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </label>

                <label className="field">
                  <span>登录方式</span>
                  <select
                    value={form.networkLoginMethod}
                    onChange={(event) => updateForm("networkLoginMethod", normalizeNetworkLoginMethod(event.target.value))}
                  >
                    {networkLoginMethodOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </label>

                <label className="field">
                  <span>根目录 ID</span>
                  <input
                    value={form.networkRootFolderId}
                    onChange={(event) => updateForm("networkRootFolderId", event.target.value)}
                    placeholder="0 表示全部文件"
                  />
                </label>

                <label className="field">
                  <span>扫码设备类型</span>
                  <select value={form.networkAppType} onChange={(event) => updateForm("networkAppType", event.target.value)}>
                    {cloud115AvailableAppOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </label>

                {form.networkLoginMethod === "qrcode" ? (
                  <div className="field field-span qr-session-panel">
                    <span>扫码登录</span>
                    <div className="action-row">
                      <button
                        type="button"
                        className="ghost-button"
                        onClick={() => void handleStartNetworkQRCode()}
                        disabled={isSavingEndpoint || isStartingQRCode || isPollingQRCode}
                      >
                        {isStartingQRCode ? <LoaderCircle size={16} className="spin" /> : null}
                        生成二维码
                      </button>
                      <button
                        type="button"
                        className="ghost-button"
                        onClick={() => void pollNetworkStorageQRCode(true)}
                        disabled={isSavingEndpoint || isStartingQRCode || isPollingQRCode || networkQRCodeSession === null}
                      >
                        {isPollingQRCode ? <LoaderCircle size={16} className="spin" /> : null}
                        轮询状态
                      </button>
                    </div>

                    {networkQRCodeSession ? (
                      <div className="qr-grid">
                        <div className="qr-box">
                          <img src={networkQRCodeSession.qrCodeUrl} alt="115 登录二维码" />
                        </div>
                        <div className="qr-meta">
                          <div>
                            <strong>状态：</strong>
                            {networkQRCodeSession.status}
                          </div>
                          <div>
                            <strong>设备：</strong>
                            {networkQRCodeSession.appType}
                          </div>
                          <div>
                            <strong>UID：</strong>
                            {networkQRCodeSession.uid}
                          </div>
                          <div>
                            <strong>时间：</strong>
                            {String(networkQRCodeSession.time)}
                          </div>
                        </div>
                      </div>
                    ) : (
                      <p className="secondary-text">生成二维码后，使用 115 客户端扫码。成功后会自动把凭证回填到表单。</p>
                    )}
                  </div>
                ) : null}

                <label className="field field-span">
                  <span>115 凭证 / Cookie</span>
                  <input
                    value={form.networkCredential}
                    onChange={(event) => {
                      updateForm("networkCredential", event.target.value);
                      if (event.target.value.trim()) {
                        updateForm("networkHasStoredCredential", true);
                      }
                    }}
                    placeholder={
                      form.networkLoginMethod === "qrcode"
                        ? "扫码成功后会自动填入，也可手动粘贴覆盖"
                        : "粘贴当前可用的 115 凭证或 Cookie 字符串"
                    }
                  />
                  <p className="secondary-text">{getNetworkCredentialHint(form, editingEndpointId !== null)}</p>
                </label>
              </>
            ) : null}

            {form.endpointType === "REMOVABLE" ? (
              <label className="field field-span">
                <span>设备挂载点</span>
                <select value={form.selectedMountPoint} onChange={(event) => updateForm("selectedMountPoint", event.target.value)}>
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

        <article className="detail-card storage-devices-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">检测到的设备</p>
              <h4>设备</h4>
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
                <p>如果要把 U 盘或移动硬盘登记为端点，请先接入设备。</p>
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

      <article className="detail-card storage-directory-card">
        <div className="section-head">
          <div>
            <p className="eyebrow">已连接端点</p>
            <h4>端点列表</h4>
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
              <p>可以先在上方添加本地磁盘、QNAP / SMB、网盘或可移动设备。</p>
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
                    <span>位置 / 根标识</span>
                    <strong>{getEndpointRootLabel(endpoint)}</strong>
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
        <article className="detail-card storage-summary-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">最近一次扫描</p>
              <h4>{isEndpointScanSummary(latestSummary) ? "端点扫描" : "全量扫描"}</h4>
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
    networkProvider: "115",
    networkStorageKey: "",
    networkMountPath: "",
    networkDriver: "",
    networkRootFolderId: "0",
    networkLoginMethod: "qrcode",
    networkAppType: "wechatmini",
    networkCredential: "",
    networkHasStoredCredential: false,
    networkPageSize: 1000,
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
    networkProvider: endpointType === "NETWORK_STORAGE" ? normalizeNetworkProvider(getString(config, "provider")) : "115",
    networkStorageKey: endpointType === "NETWORK_STORAGE" ? getString(config, "storageKey") : "",
    networkMountPath: endpointType === "NETWORK_STORAGE" ? getString(config, "mountPath") || endpoint.rootPath : "",
    networkDriver: endpointType === "NETWORK_STORAGE" ? getString(config, "driver") : "",
    networkRootFolderId:
      endpointType === "NETWORK_STORAGE" ? getString(config, "rootFolderId") || getString(config, "root_folder_id") || "0" : "0",
    networkLoginMethod:
      endpointType === "NETWORK_STORAGE" ? normalizeNetworkLoginMethod(getString(config, "loginMethod")) : "qrcode",
    networkAppType: endpointType === "NETWORK_STORAGE" ? getString(config, "appType") || "wechatmini" : "wechatmini",
    networkCredential: endpointType === "NETWORK_STORAGE" ? getString(config, "credential") : "",
    networkHasStoredCredential: endpointType === "NETWORK_STORAGE" ? endpoint.hasCredential || !!getString(config, "credential") : false,
    networkPageSize: endpointType === "NETWORK_STORAGE" ? getNumber(config, "pageSize", 1000) : 1000,
    selectedMountPoint
  };
}

function buildEndpointPayload(form: EndpointFormState, selectedDevice: DeviceInfo | null): CatalogEndpointPayload {
  const endpointType = resolveSubmissionEndpointType(form);

  switch (endpointType) {
    case "LOCAL":
      return {
        name: form.name.trim(),
        note: form.note.trim(),
        endpointType,
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
        endpointType,
        rootPath: form.qnapSharePath.trim(),
        roleMode: form.roleMode,
        availabilityStatus: form.availabilityStatus,
        connectionConfig: {
          sharePath: form.qnapSharePath.trim()
        }
      };
    case "NETWORK_STORAGE": {
      const connectionConfig: Record<string, unknown> = {
        provider: form.networkProvider,
        rootFolderId: form.networkRootFolderId.trim() || "0",
        loginMethod: form.networkLoginMethod,
        appType: form.networkAppType,
        pageSize: form.networkPageSize
      };

      if (form.networkStorageKey.trim()) {
        connectionConfig.storageKey = form.networkStorageKey.trim();
      }
      if (form.networkMountPath.trim()) {
        connectionConfig.mountPath = form.networkMountPath.trim();
      }
      if (form.networkDriver.trim()) {
        connectionConfig.driver = form.networkDriver.trim();
      }
      if (form.networkCredential.trim()) {
        connectionConfig.credential = form.networkCredential.trim();
      }

      return {
        name: form.name.trim(),
        note: form.note.trim(),
        endpointType,
        rootPath: "",
        roleMode: form.roleMode,
        availabilityStatus: form.availabilityStatus,
        connectionConfig
      };
    }
    case "REMOVABLE":
      if (!selectedDevice) {
        throw new Error("当前没有选中的可移动设备。");
      }
      return {
        name: form.name.trim(),
        note: form.note.trim(),
        endpointType,
        rootPath: selectedDevice.mountPoint,
        roleMode: form.roleMode,
        availabilityStatus: form.availabilityStatus,
        connectionConfig: {
          device: selectedDevice
        }
      };
    default:
      throw new Error(`不支持的端点类型：${endpointType}`);
  }
}

function resolveSubmissionEndpointType(form: EndpointFormState): EndpointType {
  return form.endpointType;
}

function normalizeEndpointType(value: string): EndpointType {
  switch (value) {
    case "QNAP_SMB":
      return "QNAP_SMB";
    case "NETWORK_STORAGE":
    case "NETWORK":
      return "NETWORK_STORAGE";
    case "REMOVABLE":
      return "REMOVABLE";
    default:
      return "LOCAL";
  }
}

function normalizeNetworkProvider(_value: string): NetworkProvider {
  return "115";
}

function normalizeNetworkLoginMethod(value: string): NetworkLoginMethod {
  return value === "manual" ? "manual" : "qrcode";
}

function getEndpointTypeLabel(endpointType: string) {
  switch (endpointType) {
    case "LOCAL":
      return "本地";
    case "QNAP_SMB":
      return "QNAP / SMB";
    case "NETWORK_STORAGE":
      return "网盘";
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

function getEndpointRootLabel(endpoint: CatalogEndpoint) {
  if (endpoint.endpointType !== "NETWORK_STORAGE") {
    return endpoint.rootPath;
  }

  const config = endpoint.connectionConfig ?? {};
  const provider = normalizeNetworkProvider(getString(config, "provider"));
  const rootFolderId = getString(config, "rootFolderId") || getString(config, "root_folder_id") || "0";
  const providerLabel = provider === "115" ? "115 网盘" : "网盘";

  if (rootFolderId === "0") {
    return `${providerLabel} / 全部文件`;
  }

  return `${providerLabel} / 目录 ID ${rootFolderId}`;
}

function getNetworkCredentialHint(form: EndpointFormState, isEditing: boolean) {
  if (form.networkCredential.trim()) {
    return isEditing ? "保存后将覆盖当前已保存的本机凭证。" : "保存后会写入本机凭证库，不会以明文长期展示。";
  }

  if (form.networkHasStoredCredential) {
    return "当前端点已保存本机凭证，留空则继续沿用。";
  }

  return form.networkLoginMethod === "qrcode"
    ? "建议先扫码登录，拿到有效凭证后再保存端点。"
    : "请填写当前可用的 115 凭证或 Cookie。";
}

function getString(config: Record<string, unknown>, key: string): string {
  const value = config[key];
  return typeof value === "string" ? value : "";
}

function getNumber(config: Record<string, unknown>, key: string, fallback: number): number {
  const value = config[key];
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
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
