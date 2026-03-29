import { useEffect, useMemo, useState, type Dispatch, type SetStateAction } from "react";
import {
  ArrowLeft,
  BadgeCheck,
  Cookie,
  KeyRound,
  LoaderCircle,
  PlugZap,
  RefreshCcw,
  ScanQrCode,
  ShieldCheck,
  Trash2,
  UserPlus
} from "lucide-react";
import QRCode from "qrcode";
import { Link } from "react-router-dom";
import { CD2FileOpsTestPanel } from "../components/cd2/CD2FileOpsTestPanel";
import { CD2TransfersTestPanel } from "../components/cd2/CD2TransfersTestPanel";
import {
  use115QRCodeSession,
  useCD2AuthProfile,
  useCD2CloudAccounts,
  useClearCD2AuthProfile,
  useImport115Cookie,
  useRefreshCD2AuthProfile,
  useRegisterCD2Account,
  useRemoveCD2CloudAccount,
  useStart115OpenQRCode,
  useUpdateCD2AuthProfile
} from "../hooks/useCD2";
import { formatCatalogDate } from "../lib/catalog-view";
import type { CD2AuthMode, CD2AuthStatus, CD2CloudAccount } from "../types/cd2";

type AuthFormState = {
  mode: CD2AuthMode;
  serverAddress: string;
  userName: string;
  password: string;
  apiToken: string;
  managedTokenFriendlyName: string;
  managedTokenRootDir: string;
};

type RegisterFormState = {
  serverAddress: string;
  userName: string;
  password: string;
  confirmPassword: string;
};

const defaultAuthForm: AuthFormState = {
  mode: "password",
  serverAddress: "127.0.0.1:29798",
  userName: "",
  password: "",
  apiToken: "",
  managedTokenFriendlyName: "mam-backend",
  managedTokenRootDir: "/"
};

const defaultRegisterForm: RegisterFormState = {
  serverAddress: "127.0.0.1:29798",
  userName: "",
  password: "",
  confirmPassword: ""
};

const defaultQRCodePlatform = "wechatmini";
const qrCodePlatformOptions = [
  { value: "wechatmini", label: "微信小程序" },
  { value: "alipaymini", label: "支付宝小程序" },
  { value: "qandroid", label: "115 生活" },
  { value: "tv", label: "电视" },
  { value: "android", label: "安卓" },
  { value: "ios", label: "iOS" },
  { value: "web", label: "网页" }
];

