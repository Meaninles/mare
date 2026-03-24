import type { FormEvent } from "react";
import { useEffect, useMemo, useState } from "react";
import { BellRing, LoaderCircle, Search, Sparkles } from "lucide-react";
import { Outlet, useLocation, useNavigate } from "react-router-dom";
import { SidebarNav, getRouteMeta } from "../components/SidebarNav";
import { TaskCenterDrawer } from "../components/TaskCenterDrawer";
import { useTheme } from "../components/ThemeProvider";
import { useLibraryContext } from "../context/LibraryContext";
import { useAppBootstrap } from "../hooks/useAppBootstrap";
import { useCatalogTasks } from "../hooks/useCatalog";
import { useImportDevices } from "../hooks/useImport";
import { useRemovableNoticeState } from "../hooks/useRemovableNoticeState";
import { getTaskSummary } from "../lib/task-center";

export function AppShell() {
  const location = useLocation();
  const navigate = useNavigate();
  const { currentLibrary, currentLibraryId } = useLibraryContext();
  const bootstrapQuery = useAppBootstrap();
  const tasksQuery = useCatalogTasks(20);
  const devicesQuery = useImportDevices();
  const { theme } = useTheme();
  const [searchValue, setSearchValue] = useState("");
  const [taskCenterOpen, setTaskCenterOpen] = useState(false);

  useEffect(() => {
    const params = new URLSearchParams(location.search);
    setSearchValue(params.get("q") ?? "");
  }, [location.pathname, location.search]);

  const routeMeta = useMemo(() => {
    const params = new URLSearchParams(location.search);
    if (location.pathname === "/assets" && params.has("assetId")) {
      return {
        label: "Asset Detail",
        description: "预览资产、查看副本状态，并执行恢复或删除操作。"
      };
    }

    return getRouteMeta(location.pathname);
  }, [location.pathname, location.search]);

  const taskSummary = useMemo(() => getTaskSummary(tasksQuery.data ?? []), [tasksQuery.data]);
  const removableNotices = useRemovableNoticeState(devicesQuery.data ?? [], currentLibraryId);
  const notificationCount = taskSummary.failed + removableNotices.unreadCount;

  function handleSearchSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const params = new URLSearchParams();
    const normalizedQuery = searchValue.trim();
    if (normalizedQuery) {
      params.set("q", normalizedQuery);
    }

    navigate({
      pathname: "/assets",
      search: params.toString() ? `?${params.toString()}` : ""
    });
  }

  const readyModuleCount = bootstrapQuery.data?.modules.filter((module) => module.ready).length ?? 0;
  const totalModuleCount = bootstrapQuery.data?.modules.length ?? 0;

  return (
    <div className="app-frame">
      <SidebarNav />

      <div className="content-shell">
        <header className="topbar topbar-refined">
          <div className="shell-header-row">
            <div className="topbar-copy compact">
              <p className="eyebrow">{routeMeta.label}</p>
              <h2>{routeMeta.label}</h2>
              <p>{routeMeta.description}</p>
            </div>

            <div className="shell-actions">
              <button
                type="button"
                className={`ghost-button task-center-trigger${notificationCount > 0 || taskSummary.failed > 0 ? " has-alert" : ""}`}
                onClick={() => setTaskCenterOpen(true)}
              >
                <BellRing size={16} />
                通知中心
                <span className={`status-pill ${notificationCount > 0 || taskSummary.failed > 0 ? "danger" : taskSummary.running > 0 ? "warning" : "subtle"}`}>
                  {notificationCount > 0
                    ? `${notificationCount} 条提醒`
                    : taskSummary.running > 0
                      ? `${taskSummary.running} 个进行中`
                      : "已读"}
                </span>
              </button>
            </div>
          </div>

          <div className="topbar-main topbar-main-wide">
            <form className="shell-search shell-search-wide" onSubmit={handleSearchSubmit}>
              <Search size={18} strokeWidth={1.8} />
              <input
                value={searchValue}
                onChange={(event) => setSearchValue(event.target.value)}
                placeholder="搜索名称、路径、描述或逻辑路径"
                aria-label="当前资产库搜索"
              />
            </form>

            <div className="status-strip status-strip-rich">
              {currentLibrary ? <span className="status-pill subtle">{currentLibrary.name}</span> : null}
              <span className="status-pill subtle">{theme === "light" ? "浅色" : "深色"}</span>

              {bootstrapQuery.isLoading ? (
                <span className="status-pill subtle">
                  <LoaderCircle size={14} className="spin" />
                  启动中
                </span>
              ) : null}

              {bootstrapQuery.data ? (
                <>
                  <span className={`status-pill ${bootstrapQuery.data.database.ready ? "success" : "warning"}`}>
                    应用数据库{bootstrapQuery.data.database.ready ? "已就绪" : "检查中"}
                  </span>
                  <span className="status-pill subtle">
                    模块 {readyModuleCount}/{totalModuleCount}
                  </span>
                </>
              ) : null}

              <span className="status-pill subtle">
                <Sparkles size={14} />
                桌面客户端
              </span>
            </div>
          </div>
        </header>

        <main className="content-panel">
          <Outlet />
        </main>
      </div>

      <TaskCenterDrawer open={taskCenterOpen} onClose={() => setTaskCenterOpen(false)} />
    </div>
  );
}
