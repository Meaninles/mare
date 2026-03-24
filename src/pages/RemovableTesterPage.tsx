import { FormEvent, useState } from "react";
import { getDefaultBackendUrl, listRemovableDevices, testRemovableConnector } from "../services/connector-test";
import type { ConnectorTestResponse, DeviceInfo } from "../types/connector-test";

type Operation = "health_check" | "list_entries" | "stat_entry" | "copy_in" | "copy_out" | "delete_entry";

export function RemovableTesterPage() {
  const [backendUrl, setBackendUrl] = useState(getDefaultBackendUrl());
  const [devices, setDevices] = useState<DeviceInfo[]>([]);
  const [selectedMountPoint, setSelectedMountPoint] = useState("");
  const [path, setPath] = useState("");
  const [destinationPath, setDestinationPath] = useState("");
  const [content, setContent] = useState("mare-removable-test");
  const [recursive, setRecursive] = useState(false);
  const [includeDirectories, setIncludeDirectories] = useState(true);
  const [mediaOnly, setMediaOnly] = useState(false);
  const [result, setResult] = useState<ConnectorTestResponse | null>(null);
  const [detectError, setDetectError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const selectedDevice = devices.find((device) => device.mountPoint === selectedMountPoint) ?? null;

  async function refreshDevices() {
    setIsLoading(true);
    try {
      const response = await listRemovableDevices(backendUrl);
      setDetectError(response.success ? null : response.error ?? "检测移动设备失败。");
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
    } catch (error) {
      setDevices([]);
      setSelectedMountPoint("");
      setDetectError(error instanceof Error ? error.message : "检测移动设备失败。");
    } finally {
      setIsLoading(false);
    }
  }

  async function runOperation(operation: Operation) {
    if (!selectedDevice) {
      return;
    }

    setIsLoading(true);
    try {
      const response = await testRemovableConnector(backendUrl, selectedDevice, {
        name: selectedDevice.volumeLabel || "移动设备",
        operation,
        path,
        destinationPath,
        recursive,
        includeDirectories,
        mediaOnly,
        content
      });
      setResult(response);
    } finally {
      setIsLoading(false);
    }
  }

  function preventSubmit(event: FormEvent) {
    event.preventDefault();
  }

  return (
    <section className="page-stack">
      <article className="hero-card">
        <p className="eyebrow">移动设备验证</p>
        <h3>检测已插入的 U 盘与移动硬盘，并验证可移动连接器的基础文件操作。</h3>
        <p>
          这个页面会调用 Go 后端里的 Windows 可移动设备识别器，方便直接执行连接检查、目录枚举、
          元数据读取、上传、下载与删除等诊断操作。
        </p>
      </article>

      <article className="detail-card tester-card">
        <label className="field">
          <span>后端地址</span>
          <input value={backendUrl} onChange={(event) => setBackendUrl(event.target.value)} />
        </label>
        <div className="action-row">
          <button onClick={() => void refreshDevices()} disabled={isLoading}>
            检测设备
          </button>
        </div>
      </article>

      <div className="tester-grid">
        <article className="detail-card tester-card">
          <div className="tester-header">
            <div>
              <p className="eyebrow">已识别设备</p>
              <h4>Windows USB / 外接存储</h4>
            </div>
          </div>
          <div className="device-list">
            {detectError ? <p className="error-copy">{detectError}</p> : null}
            {devices.length === 0 ? <p>暂未识别到可移动设备。</p> : null}
            {devices.map((device) => (
              <label key={`${device.mountPoint}-${device.volumeSerialNumber}`} className="device-item">
                <input
                  type="radio"
                  name="device"
                  checked={selectedMountPoint === device.mountPoint}
                  onChange={() => setSelectedMountPoint(device.mountPoint)}
                />
                <span>
                  {device.mountPoint} | {device.volumeLabel || "未命名"} | {device.fileSystem || "未知文件系统"} |{" "}
                  {device.model || device.interfaceType || "未知设备"}
                </span>
              </label>
            ))}
          </div>
        </article>

        <article className="detail-card tester-card">
          <div className="tester-header">
            <div>
              <p className="eyebrow">当前设备测试</p>
              <h4>{selectedDevice ? selectedDevice.mountPoint : "尚未选择设备"}</h4>
            </div>
          </div>
          <form className="field-grid" onSubmit={preventSubmit}>
            <label className="field">
              <span>路径</span>
              <input value={path} onChange={(event) => setPath(event.target.value)} />
            </label>
            <label className="field">
              <span>目标路径</span>
              <input value={destinationPath} onChange={(event) => setDestinationPath(event.target.value)} />
            </label>
            <label className="field field-span">
              <span>上传 / 覆盖内容</span>
              <textarea value={content} onChange={(event) => setContent(event.target.value)} />
            </label>
            <label className="checkbox-field">
              <input type="checkbox" checked={recursive} onChange={(event) => setRecursive(event.target.checked)} />
              <span>递归列出</span>
            </label>
            <label className="checkbox-field">
              <input
                type="checkbox"
                checked={includeDirectories}
                onChange={(event) => setIncludeDirectories(event.target.checked)}
              />
              <span>包含目录</span>
            </label>
            <label className="checkbox-field">
              <input type="checkbox" checked={mediaOnly} onChange={(event) => setMediaOnly(event.target.checked)} />
              <span>仅媒体文件</span>
            </label>
          </form>
          <div className="action-row">
            <button onClick={() => void runOperation("health_check")} disabled={!selectedDevice || isLoading}>
              连接检查
            </button>
            <button onClick={() => void runOperation("list_entries")} disabled={!selectedDevice || isLoading}>
              列目录
            </button>
            <button onClick={() => void runOperation("stat_entry")} disabled={!selectedDevice || isLoading}>
              读取信息
            </button>
            <button onClick={() => void runOperation("copy_in")} disabled={!selectedDevice || isLoading}>
              上传 / 修改
            </button>
            <button onClick={() => void runOperation("copy_out")} disabled={!selectedDevice || isLoading}>
              下载
            </button>
            <button onClick={() => void runOperation("delete_entry")} disabled={!selectedDevice || isLoading}>
              删除
            </button>
          </div>
          <div className="result-panel">
            <p className="eyebrow">结果</p>
            <pre>{result ? JSON.stringify(result, null, 2) : "暂时还没有结果。"}</pre>
          </div>
        </article>
      </div>
    </section>
  );
}