export function CD2AuthTestPage() {
  const authQuery = useCD2AuthProfile();
  const cloudAccountsQuery = useCD2CloudAccounts();
  const updateMutation = useUpdateCD2AuthProfile();
  const refreshMutation = useRefreshCD2AuthProfile();
  const clearMutation = useClearCD2AuthProfile();
  const registerMutation = useRegisterCD2Account();
  const import115Mutation = useImport115Cookie();
  const start115OpenQRCodeMutation = useStart115OpenQRCode();
  const removeCloudAccountMutation = useRemoveCD2CloudAccount();

  const [authForm, setAuthForm] = useState<AuthFormState>(defaultAuthForm);
  const [registerForm, setRegisterForm] = useState<RegisterFormState>(defaultRegisterForm);
  const [authHydrated, setAuthHydrated] = useState(false);
  const [authNotice, setAuthNotice] = useState<string | null>(null);
  const [authError, setAuthError] = useState<string | null>(null);
  const [registerNotice, setRegisterNotice] = useState<string | null>(null);
  const [registerError, setRegisterError] = useState<string | null>(null);
  const [cookieText, setCookieText] = useState("");
  const [cloudNotice, setCloudNotice] = useState<string | null>(null);
  const [cloudError, setCloudError] = useState<string | null>(null);
  const [qrCodePlatform, setQRCodePlatform] = useState(defaultQRCodePlatform);
  const [activeQRCodeSessionId, setActiveQRCodeSessionId] = useState<string | null>(null);
  const [renderedQRCodeSrc, setRenderedQRCodeSrc] = useState<string | null>(null);

  const authStatus = authQuery.data;
  const authQueryError = authQuery.error instanceof Error ? authQuery.error.message : null;
  const cloudAccountsError = cloudAccountsQuery.error instanceof Error ? cloudAccountsQuery.error.message : null;
  const qrSessionQuery = use115QRCodeSession(activeQRCodeSessionId);
  const qrSession = qrSessionQuery.data;
  const qrSessionError = qrSessionQuery.error instanceof Error ? qrSessionQuery.error.message : null;
  const currentTarget = authStatus?.profile?.serverAddress || authStatus?.client.target || "127.0.0.1:29798";
  const cloudAccounts = cloudAccountsQuery.data ?? [];
  const accounts115 = useMemo(
    () => cloudAccounts.filter((account) => {
      const cloudName = account.cloudName.toLowerCase();
      return cloudName === "115" || cloudName === "115open";
    }),
    [cloudAccounts]
  );

  useEffect(() => {
    if (!authStatus || authHydrated) {
      return;
    }

    applyStatusToForms(authStatus, setAuthForm, setRegisterForm);
    setAuthHydrated(true);
  }, [authHydrated, authStatus]);

  const statusJson = useMemo(() => {
    return authStatus ? JSON.stringify(authStatus, null, 2) : "";
  }, [authStatus]);

  useEffect(() => {
    if (!qrSession?.finishedAt) {
      return;
    }

    void cloudAccountsQuery.refetch();

    if (qrSession.status === "success") {
      const addedCount = qrSession.addedAccounts?.length ?? 0;
      setCloudNotice(addedCount > 0 ? `115 二维码登录成功，新增 ${addedCount} 个账号。` : "115 二维码登录已完成。");
      setCloudError(null);
      return;
    }

    if (qrSession.error) {
      setCloudError(qrSession.error);
    }
  }, [cloudAccountsQuery, qrSession]);

  useEffect(() => {
    let cancelled = false;

    async function renderQRCode() {
      const directSrc = resolveQRCodeImageSrc(qrSession ?? {});
      if (directSrc) {
        setRenderedQRCodeSrc(directSrc);
        return;
      }

      const content = qrSession?.qrCodeContent?.trim() || qrSession?.lastMessage?.trim() || "";
      if (!content || !looksLikeQRCodeText(content)) {
        setRenderedQRCodeSrc(null);
        return;
      }

      try {
        const dataUrl = await QRCode.toDataURL(content, {
          margin: 1,
          width: 320
        });
        if (!cancelled) {
          setRenderedQRCodeSrc(dataUrl);
        }
      } catch {
        if (!cancelled) {
          setRenderedQRCodeSrc(null);
        }
      }
    }

    void renderQRCode();

    return () => {
      cancelled = true;
    };
  }, [qrSession]);

  async function handleSaveAuthProfile() {
    setAuthNotice(null);
    setAuthError(null);

    if (!authForm.serverAddress.trim()) {
      setAuthError("请先填写 CD2 服务地址。");
      return;
    }
    if (authForm.mode === "password") {
      if (!authForm.userName.trim() || !authForm.password.trim()) {
        setAuthError("账号密码模式需要填写 CD2 账号邮箱和密码。");
        return;
      }
    } else if (!authForm.apiToken.trim()) {
      setAuthError("API Token 模式需要填写 Token。");
      return;
    }

    try {
      const status = await updateMutation.mutateAsync({
        mode: authForm.mode,
        serverAddress: authForm.serverAddress.trim(),
        userName: authForm.userName.trim(),
        password: authForm.mode === "password" ? authForm.password : undefined,
        apiToken: authForm.mode === "api_token" ? authForm.apiToken.trim() : undefined,
        managedTokenFriendlyName: authForm.managedTokenFriendlyName.trim(),
        managedTokenRootDir: authForm.managedTokenRootDir.trim()
      });

      setAuthNotice(status.client.authReady ? "CD2 认证配置已保存并验证成功。" : "CD2 认证配置已保存，但认证还未就绪。");
      applyStatusToForms(status, setAuthForm, setRegisterForm, authForm.password);
    } catch (requestError) {
      setAuthError(requestError instanceof Error ? requestError.message : "保存 CD2 认证配置失败。");
    }
  }

  async function handleRefreshStatus() {
    setAuthNotice(null);
    setAuthError(null);

    try {
      const status = await refreshMutation.mutateAsync();
      setAuthNotice(status.client.ready ? "CD2 认证状态已刷新。" : "已刷新状态，请根据错误信息继续排查。");
      applyStatusToForms(status, setAuthForm, setRegisterForm, authForm.password);
    } catch (requestError) {
      setAuthError(requestError instanceof Error ? requestError.message : "刷新 CD2 状态失败。");
    }
  }

  async function handleClearProfile() {
    const confirmed = window.confirm("确认清除当前保存的 CD2 认证配置吗？这不会删除 CD2 账号本身。");
    if (!confirmed) {
      return;
    }

    setAuthNotice(null);
    setAuthError(null);

    try {
      const status = await clearMutation.mutateAsync();
      setAuthForm({
        ...defaultAuthForm,
        serverAddress: status.client.target || defaultAuthForm.serverAddress
      });
      setRegisterForm((previous) => ({
        ...previous,
        serverAddress: status.client.target || previous.serverAddress || defaultRegisterForm.serverAddress
      }));
      setAuthNotice("已清除本系统中保存的 CD2 认证配置。");
    } catch (requestError) {
      setAuthError(requestError instanceof Error ? requestError.message : "清除 CD2 认证配置失败。");
    }
  }

  async function handleRegisterAccount() {
    setRegisterNotice(null);
    setRegisterError(null);

    if (!registerForm.serverAddress.trim()) {
      setRegisterError("请先填写用于注册的 CD2 服务地址。");
      return;
    }
    if (!registerForm.userName.trim() || !registerForm.password.trim()) {
      setRegisterError("注册 CD2 账号时必须填写账号邮箱和密码。");
      return;
    }
    if (registerForm.password !== registerForm.confirmPassword) {
      setRegisterError("两次输入的密码不一致。");
      return;
    }

    try {
      const result = await registerMutation.mutateAsync({
        serverAddress: registerForm.serverAddress.trim(),
        userName: registerForm.userName.trim(),
        password: registerForm.password
      });

      setRegisterNotice("CD2 账号注册成功，已自动带入下方认证配置表单。");
      setAuthForm((previous) => ({
        ...previous,
        mode: "password",
        serverAddress: result.serverAddress || registerForm.serverAddress.trim(),
        userName: result.userName || registerForm.userName.trim(),
        password: registerForm.password,
        apiToken: ""
      }));
    } catch (requestError) {
      setRegisterError(requestError instanceof Error ? requestError.message : "注册 CD2 账号失败。");
    }
  }

  async function handleImport115Cookie() {
    setCloudNotice(null);
    setCloudError(null);

    if (!cookieText.trim()) {
      setCloudError("请先粘贴 115 的 editthiscookie 内容。");
      return;
    }

    try {
      const result = await import115Mutation.mutateAsync({
        editThisCookie: cookieText.trim()
      });
      setCookieText("");
      await cloudAccountsQuery.refetch();
      const addedCount = result.accounts?.length ?? 0;
      setCloudNotice(addedCount > 0 ? `115 Cookie 导入成功，新增 ${addedCount} 个账号。` : result.message ?? "115 Cookie 导入成功。");
    } catch (requestError) {
      setCloudError(requestError instanceof Error ? requestError.message : "导入 115 Cookie 失败。");
    }
  }

  async function handleStart115OpenQRCode() {
    setCloudNotice(null);
    setCloudError(null);

    try {
      const session = await start115OpenQRCodeMutation.mutateAsync({
        platform: qrCodePlatform.trim()
      });
      setActiveQRCodeSessionId(session.id);
      setCloudNotice("115open 二维码登录已启动，请使用对应客户端扫码并确认。");
    } catch (requestError) {
      setCloudError(requestError instanceof Error ? requestError.message : "启动 115open 二维码登录失败。");
    }
  }

  async function handleRemoveCloudAccount(account: CD2CloudAccount) {
    const confirmed = window.confirm(`确认删除 ${account.cloudName} 账号 ${account.userName} 吗？`);
    if (!confirmed) {
      return;
    }

    setCloudNotice(null);
    setCloudError(null);

    try {
      await removeCloudAccountMutation.mutateAsync({
        cloudName: account.cloudName,
        userName: account.userName
      });
      setCloudNotice(`已删除账号 ${account.userName}。`);
      await cloudAccountsQuery.refetch();
    } catch (requestError) {
      setCloudError(requestError instanceof Error ? requestError.message : "删除云账号失败。");
    }
  }

  return (
    <section className="page-stack">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">设置 / CD2 调试</p>
          <h3>CD2 认证测试页</h3>
          <p>
            这个页面用于手动验证当前 Docker CD2 开发环境的账号注册、账号密码认证和 API Token 认证。
            默认服务地址已指向开发测试环境 {currentTarget}。
          </p>
        </div>

        <div className="hero-metrics">
          <MetricCard label="服务地址" value={currentTarget} tone="neutral" />
          <MetricCard label="公共 API" value={authStatus?.client.publicReady ? "已连通" : "未就绪"} tone={authStatus?.client.publicReady ? "success" : "warning"} />
          <MetricCard label="认证状态" value={authStatus?.client.authReady ? "已认证" : "未认证"} tone={authStatus?.client.authReady ? "success" : "warning"} />
          <MetricCard label="当前模式" value={getModeLabel(authStatus?.profile?.mode ?? authForm.mode)} tone="neutral" />
        </div>
      </article>

      <div className="action-row">
        <Link to="/settings" className="ghost-button">
          <ArrowLeft size={16} />
          返回设置
        </Link>
      </div>

      {authQueryError ? <p className="error-copy">读取 CD2 状态失败：{authQueryError}</p> : null}
      {authNotice ? <p className="inline-note">{authNotice}</p> : null}
      {authError ? <p className="error-copy">{authError}</p> : null}
      {registerNotice ? <p className="inline-note">{registerNotice}</p> : null}
      {registerError ? <p className="error-copy">{registerError}</p> : null}
      {cloudNotice ? <p className="inline-note">{cloudNotice}</p> : null}
      {cloudError ? <p className="error-copy">{cloudError}</p> : null}
      {cloudAccountsError ? <p className="error-copy">读取云账号列表失败：{cloudAccountsError}</p> : null}
      {qrSessionError ? <p className="error-copy">读取二维码会话失败：{qrSessionError}</p> : null}

      <div className="page-grid settings-layout">
        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">连接状态</p>
              <h4>当前 CD2 状态</h4>
            </div>

            <button
              type="button"
              className="ghost-button"
              onClick={() => void handleRefreshStatus()}
              disabled={refreshMutation.isPending || authQuery.isLoading}
            >
              {refreshMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
              刷新状态
            </button>
          </div>

          {authQuery.isLoading ? (
            <div className="sync-empty-block">
              <LoaderCircle size={18} className="spin" />
              <div>
                <strong>正在读取 CD2 当前状态</strong>
                <p>会同时探测公共接口和认证后的接口是否可用。</p>
              </div>
            </div>
          ) : authStatus ? (
            <>
              <div className="scan-summary-grid">
                <SummaryCell label="已配置" value={authStatus.configured ? "是" : "否"} />
                <SummaryCell label="公共接口" value={authStatus.client.publicReady ? "就绪" : "未就绪"} />
                <SummaryCell label="认证接口" value={authStatus.client.authReady ? "就绪" : "未就绪"} />
                <SummaryCell label="CD2 SystemReady" value={authStatus.client.systemInfo.systemReady ? "是" : "否"} />
              </div>

              <div className="settings-note-card">
                <PlugZap size={18} />
                <div>
                  <strong>目标地址：{authStatus.client.target || currentTarget}</strong>
                  <p>
                    当前鉴权方式：{getModeLabel(authStatus.profile?.mode ?? authStatus.client.activeAuthMode)}
                    {authStatus.profile?.lastVerifiedAt ? `，最近验证时间 ${formatCatalogDate(authStatus.profile.lastVerifiedAt)}` : ""}
                  </p>
                </div>
              </div>

              {authStatus.client.lastError ? (
                <p className="error-copy">最近错误：{authStatus.client.lastError}</p>
              ) : null}

              <label className="field">
                <span>状态 JSON</span>
                <textarea value={statusJson} readOnly />
              </label>
            </>
          ) : (
            <div className="sync-empty-block">
              <ShieldCheck size={18} />
              <div>
                <strong>还没有拿到 CD2 状态</strong>
                <p>请先检查后端和 Docker CD2 是否都已启动。</p>
              </div>
            </div>
          )}
        </article>

        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">认证配置</p>
              <h4>保存到本系统</h4>
            </div>
          </div>

          <div className="settings-card-list">
            <div className="settings-action-card">
              <div className="settings-action-head">
                <div className="theme-option-icon">
                  <KeyRound size={18} />
                </div>
                <span className={`status-pill ${authStatus?.client.authReady ? "success" : "warning"}`}>
                  {authStatus?.client.authReady ? "认证已就绪" : "等待配置"}
                </span>
              </div>

              <strong>认证方式与参数</strong>
              <p>密码模式会用 CD2 账号换取后端受管 Token；API Token 模式会直接验证并保存现成 Token。</p>

              <div className="field-grid">
                <label className="field">
                  <span>认证方式</span>
                  <select
                    value={authForm.mode}
                    onChange={(event) =>
                      setAuthForm((previous) => ({
                        ...previous,
                        mode: event.target.value as CD2AuthMode
                      }))
                    }
                  >
                    <option value="password">账号密码</option>
                    <option value="api_token">API Token</option>
                  </select>
                </label>

                <label className="field">
                  <span>CD2 服务地址</span>
                  <input
                    value={authForm.serverAddress}
                    onChange={(event) => setAuthForm((previous) => ({ ...previous, serverAddress: event.target.value }))}
                    placeholder="127.0.0.1:29798"
                  />
                </label>

                <label className="field">
                  <span>账号邮箱</span>
                  <input
                    value={authForm.userName}
                    onChange={(event) => setAuthForm((previous) => ({ ...previous, userName: event.target.value }))}
                    placeholder="CD2 账号邮箱"
                    disabled={authForm.mode === "api_token"}
                  />
                </label>

                {authForm.mode === "password" ? (
                  <label className="field">
                    <span>密码</span>
                    <input
                      type="password"
                      value={authForm.password}
                      onChange={(event) => setAuthForm((previous) => ({ ...previous, password: event.target.value }))}
                      placeholder="输入 CD2 密码"
                    />
                  </label>
                ) : (
                  <label className="field">
                    <span>API Token</span>
                    <input
                      value={authForm.apiToken}
                      onChange={(event) => setAuthForm((previous) => ({ ...previous, apiToken: event.target.value }))}
                      placeholder="输入现成的 CD2 API Token"
                    />
                  </label>
                )}

                <label className="field">
                  <span>受管 Token 名称</span>
                  <input
                    value={authForm.managedTokenFriendlyName}
                    onChange={(event) =>
                      setAuthForm((previous) => ({ ...previous, managedTokenFriendlyName: event.target.value }))
                    }
                    placeholder="mam-backend"
                  />
                </label>

                <label className="field">
                  <span>受管 Token 根目录</span>
                  <input
                    value={authForm.managedTokenRootDir}
                    onChange={(event) =>
                      setAuthForm((previous) => ({ ...previous, managedTokenRootDir: event.target.value }))
                    }
                    placeholder="/"
                  />
                </label>
              </div>

              <div className="action-row">
                <button
                  type="button"
                  className="primary-button"
                  onClick={() => void handleSaveAuthProfile()}
                  disabled={updateMutation.isPending}
                >
                  {updateMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <BadgeCheck size={16} />}
                  保存并验证
                </button>

                <button
                  type="button"
                  className="ghost-button"
                  onClick={() => authStatus && applyStatusToForms(authStatus, setAuthForm, setRegisterForm, authForm.password)}
                  disabled={!authStatus}
                >
                  <RefreshCcw size={16} />
                  载入已保存配置
                </button>

                <button
                  type="button"
                  className="danger-button"
                  onClick={() => void handleClearProfile()}
                  disabled={clearMutation.isPending}
                >
                  {clearMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <Trash2 size={16} />}
                  清除配置
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
              <p className="eyebrow">注册</p>
              <h4>创建新的 CD2 账号</h4>
            </div>
          </div>

          <div className="settings-card-list">
            <div className="settings-action-card">
              <div className="settings-action-head">
                <div className="theme-option-icon">
                  <UserPlus size={18} />
                </div>
                <span className="status-pill subtle">Public RPC</span>
              </div>

              <strong>注册后可直接用于下方认证</strong>
              <p>这里调用的是 CD2 官方 gRPC `Register`。注册成功后，账号邮箱和密码会自动回填到认证配置表单。</p>

              <div className="field-grid">
                <label className="field">
                  <span>CD2 服务地址</span>
                  <input
                    value={registerForm.serverAddress}
                    onChange={(event) =>
                      setRegisterForm((previous) => ({ ...previous, serverAddress: event.target.value }))
                    }
                    placeholder="127.0.0.1:29798"
                  />
                </label>

                <label className="field">
                  <span>账号邮箱</span>
                  <input
                    value={registerForm.userName}
                    onChange={(event) => setRegisterForm((previous) => ({ ...previous, userName: event.target.value }))}
                    placeholder="新的 CD2 账号邮箱"
                  />
                </label>

                <label className="field">
                  <span>密码</span>
                  <input
                    type="password"
                    value={registerForm.password}
                    onChange={(event) => setRegisterForm((previous) => ({ ...previous, password: event.target.value }))}
                    placeholder="设置密码"
                  />
                </label>

                <label className="field">
                  <span>确认密码</span>
                  <input
                    type="password"
                    value={registerForm.confirmPassword}
                    onChange={(event) =>
                      setRegisterForm((previous) => ({ ...previous, confirmPassword: event.target.value }))
                    }
                    placeholder="再次输入密码"
                  />
                </label>
              </div>

              <div className="action-row">
                <button
                  type="button"
                  className="primary-button"
                  onClick={() => void handleRegisterAccount()}
                  disabled={registerMutation.isPending}
                >
                  {registerMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <UserPlus size={16} />}
                  注册 CD2 账号
                </button>
              </div>
            </div>
          </div>
        </article>

        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">测试建议</p>
              <h4>手动验证顺序</h4>
            </div>
          </div>

          <div className="settings-note-card">
            <ShieldCheck size={18} />
            <div>
              <strong>推荐按这三个步骤测试</strong>
              <p>先看状态页是否能连通 Docker CD2，再尝试注册账号，最后用账号密码模式保存并验证认证配置。</p>
            </div>
          </div>

          <div className="scan-summary-grid">
            <SummaryCell label="步骤 1" value="刷新状态" />
            <SummaryCell label="步骤 2" value="注册账号" />
            <SummaryCell label="步骤 3" value="保存认证" />
            <SummaryCell label="默认目标" value={currentTarget} />
          </div>

          {authStatus?.profile?.updatedAt ? (
            <p className="inline-note">当前保存配置更新时间：{formatCatalogDate(authStatus.profile.updatedAt)}</p>
          ) : null}
        </article>
      </div>

      <div className="page-grid settings-layout">
        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">115 接入测试</p>
              <h4>通过 Cookie 导入</h4>
            </div>
          </div>

          <div className="settings-card-list">
            <div className="settings-action-card">
              <div className="settings-action-head">
                <div className="theme-option-icon">
                  <Cookie size={18} />
                </div>
                <span className="status-pill subtle">editthiscookie</span>
              </div>

              <strong>导入 115 Cookie 到 CD2</strong>
              <p>把现有 115 登录获得的 editthiscookie 粘贴到这里，后端会直接调用 CD2 的 `APILogin115Editthiscookie`。</p>

              <label className="field">
                <span>editthiscookie</span>
                <textarea
                  value={cookieText}
                  onChange={(event) => setCookieText(event.target.value)}
                  placeholder="在这里粘贴 115 的 editthiscookie 文本"
                />
              </label>

              <div className="action-row">
                <button
                  type="button"
                  className="primary-button"
                  onClick={() => void handleImport115Cookie()}
                  disabled={import115Mutation.isPending}
                >
                  {import115Mutation.isPending ? <LoaderCircle size={16} className="spin" /> : <Cookie size={16} />}
                  导入 Cookie
                </button>
              </div>
            </div>
          </div>
        </article>

        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">115 接入测试</p>
              <h4>通过二维码登录</h4>
            </div>
          </div>

          <div className="settings-card-list">
            <div className="settings-action-card qr-session-panel">
              <div className="settings-action-head">
                <div className="theme-option-icon">
                  <ScanQrCode size={18} />
                </div>
                <span className={`status-pill ${qrSession?.finishedAt ? (qrSession.status === "success" ? "success" : "warning") : "subtle"}`}>
                  {qrSession ? getQRCodeStatusLabel(qrSession.status) : "未启动"}
                </span>
              </div>

              <strong>启动 115open 二维码接入</strong>
              <p>这里已经切到 CD2 当前可用的 `115open` 扫码能力。平台类型选项保持和旧页面一致，便于你按原测试习惯操作。</p>

              <div className="field-grid">
                <label className="field">
                  <span>二维码平台</span>
                  <select value={qrCodePlatform} onChange={(event) => setQRCodePlatform(event.target.value)}>
                    {qrCodePlatformOptions.map((option) => (
                      <option key={option.value} value={option.value}>
                        {option.label}
                      </option>
                    ))}
                  </select>
                </label>
              </div>

              <div className="action-row">
                <button
                  type="button"
                  className="primary-button"
                  onClick={() => void handleStart115OpenQRCode()}
                  disabled={start115OpenQRCodeMutation.isPending}
                >
                  {start115OpenQRCodeMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <ScanQrCode size={16} />}
                  启动 115open 扫码
                </button>
              </div>

              {qrSession ? (
                <div className="qr-grid">
                  <div className="qr-box">
                    {renderedQRCodeSrc ? (
                      <img src={renderedQRCodeSrc} alt="115open 登录二维码" />
                    ) : (
                      <span>{qrSession.qrCodeContent || qrSession.lastMessage || "等待二维码生成"}</span>
                    )}
                  </div>

                  <div className="qr-meta">
                    <p>会话 ID：{qrSession.id}</p>
                    <p>当前状态：{getQRCodeStatusLabel(qrSession.status)}</p>
                    {qrSession.lastMessage ? <p>最近消息：{qrSession.lastMessage}</p> : null}
                    {qrSession.error ? <p>错误信息：{qrSession.error}</p> : null}
                    <p>开始时间：{formatCatalogDate(qrSession.startedAt)}</p>
                    <p>更新时间：{formatCatalogDate(qrSession.updatedAt)}</p>
                    {qrSession.finishedAt ? <p>完成时间：{formatCatalogDate(qrSession.finishedAt)}</p> : null}
                  </div>
                </div>
              ) : null}
            </div>
          </div>
        </article>
      </div>

      <article className="detail-card">
        <div className="section-head">
          <div>
            <p className="eyebrow">云账号列表</p>
            <h4>当前 CD2 接入结果</h4>
          </div>

          <button
            type="button"
            className="ghost-button"
            onClick={() => void cloudAccountsQuery.refetch()}
            disabled={cloudAccountsQuery.isFetching}
          >
            {cloudAccountsQuery.isFetching ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
            刷新账号列表
          </button>
        </div>

        <div className="scan-summary-grid">
          <SummaryCell label="全部账号" value={String(cloudAccounts.length)} />
          <SummaryCell label="115 账号" value={String(accounts115.length)} />
          <SummaryCell label="事件监听运行中" value={String(cloudAccounts.filter((item) => item.isCloudEventListenerRunning).length)} />
          <SummaryCell label="已锁定" value={String(cloudAccounts.filter((item) => item.isLocked).length)} />
        </div>

        {cloudAccountsQuery.isLoading ? (
          <div className="sync-empty-block">
            <LoaderCircle size={20} className="spin" />
            <div>
              <strong>正在读取 CD2 云账号列表</strong>
              <p>导入 Cookie、扫码登录完成后，账号会显示在这里。</p>
            </div>
          </div>
        ) : cloudAccounts.length === 0 ? (
          <div className="sync-empty-block">
            <ShieldCheck size={20} />
            <div>
              <strong>当前还没有接入任何云账号</strong>
              <p>你可以先用 Cookie 导入，或者启动 115 二维码登录。</p>
            </div>
          </div>
        ) : (
          <div className="endpoint-grid">
            {cloudAccounts.map((account) => (
              <article key={`${account.cloudName}-${account.userName}`} className="endpoint-panel">
                <div className="endpoint-panel-head">
                  <div>
                    <strong>{account.displayName || account.userName}</strong>
                    <p>{account.cloudName} / {account.userName}</p>
                  </div>
                  <span className={`status-pill ${account.isLocked ? "warning" : "success"}`}>
                    {account.isLocked ? "已锁定" : "正常"}
                  </span>
                </div>

                <div className="endpoint-panel-meta">
                  <div>
                    <span>昵称</span>
                    <strong>{account.nickName || "-"}</strong>
                  </div>
                  <div>
                    <span>路径</span>
                    <strong>{account.path || "-"}</strong>
                  </div>
                  <div>
                    <span>HTTP 下载</span>
                    <strong>{account.supportHttpDownload ? "支持" : "不支持"}</strong>
                  </div>
                  <div>
                    <span>多线程上传</span>
                    <strong>{account.supportMultiThreadUploading ? "支持" : "不支持"}</strong>
                  </div>
                  <div>
                    <span>QPS 限速</span>
                    <strong>{account.supportQpsLimit ? "支持" : "不支持"}</strong>
                  </div>
                  <div>
                    <span>事件监听</span>
                    <strong>{account.isCloudEventListenerRunning ? "运行中" : "未运行"}</strong>
                  </div>
                </div>

                <div className="endpoint-panel-actions">
                  <button
                    type="button"
                    className="danger-button"
                    onClick={() => void handleRemoveCloudAccount(account)}
                    disabled={removeCloudAccountMutation.isPending}
                  >
                    {removeCloudAccountMutation.isPending ? <LoaderCircle size={16} className="spin" /> : <Trash2 size={16} />}
                    删除账号
                  </button>
                </div>
              </article>
            ))}
          </div>
        )}
      </article>
      <CD2FileOpsTestPanel accounts={cloudAccounts} />
      <CD2TransfersTestPanel />
    </section>
  );
}

function applyStatusToForms(
  status: CD2AuthStatus,
  setAuthForm: Dispatch<SetStateAction<AuthFormState>>,
  setRegisterForm: Dispatch<SetStateAction<RegisterFormState>>,
  password = ""
) {
  const serverAddress = status.profile?.serverAddress || status.client.target || defaultAuthForm.serverAddress;

  setAuthForm((previous) => ({
    ...previous,
    mode: status.profile?.mode ?? previous.mode,
    serverAddress,
    userName: status.profile?.userName ?? previous.userName,
    password,
    apiToken: status.profile?.mode === "api_token" ? previous.apiToken : "",
    managedTokenFriendlyName: status.profile?.managedTokenFriendlyName ?? previous.managedTokenFriendlyName,
    managedTokenRootDir: status.profile?.managedTokenRootDir ?? previous.managedTokenRootDir
  }));

  setRegisterForm((previous) => ({
    ...previous,
    serverAddress
  }));
}

function MetricCard({
  label,
  value,
  tone
}: {
  label: string;
  value: string;
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

function getModeLabel(mode: string | undefined) {
  switch (mode) {
    case "password":
      return "账号密码";
    case "api_token":
      return "API Token";
    case "managed_token":
      return "受管 Token";
    case "jwt":
      return "JWT";
    case "none":
    case "":
    case undefined:
      return "未配置";
    default:
      return mode;
  }
}

function getQRCodeStatusLabel(status: string | undefined) {
  switch (status) {
    case "starting":
      return "启动中";
    case "show_image":
    case "show_image_content":
      return "等待扫码";
    case "waiting":
    case "waiting_scan":
      return "等待扫码";
    case "waiting_confirm":
      return "等待确认";
    case "success":
      return "登录成功";
    case "closed":
      return "已关闭";
    case "expired":
      return "已过期";
    case "error":
      return "出错";
    default:
      return status || "未知";
  }
}

function resolveQRCodeImageSrc(session: { qrCodeImage?: string; qrCodeContent?: string }) {
  if (session.qrCodeImage?.trim()) {
    return session.qrCodeImage.trim();
  }
  if (session.qrCodeContent?.trim().startsWith("data:image/")) {
    return session.qrCodeContent.trim();
  }
  return undefined;
}

function looksLikeQRCodeText(value: string) {
  const trimmed = value.trim();
  return /^https?:\/\//i.test(trimmed) || trimmed.startsWith("115://") || trimmed.length > 16;
}
