import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowRight,
  FolderOpen,
  FolderPlus,
  LibraryBig,
  LoaderCircle,
  Pencil,
  Plus,
  Save,
  Trash2
} from "lucide-react";
import { useNavigate } from "react-router-dom";
import { useLibraryContext } from "../context/LibraryContext";
import { formatCatalogDate } from "../lib/catalog-view";
import { listLibraries } from "../services/desktop";
import type { RegisteredLibrary } from "../types/libraries";

type ManagerMode = "edit" | "create" | "register";
type FormState = {
  name: string;
  path: string;
};

const emptyForm: FormState = {
  name: "",
  path: ""
};

export function LibraryManagerPage() {
  const navigate = useNavigate();
  const {
    createLibrary,
    currentLibrary,
    deleteRegisteredLibrary,
    isLibraryOpen,
    openLibraryPath,
    openRegisteredLibrary,
    refreshState,
    updateRegisteredLibrary
  } = useLibraryContext();
  const librariesQuery = useQuery({
    queryKey: ["library-manager", "all"],
    queryFn: listLibraries,
    staleTime: 10_000
  });

  const libraries = librariesQuery.data ?? [];
  const [mode, setMode] = useState<ManagerMode>("edit");
  const [selectedLibraryId, setSelectedLibraryId] = useState<string | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [busyAction, setBusyAction] = useState<"submit" | "delete" | "open" | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const selectedLibrary = useMemo(
    () => libraries.find((library) => library.id === selectedLibraryId) ?? null,
    [libraries, selectedLibraryId]
  );

  const isEditingCurrentOpenLibrary =
    Boolean(selectedLibrary) && Boolean(currentLibrary) && isLibraryOpen && selectedLibrary?.id === currentLibrary?.id;

  useEffect(() => {
    if (libraries.length === 0) {
      setSelectedLibraryId(null);
      setMode((current) => (current === "edit" ? "create" : current));
      return;
    }

    if (selectedLibraryId && libraries.some((library) => library.id === selectedLibraryId)) {
      return;
    }

    setSelectedLibraryId(currentLibrary?.id ?? libraries[0].id);
  }, [currentLibrary?.id, libraries, selectedLibraryId]);

  useEffect(() => {
    if (mode === "edit" && selectedLibrary) {
      setForm({
        name: selectedLibrary.name,
        path: selectedLibrary.path
      });
      return;
    }

    if (mode === "create" || mode === "register") {
      setForm(emptyForm);
    }
  }, [mode, selectedLibrary]);

  async function handleSubmit() {
    setBusyAction("submit");
    setNotice(null);
    setError(null);

    try {
      if (mode === "create") {
        const library = await createLibrary(form);
        await librariesQuery.refetch();
        setSelectedLibraryId(library.id);
        setMode("edit");
        setNotice(`已创建“${library.name}”，请先配置存储节点。`);
        navigate("/storage?setup=first-endpoint");
        return;
      }

      if (mode === "register") {
        const library = await openLibraryPath(form);
        await librariesQuery.refetch();
        setSelectedLibraryId(library.id);
        setMode("edit");
        navigate("/assets");
        return;
      }

      if (!selectedLibrary) {
        throw new Error("请选择一个资产库。");
      }

      const updated = await updateRegisteredLibrary({
        id: selectedLibrary.id,
        name: form.name,
        path: form.path
      });
      await librariesQuery.refetch();
      setSelectedLibraryId(updated.id);
      setNotice(`已更新“${updated.name}”。`);
    } catch (submitError) {
      setError(submitError instanceof Error ? submitError.message : "操作失败");
    } finally {
      setBusyAction(null);
    }
  }

  async function handleOpenLibrary(library: RegisteredLibrary) {
    setBusyAction("open");
    setNotice(null);
    setError(null);

    try {
      await openRegisteredLibrary(library);
      navigate("/assets");
    } catch (openError) {
      setError(openError instanceof Error ? openError.message : "打开资产库失败");
    } finally {
      setBusyAction(null);
    }
  }

  async function handleDeleteLibrary() {
    if (!selectedLibrary) {
      return;
    }

    const confirmed = window.confirm(`确认删除“${selectedLibrary.name}”的登记记录？\n不会删除磁盘上的库文件。`);
    if (!confirmed) {
      return;
    }

    setBusyAction("delete");
    setNotice(null);
    setError(null);

    try {
      await deleteRegisteredLibrary(selectedLibrary);
      await Promise.all([librariesQuery.refetch(), refreshState()]);
      setNotice(`已移除“${selectedLibrary.name}”的登记。`);
    } catch (deleteError) {
      setError(deleteError instanceof Error ? deleteError.message : "删除资产库失败");
    } finally {
      setBusyAction(null);
    }
  }

  return (
    <section className="page-stack">
      <article className="hero-card library-hero">
        <div className="library-hero-copy">
          <p className="eyebrow">Manager</p>
          <h3>资产库管理</h3>
          <p>新增、登记、修改、删除。</p>
        </div>

        <div className="hero-metrics">
          <MetricCard label="总数" value={libraries.length} tone="neutral" />
          <MetricCard label="当前" value={currentLibrary ? 1 : 0} tone="success" />
          <MetricCard label="模式" value={getModeLabel(mode)} tone="warning" />
          <MetricCard label="状态" value={isLibraryOpen ? "已打开" : "未打开"} tone="neutral" />
        </div>
      </article>

      {notice ? <p className="inline-note">{notice}</p> : null}
      {error ? <p className="error-copy">{error}</p> : null}

      <div className="page-grid library-manager-grid">
        <article className="detail-card library-manager-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">Libraries</p>
              <h4>列表</h4>
            </div>

            <div className="action-row">
              <button type="button" className="ghost-button" onClick={() => setMode("create")}>
                <Plus size={16} />
                新建
              </button>
              <button type="button" className="ghost-button" onClick={() => setMode("register")}>
                <FolderOpen size={16} />
                登记
              </button>
            </div>
          </div>

          {librariesQuery.isLoading ? (
            <div className="sync-empty-block">
              <LoaderCircle size={20} className="spin" />
              <div>
                <strong>正在读取资产库</strong>
                <p>请稍候。</p>
              </div>
            </div>
          ) : libraries.length === 0 ? (
            <div className="sync-empty-block">
              <LibraryBig size={20} />
              <div>
                <strong>还没有资产库</strong>
                <p>先新建，或登记一个已有库。</p>
              </div>
            </div>
          ) : (
            <div className="library-manager-list">
              {libraries.map((library) => {
                const isSelected = library.id === selectedLibraryId;
                const isCurrent = library.id === currentLibrary?.id;

                return (
                  <button
                    key={library.id}
                    type="button"
                    className={`library-manager-row${isSelected ? " is-selected" : ""}`}
                    onClick={() => {
                      setSelectedLibraryId(library.id);
                      setMode("edit");
                    }}
                  >
                    <div className="library-manager-row-copy">
                      <strong>{library.name}</strong>
                      <p>{library.path}</p>
                    </div>
                    <div className="library-manager-row-meta">
                      <span className={`status-pill ${isCurrent && isLibraryOpen ? "success" : "subtle"}`}>
                        {isCurrent && isLibraryOpen ? "当前" : "已登记"}
                      </span>
                      <small>{formatCatalogDate(library.lastOpenedAt ?? library.updatedAt)}</small>
                    </div>
                  </button>
                );
              })}
            </div>
          )}
        </article>

        <article className="detail-card library-manager-card">
          <div className="section-head">
            <div>
              <p className="eyebrow">{mode === "edit" ? "Edit" : mode === "create" ? "Create" : "Register"}</p>
              <h4>{getEditorTitle(mode, selectedLibrary?.name)}</h4>
            </div>

            {mode === "edit" && selectedLibrary ? (
              <button
                type="button"
                className="ghost-button"
                onClick={() => void handleOpenLibrary(selectedLibrary)}
                disabled={busyAction !== null}
              >
                {busyAction === "open" ? <LoaderCircle size={16} className="spin" /> : <ArrowRight size={16} />}
                打开
              </button>
            ) : null}
          </div>

          <div className="field-grid">
            <label className="field">
              <span>名称</span>
              <input
                value={form.name}
                onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
                placeholder="输入名称"
              />
            </label>

            <label className="field field-span">
              <span>路径</span>
              <input
                value={form.path}
                onChange={(event) => setForm((current) => ({ ...current, path: event.target.value }))}
                placeholder="B:\\Libraries\\example.maredb"
                disabled={mode === "edit" && isEditingCurrentOpenLibrary}
              />
            </label>
          </div>

          {mode === "edit" && isEditingCurrentOpenLibrary ? (
            <span className="status-pill subtle">路径已锁定</span>
          ) : null}

          <div className="action-row">
            <button
              type="button"
              className="primary-button"
              onClick={() => void handleSubmit()}
              disabled={busyAction !== null}
            >
              {busyAction === "submit" ? (
                <LoaderCircle size={16} className="spin" />
              ) : mode === "create" ? (
                <FolderPlus size={16} />
              ) : mode === "register" ? (
                <FolderOpen size={16} />
              ) : (
                <Save size={16} />
              )}
              {mode === "create" ? "创建并打开" : mode === "register" ? "登记并打开" : "保存修改"}
            </button>

            {mode === "edit" ? (
              <>
                <button type="button" className="ghost-button" onClick={() => setMode("create")}>
                  <FolderPlus size={16} />
                  新建模式
                </button>
                <button type="button" className="ghost-button" onClick={() => setMode("register")}>
                  <Pencil size={16} />
                  登记模式
                </button>
              </>
            ) : (
              <button
                type="button"
                className="ghost-button"
                onClick={() => {
                  setMode("edit");
                  if (!selectedLibrary && libraries.length > 0) {
                    setSelectedLibraryId(libraries[0].id);
                  }
                }}
              >
                <Pencil size={16} />
                返回编辑
              </button>
            )}

            {mode === "edit" && selectedLibrary ? (
              <button
                type="button"
                className="danger-button"
                onClick={() => void handleDeleteLibrary()}
                disabled={busyAction !== null}
              >
                {busyAction === "delete" ? <LoaderCircle size={16} className="spin" /> : <Trash2 size={16} />}
                删除
              </button>
            ) : null}
          </div>
        </article>
      </div>
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

function getModeLabel(mode: ManagerMode) {
  switch (mode) {
    case "create":
      return "新建";
    case "register":
      return "登记";
    default:
      return "编辑";
  }
}

function getEditorTitle(mode: ManagerMode, libraryName?: string | null) {
  switch (mode) {
    case "create":
      return "新建资产库";
    case "register":
      return "登记已有资产库";
    default:
      return libraryName ?? "编辑资产库";
  }
}
