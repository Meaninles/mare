import { useMemo, useState } from "react";
import {
  FolderOpen,
  FolderPlus,
  HardDrive,
  LibraryBig,
  LoaderCircle,
  Sparkles
} from "lucide-react";
import { useNavigate } from "react-router-dom";
import { useLibraryContext } from "../context/LibraryContext";
import { formatCatalogDate } from "../lib/catalog-view";

type FormState = {
  path: string;
  name: string;
};

const emptyForm: FormState = {
  path: "",
  name: ""
};

export function WelcomePage() {
  const navigate = useNavigate();
  const {
    bootstrapQuery,
    closeLibrary,
    createLibrary,
    currentLibrary,
    currentLibrarySession,
    isLibraryOpen,
    openLibraryPath,
    openRegisteredLibrary,
    recentLibraries
  } = useLibraryContext();
  const [createForm, setCreateForm] = useState<FormState>(emptyForm);
  const [openForm, setOpenForm] = useState<FormState>(emptyForm);
  const [busyAction, setBusyAction] = useState<"create" | "open" | "recent" | "close" | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const readyModuleCount = useMemo(() => {
    return bootstrapQuery.data?.modules.filter((module) => module.ready).length ?? 0;
  }, [bootstrapQuery.data]);

  async function handleCreateLibrary() {
    setBusyAction("create");
    setNotice(null);
    setError(null);

    try {
      const library = await createLibrary(createForm);
      setCreateForm(emptyForm);
      setNotice(`已创建并挂载资产库“${library.name}”。`);
      navigate("/assets");
    } catch (createError) {
      setError(createError instanceof Error ? createError.message : "创建资产库失败。");
    } finally {
      setBusyAction(null);
    }
  }

  async function handleOpenLibrary() {
    setBusyAction("open");
    setNotice(null);
    setError(null);

    try {
      const library = await openLibraryPath(openForm);
      setOpenForm(emptyForm);
      setNotice(`已打开资产库“${library.name}”。`);
      navigate("/assets");
    } catch (openError) {
      setError(openError instanceof Error ? openError.message : "打开资产库失败。");
    } finally {
      setBusyAction(null);
    }
  }

  async function handleOpenRecentLibrary(libraryId: string) {
    const target = recentLibraries.find((library) => library.id === libraryId);
    if (!target) {
      return;
    }

    setBusyAction("recent");
    setNotice(null);
    setError(null);

    try {
      await openRegisteredLibrary(target);
      setNotice(`已打开最近使用的资产库“${target.name}”。`);
      navigate("/assets");
    } catch (openError) {
      setError(openError instanceof Error ? openError.message : "打开最近使用的资产库失败。");
    } finally {
      setBusyAction(null);
    }
  }

  async function handleCloseLibrary() {
    setBusyAction("close");
    setNotice(null);
    setError(null);

    try {
      await closeLibrary();
      setNotice("当前资产库已关闭。");
    } catch (closeError) {
      setError(closeError instanceof Error ? closeError.message : "关闭资产库失败。");
    } finally {
      setBusyAction(null);
    }
  }

  return (
    <section className="page-stack">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">欢迎页</p>
          <h3>从资产库开始，而不是直接从某个存储端开始。</h3>
          <p>
            先创建或打开一个资产库，再在这个资产库下面配置存储节点、导入规则和统一的文件资产视图。
          </p>
        </div>

        <div className="hero-metrics">
          <MetricCard label="最近资产库" value={recentLibraries.length} tone="neutral" />
          <MetricCard label="当前会话" value={isLibraryOpen ? 1 : 0} tone={isLibraryOpen ? "success" : "warning"} />
          <MetricCard label="应用模块" value={`${readyModuleCount}/${bootstrapQuery.data?.modules.length ?? 0}`} tone="neutral" />
          <MetricCard label="应用数据库" value={bootstrapQuery.data?.database.ready ? "就绪" : "检查中"} tone={bootstrapQuery.data?.database.ready ? "success" : "warning"} />
        </div>
      </article>

      {notice ? <p className="inline-note">{notice}</p> : null}
      {error ? <p className="error-copy">{error}</p> : null}

      {currentLibrary ? (
        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">当前资产库</p>
              <h4>{currentLibrary.name}</h4>
            </div>
            <span className={`status-pill ${isLibraryOpen ? "success" : "subtle"}`}>
              {isLibraryOpen ? "已挂载" : "未挂载"}
            </span>
          </div>

          <div className="field-grid">
            <div className="field">
              <span>库文件路径</span>
              <strong>{currentLibrary.path}</strong>
            </div>
            <div className="field">
              <span>最近打开</span>
              <strong>{formatCatalogDate(currentLibrary.lastOpenedAt ?? currentLibrary.updatedAt)}</strong>
            </div>
            {currentLibrarySession?.ready ? (
              <>
                <div className="field">
                  <span>架构</span>
                  <strong>{currentLibrarySession.schemaFamily ?? "unknown"}</strong>
                </div>
                <div className="field">
                  <span>缓存根目录</span>
                  <strong>{currentLibrarySession.cacheRoot ?? "-"}</strong>
                </div>
              </>
            ) : null}
          </div>

          <div className="action-row">
            {isLibraryOpen ? (
              <>
                <button type="button" className="primary-button" onClick={() => navigate("/assets")}>
                  <FolderOpen size={16} />
                  进入资产视图
                </button>
                <button
                  type="button"
                  className="ghost-button"
                  onClick={() => void handleCloseLibrary()}
                  disabled={busyAction === "close"}
                >
                  {busyAction === "close" ? <LoaderCircle size={16} className="spin" /> : <HardDrive size={16} />}
                  关闭当前资产库
                </button>
              </>
            ) : (
              <button
                type="button"
                className="primary-button"
                onClick={() => void handleOpenRecentLibrary(currentLibrary.id)}
                disabled={busyAction !== null}
              >
                {busyAction === "recent" ? <LoaderCircle size={16} className="spin" /> : <LibraryBig size={16} />}
                重新打开当前资产库
              </button>
            )}
          </div>
        </article>
      ) : null}

      <div className="page-grid settings-layout">
        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">新建</p>
              <h4>新建资产库</h4>
            </div>
            <span className="status-pill subtle">`.maredb`</span>
          </div>

          <div className="field-grid">
            <label className="field">
              <span>资产库名称</span>
              <input
                value={createForm.name}
                onChange={(event) => setCreateForm((current) => ({ ...current, name: event.target.value }))}
                placeholder="例如：2026新疆西藏自驾"
              />
            </label>

            <label className="field field-span">
              <span>资产库文件路径</span>
              <input
                value={createForm.path}
                onChange={(event) => setCreateForm((current) => ({ ...current, path: event.target.value }))}
                placeholder="B:\\Libraries\\2026-xinjiang-trip.maredb"
              />
            </label>
          </div>

          <p className="secondary-text">
            这里填的是资产库数据库文件路径，不是某个存储节点路径。没有后缀时系统会自动补 `.maredb`。
          </p>

          <div className="action-row">
            <button
              type="button"
              className="primary-button"
              onClick={() => void handleCreateLibrary()}
              disabled={busyAction !== null}
            >
              {busyAction === "create" ? <LoaderCircle size={16} className="spin" /> : <FolderPlus size={16} />}
              创建并打开
            </button>
          </div>
        </article>

        <article className="detail-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">打开</p>
              <h4>打开已有资产库</h4>
            </div>
            <span className="status-pill subtle">按需挂载</span>
          </div>

          <div className="field-grid">
            <label className="field">
              <span>显示名称</span>
              <input
                value={openForm.name}
                onChange={(event) => setOpenForm((current) => ({ ...current, name: event.target.value }))}
                placeholder="可选；留空则取文件名"
              />
            </label>

            <label className="field field-span">
              <span>资产库文件路径</span>
              <input
                value={openForm.path}
                onChange={(event) => setOpenForm((current) => ({ ...current, path: event.target.value }))}
                placeholder="B:\\Libraries\\2026-xinjiang-trip.maredb"
              />
            </label>
          </div>

          <div className="action-row">
            <button
              type="button"
              className="primary-button"
              onClick={() => void handleOpenLibrary()}
              disabled={busyAction !== null}
            >
              {busyAction === "open" ? <LoaderCircle size={16} className="spin" /> : <FolderOpen size={16} />}
              打开并登记
            </button>
          </div>
        </article>
      </div>

      <article className="detail-card">
        <div className="section-head">
          <div>
            <p className="eyebrow">最近资产库</p>
            <h4>最近使用</h4>
          </div>
          <span className="status-pill subtle">
            <Sparkles size={14} />
            资产库优先
          </span>
        </div>

        {recentLibraries.length === 0 ? (
          <div className="sync-empty-block">
            <LibraryBig size={20} />
            <div>
              <strong>还没有登记过任何资产库</strong>
              <p>先创建一个新的资产库，或者手动打开已有的库文件。</p>
            </div>
          </div>
        ) : (
          <div className="library-record-list">
            {recentLibraries.map((library) => (
              <article key={library.id} className="library-record-card">
                <div className="library-record-head">
                  <div>
                    <strong>{library.name}</strong>
                    <p>{library.path}</p>
                  </div>
                  <span className={`status-pill ${library.id === currentLibrary?.id ? (isLibraryOpen ? "success" : "subtle") : "subtle"}`}>
                    {library.id === currentLibrary?.id ? (isLibraryOpen ? "当前已打开" : "当前选中") : "最近使用"}
                  </span>
                </div>

                <div className="library-record-meta">
                  <span>最近打开：{formatCatalogDate(library.lastOpenedAt ?? library.updatedAt)}</span>
                  <span>创建于：{formatCatalogDate(library.createdAt)}</span>
                </div>

                <div className="action-row">
                  <button
                    type="button"
                    className="ghost-button"
                    onClick={() => void handleOpenRecentLibrary(library.id)}
                    disabled={busyAction !== null}
                  >
                    {busyAction === "recent" ? <LoaderCircle size={16} className="spin" /> : <FolderOpen size={16} />}
                    打开这个资产库
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
