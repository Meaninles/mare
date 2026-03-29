import { useEffect, useState } from "react";
import { Cookie, FolderOpen, HardDrive, LoaderCircle, RefreshCcw, Save, ScanQrCode, ShieldCheck, Trash2 } from "lucide-react";
import {
  deleteCatalogEndpoint,
  getDefaultCatalogBackendUrl,
  runCatalogEndpointScan,
  runCatalogFullScan,
  saveCatalogEndpoint,
  updateCatalogEndpoint
} from "../services/catalog";
import {
  use115QRCodeSession,
  useCD2AuthProfile,
  useCD2CloudAccounts,
  useClearCD2AuthProfile,
  useImport115Cookie,
  useListCD2Files,
  useRefreshCD2AuthProfile,
  useRemoveCD2CloudAccount,
  useStart115OpenQRCode,
  useUpdateCD2AuthProfile
} from "../hooks/useCD2";
import { useCatalogEndpoints } from "../hooks/useCatalog";
import { formatCatalogDate } from "../lib/catalog-view";
import { getCatalogEndpointTypeLabel } from "../lib/storage-endpoints";
import type { CatalogEndpointPayload } from "../types/catalog";
import type { CD2AuthMode } from "../types/cd2";

const backendUrl = getDefaultCatalogBackendUrl();

