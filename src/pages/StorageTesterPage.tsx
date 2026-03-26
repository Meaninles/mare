import { FormEvent, useEffect, useRef, useState } from "react";
import {
  getDefaultBackendUrl,
  pollCloud115QRCodeLogin,
  startCloud115QRCodeLogin,
  testNetworkStorageConnector,
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

const NETWORK_STORAGE_APP_OPTIONS = [
  { value: "wechatmini", label: "微信小程序" },
  { value: "alipaymini", label: "支付宝小程序" },
  { value: "qandroid", label: "115 生活" },
  { value: "tv", label: "电视" },
  { value: "android", label: "安卓" },
  { value: "ios", label: "iOS" },
  { value: "web", label: "网页" }
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
  const [networkStorage, setNetworkStorage] = useState({
    name: "网络存储（115 网盘）",
    provider: "115",
    rootFolderId: "0",
    appType: "wechatmini",
    loginMethod: "manual",
    credential: "",
    path: "",
    destinationPath: "",
    newName: "",
    recursive: false,
    includeDirectories: true,
    mediaOnly: false,
    limit: 50,
    content: "mare-network-storage-test"
  });
  const [qnapResult, setQnapResult] = useState<ConnectorTestResponse | null>(null);
  const [networkStorageResult, setNetworkStorageResult] = useState<ConnectorTestResponse | null>(null);
  const [networkStorageQR, setNetworkStorageQR] = useState<Cloud115QRCodeSession | null>(null);
  const [runningTarget, setRunningTarget] = useState<string | null>(null);
  const qrPollTimerRef = useRef<number | null>(null);
  const qrSessionRef = useRef<Cloud115QRCodeSession | null>(null);

  useEffect(() => {
    qrSessionRef.current = networkStorageQR;
  }, [networkStorageQR]);

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

  async function runNetworkStorage(operation: Operation) {
    setRunningTarget(`network-storage:${operation}`);
    try {
      const result = await testNetworkStorageConnector(backendUrl, {
        name: networkStorage.name,
        provider: networkStorage.provider,
        rootFolderId: networkStorage.rootFolderId,
        appType: networkStorage.appType,
        loginMethod: networkStorage.loginMethod,
        credential: networkStorage.credential,
        operation,
        path: networkStorage.path,
        destinationPath: networkStorage.destinationPath,
        newName: networkStorage.newName,
        recursive: networkStorage.recursive,
        includeDirectories: networkStorage.includeDirectories,
        mediaOnly: networkStorage.mediaOnly,
        limit: Number(networkStorage.limit),
        pageSize: 1000,
        content: networkStorage.content
      });
      setNetworkStorageResult(result);
    } finally {
      setRunningTarget(null);
    }
  }

  async function startQRCode() {
    if (qrPollTimerRef.current !== null) {
      window.clearTimeout(qrPollTimerRef.current);
      qrPollTimerRef.current = null;
    }
    setRunningTarget("network-storage:qrcode_start");
    try {
      const result = await startCloud115QRCodeLogin(backendUrl, networkStorage.appType);
      setNetworkStorageResult(result);
      if (result.qrCodeSession) {
        qrSessionRef.current = result.qrCodeSession;
        setNetworkStorageQR(result.qrCodeSession);
        setNetworkStorage((current) => ({ ...current, loginMethod: "qrcode" }));
        qrPollTimerRef.current = window.setTimeout(() => {
          void pollQRCode(false);
        }, 2500);
      }
    } finally {
      setRunningTarget(null);
    }
  }

  async function pollQRCode(manual = false) {
    const session = qrSessionRef.current;
    if (!session) {
      return;
    }

    setRunningTarget(manual ? "network-storage:qrcode_poll" : null);
    try {
      const result = await pollCloud115QRCodeLogin(backendUrl, session);
      setNetworkStorageResult(result);
      if (result.qrCodeSession) {
        qrSessionRef.current = result.qrCodeSession;
        setNetworkStorageQR(result.qrCodeSession);

        if (result.qrCodeSession.credential) {
          if (qrPollTimerRef.current !== null) {
            window.clearTimeout(qrPollTimerRef.current);
            qrPollTimerRef.current = null;
          }
          setNetworkStorage((current) => ({
            ...current,
            credential: result.qrCodeSession?.credential ?? current.credential,
            appType: result.qrCodeSession?.appType ?? current.appType,
            loginMethod: "qrcode"
          }));
          return;
        }

        if ((result.qrCodeSession.statusCode === 0 || result.qrCodeSession.statusCode === 1) && !manual) {
          qrPollTimerRef.current = window.setTimeout(() => {
            void pollQRCode(false);
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
        <p className="eyebrow">存储测试</p>
        <h3>在这里手动验证 QNAP / SMB 与网络存储（115 网盘）的连接和基础文件操作。</h3>
        <p>网络存储测试会复用正式配置模型与扫码流程，方便先验证凭证、目录访问和上传下载是否正常，再回到业务页面使用。</p>
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
              <h4>QNAP / SMB 测试</h4>
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
              <input value={qnap.destinationPath} onChange={(event) => setQnap({ ...qnap, destinationPath: event.target.value })} />
            </label>
            <label className="field">
              <span>新名称</span>
              <input value={qnap.newName} onChange={(event) => setQnap({ ...qnap, newName: event.target.value })} />
            </label>
            <label className="field field-span">
              <span>写入内容</span>
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
              <input
                type="checkbox"
                checked={qnap.includeDirectories}
                onChange={(event) => setQnap({ ...qnap, includeDirectories: event.target.checked })}
              />
              <span>包含目录</span>
            </label>
            <label className="checkbox-field">
              <input type="checkbox" checked={qnap.mediaOnly} onChange={(event) => setQnap({ ...qnap, mediaOnly: event.target.checked })} />
              <span>仅媒体文件</span>
            </label>
          </form>
          <div className="action-row">
            <button onClick={() => void runQNAP("health_check")} disabled={runningTarget !== null}>连接检查</button>
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
              <p className="eyebrow">网络存储</p>
              <h4>115 网盘测试</h4>
            </div>
          </div>
          <form className="field-grid" onSubmit={preventSubmit}>
            <label className="field">
              <span>名称</span>
              <input value={networkStorage.name} onChange={(event) => setNetworkStorage({ ...networkStorage, name: event.target.value })} />
            </label>
            <label className="field">
              <span>提供方</span>
              <input value={networkStorage.provider} readOnly />
            </label>
            <label className="field">
              <span>根目录 ID</span>
              <input
                value={networkStorage.rootFolderId}
                onChange={(event) => setNetworkStorage({ ...networkStorage, rootFolderId: event.target.value })}
              />
            </label>
            <label className="field">
              <span>扫码设备类型</span>
              <select value={networkStorage.appType} onChange={(event) => setNetworkStorage({ ...networkStorage, appType: event.target.value })}>
                {NETWORK_STORAGE_APP_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </label>

            <div className="field field-span qr-session-panel">
              <span>扫码登录</span>
              <div className="action-row">
                <button type="button" onClick={() => void startQRCode()} disabled={runningTarget !== null}>生成二维码</button>
                <button type="button" onClick={() => void pollQRCode(true)} disabled={runningTarget !== null || networkStorageQR === null}>轮询状态</button>
              </div>
              {networkStorageQR ? (
                <div className="qr-grid">
                  <div className="qr-box">
                    <img src={networkStorageQR.qrCodeUrl} alt="115 登录二维码" />
                  </div>
                  <div className="qr-meta">
                    <div><strong>状态：</strong>{networkStorageQR.status}</div>
                    <div><strong>设备：</strong>{networkStorageQR.appType}</div>
                    <div><strong>UID：</strong>{networkStorageQR.uid}</div>
                    <div><strong>时间：</strong>{networkStorageQR.time}</div>
                  </div>
                </div>
              ) : (
                <p className="secondary-text">建议优先扫码登录。成功后会自动回填 115 凭证，你也可以改用手动填写凭证继续测试。</p>
              )}
            </div>

            <label className="field field-span">
              <span>115 凭证 / Cookie</span>
              <input
                value={networkStorage.credential}
                onChange={(event) => setNetworkStorage({ ...networkStorage, credential: event.target.value, loginMethod: "manual" })}
              />
            </label>
            <label className="field">
              <span>路径</span>
              <input value={networkStorage.path} onChange={(event) => setNetworkStorage({ ...networkStorage, path: event.target.value })} />
            </label>
            <label className="field">
              <span>目标路径</span>
              <input
                value={networkStorage.destinationPath}
                onChange={(event) => setNetworkStorage({ ...networkStorage, destinationPath: event.target.value })}
              />
            </label>
            <label className="field">
              <span>新名称</span>
              <input value={networkStorage.newName} onChange={(event) => setNetworkStorage({ ...networkStorage, newName: event.target.value })} />
            </label>
            <label className="field field-span">
              <span>写入内容</span>
              <textarea
                value={networkStorage.content}
                onChange={(event) => setNetworkStorage({ ...networkStorage, content: event.target.value })}
              />
            </label>
            <label className="field">
              <span>数量上限</span>
              <input
                type="number"
                value={networkStorage.limit}
                onChange={(event) => setNetworkStorage({ ...networkStorage, limit: Number(event.target.value) })}
              />
            </label>
            <label className="checkbox-field">
              <input
                type="checkbox"
                checked={networkStorage.recursive}
                onChange={(event) => setNetworkStorage({ ...networkStorage, recursive: event.target.checked })}
              />
              <span>递归列出</span>
            </label>
            <label className="checkbox-field">
              <input
                type="checkbox"
                checked={networkStorage.includeDirectories}
                onChange={(event) => setNetworkStorage({ ...networkStorage, includeDirectories: event.target.checked })}
              />
              <span>包含目录</span>
            </label>
            <label className="checkbox-field">
              <input
                type="checkbox"
                checked={networkStorage.mediaOnly}
                onChange={(event) => setNetworkStorage({ ...networkStorage, mediaOnly: event.target.checked })}
              />
              <span>仅媒体文件</span>
            </label>
          </form>
          <div className="action-row">
            <button onClick={() => void runNetworkStorage("health_check")} disabled={runningTarget !== null}>连接检查</button>
            <button onClick={() => void runNetworkStorage("list_entries")} disabled={runningTarget !== null}>列目录</button>
            <button onClick={() => void runNetworkStorage("stat_entry")} disabled={runningTarget !== null}>读取信息</button>
            <button onClick={() => void runNetworkStorage("copy_in")} disabled={runningTarget !== null}>上传 / 修改</button>
            <button onClick={() => void runNetworkStorage("copy_out")} disabled={runningTarget !== null}>下载</button>
            <button onClick={() => void runNetworkStorage("delete_entry")} disabled={runningTarget !== null}>删除</button>
            <button onClick={() => void runNetworkStorage("rename_entry")} disabled={runningTarget !== null}>重命名</button>
            <button onClick={() => void runNetworkStorage("move_entry")} disabled={runningTarget !== null}>移动</button>
            <button onClick={() => void runNetworkStorage("make_directory")} disabled={runningTarget !== null}>建目录</button>
          </div>
          <ResultPanel result={networkStorageResult} />
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
