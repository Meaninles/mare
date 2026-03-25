import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowRight,
  FolderOpen,
  FolderPlus,
  HardDrive,
  LibraryBig,
  LoaderCircle,
  PencilLine,
  Pin,
  PinOff,
  RefreshCcw,
  Settings2,
  Trash2,
  X
} from "lucide-react";
import { useNavigate } from "react-router-dom";
import { useLibraryContext } from "../context/LibraryContext";
import { formatCatalogDate } from "../lib/catalog-view";
import { listLibraries, setLibraryPinned } from "../services/desktop";
import type { RegisteredLibrary } from "../types/libraries";

type FormState = {
  path: string;
  name: string;
};

type DialogMode = "create" | "open" | "edit" | null;
type BusyAction = "submit" | "open" | "delete" | "close" | "refresh" | "pin" | null;

const emptyForm: FormState = {
  path: "",
  name: ""
};

export function WelcomePage() {
  const navigate = useNavigate();
  const {
    closeLibrary,
    createLibrary,
    currentLibrary,
    isLibraryOpen,
    openLibraryPath,
    openRegisteredLibrary,
    refreshState,
    sessionQuery,
    updateRegisteredLibrary,
    deleteRegisteredLibrary
  } = useLibraryContext();

  const librariesQuery = useQuery({
    queryKey: ["welcome", "libraries"],
    queryFn: listLibraries,
    staleTime: 10_000
  });

  const [dialogMode, setDialogMode] = useState<DialogMode>(null);
  const [selectedLibraryId, setSelectedLibraryId] = useState<string | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [busyAction, setBusyAction] = useState<BusyAction>(null);
  const [busyLibraryId, setBusyLibraryId] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const libraries = useMemo(() => {
    return [...(librariesQuery.data ?? [])].sort((left, right) => {
      if (left.isPinned !== right.isPinned) {
        return left.isPinned ? -1 : 1;
      }

      const leftIsCurrent = left.id === currentLibrary?.id;
      const rightIsCurrent = right.id === currentLibrary?.id;
      if (leftIsCurrent !== rightIsCurrent) {
        return leftIsCurrent ? -1 : 1;
      }

      const leftTime = Date.parse(left.lastOpenedAt ?? left.updatedAt ?? left.createdAt);
      const rightTime = Date.parse(right.lastOpenedAt ?? right.updatedAt ?? right.createdAt);
      if (leftTime !== rightTime) {
        return rightTime - leftTime;
      }

      return left.name.localeCompare(right.name, "zh-CN");
    });
  }, [currentLibrary?.id, librariesQuery.data]);

  const selectedLibrary = useMemo(
    () => libraries.find((library) => library.id === selectedLibraryId) ?? null,
    [libraries, selectedLibraryId]
  );

  const sessionErrorMessage = sessionQuery.error instanceof Error ? sessionQuery.error.message : null;
  const librariesErrorMessage = librariesQuery.error instanceof Error ? librariesQuery.error.message : null;

  useEffect(() => {
    if (libraries.length === 0) {
      setSelectedLibraryId(null);
      return;
    }

    if (selectedLibraryId && libraries.some((library) => library.id === selectedLibraryId)) {
      return;
    }

    setSelectedLibraryId(currentLibrary?.id ?? libraries[0].id);
  }, [currentLibrary?.id, libraries, selectedLibraryId]);

  function openDialog(mode: Exclude<DialogMode, null>, library?: RegisteredLibrary | null) {
    setError(null);

    if (mode === "edit" && library) {
      setSelectedLibraryId(library.id);
      setForm({
        name: library.name,
        path: library.path
      });
    } else {
      setForm(emptyForm);
    }

    setDialogMode(mode);
  }

  function closeDialog() {
    if (busyAction !== null) {
      return;
    }

    setDialogMode(null);
    setForm(emptyForm);
  }

  async function handleSubmitDialog() {
    setBusyAction("submit");
    setNotice(null);
    setError(null);

    try {
      if (dialogMode === "create") {
        const library = await createLibrary(form);
        await Promise.all([librariesQuery.refetch(), refreshState()]);
        setSelectedLibraryId(library.id);
        setDialogMode(null);
        setForm(emptyForm);
        setNotice(`已创建“${library.name}”，请继续配置存储节点。`);
        navigate("/storage?setup=first-endpoint");
        return;
      }

      if (dialogMode === "open") {
        const library = await openLibraryPath(form);
        await Promise.all([librariesQuery.refetch(), refreshState()]);
        setSelectedLibraryId(library.id);
        setDialogMode(null);
        setForm(emptyForm);
        setNotice(`已打开“${library.name}”。`);
        navigate("/assets");
        return;
      }

      if (dialogMode === "edit" && selectedLibrary) {
        const updated = await updateRegisteredLibrary({
          id: selectedLibrary.id,
          name: form.name,
          path: form.path
        });
        await Promise.all([librariesQuery.refetch(), refreshState()]);
        setSelectedLibraryId(updated.id);
        setDialogMode(null);
        setForm(emptyForm);
        setNotice(`已更新“${updated.name}”。`);
      }
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "资产库操作失败。");
    } finally {
      setBusyAction(null);
    }
  }

  async function handleOpenLibrary(library: RegisteredLibrary) {
    setBusyAction("open");
    setBusyLibraryId(library.id);
    setNotice(null);
    setError(null);

    try {
      await openRegisteredLibrary(library);
      await Promise.all([librariesQuery.refetch(), refreshState()]);
      setSelectedLibraryId(library.id);
      navigate("/assets");
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "打开资产库失败。");
    } finally {
      setBusyAction(null);
      setBusyLibraryId(null);
    }
  }

  async function handleDeleteLibrary(library: RegisteredLibrary) {
    const confirmed = window.confirm(
      `确认删除“${library.name}”的登记记录？\n不会删除磁盘上的资产库文件。`
    );
    if (!confirmed) {
      return;
    }

    setBusyAction("delete");
    setNotice(null);
    setError(null);

    try {
      await deleteRegisteredLibrary(library);
      await Promise.all([librariesQuery.refetch(), refreshState()]);
      setDialogMode(null);
      setForm(emptyForm);
      setNotice(`已移除“${library.name}”的登记。`);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "删除资产库失败。");
    } finally {
      setBusyAction(null);
    }
  }

  async function handleCloseCurrentLibrary() {
    setBusyAction("close");
    setNotice(null);
    setError(null);

    try {
      await closeLibrary();
      await Promise.all([librariesQuery.refetch(), refreshState()]);
      setNotice("当前资产库已关闭。");
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "关闭资产库失败。");
    } finally {
      setBusyAction(null);
    }
  }

  async function handleRefreshLibraries() {
    setBusyAction("refresh");
    setBusyLibraryId(null);
    setNotice(null);
    setError(null);

    try {
      await Promise.all([librariesQuery.refetch(), refreshState()]);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "刷新资产库列表失败。");
    } finally {
      setBusyAction(null);
    }
  }

  async function handleTogglePinned(library: RegisteredLibrary) {
    setBusyAction("pin");
    setBusyLibraryId(library.id);
    setNotice(null);
    setError(null);

    try {
      const updated = await setLibraryPinned(library.id, !library.isPinned);
      await Promise.all([librariesQuery.refetch(), refreshState()]);
      setSelectedLibraryId(updated.id);
      setNotice(updated.isPinned ? `已置顶“${updated.name}”。` : `已取消置顶“${updated.name}”。`);
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "设置资产库置顶状态失败。");
    } finally {
      setBusyAction(null);
      setBusyLibraryId(null);
    }
  }

  return (
    <section className="page-stack library-hub-page">
      <article className="detail-card compact-page-header library-hub-header">
        <div className="compact-page-header-main">
          <div className="compact-page-header-title">
            <h3>资产库</h3>
          </div>
        </div>

        <div className="compact-page-header-actions">
          <button
            type="button"
            className="primary-button"
            onClick={() => openDialog("create")}
            disabled={busyAction !== null}
          >
            <FolderPlus size={16} />
            新建资产库
          </button>

          <button
            type="button"
            className="ghost-button"
            onClick={() => openDialog("open")}
            disabled={busyAction !== null}
          >
            <FolderOpen size={16} />
            打开已有
          </button>

          <button
            type="button"
            className="ghost-button"
            onClick={() => void handleRefreshLibraries()}
            disabled={busyAction !== null}
          >
            {busyAction === "refresh" ? <LoaderCircle size={16} className="spin" /> : <RefreshCcw size={16} />}
            刷新
          </button>

          {false ? (
            <button
              type="button"
              className="ghost-button"
              onClick={() => void handleCloseCurrentLibrary()}
              disabled={busyAction !== null}
            >
              {busyAction === "close" ? <LoaderCircle size={16} className="spin" /> : <HardDrive size={16} />}
              关闭当前
            </button>
          ) : null}
        </div>
      </article>

      {notice ? <p className="inline-note">{notice}</p> : null}
      {error ? <p className="error-copy">{error}</p> : null}
      {librariesErrorMessage ? <p className="error-copy">资产库列表读取失败：{librariesErrorMessage}</p> : null}
      {sessionErrorMessage ? <p className="error-copy">库内服务连接失败：{sessionErrorMessage}</p> : null}

      <article className="detail-card library-browser-card">
        <div className="library-browser-head">
          <div>
            <p className="eyebrow">Libraries</p>
            <h4>资产库列表</h4>
          </div>
          <span className="status-pill subtle">{libraries.length}</span>
        </div>

        {librariesQuery.isLoading ? (
          <div className="sync-empty-block">
            <LoaderCircle size={20} className="spin" />
            <div>
              <strong>正在读取资产库列表</strong>
              <p>请稍候。</p>
            </div>
          </div>
        ) : libraries.length === 0 ? (
          <div className="sync-empty-block">
            <LibraryBig size={20} />
            <div>
              <strong>还没有资产库</strong>
              <p>可以先新建一个，或者登记磁盘上已有的资产库文件。</p>
            </div>
          </div>
        ) : (
          <div className="library-browser-list">
            {libraries.map((library) => {
              const isCurrent = library.id === currentLibrary?.id;
              const canEnter = isCurrent && isLibraryOpen;

              return (
                <article
                  key={library.id}
                  className={`library-browser-row${canEnter ? " is-current" : ""}`}
                >
                  <div className="library-browser-main">
                    <span className="library-browser-icon">
                      <LibraryBig size={18} />
                    </span>

                    <div className="library-browser-copy">
                      <div className="library-browser-title">
                        <div className="library-browser-title-main">
                          <strong>{library.name}</strong>
                          <div className="replica-chip-row library-browser-badges">
                            {library.isPinned ? <span className="replica-chip neutral">置顶</span> : null}
                            <span className={`replica-chip ${canEnter ? "success" : "neutral"}`}>
                              {canEnter ? "当前" : "已登记"}
                            </span>
                          </div>
                        </div>
                      </div>

                      <p>{library.path}</p>
                      <span className="library-browser-meta">
                        {formatCatalogDate(library.lastOpenedAt ?? library.updatedAt ?? library.createdAt)}
                      </span>
                    </div>
                  </div>

                  <div className="library-browser-actions">
                    <button
                      type="button"
                      className="primary-button"
                      onClick={() => void handleOpenLibrary(library)}
                      disabled={busyAction !== null}
                    >
                      {busyAction === "open" && busyLibraryId === library.id ? (
                        <LoaderCircle size={16} className="spin" />
                      ) : (
                        <ArrowRight size={16} />
                      )}
                      {canEnter ? "进入资产" : "打开"}
                    </button>

                    <button
                      type="button"
                      className={`ghost-button library-action-button library-pin-button${library.isPinned ? " is-pinned" : ""}`}
                      title={library.isPinned ? "取消置顶" : "置顶资产库"}
                      aria-label={`${library.isPinned ? "取消置顶" : "置顶资产库"} ${library.name}`}
                      onClick={() => void handleTogglePinned(library)}
                      disabled={busyAction !== null}
                    >
                      {busyAction === "pin" && busyLibraryId === library.id ? (
                        <LoaderCircle size={16} className="spin" />
                      ) : library.isPinned ? (
                        <PinOff size={16} />
                      ) : (
                        <Pin size={16} />
                      )}
                    </button>

                    <button
                      type="button"
                      className="ghost-button library-action-button library-manage-button"
                      title="管理资产库"
                      aria-label={`管理资产库 ${library.name}`}
                      onClick={() => openDialog("edit", library)}
                      disabled={busyAction !== null}
                    >
                      <Settings2 size={16} />
                    </button>
                  </div>
                </article>
              );
            })}
          </div>
        )}
      </article>

      {dialogMode ? (
        <LibraryFormDialog
          mode={dialogMode}
          form={form}
          busyAction={busyAction}
          selectedLibrary={selectedLibrary}
          onChange={setForm}
          onClose={closeDialog}
          onDelete={dialogMode === "edit" && selectedLibrary ? () => void handleDeleteLibrary(selectedLibrary) : undefined}
          onSubmit={() => void handleSubmitDialog()}
        />
      ) : null}
    </section>
  );
}

