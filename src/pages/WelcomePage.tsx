import { useMemo, useState, type ReactNode } from "react";
import {
  ArrowRight,
  FolderOpen,
  FolderPlus,
  Folders,
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
      setNotice(`已创建“${library.name}”，请先配置存储节点。`);
      navigate("/storage?setup=first-endpoint");
    } catch (createError) {
      setError(createError instanceof Error ? createError.message : "创建资产库失败");
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
      setNotice(`已打开“${library.name}”`);
      navigate("/assets");
    } catch (openError) {
      setError(openError instanceof Error ? openError.message : "打开资产库失败");
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
      setNotice(`已切换到“${target.name}”`);
      navigate("/assets");
    } catch (openError) {
      setError(openError instanceof Error ? openError.message : "打开最近资产库失败");
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
      setNotice("当前资产库已关闭");
    } catch (closeError) {
      setError(closeError instanceof Error ? closeError.message : "关闭资产库失败");
    } finally {
      setBusyAction(null);
    }
  }

  return (
    <section className="page-stack welcome-stage">
      <article className="hero-card welcome-hero">
        <div className="welcome-hero-head">
          <p className="eyebrow">Libraries</p>
        </div>

        <div className="welcome-hero-copy">
          <h3>资产库</h3>
          <div className="welcome-stat-row">
            <QuickStat icon={<LibraryBig size={14} />} label="最近" value={recentLibraries.length} />
            <QuickStat
              icon={<Sparkles size={14} />}
              label="会话"
              value={isLibraryOpen ? "已打开" : "未打开"}
            />
            <QuickStat
              icon={<HardDrive size={14} />}
              label="模块"
              value={`${readyModuleCount}/${bootstrapQuery.data?.modules.length ?? 0}`}
            />
          </div>

          <div className="action-row">
            <button
              type="button"
              className="ghost-button inline-button"
              onClick={() => navigate("/library-manager")}
            >
              <Folders size={16} />
              管理资产库
            </button>
          </div>
        </div>
      </article>

      {notice ? <p className="inline-note">{notice}</p> : null}
      {error ? <p className="error-copy">{error}</p> : null}

      <div className="welcome-zone-grid">
        <article className="detail-card welcome-zone-card welcome-zone-tall">
          <div className="section-head">
            <div>
              <p className="eyebrow">当前</p>
              <h4>{currentLibrary?.name ?? "未打开资产库"}</h4>
            </div>
            <span className={`status-pill ${isLibraryOpen ? "success" : "subtle"}`}>
              {isLibraryOpen ? "已打开" : "待选择"}
            </span>
          </div>

          {currentLibrary ? (
            <>
              <div className="welcome-zone-meta">
                <span>{currentLibrary.path}</span>
                <span>{formatCatalogDate(currentLibrary.lastOpenedAt ?? currentLibrary.updatedAt)}</span>
              </div>

              <div className="welcome-zone-actions">
                {isLibraryOpen ? (
                  <>
                    <button
                      type="button"
                      className="primary-button"
                      onClick={() => navigate("/assets")}
                    >
                      <ArrowRight size={16} />
                      进入资产
                    </button>
                    <button
                      type="button"
                      className="ghost-button"
                      onClick={() => void handleCloseLibrary()}
                      disabled={busyAction === "close"}
                    >
                      {busyAction === "close" ? <LoaderCircle size={16} className="spin" /> : <HardDrive size={16} />}
                      关闭
                    </button>
                  </>
                ) : (
                  <button
                    type="button"
                    className="primary-button"
                    onClick={() => void handleOpenRecentLibrary(currentLibrary.id)}
                    disabled={busyAction !== null}
                  >
                    {busyAction === "recent" ? <LoaderCircle size={16} className="spin" /> : <FolderOpen size={16} />}
                    打开
                  </button>
                )}
              </div>
            </>
          ) : (
            <div className="welcome-zone-empty">
              <LibraryBig size={20} />
              <div>
                <strong>新建或打开</strong>
              </div>
            </div>
          )}
        </article>

        <article className="detail-card welcome-zone-card">
          <div className="quick-action-head">
            <span className="quick-action-icon">
              <FolderPlus size={18} />
            </span>
            <div>
              <p className="eyebrow">Create</p>
              <h4>新建</h4>
            </div>
          </div>

          <div className="field-grid quick-field-grid">
            <label className="field">
              <span>名称</span>
              <input
                value={createForm.name}
                onChange={(event) =>
                  setCreateForm((current) => ({ ...current, name: event.target.value }))
                }
                placeholder="2026 新疆"
              />
            </label>

            <label className="field field-span">
              <span>路径</span>
              <input
                value={createForm.path}
                onChange={(event) =>
                  setCreateForm((current) => ({ ...current, path: event.target.value }))
                }
                placeholder="B:\\Libraries\\2026-xinjiang-trip.maredb"
              />
            </label>
          </div>

          <div className="action-row">
            <button
              type="button"
              className="primary-button"
              onClick={() => void handleCreateLibrary()}
              disabled={busyAction !== null}
            >
              {busyAction === "create" ? <LoaderCircle size={16} className="spin" /> : <FolderPlus size={16} />}
              创建
            </button>
          </div>
        </article>

        <article className="detail-card welcome-zone-card">
          <div className="quick-action-head">
            <span className="quick-action-icon">
              <FolderOpen size={18} />
            </span>
            <div>
              <p className="eyebrow">Open</p>
              <h4>打开</h4>
            </div>
          </div>

          <div className="field-grid quick-field-grid">
            <label className="field">
              <span>名称</span>
              <input
                value={openForm.name}
                onChange={(event) =>
                  setOpenForm((current) => ({ ...current, name: event.target.value }))
                }
                placeholder="可选"
              />
            </label>

            <label className="field field-span">
              <span>路径</span>
              <input
                value={openForm.path}
                onChange={(event) =>
                  setOpenForm((current) => ({ ...current, path: event.target.value }))
                }
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
              打开
            </button>
          </div>
        </article>
      </div>

      <article className="detail-card recent-libraries-card">
        <div className="section-head">
          <div>
            <p className="eyebrow">Recent</p>
            <h4>最近使用</h4>
          </div>
          <span className="status-pill subtle">{recentLibraries.length}</span>
        </div>

        {recentLibraries.length === 0 ? (
          <div className="sync-empty-block">
            <LibraryBig size={20} />
            <div>
              <strong>还没有资产库</strong>
            </div>
          </div>
        ) : (
          <div className="recent-library-grid">
            {recentLibraries.map((library) => {
              const active = library.id === currentLibrary?.id;

              return (
                <article key={library.id} className={`library-mini-card${active ? " is-active" : ""}`}>
                  <div className="library-mini-head">
                    <div>
                      <strong>{library.name}</strong>
                      <p>{library.path}</p>
                    </div>
                    <span className={`status-pill ${active && isLibraryOpen ? "success" : "subtle"}`}>
                      {active && isLibraryOpen ? "当前" : "最近"}
                    </span>
                  </div>

                  <div className="library-mini-meta">
                    <span>{formatCatalogDate(library.lastOpenedAt ?? library.updatedAt)}</span>
                    <button
                      type="button"
                      className="ghost-button inline-button"
                      onClick={() => void handleOpenRecentLibrary(library.id)}
                      disabled={busyAction !== null}
                    >
                      {busyAction === "recent" ? <LoaderCircle size={16} className="spin" /> : <ArrowRight size={16} />}
                      打开
                    </button>
                  </div>
                </article>
              );
            })}
          </div>
        )}
      </article>
    </section>
  );
}

function QuickStat({
  icon,
  label,
  value
}: {
  icon: ReactNode;
  label: string;
  value: string | number;
}) {
  return (
    <span className="status-pill subtle quick-stat-pill">
      {icon}
      {label} {value}
    </span>
  );
}
