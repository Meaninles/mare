import { useEffect, useMemo, useState } from "react";
import { listRemovableDevices } from "../services/connector-test";
import {
  getDefaultCatalogBackendUrl,
  listCatalogEndpoints,
  runCatalogEndpointScan,
  runCatalogFullScan,
  saveCatalogEndpoint
} from "../services/catalog";
import type { CatalogEndpoint, EndpointScanSummary, FullScanSummary } from "../types/catalog";
import type { DeviceInfo } from "../types/connector-test";

const CLOUD115_APP_OPTIONS = [
  { value: "windows", label: "windows" },
  { value: "android", label: "android" },
  { value: "ios", label: "ios" },
  { value: "mac", label: "mac" },
  { value: "linux", label: "linux" },
  { value: "web", label: "web" }
];

export function StoragePage() {
  const [backendUrl, setBackendUrl] = useState(getDefaultCatalogBackendUrl());
  const [endpointType, setEndpointType] = useState("LOCAL");
  const [name, setName] = useState("");
  const [roleMode, setRoleMode] = useState("MANAGED");
  const [availabilityStatus, setAvailabilityStatus] = useState("AVAILABLE");
  const [localRootPath, setLocalRootPath] = useState("");
  const [qnapSharePath, setQnapSharePath] = useState("");
  const [cloud115RootId, setCloud115RootId] = useState("0");
  const [cloud115AccessToken, setCloud115AccessToken] = useState("");
  const [cloud115AppType, setCloud115AppType] = useState("windows");
  const [devices, setDevices] = useState<DeviceInfo[]>([]);
  const [selectedMountPoint, setSelectedMountPoint] = useState("");
  const [endpoints, setEndpoints] = useState<CatalogEndpoint[]>([]);
  const [latestSummary, setLatestSummary] = useState<FullScanSummary | EndpointScanSummary | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isBusy, setIsBusy] = useState(false);

  const selectedDevice = useMemo(
    () => devices.find((device) => device.mountPoint === selectedMountPoint) ?? null,
    [devices, selectedMountPoint]
  );

  useEffect(() => {
    void refreshEndpoints();
  }, []);

  async function refreshEndpoints() {
    setIsBusy(true);
    setError(null);
    try {
      const response = await listCatalogEndpoints(backendUrl);
      if (!response.success) {
        setError(response.error ?? "读取端点列表失败。");
        return;
      }

      setEndpoints(response.endpoints ?? []);
    } finally {
      setIsBusy(false);
    }
  }

  async function detectRemovableDevices() {
    setIsBusy(true);
    setError(null);
    try {
      const response = await listRemovableDevices(backendUrl);
      if (!response.success) {
        setError(response.error ?? "检测移动设备失败。");
        return;
      }

      setDevices(response.devices ?? []);
      if (response.devices?.length) {
        setSelectedMountPoint((current) =>
          current && response.devices.some((device) => device.mountPoint === current)
            ? current
            : response.devices[0].mountPoint
        );
      } else {
        setSelectedMountPoint("");
      }
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "检测移动设备失败。");
    } finally {
      setIsBusy(false);
    }
  }

  async function createEndpoint() {
    setIsBusy(true);
    setNotice(null);
    setError(null);
    try {
      const payload = buildEndpointPayload();
      const response = await saveCatalogEndpoint(backendUrl, payload);
      if (!response.success) {
        setError(response.error ?? "保存端点失败。");
        return;
      }

      setNotice(`端点已保存：${response.endpoint?.name ?? payload.name}`);
      await refreshEndpoints();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "保存端点失败。");
    } finally {
      setIsBusy(false);
    }
  }

  async function runFullScan() {
    setIsBusy(true);
    setNotice(null);
    setError(null);
    try {
      const response = await runCatalogFullScan(backendUrl);
      if (!response.success) {
        setError(response.error ?? "执行全量扫描失败。");
        return;
      }

      setLatestSummary((response.summary as FullScanSummary) ?? null);
      setNotice("全量扫描完成，可以前往“资产库”页面查看统一 Catalog。");
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "执行全量扫描失败。");
    } finally {
      setIsBusy(false);
    }
  }

  async function runSingleEndpointScan(endpointId: string) {
    setIsBusy(true);
    setNotice(null);
    setError(null);
    try {
      const response = await runCatalogEndpointScan(backendUrl, endpointId);
      if (!response.success) {
        setError(response.error ?? "执行单端点重扫失败。");
        if (response.summary) {
          setLatestSummary(response.summary as EndpointScanSummary);
        }
        return;
      }

      setLatestSummary((response.summary as EndpointScanSummary) ?? null);
      setNotice("单端点重扫完成，资产和副本状态已经更新。");
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "执行单端点重扫失败。");
    } finally {
      setIsBusy(false);
    }
  }

  function buildEndpointPayload() {
    switch (endpointType) {
      case "LOCAL":
        return {
          name: name.trim(),
          endpointType,
          rootPath: localRootPath.trim(),
          roleMode,
          availabilityStatus,
          connectionConfig: {
            rootPath: localRootPath.trim()
          }
        };
      case "QNAP_SMB":
        return {
          name: name.trim(),
          endpointType,
          rootPath: qnapSharePath.trim(),
          roleMode,
          availabilityStatus,
          connectionConfig: {
            sharePath: qnapSharePath.trim()
          }
        };
      case "CLOUD_115":
        return {
          name: name.trim(),
          endpointType,
          rootPath: cloud115RootId.trim(),
          roleMode,
          availabilityStatus,
          connectionConfig: {
            rootId: cloud115RootId.trim(),
            accessToken: cloud115AccessToken.trim(),
            appType: cloud115AppType
          }
        };
      case "REMOVABLE":
        return {
          name: name.trim(),
          endpointType,
          rootPath: selectedDevice?.mountPoint ?? "",
          roleMode,
          availabilityStatus,
          connectionConfig: {
            device: selectedDevice
          }
        };
      default:
        return {
          name: name.trim(),
          endpointType,
          rootPath: "",
          roleMode,
          availabilityStatus,
          connectionConfig: {}
        };
    }
  }

  return (
    <section className="page-stack">
      <article className="hero-card">
        <p className="eyebrow">端点配置与扫描</p>
        <h3>在这里注册 Catalog 端点，并触发全量扫描或单端点重扫。</h3>
        <p>
          这页用于验证 D2-D5 主流程：端点写入 `storage_endpoints`、扫描任务写入 `tasks`、
          资产归并写入 `assets / replicas / replica_versions`，以及单端点重扫后的缺失副本标记。
        </p>
      </article>

      <article className="detail-card tester-card">
        <div className="tester-header">
          <div>
            <p className="eyebrow">连接设置</p>
            <h4>后端地址与扫描入口</h4>
          </div>
        </div>
        <div className="field-grid">
          <label className="field field-span">
            <span>Go 后端地址</span>
            <input value={backendUrl} onChange={(event) => setBackendUrl(event.target.value)} />
          </label>
        </div>
        <div className="action-row">
          <button onClick={() => void refreshEndpoints()} disabled={isBusy}>
            刷新端点列表
          </button>
          <button onClick={() => void runFullScan()} disabled={isBusy}>
            全量扫描全部已启用端点
          </button>
        </div>
        {notice ? <p className="success-copy">{notice}</p> : null}
        {error ? <p className="error-copy">{error}</p> : null}
      </article>

      <div className="tester-grid">
        <article className="detail-card tester-card">
          <div className="tester-header">
            <div>
              <p className="eyebrow">新增端点</p>
              <h4>注册本地、QNAP、115 或移动设备端点</h4>
            </div>
          </div>

          <div className="field-grid">
            <label className="field">
              <span>端点类型</span>
              <select value={endpointType} onChange={(event) => setEndpointType(event.target.value)}>
                <option value="LOCAL">本地目录</option>
                <option value="QNAP_SMB">QNAP / SMB</option>
                <option value="CLOUD_115">115 网盘</option>
                <option value="REMOVABLE">移动硬盘 / U 盘</option>
              </select>
            </label>

            <label className="field">
              <span>端点名称</span>
              <input
                placeholder="可留空，系统会给默认名称"
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </label>

            <label className="field">
              <span>角色</span>
              <select value={roleMode} onChange={(event) => setRoleMode(event.target.value)}>
                <option value="MANAGED">管理存储</option>
                <option value="IMPORT_SOURCE">导入源</option>
              </select>
            </label>

            <label className="field">
              <span>可用状态</span>
              <select value={availabilityStatus} onChange={(event) => setAvailabilityStatus(event.target.value)}>
                <option value="AVAILABLE">启用</option>
                <option value="DISABLED">禁用</option>
              </select>
            </label>

            {endpointType === "LOCAL" ? (
              <label className="field field-span">
                <span>本地根目录</span>
                <input
                  placeholder="例如：D:\\Media"
                  value={localRootPath}
                  onChange={(event) => setLocalRootPath(event.target.value)}
                />
              </label>
            ) : null}

            {endpointType === "QNAP_SMB" ? (
              <label className="field field-span">
                <span>QNAP 共享路径</span>
                <input
                  placeholder="例如：\\\\qnap\\share\\media"
                  value={qnapSharePath}
                  onChange={(event) => setQnapSharePath(event.target.value)}
                />
              </label>
            ) : null}

            {endpointType === "CLOUD_115" ? (
              <>
                <label className="field">
                  <span>115 根目录 ID</span>
                  <input value={cloud115RootId} onChange={(event) => setCloud115RootId(event.target.value)} />
                </label>

                <label className="field">
                  <span>115 设备类型</span>
                  <select
                    value={cloud115AppType}
                    onChange={(event) => setCloud115AppType(event.target.value)}
                  >
                    {CLOUD115_APP_OPTIONS.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </label>

                <label className="field field-span">
                  <span>115 会话凭证</span>
                  <textarea
                    placeholder="填写扫码登录得到的 credential / cookie"
                    value={cloud115AccessToken}
                    onChange={(event) => setCloud115AccessToken(event.target.value)}
                  />
                </label>
              </>
            ) : null}

            {endpointType === "REMOVABLE" ? (
              <>
                <div className="field field-span">
                  <span>移动设备检测</span>
                  <div className="action-row">
                    <button type="button" onClick={() => void detectRemovableDevices()} disabled={isBusy}>
                      检测已插入的移动设备
                    </button>
                  </div>
                </div>

                <label className="field field-span">
                  <span>选择设备</span>
                  <select
                    value={selectedMountPoint}
                    onChange={(event) => setSelectedMountPoint(event.target.value)}
                  >
                    <option value="">请选择设备</option>
                    {devices.map((device) => (
                      <option key={`${device.mountPoint}-${device.volumeSerialNumber}`} value={device.mountPoint}>
                        {device.mountPoint} | {device.volumeLabel || "未命名"} | {device.fileSystem || "未知文件系统"}
                      </option>
                    ))}
                  </select>
                </label>

                {selectedDevice ? (
                  <div className="inline-note">
                    当前设备：{selectedDevice.mountPoint} / {selectedDevice.volumeLabel || "未命名"} /{" "}
                    {selectedDevice.model || selectedDevice.interfaceType || "未知型号"}
                  </div>
                ) : null}
              </>
            ) : null}
          </div>

          <div className="action-row">
            <button onClick={() => void createEndpoint()} disabled={isBusy}>
              保存端点到 Catalog
            </button>
          </div>
        </article>

        <article className="detail-card tester-card">
          <div className="tester-header">
            <div>
              <p className="eyebrow">已配置端点</p>
              <h4>逐个重扫并查看当前配置</h4>
            </div>
          </div>

          <div className="endpoint-list">
            {endpoints.length === 0 ? <p className="secondary-text">还没有已注册端点。</p> : null}
            {endpoints.map((endpoint) => (
              <div key={endpoint.id} className="endpoint-card">
                <div className="endpoint-card-header">
                  <div>
                    <strong>{endpoint.name}</strong>
                    <div className="secondary-text">{endpoint.endpointType}</div>
                  </div>
                  <span className={`status-pill ${endpoint.availabilityStatus === "AVAILABLE" ? "success" : ""}`}>
                    {endpoint.availabilityStatus}
                  </span>
                </div>
                <div className="endpoint-meta">
                  <div>根路径 / 根 ID：{endpoint.rootPath}</div>
                  <div>角色：{endpoint.roleMode}</div>
                  <div>身份签名：{endpoint.identitySignature}</div>
                </div>
                <div className="action-row">
                  <button onClick={() => void runSingleEndpointScan(endpoint.id)} disabled={isBusy}>
                    重扫该端点
                  </button>
                </div>
              </div>
            ))}
          </div>
        </article>
      </div>

      <article className="detail-card tester-card">
        <div className="tester-header">
          <div>
            <p className="eyebrow">最新扫描结果</p>
            <h4>用于人工核对任务摘要和副本缺失标记</h4>
          </div>
        </div>
        <div className="result-panel">
          <pre>{latestSummary ? JSON.stringify(latestSummary, null, 2) : "尚未执行扫描。"}</pre>
        </div>
      </article>
    </section>
  );
}