function LibraryFormDialog({
  mode,
  form,
  busyAction,
  selectedLibrary,
  onChange,
  onClose,
  onDelete,
  onSubmit
}: {
  mode: Exclude<DialogMode, null>;
  form: FormState;
  busyAction: BusyAction;
  selectedLibrary: RegisteredLibrary | null;
  onChange: (next: FormState) => void;
  onClose: () => void;
  onDelete?: () => void;
  onSubmit: () => void;
}) {
  const title =
    mode === "create"
      ? "新建资产库"
      : mode === "open"
        ? "打开已有资产库"
        : `管理资产库：${selectedLibrary?.name ?? ""}`;

  const submitLabel =
    mode === "create" ? "创建并打开" : mode === "open" ? "登记并打开" : "保存修改";

  return (
    <div className="dialog-overlay" role="presentation" onClick={onClose}>
      <article
        className="dialog-card library-form-dialog"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onClick={(event) => event.stopPropagation()}
      >
        <div className="dialog-header">
          <div>
            <p className="eyebrow">{mode === "create" ? "Create" : mode === "open" ? "Open" : "Manage"}</p>
            <h4>{title}</h4>
          </div>

          <button
            type="button"
            className="ghost-button icon-button"
            onClick={onClose}
            disabled={busyAction !== null}
            aria-label="关闭弹窗"
          >
            <X size={16} />
          </button>
        </div>

        <div className="field-grid library-form-grid">
          <label className="field">
            <span>名称</span>
            <input
              value={form.name}
              onChange={(event) => onChange({ ...form, name: event.target.value })}
              placeholder={mode === "open" ? "可选，不填则沿用库内名称" : "输入资产库名称"}
            />
          </label>

          <label className="field field-span">
            <span>路径</span>
            <input
              value={form.path}
              onChange={(event) => onChange({ ...form, path: event.target.value })}
              placeholder="B:\\Libraries\\example.maredb"
            />
          </label>
        </div>

        <div className="dialog-actions library-form-actions">
          <button type="button" className="primary-button" onClick={onSubmit} disabled={busyAction !== null}>
            {busyAction === "submit" ? (
              <LoaderCircle size={16} className="spin" />
            ) : mode === "create" ? (
              <FolderPlus size={16} />
            ) : mode === "open" ? (
              <FolderOpen size={16} />
            ) : (
              <PencilLine size={16} />
            )}
            {submitLabel}
          </button>

          {mode === "edit" && onDelete ? (
            <button type="button" className="danger-button" onClick={onDelete} disabled={busyAction !== null}>
              {busyAction === "delete" ? <LoaderCircle size={16} className="spin" /> : <Trash2 size={16} />}
              删除登记
            </button>
          ) : null}

          <button type="button" className="ghost-button" onClick={onClose} disabled={busyAction !== null}>
            取消
          </button>
        </div>
      </article>
    </div>
  );
}
