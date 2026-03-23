import { FormEvent, useEffect, useRef, useState } from "react";
import {
  getDefaultBackendUrl,
  pollCloud115QRCodeLogin,
  startCloud115QRCodeLogin,
  testCloud115Connector,
  testQNAPConnector
} from "../services/connector-test";
import type { Cloud115QRCodeSession, ConnectorTestResponse } from "../types/connector-test";

type Operation =
  | "health_check"
  | "list_entries"
  | "stat_entry"
  | "copy_in"
  | "copy_out"
  | "delete_entry"
  | "rename_entry"
  | "move_entry"
  | "make_directory";

const CLOUD115_APP_OPTIONS = [
  { value: "windows", label: "windows" },
  { value: "android", label: "android" },
  { value: "115android", label: "115android" },
  { value: "ios", label: "ios" },
  { value: "115ios", label: "115ios" },
  { value: "mac", label: "mac" },
  { value: "linux", label: "linux" },
  { value: "web", label: "web" }
];

export function StorageTesterPage() {
  const [backendUrl, setBackendUrl] = useState(getDefaultBackendUrl());
  const [qnap, setQnap] = useState({
    name: "QNAP SMB",
    sharePath: "",
    path: "",
    destinationPath: "",
    newName: "",
    recursive: false,
    includeDirectories: true,
    mediaOnly: false,
    limit: 50,
    content: "mare-qnap-test"
  });
  const [cloud115, setCloud115] = useState({
    name: "115 Cloud",
    rootId: "0",
    appType: "windows",
    accessToken: "",
    path: "",
    destinationPath: "",
    newName: "",
    recursive: false,
    includeDirectories: true,
    mediaOnly: false,
    limit: 50,
    content: "mare-115-test"
  });
  const [qnapResult, setQnapResult] = useState<ConnectorTestResponse | null>(null);
  const [cloud115Result, setCloud115Result] = useState<ConnectorTestResponse | null>(null);
  const [cloud115QR, setCloud115QR] = useState<Cloud115QRCodeSession | null>(null);
  const [runningTarget, setRunningTarget] = useState<string | null>(null);
  const qrPollTimerRef = useRef<number | null>(null);
  const cloud115QRRef = useRef<Cloud115QRCodeSession | null>(null);

  useEffect(() => {
    cloud115QRRef.current = cloud115QR;
  }, [cloud115QR]);

  useEffect(() => {
    return () => {
      if (qrPollTimerRef.current !== null) {
        window.clearTimeout(qrPollTimerRef.current);
      }
    };
  }, []);

  async function runQNAP(operation: Operation) {
    setRunningTarget(`qnap:${operation}`);
    try {
      const result = await testQNAPConnector(backendUrl, {
        name: qnap.name,
        sharePath: qnap.sharePath,
        operation,
        path: qnap.path,
        destinationPath: qnap.destinationPath,
        newName: qnap.newName,
        recursive: qnap.recursive,
        includeDirectories: qnap.includeDirectories,
        mediaOnly: qnap.mediaOnly,
        limit: Number(qnap.limit),
        content: qnap.content
      });
      setQnapResult(result);
    } finally {
      setRunningTarget(null);
    }
  }

  async function run115(operation: Operation) {
    setRunningTarget(`115:${operation}`);
    try {
      const result = await testCloud115Connector(backendUrl, {
        name: cloud115.name,
        rootId: cloud115.rootId,
        appType: cloud115.appType,
        accessToken: cloud115.accessToken,
        operation,
        path: cloud115.path,
        destinationPath: cloud115.destinationPath,
        newName: cloud115.newName,
        recursive: cloud115.recursive,
        includeDirectories: cloud115.includeDirectories,
        mediaOnly: cloud115.mediaOnly,
        limit: Number(cloud115.limit),
        content: cloud115.content
      });
      setCloud115Result(result);
    } finally {
      setRunningTarget(null);
    }
  }

  async function start115QRCode() {
    if (qrPollTimerRef.current !== null) {
      window.clearTimeout(qrPollTimerRef.current);
      qrPollTimerRef.current = null;
    }
    setRunningTarget("115:qrcode_start");
    try {
      const result = await startCloud115QRCodeLogin(backendUrl, cloud115.appType);
      setCloud115Result(result);
      if (result.qrCodeSession) {
        cloud115QRRef.current = result.qrCodeSession;
        setCloud115QR(result.qrCodeSession);
        qrPollTimerRef.current = window.setTimeout(() => {
          void poll115QRCode(false);
        }, 2500);
      }
    } finally {
      setRunningTarget(null);
    }
  }

  async function poll115QRCode(manual = false) {
    const session = cloud115QRRef.current;
    if (!session) {
      return;
    }

    setRunningTarget(manual ? "115:qrcode_poll" : null);
    try {
      const result = await pollCloud115QRCodeLogin(backendUrl, session);
      setCloud115Result(result);
      if (result.qrCodeSession) {
        cloud115QRRef.current = result.qrCodeSession;
        setCloud115QR(result.qrCodeSession);

        if (result.qrCodeSession.credential) {
          if (qrPollTimerRef.current !== null) {
            window.clearTimeout(qrPollTimerRef.current);
            qrPollTimerRef.current = null;
          }
          setCloud115((current) => ({
            ...current,
            accessToken: result.qrCodeSession?.credential ?? current.accessToken,
            appType: result.qrCodeSession?.appType ?? current.appType
          }));
          return;
        }

        if ((result.qrCodeSession.statusCode === 0 || result.qrCodeSession.statusCode === 1) && !manual) {
          qrPollTimerRef.current = window.setTimeout(() => {
            void poll115QRCode(false);
          }, 2500);
        }
      }
    } finally {
      if (manual) {
        setRunningTarget(null);
      }
    }
  }

  function preventSubmit(event: FormEvent) {
    event.preventDefault();
  }

  return (
    <section className="page-stack">
      <article className="hero-card">
        <p className="eyebrow">手动连接器验证</p>
        <h3>在这里手动验证 QNAP SMB 和 115 网盘的连接与文件操作。</h3>
        <p>
          请先启动 Go 后端。115 建议优先使用扫码登录，这样后端能拿到有效会话，并让所有 115 请求统一走同一设备类型。
        </p>
      </article>

      <article className="detail-card tester-card">
        <label className="field">
          <span>后端地址</span>
          <input value={backendUrl} onChange={(event) => setBackendUrl(event.target.value)} />
        </label>
      </article>

      <div className="tester-grid">
        <article className="detail-card tester-card">
          <div className="tester-header">
            <div>
              <p className="eyebrow">QNAP</p>
              <h4>SMB 连接器测试</h4>
            </div>
          </div>
          <form className="field-grid" onSubmit={preventSubmit}>
            <label className="field">
              <span>名称</span>
              <input value={qnap.name} onChange={(event) => setQnap({ ...qnap, name: event.target.value })} />
            </label>
            <label className="field field-span">
              <span>共享路径</span>
              <input
                placeholder="\\\\qnap\\share\\media"
                value={qnap.sharePath}
                onChange={(event) => setQnap({ ...qnap, sharePath: event.target.value })}
              />
            </label>
            <label className="field">
              <span>路径</span>
              <input value={qnap.path} onChange={(event) => setQnap({ ...qnap, path: event.target.value })} />
            </label>
            <label className="field">
              <span>目标路径</span>
              <input
                value={qnap.destinationPath}
                onChange={(event) => setQnap({ ...qnap, destinationPath: event.target.value })}
              />
            </label>
            <label className="field">
              <span>新名称</span>
              <input value={qnap.newName} onChange={(event) => setQnap({ ...qnap, newName: event.target.value })} />
            </label>
            <label className="field field-span">
              <span>上传 / 覆盖内容</span>
              <textarea value={qnap.content} onChange={(event) => setQnap({ ...qnap, content: event.target.value })} />
            </label>
            <label className="field">
              <span>数量上限</span>
              <input type="number" value={qnap.limit} onChange={(event) => setQnap({ ...qnap, limit: Number(event.target.value) })} />
            </label>
            <label className="checkbox-field">
              <input type="checkbox" checked={qnap.recursive} onChange={(event) => setQnap({ ...qnap, recursive: event.target.checked })} />
              <span>递归列出</span>
            </label>
            <label className="checkbox-field">
              <input type="checkbox" checked={qnap.includeDirectories} onChange={(event) => setQnap({ ...qnap, includeDirectories: event.target.checked })} />
              <span>包含目录</span>
            </label>
            <label className="checkbox-field">
              <input type="checkbox" checked={qnap.mediaOnly} onChange={(event) => setQnap({ ...qnap, mediaOnly: event.target.checked })} />
              <span>仅媒体文件</span>
            </label>
          </form>
          <div className="action-row">
            <button onClick={() => void runQNAP("health_check")} disabled={runningTarget !== null}>连接检测</button>
            <button onClick={() => void runQNAP("list_entries")} disabled={runningTarget !== null}>列目录</button>
            <button onClick={() => void runQNAP("stat_entry")} disabled={runningTarget !== null}>读取信息</button>
            <button onClick={() => void runQNAP("copy_in")} disabled={runningTarget !== null}>上传 / 修改</button>
            <button onClick={() => void runQNAP("copy_out")} disabled={runningTarget !== null}>下载</button>
            <button onClick={() => void runQNAP("delete_entry")} disabled={runningTarget !== null}>删除</button>
            <button onClick={() => void runQNAP("rename_entry")} disabled={runningTarget !== null}>重命名</button>
            <button onClick={() => void runQNAP("move_entry")} disabled={runningTarget !== null}>移动</button>
            <button onClick={() => void runQNAP("make_directory")} disabled={runningTarget !== null}>建目录</button>
          </div>
          <ResultPanel result={qnapResult} />
        </article>

        <article className="detail-card tester-card">
          <div className="tester-header">
            <div>
              <p className="eyebrow">115</p>
              <h4>网盘连接器测试</h4>
            </div>
          </div>
          <form className="field-grid" onSubmit={preventSubmit}>
            <label className="field">
              <span>名称</span>
              <input value={cloud115.name} onChange={(event) => setCloud115({ ...cloud115, name: event.target.value })} />
            </label>
            <label className="field">
              <span>根目录 ID</span>
              <input value={cloud115.rootId} onChange={(event) => setCloud115({ ...cloud115, rootId: event.target.value })} />
            </label>
            <label className="field">
              <span>设备类型</span>
              <select value={cloud115.appType} onChange={(event) => setCloud115({ ...cloud115, appType: event.target.value })}>
                {CLOUD115_APP_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </label>
            <div className="field field-span qr-session-panel">
              <span>扫码登录</span>
              <div className="action-row">
                <button type="button" onClick={() => void start115QRCode()} disabled={runningTarget !== null}>开始扫码登录</button>
                <button type="button" onClick={() => void poll115QRCode(true)} disabled={runningTarget !== null || cloud115QR === null}>轮询扫码状态</button>
              </div>
              {cloud115QR ? (
                <div className="qr-grid">
                  <div className="qr-box">
                    <img src={cloud115QR.qrCodeUrl} alt="115 二维码" />
                  </div>
                  <div className="qr-meta">
                    <div><strong>状态：</strong> {cloud115QR.status}</div>
                    <div><strong>设备：</strong> {cloud115QR.appType}</div>
                    <div><strong>UID：</strong> {cloud115QR.uid}</div>
                    <div><strong>时间：</strong> {cloud115QR.time}</div>
                  </div>
                </div>
              ) : (
                <p className="secondary-text">生成二维码后用 115 扫码，直到页面回填会话凭证为止。</p>
              )}
            </div>
            <label className="field field-span">
              <span>115 会话凭证</span>
              <input value={cloud115.accessToken} onChange={(event) => setCloud115({ ...cloud115, accessToken: event.target.value })} />
            </label>
            <label className="field">
              <span>路径</span>
              <input value={cloud115.path} onChange={(event) => setCloud115({ ...cloud115, path: event.target.value })} />
            </label>
            <label className="field">
              <span>目标路径</span>
              <input value={cloud115.destinationPath} onChange={(event) => setCloud115({ ...cloud115, destinationPath: event.target.value })} />
            </label>
            <label className="field">
              <span>新名称</span>
              <input value={cloud115.newName} onChange={(event) => setCloud115({ ...cloud115, newName: event.target.value })} />
            </label>
            <label className="field field-span">
              <span>上传 / 覆盖内容</span>
              <textarea value={cloud115.content} onChange={(event) => setCloud115({ ...cloud115, content: event.target.value })} />
            </label>
            <label className="field">
              <span>数量上限</span>
              <input type="number" value={cloud115.limit} onChange={(event) => setCloud115({ ...cloud115, limit: Number(event.target.value) })} />
            </label>
            <label className="checkbox-field">
              <input type="checkbox" checked={cloud115.recursive} onChange={(event) => setCloud115({ ...cloud115, recursive: event.target.checked })} />
              <span>递归列出</span>
            </label>
            <label className="checkbox-field">
              <input type="checkbox" checked={cloud115.includeDirectories} onChange={(event) => setCloud115({ ...cloud115, includeDirectories: event.target.checked })} />
              <span>包含目录</span>
            </label>
            <label className="checkbox-field">
              <input type="checkbox" checked={cloud115.mediaOnly} onChange={(event) => setCloud115({ ...cloud115, mediaOnly: event.target.checked })} />
              <span>仅媒体文件</span>
            </label>
          </form>
          <div className="action-row">
            <button onClick={() => void run115("health_check")} disabled={runningTarget !== null}>连接检测</button>
            <button onClick={() => void run115("list_entries")} disabled={runningTarget !== null}>列目录</button>
            <button onClick={() => void run115("stat_entry")} disabled={runningTarget !== null}>读取信息</button>
            <button onClick={() => void run115("copy_in")} disabled={runningTarget !== null}>上传 / 修改</button>
            <button onClick={() => void run115("copy_out")} disabled={runningTarget !== null}>下载</button>
            <button onClick={() => void run115("delete_entry")} disabled={runningTarget !== null}>删除</button>
            <button onClick={() => void run115("rename_entry")} disabled={runningTarget !== null}>重命名</button>
            <button onClick={() => void run115("move_entry")} disabled={runningTarget !== null}>移动</button>
            <button onClick={() => void run115("make_directory")} disabled={runningTarget !== null}>建目录</button>
          </div>
          <ResultPanel result={cloud115Result} />
        </article>
      </div>
    </section>
  );
}

function ResultPanel({ result }: { result: ConnectorTestResponse | null }) {
  return (
    <div className="result-panel">
      <p className="eyebrow">结果</p>
      <pre>{result ? JSON.stringify(result, null, 2) : "暂时还没有结果。"}</pre>
    </div>
  );
}
