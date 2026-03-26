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

const NETWORK_APP_OPTIONS = [
  { value: "wechatmini", label: "微信小程序" },
  { value: "android", label: "安卓" },
  { value: "alipaymini", label: "支付宝小程序" },
  { value: "qandroid", label: "115 生活" },
  { value: "tv", label: "电视" },
  { value: "ios", label: "iOS" },
  { value: "web", label: "网页" }
] as const;

export function StorageTesterPage() {
  const [backendUrl, setBackendUrl] = useState(getDefaultBackendUrl());
  const [qnap, setQnap] = useState({
    name: "QNAP / SMB",
    sharePath: "",
    path: "",
    destinationPath: "",
    newName: "",
    recursive: false,
    includeDirectories: true,
    mediaOnly: false,
    limit: 50,
    content: "mam-qnap-test"
  });
  const [networkStorage, setNetworkStorage] = useState({
    name: "网络存储",
    provider: "115",
    loginMethod: "qrcode",
    rootFolderId: "0",
    appType: "wechatmini",
    credential: "",
    path: "",
    destinationPath: "",
    newName: "",
    recursive: false,
    includeDirectories: true,
    mediaOnly: false,
    limit: 50,
    content: "mam-network-storage-test"
  });
  const [qnapResult, setQnapResult] = useState<ConnectorTestResponse | null>(null);
  const [networkResult, setNetworkResult] = useState<ConnectorTestResponse | null>(null);
  const [qrSession, setQrSession] = useState<Cloud115QRCodeSession | null>(null);
  const [runningTarget, setRunningTarget] = useState<string | null>(null);
  const qrPollTimerRef = useRef<number | null>(null);
  const qrSessionRef = useRef<Cloud115QRCodeSession | null>(null);

  useEffect(() => {
    qrSessionRef.current = qrSession;
  }, [qrSession]);

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
    setRunningTarget(`network:${operation}`);
    try {
      const result = await testNetworkStorageConnector(backendUrl, {
        name: networkStorage.name,
        provider: networkStorage.provider,
        loginMethod: networkStorage.loginMethod,
        rootFolderId: networkStorage.rootFolderId,
        appType: networkStorage.appType,
        credential: networkStorage.credential,
        operation,
        path: networkStorage.path,
        destinationPath: networkStorage.destinationPath,
        newName: networkStorage.newName,
        recursive: networkStorage.recursive,
        includeDirectories: networkStorage.includeDirectories,
        mediaOnly: networkStorage.mediaOnly,
        limit: Number(networkStorage.limit),
        content: networkStorage.content
      });
      setNetworkResult(result);
    } finally {
      setRunningTarget(null);
    }
  }

  async function startQRCode() {
    if (qrPollTimerRef.current !== null) {
      window.clearTimeout(qrPollTimerRef.current);
      qrPollTimerRef.current = null;
    }

    setRunningTarget("network:qrcode_start");
    try {
      const result = await startCloud115QRCodeLogin(backendUrl, networkStorage.appType);
      setNetworkResult(result);
      if (!result.qrCodeSession) {
        return;
      }

      qrSessionRef.current = result.qrCodeSession;
      setQrSession(result.qrCodeSession);
      qrPollTimerRef.current = window.setTimeout(() => {
        void pollQRCode(false);
      }, 2500);
    } finally {
      setRunningTarget(null);
    }
  }

  async function pollQRCode(manual = false) {
    const currentSession = qrSessionRef.current;
    if (!currentSession) {
      return;
    }

    setRunningTarget(manual ? "network:qrcode_poll" : null);
    try {
      const result = await pollCloud115QRCodeLogin(backendUrl, currentSession);
      setNetworkResult(result);
      if (!result.qrCodeSession) {
        return;
      }

      qrSessionRef.current = result.qrCodeSession;
      setQrSession(result.qrCodeSession);

      if (result.qrCodeSession.credential) {
        if (qrPollTimerRef.current !== null) {
          window.clearTimeout(qrPollTimerRef.current);
          qrPollTimerRef.current = null;
        }
        setNetworkStorage((current) => ({
          ...current,
          loginMethod: "manual",
          credential: result.qrCodeSession?.credential ?? current.credential,
          appType: result.qrCodeSession?.appType ?? current.appType
        }));
        return;
      }

      if (!manual && (result.qrCodeSession.statusCode === 0 || result.qrCodeSession.statusCode === 1)) {
        qrPollTimerRef.current = window.setTimeout(() => {
          void pollQRCode(false);
        }, 2500);
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
        <h3>手动验证 QNAP / SMB 与网络存储端点的连接、目录和基础文件操作。</h3>
        <p>网络存储测试统一按“网络存储 / 115 网盘”模型执行，底层细节已经内部统一，不再要求用户理解额外的技术类型。</p>
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
              <p className="eyebrow">QNAP / SMB</p>
              <h4>本地网络共享测试</h4>
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
              <span>上传或覆盖内容</span>
              <textarea value={qnap.content} onChange={(event) => setQnap({ ...qnap, content: event.target.value })} />
            </label>
            <label className="field">
              <span>数量上限</span>
              <input
                type="number"
                value={qnap.limit}
                onChange={(event) => setQnap({ ...qnap, limit: Number(event.target.value) })}
              />
            </label>
            <label className="checkbox-field">
              <input
                type="checkbox"
                checked={qnap.recursive}
                onChange={(event) => setQnap({ ...qnap, recursive: event.target.checked })}
              />
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
              <input
                type="checkbox"
                checked={qnap.mediaOnly}
                onChange={(event) => setQnap({ ...qnap, mediaOnly: event.target.checked })}
              />
              <span>仅媒体文件</span>
            </label>
          </form>
          <ActionRow onRun={runQNAP} disabled={runningTarget !== null} />
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
              <input
                value={networkStorage.name}
                onChange={(event) => setNetworkStorage({ ...networkStorage, name: event.target.value })}
              />
            </label>
            <label className="field">
              <span>类型</span>
              <input value="115 网盘" readOnly />
            </label>
            <label className="field">
              <span>登录方式</span>
              <select
                value={networkStorage.loginMethod}
                onChange={(event) => setNetworkStorage({ ...networkStorage, loginMethod: event.target.value })}
              >
                <option value="qrcode">扫码登录</option>
                <option value="manual">手动填写凭证</option>
              </select>
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
              <select
                value={networkStorage.appType}
                onChange={(event) => setNetworkStorage({ ...networkStorage, appType: event.target.value })}
              >
                {NETWORK_APP_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </label>
            <div className="field field-span qr-session-panel">
              <span>扫码登录</span>
              <div className="action-row">
                <button type="button" onClick={() => void startQRCode()} disabled={runningTarget !== null}>
                  开始扫码登录
                </button>
                <button
                  type="button"
                  onClick={() => void pollQRCode(true)}
                  disabled={runningTarget !== null || qrSession === null}
                >
                  轮询扫码状态
                </button>
              </div>
              {qrSession ? (
                <div className="qr-grid">
                  <div className="qr-box">
                    <img src={qrSession.qrCodeUrl} alt="115 网盘二维码" />
                  </div>
                  <div className="qr-meta">
                    <div>
                      <strong>状态：</strong>
                      {qrSession.status}
                    </div>
                    <div>
                      <strong>设备：</strong>
                      {qrSession.appType}
                    </div>
                    <div>
                      <strong>UID：</strong>
                      {qrSession.uid}
                    </div>
                    <div>
                      <strong>时间：</strong>
                      {qrSession.time}
                    </div>
                  </div>
                </div>
              ) : (
                <p className="secondary-text">扫码成功后会自动回填 115 会话凭证，也可以切换到手动填写凭证。</p>
              )}
            </div>
            <label className="field field-span">
              <span>凭证</span>
              <input
                value={networkStorage.credential}
                onChange={(event) => setNetworkStorage({ ...networkStorage, credential: event.target.value })}
                placeholder="扫码成功后会自动填入，或手动粘贴 token / cookie"
              />
            </label>
            <label className="field">
              <span>路径</span>
              <input
                value={networkStorage.path}
                onChange={(event) => setNetworkStorage({ ...networkStorage, path: event.target.value })}
              />
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
              <input
                value={networkStorage.newName}
                onChange={(event) => setNetworkStorage({ ...networkStorage, newName: event.target.value })}
              />
            </label>
            <label className="field field-span">
              <span>上传或覆盖内容</span>
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
          <ActionRow onRun={runNetworkStorage} disabled={runningTarget !== null} />
          <ResultPanel result={networkResult} />
        </article>
      </div>
    </section>
  );
}

function ActionRow({
  onRun,
  disabled
}: {
  onRun: (operation: Operation) => Promise<void>;
  disabled: boolean;
}) {
  return (
    <div className="action-row">
      <button onClick={() => void onRun("health_check")} disabled={disabled}>
        连接检查
      </button>
      <button onClick={() => void onRun("list_entries")} disabled={disabled}>
        列目录
      </button>
      <button onClick={() => void onRun("stat_entry")} disabled={disabled}>
        读取信息
      </button>
      <button onClick={() => void onRun("copy_in")} disabled={disabled}>
        上传 / 修改
      </button>
      <button onClick={() => void onRun("copy_out")} disabled={disabled}>
        下载
      </button>
      <button onClick={() => void onRun("delete_entry")} disabled={disabled}>
        删除
      </button>
      <button onClick={() => void onRun("rename_entry")} disabled={disabled}>
        重命名
      </button>
      <button onClick={() => void onRun("move_entry")} disabled={disabled}>
        移动
      </button>
      <button onClick={() => void onRun("make_directory")} disabled={disabled}>
        建目录
      </button>
    </div>
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
