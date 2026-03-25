import type { FormEvent } from "react";
import { useEffect, useMemo, useState } from "react";
import {
  BellRing,
  LibraryBig,
  Search,
  Settings2
} from "lucide-react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { SidebarNav, getRouteMeta } from "../components/SidebarNav";
import { TaskCenterDrawer } from "../components/TaskCenterDrawer";
import { useLibraryContext } from "../context/LibraryContext";
import { useCatalogTasks } from "../hooks/useCatalog";
import { useImportDevices } from "../hooks/useImport";
import { useRemovableNoticeState } from "../hooks/useRemovableNoticeState";
import { getTaskSummary, getVisibleTasks } from "../lib/task-center";

export function AppShell() {
  const location = useLocation();
  const navigate = useNavigate();
  const { currentLibrary, currentLibraryId } = useLibraryContext();
  const tasksQuery = useCatalogTasks(500);
  const devicesQuery = useImportDevices();
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
    if (location.pathname === "/assets" && params.get("q")?.trim()) {
      return {
        label: "搜索",
        caption: "统一检索入口"
      };
    }

    return getRouteMeta(location.pathname);
  }, [location.pathname, location.search]);

  const taskSummary = useMemo(() => getTaskSummary(getVisibleTasks(tasksQuery.data ?? [])), [tasksQuery.data]);
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

  return (
    <div className="app-frame">
      <SidebarNav />

      <div className="content-shell content-shell-refined">
        <header className="topbar shell-topbar">
          <div className="shell-title-row">
            <div className="topbar-copy shell-topbar-copy">
              <p className="eyebrow">{routeMeta.caption}</p>
              <h2>{routeMeta.label}</h2>
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
              <span
                className="status-pill subtle toolbar-pill shell-context-pill"
                title={currentLibrary?.name ?? "未打开资产库"}
              >
                <LibraryBig size={14} />
                <span>{currentLibrary?.name ?? "未打开资产库"}</span>
              </span>

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

              <NavLink
                to="/settings"
                className="ghost-button icon-button shell-action-button shell-settings-button"
                aria-label="设置"
                title="设置"
              >
                <Settings2 size={18} />
              </NavLink>
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