export function StoragePage() {
  const authQuery = useCD2AuthProfile();
  const accountsQuery = useCD2CloudAccounts();
  const endpointsQuery = useCatalogEndpoints();
  const updateAuth = useUpdateCD2AuthProfile();
  const refreshAuth = useRefreshCD2AuthProfile();
  const clearAuth = useClearCD2AuthProfile();
  const importCookie = useImport115Cookie();
  const startQRCode = useStart115OpenQRCode();
  const removeAccount = useRemoveCD2CloudAccount();
  const listFiles = useListCD2Files();

  const [mode, setMode] = useState<CD2AuthMode>("password");
  const [serverAddress, setServerAddress] = useState("127.0.0.1:29798");
  const [userName, setUserName] = useState("");
  const [password, setPassword] = useState("");
  const [apiToken, setApiToken] = useState("");
  const [cookieText, setCookieText] = useState("");
  const [qrPlatform, setQRPlatform] = useState("wechatmini");
  const [qrSessionId, setQRSessionId] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const [editingEndpointId, setEditingEndpointId] = useState<string | null>(null);
  const [endpointName, setEndpointName] = useState("");
  const [endpointNote, setEndpointNote] = useState("");
  const [cloudKey, setCloudKey] = useState("");
  const [browsePath, setBrowsePath] = useState("/");
  const [rootPath, setRootPath] = useState("");

  const qrSessionQuery = use115QRCodeSession(qrSessionId);
  const accounts = accountsQuery.data ?? [];
  const endpoints = endpointsQuery.data ?? [];
  const cd2Endpoints = endpoints.filter((item) => item.endpointType.trim().toUpperCase() === "CD2");
  const selectedAccount = accounts.find((item) => `${item.cloudName}::${item.userName}` === cloudKey) ?? null;
  const currentPath = browsePath.trim() || selectedAccount?.path || "/";
  const directories = (listFiles.data?.entries ?? []).filter((item) => item.isDirectory);

  useEffect(() => {
    const profile = authQuery.data?.profile;
    if (!profile) {
      return;
    }
    setMode(profile.mode);
    setServerAddress(profile.serverAddress || authQuery.data?.client.target || "127.0.0.1:29798");
    setUserName(profile.userName || "");
  }, [authQuery.data]);

  useEffect(() => {
    if (selectedAccount || accounts.length === 0) {
      return;
    }
    const first = accounts[0];
    setCloudKey(`${first.cloudName}::${first.userName}`);
    setBrowsePath(first.path || "/");
    setRootPath(first.path || "/");
  }, [accounts, selectedAccount]);

  useEffect(() => {
    if (!selectedAccount || !authQuery.data?.client.authReady) {
      return;
    }
    void listFiles.mutateAsync({ path: currentPath }).catch(() => undefined);
  }, [authQuery.data?.client.authReady, currentPath, listFiles, selectedAccount]);

  useEffect(() => {
    if (qrSessionQuery.data?.finishedAt && qrSessionQuery.data.status === "success") {
      setNotice("115open 扫码接入成功。");
      setError(null);
      void accountsQuery.refetch();
    }
  }, [accountsQuery, qrSessionQuery.data]);

  async function saveAuthProfile() {
    setNotice(null);
    setError(null);
    try {
      const status = await updateAuth.mutateAsync({
        mode,
        serverAddress: serverAddress.trim(),
        userName: userName.trim(),
        password: mode === "password" ? password : undefined,
        apiToken: mode === "api_token" ? apiToken.trim() : undefined,
        managedTokenFriendlyName: "mam-backend",
        managedTokenRootDir: "/"
      });
      setNotice(status.client.authReady ? "CD2 认证已保存并验证成功。" : "CD2 配置已保存，但认证尚未就绪。");
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "保存 CD2 认证失败。");
    }
  }

  async function saveEndpoint() {
    setNotice(null);
    setError(null);
    if (!selectedAccount || !rootPath.trim()) {
      setError("请先选择云账号并确定扫描根目录。");
      return;
    }
    const payload: CatalogEndpointPayload = {
      name: endpointName.trim() || `${selectedAccount.displayName}-${rootPath.split("/").filter(Boolean).pop() || "root"}`,
      note: endpointNote.trim(),
      endpointType: "CD2",
      rootPath: rootPath.trim(),
      roleMode: "MANAGED",
      availabilityStatus: "AVAILABLE",
      connectionConfig: {
        rootPath: rootPath.trim(),
        cloudName: selectedAccount.cloudName,
        userName: selectedAccount.userName
      }
    };
    const response = editingEndpointId
      ? await updateCatalogEndpoint(backendUrl, editingEndpointId, payload)
      : await saveCatalogEndpoint(backendUrl, payload);
    if (!response.success || !response.endpoint) {
      setError(response.error ?? "保存 CD2 目录端点失败。");
      return;
    }
    setNotice(editingEndpointId ? `已更新端点：${response.endpoint.name}` : `已创建端点：${response.endpoint.name}`);
    setEditingEndpointId(null);
    setEndpointName("");
    setEndpointNote("");
    await endpointsQuery.refetch();
  }

  return (
    <section className="page-stack">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">存储管理</p>
          <h3>正式流程已切换为 CD2 云盘接入、目录端点管理和扫描入库。</h3>
          <p>旧的 115/AList 桥接表单已从正式页面移除。现在先完成 CD2 认证，再接入云账号，最后把目录保存为正式扫描端点。</p>
        </div>
        <div className="hero-metrics">
          <MetricCard label="CD2 连通" value={authQuery.data?.client.connected ? "正常" : "未连接"} />
          <MetricCard label="认证状态" value={authQuery.data?.client.authReady ? "已就绪" : "未就绪"} />
          <MetricCard label="云账号" value={String(accounts.length)} />
          <MetricCard label="CD2 端点" value={String(cd2Endpoints.length)} />
        </div>
      </article>

      {notice ? <p className="inline-note">{notice}</p> : null}
      {error ? <p className="error-copy">{error}</p> : null}

      <article className="detail-card">
        <div className="section-head">
          <div>
            <p className="eyebrow">CD2 认证</p>
            <h4>后端正式运行配置</h4>
          </div>
          <div className="action-row">
            <button type="button" className="ghost-button" onClick={() => void refreshAuth.mutateAsync().then(() => setNotice("CD2 状态已刷新。")).catch((e) => setError(e instanceof Error ? e.message : "刷新失败"))}>
              <RefreshCcw size={16} />
              刷新状态
            </button>
            <button type="button" className="ghost-button danger-text" onClick={() => void clearAuth.mutateAsync().then(() => setNotice("已清除 CD2 认证配置。")).catch((e) => setError(e instanceof Error ? e.message : "清除失败"))}>
              清除配置
            </button>
          </div>
        </div>
        <div className="form-grid">
          <label className="field">
            <span>认证模式</span>
            <select value={mode} onChange={(e) => setMode(e.target.value as CD2AuthMode)}>
              <option value="password">账号密码</option>
              <option value="api_token">API Token</option>
            </select>
          </label>
          <label className="field">
            <span>CD2 服务地址</span>
            <input value={serverAddress} onChange={(e) => setServerAddress(e.target.value)} />
          </label>
          {mode === "password" ? (
            <>
              <label className="field">
                <span>CD2 账号邮箱</span>
                <input value={userName} onChange={(e) => setUserName(e.target.value)} />
              </label>
              <label className="field">
                <span>密码</span>
                <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
              </label>
            </>
          ) : (
            <label className="field">
              <span>API Token</span>
              <textarea value={apiToken} onChange={(e) => setApiToken(e.target.value)} />
            </label>
          )}
        </div>
        <div className="action-row">
          <button type="button" className="primary-button" onClick={() => void saveAuthProfile()} disabled={updateAuth.isPending}>
            {updateAuth.isPending ? <LoaderCircle size={16} className="spin" /> : <Save size={16} />}
            保存并验证
          </button>
        </div>
      </article>

      <div className="page-grid import-layout import-page-grid">
        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">云账号接入</p>
              <h4>115open 与 Cookie 导入</h4>
            </div>
          </div>
          <div className="form-grid">
            <label className="field">
              <span>115 Cookie 导入</span>
              <textarea value={cookieText} onChange={(e) => setCookieText(e.target.value)} placeholder="粘贴 editthiscookie" />
            </label>
            <label className="field">
              <span>115open 扫码平台</span>
              <select value={qrPlatform} onChange={(e) => setQRPlatform(e.target.value)}>
                {["wechatmini", "alipaymini", "qandroid", "tv", "android", "ios", "web"].map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <div className="action-row">
            <button type="button" className="ghost-button" onClick={() => void importCookie.mutateAsync({ editThisCookie: cookieText.trim() }).then(() => { setCookieText(""); setNotice("115 Cookie 已导入 CD2。"); return accountsQuery.refetch(); }).catch((e) => setError(e instanceof Error ? e.message : "导入失败"))} disabled={importCookie.isPending}>
              {importCookie.isPending ? <LoaderCircle size={16} className="spin" /> : <Cookie size={16} />}
              导入 Cookie
            </button>
            <button type="button" className="primary-button" onClick={() => void startQRCode.mutateAsync({ platform: qrPlatform }).then((session) => { setQRSessionId(session.id); setNotice("115open 二维码已生成。"); }).catch((e) => setError(e instanceof Error ? e.message : "扫码启动失败"))} disabled={startQRCode.isPending}>
              {startQRCode.isPending ? <LoaderCircle size={16} className="spin" /> : <ScanQrCode size={16} />}
              启动 115open 扫码
            </button>
          </div>
          {qrSessionQuery.data ? (
            <div className="settings-note-card">
              <ShieldCheck size={18} />
              <div>
                <strong>扫码状态：{qrSessionQuery.data.status}</strong>
                <p>{qrSessionQuery.data.qrCodeContent || qrSessionQuery.data.lastMessage || "等待扫码中"}</p>
              </div>
            </div>
          ) : null}
        </article>

        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">已接入云账号</p>
              <h4>当前 CD2 云账号</h4>
            </div>
            <button type="button" className="ghost-button" onClick={() => void accountsQuery.refetch()} disabled={accountsQuery.isFetching}>
              {accountsQuery.isFetching ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
              刷新账号
            </button>
          </div>
          {accounts.length === 0 ? (
            <div className="sync-empty-block">
              <HardDrive size={18} />
              <div>
                <strong>当前没有可用云账号</strong>
                <p>请先完成 CD2 认证，并接入至少一个云账号。</p>
              </div>
            </div>
          ) : (
            <div className="endpoint-grid compact-grid">
              {accounts.map((account) => (
                <article key={`${account.cloudName}-${account.userName}`} className="endpoint-panel compact-panel">
                  <div className="endpoint-panel-head">
                    <strong>{account.displayName}</strong>
                    <span className={`status-pill ${account.isLocked ? "warning" : "success"}`}>{account.isLocked ? "锁定" : "正常"}</span>
                  </div>
                  <p>{account.cloudName} / {account.userName}</p>
                  <small>{account.path || "/"}</small>
                  <div className="endpoint-panel-actions">
                    <button type="button" className="danger-button" onClick={() => void removeAccount.mutateAsync({ cloudName: account.cloudName, userName: account.userName }).then(() => { setNotice(`已移除云账号：${account.displayName}`); return accountsQuery.refetch(); }).catch((e) => setError(e instanceof Error ? e.message : "移除失败"))}>
                      <Trash2 size={15} />
                      移除账号
                    </button>
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
            <p className="eyebrow">CD2 目录端点</p>
            <h4>保存目录并执行扫描入库</h4>
          </div>
          <div className="action-row">
            <button type="button" className="ghost-button" onClick={() => setBrowsePath(currentPath === "/" ? "/" : `/${currentPath.split("/").filter(Boolean).slice(0, -1).join("/")}`)} disabled={currentPath === "/"}>
              <FolderOpen size={16} />
              返回上级
            </button>
            <button type="button" className="ghost-button" onClick={() => void listFiles.mutateAsync({ path: currentPath, forceRefresh: true })} disabled={listFiles.isPending}>
              {listFiles.isPending ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
              刷新目录
            </button>
            <button type="button" className="primary-button" onClick={() => void saveEndpoint()}>
              <Save size={16} />
              {editingEndpointId ? "更新端点" : "保存端点"}
            </button>
          </div>
        </div>
        <div className="form-grid">
          <label className="field">
            <span>云账号</span>
            <select value={cloudKey} onChange={(e) => { const next = accounts.find((item) => `${item.cloudName}::${item.userName}` === e.target.value) ?? null; setCloudKey(e.target.value); setBrowsePath(next?.path || "/"); setRootPath(next?.path || "/"); }}>
              {accounts.map((account) => (
                <option key={`${account.cloudName}::${account.userName}`} value={`${account.cloudName}::${account.userName}`}>
                  {account.displayName}
                </option>
              ))}
            </select>
          </label>
          <label className="field">
            <span>端点名称</span>
            <input value={endpointName} onChange={(e) => setEndpointName(e.target.value)} placeholder="例如：115open-素材总库" />
          </label>
          <label className="field">
            <span>备注</span>
            <input value={endpointNote} onChange={(e) => setEndpointNote(e.target.value)} />
          </label>
          <label className="field">
            <span>当前浏览路径</span>
            <input value={currentPath} readOnly />
          </label>
          <label className="field">
            <span>已选扫描根目录</span>
            <input value={rootPath} readOnly />
          </label>
        </div>
        <div className="endpoint-grid compact-grid">
          {directories.map((entry) => (
            <article key={entry.fullPathName} className="endpoint-panel compact-panel">
              <div className="endpoint-panel-head">
                <strong>{entry.name}</strong>
                <span className="status-pill subtle">目录</span>
              </div>
              <small>{entry.fullPathName}</small>
              <div className="endpoint-panel-actions">
                <button type="button" className="ghost-button" onClick={() => setBrowsePath(entry.fullPathName)}>
                  <FolderOpen size={15} />
                  进入
                </button>
                <button type="button" className="ghost-button" onClick={() => { setRootPath(entry.fullPathName); if (!endpointName.trim() && selectedAccount) { setEndpointName(`${selectedAccount.displayName}-${entry.name}`); } }}>
                  <Save size={15} />
                  选为根目录
                </button>
              </div>
            </article>
          ))}
        </div>
      </article>

      <article className="detail-card">
        <div className="section-head">
          <div>
            <p className="eyebrow">正式扫描端点</p>
            <h4>当前资产库中的 CD2 端点</h4>
          </div>
          <div className="action-row">
            <button type="button" className="ghost-button" onClick={() => void endpointsQuery.refetch()} disabled={endpointsQuery.isFetching}>
              {endpointsQuery.isFetching ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
              刷新端点
            </button>
            <button type="button" className="primary-button" onClick={() => void runCatalogFullScan(backendUrl).then((r) => setNotice(r.success && r.summary ? "全库扫描完成。" : r.error || "扫描失败")).catch((e) => setError(e instanceof Error ? e.message : "扫描失败"))} disabled={cd2Endpoints.length === 0}>
              <RefreshCcw size={16} />
              执行全库扫描
            </button>
          </div>
        </div>
        {cd2Endpoints.length === 0 ? (
          <div className="sync-empty-block">
            <HardDrive size={18} />
            <div>
              <strong>当前还没有 CD2 目录端点</strong>
              <p>先从上方目录浏览器中选一个目录保存为端点。</p>
            </div>
          </div>
        ) : (
          <div className="endpoint-grid">
            {cd2Endpoints.map((endpoint) => (
              <article key={endpoint.id} className="endpoint-panel">
                <div className="endpoint-panel-head">
                  <div>
                    <strong>{endpoint.name}</strong>
                    <p>{getCatalogEndpointTypeLabel(endpoint.endpointType)}</p>
                  </div>
                  <span className={`status-pill ${endpoint.availabilityStatus === "AVAILABLE" ? "success" : "warning"}`}>{endpoint.availabilityStatus}</span>
                </div>
                <small>{endpoint.rootPath}</small>
                <div className="task-card-meta">
                  <span>创建于 {formatCatalogDate(endpoint.createdAt)}</span>
                  <span>更新于 {formatCatalogDate(endpoint.updatedAt)}</span>
                </div>
                <div className="endpoint-panel-actions">
                  <button type="button" className="ghost-button" onClick={() => { const config = endpoint.connectionConfig as Record<string, unknown>; setEditingEndpointId(endpoint.id); setEndpointName(endpoint.name); setEndpointNote(endpoint.note || ""); setRootPath(String(config.rootPath ?? endpoint.rootPath ?? "")); setBrowsePath(String(config.rootPath ?? endpoint.rootPath ?? "")); setCloudKey(`${String(config.cloudName ?? "")}::${String(config.userName ?? "")}`); }}>
                    <Save size={15} />
                    编辑
                  </button>
                  <button type="button" className="ghost-button" onClick={() => void runCatalogEndpointScan(backendUrl, endpoint.id).then((r) => setNotice(r.success && r.summary ? `扫描完成：${endpoint.name}` : r.error || "扫描失败")).catch((e) => setError(e instanceof Error ? e.message : "扫描失败"))}>
                    <RefreshCcw size={15} />
                    扫描
                  </button>
                  <button type="button" className="danger-button" onClick={() => void deleteCatalogEndpoint(backendUrl, endpoint.id).then((r) => { if (!r.success) { throw new Error(r.error || "删除失败"); } setNotice(`已删除端点：${endpoint.name}`); return endpointsQuery.refetch(); }).catch((e) => setError(e instanceof Error ? e.message : "删除失败"))}>
                    <Trash2 size={15} />
                    删除
                  </button>
                </div>
              </article>
            ))}
          </div>
        )}
      </article>
    </section>
  );
}

function MetricCard({ label, value }: { label: string; value: string }) {
  return <div className="metric-card neutral"><span>{label}</span><strong>{value}</strong></div>;
}
