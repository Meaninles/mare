import { Archive, ArrowDownToLine, ChevronLeft, FolderTree, RefreshCcw, Server, Settings2 } from "lucide-react";
import { NavLink, useLocation } from "react-router-dom";
import { useLibraryContext } from "../context/LibraryContext";
import type { NavigationItem } from "../types/navigation";
import { MareLogo } from "./MareLogo";
import { MareWordmark } from "./MareWordmark";

export const primaryNavigationItems: NavigationItem[] = [
  { label: "资产", path: "/assets", icon: Archive },
  { label: "集合", path: "/collections", icon: FolderTree },
  { label: "节点", path: "/storage", icon: Server },
  { label: "导入", path: "/ingest", icon: ArrowDownToLine },
  { label: "同步", path: "/sync", icon: RefreshCcw }
];

const routeMeta: Record<string, { label: string; caption: string }> = {
  "/assets": { label: "资产", caption: "" },
  "/collections": { label: "集合", caption: "" },
  "/sync": { label: "同步", caption: "" },
  "/ingest": { label: "导入", caption: "" },
  "/storage": { label: "节点", caption: "" },
  "/settings": { label: "设置", caption: "" },
  "/welcome": { label: "资产库", caption: "" },
  "/media-lab": { label: "媒体实验室", caption: "" },
  "/storage-test": { label: "存储测试", caption: "" },
  "/removable-test": { label: "移动设备测试", caption: "" },
  "/system-tasks": { label: "系统任务", caption: "" }
};

export function getRouteMeta(pathname: string) {
  const matched = primaryNavigationItems.find(
    (item) => pathname === item.path || pathname.startsWith(`${item.path}/`)
  );
  if (matched) {
    return routeMeta[matched.path] ?? { label: matched.label, caption: "" };
  }

  return routeMeta[pathname] ?? {
    label: "Mare",
    caption: ""
  };
}

export function SidebarNav() {
  const location = useLocation();
  const { currentLibrary } = useLibraryContext();

  return (
    <aside className="sidebar sidebar-rail sidebar-trident">
      <div className="sidebar-inner">
        <div className="brand-panel rail-brand sidebar-brand">
          <MareLogo className="brand-mark rail-brand-mark" />
          <div className="rail-brand-copy sidebar-brand-copy">
            <MareWordmark />
          </div>
        </div>

        {currentLibrary ? (
          <div className="sidebar-library-status" title={currentLibrary.name}>
            <span>当前资产库</span>
            <strong>{currentLibrary.name}</strong>
          </div>
        ) : null}

        <div className="sidebar-nav-group">
          <p className="sidebar-section-label">功能导航</p>

          <nav className="nav-list nav-rail sidebar-nav-list" aria-label="库内导航">
            {[...primaryNavigationItems, { label: "返回资产库", path: "/welcome", icon: ChevronLeft }].map((item) => {
              const Icon = item.icon;
              const isActive =
                location.pathname === item.path || location.pathname.startsWith(`${item.path}/`);

              return (
                <NavLink
                  key={item.path}
                  to={item.path}
                  className={`nav-item rail-nav-item${isActive ? " active" : ""}`}
                  title={item.label}
                >
                  <span className="nav-item-icon rail-nav-icon">
                    <Icon size={18} strokeWidth={1.85} />
                  </span>
                  <span className="nav-item-copy rail-nav-copy">
                    <strong>{item.label}</strong>
                  </span>
                </NavLink>
              );
            })}
          </nav>
        </div>
      </div>

      <div className="sidebar-footer-actions">
        <NavLink to="/settings" className="nav-item rail-nav-item sidebar-settings-link" title="设置">
          <span className="nav-item-icon rail-nav-icon">
            <Settings2 size={18} strokeWidth={1.85} />
          </span>
          <span className="nav-item-copy rail-nav-copy">
            <strong>设置</strong>
          </span>
        </NavLink>
      </div>
    </aside>
  );
}
