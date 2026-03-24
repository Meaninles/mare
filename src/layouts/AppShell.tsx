import type { FormEvent } from "react";
import { useEffect, useMemo, useState } from "react";
import {
  BellRing,
  LibraryBig,
  MoonStar,
  Search,
  Settings2,
  SunMedium
} from "lucide-react";
import { Outlet, useLocation, useNavigate } from "react-router-dom";
import { SidebarNav, getRouteMeta } from "../components/SidebarNav";
import { TaskCenterDrawer } from "../components/TaskCenterDrawer";
import { useTheme } from "../components/ThemeProvider";
import { useLibraryContext } from "../context/LibraryContext";
import { useCatalogTasks } from "../hooks/useCatalog";
import { useImportDevices } from "../hooks/useImport";
import { useRemovableNoticeState } from "../hooks/useRemovableNoticeState";
import { getTaskSummary } from "../lib/task-center";

export function AppShell() {
  const location = useLocation();
  const navigate = useNavigate();
  const { currentLibrary, currentLibraryId } = useLibraryContext();
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
        label: "详情",
        caption: "单个资产"
      };
    }

    return getRouteMeta(location.pathname);
  }, [location.pathname, location.search]);

  const taskSummary = useMemo(() => getTaskSummary(tasksQuery.data ?? []), [tasksQuery.data]);
  const removableNotices = useRemovableNoticeState(devicesQuery.data ?? [], currentLibraryId);
  const notificationCount = taskSummary.failed + removableNotices.unreadCount;
  const ThemeIcon = theme === "light" ? SunMedium : MoonStar;

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

  return (
    <div className="app-frame">
      <SidebarNav />

      <div className="content-shell content-shell-refined">
        <header className="topbar shell-topbar">
          <div className="shell-title-row">
            <div className="window-dots window-dots-inline" aria-hidden="true">
              <span />
              <span />
              <span />
            </div>

            <div className="topbar-copy shell-topbar-copy">
              <p className="eyebrow">{routeMeta.caption}</p>
              <h2>{routeMeta.label}</h2>
              <p className="shell-subtitle">{currentLibrary?.name ?? "未打开资产库"}</p>
            </div>
          </div>

          <div className="shell-toolbar">
            <form className="shell-search shell-search-minimal" onSubmit={handleSearchSubmit}>
              <Search size={17} strokeWidth={1.9} />
              <input
                value={searchValue}
                onChange={(event) => setSearchValue(event.target.value)}
                placeholder="搜索资产"
                aria-label="搜索当前资产库"
              />
            </form>

            <div className="shell-toolbar-actions">
              <span className="status-pill subtle toolbar-pill" title={theme === "light" ? "浅色模式" : "深色模式"}>
                <ThemeIcon size={14} />
                {theme === "light" ? "浅色" : "深色"}
              </span>

              {currentLibrary ? (
                <button
                  type="button"
                  className="ghost-button icon-button shell-action-button"
                  onClick={() => navigate("/welcome")}
                  aria-label="打开资产库入口"
                  title="资产库"
                >
                  <LibraryBig size={18} />
                </button>
              ) : null}

              <button
                type="button"
                className="ghost-button icon-button shell-action-button"
                onClick={() => navigate("/settings")}
                aria-label="打开设置"
                title="设置"
              >
                <Settings2 size={18} />
              </button>

              <button
                type="button"
                className={`ghost-button icon-button shell-action-button task-center-icon${
                  notificationCount > 0 || taskSummary.failed > 0 ? " has-alert" : ""
                }`}
                onClick={() => setTaskCenterOpen(true)}
                aria-label="打开通知中心"
                title="通知中心"
              >
                <BellRing size={18} />
                {notificationCount > 0 ? <span className="task-center-badge">{notificationCount}</span> : null}
              </button>
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
