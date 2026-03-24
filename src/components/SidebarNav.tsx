import {
  Archive,
  ArrowDownToLine,
  FolderTree,
  RefreshCcw,
  Server
} from "lucide-react";
import { NavLink, useLocation } from "react-router-dom";
import { useLibraryContext } from "../context/LibraryContext";
import type { NavigationItem } from "../types/navigation";
import { MareLogo } from "./MareLogo";

export const primaryNavigationItems: NavigationItem[] = [
  { label: "资产", path: "/assets", icon: Archive },
  { label: "集合", path: "/collections", icon: FolderTree },
  { label: "节点", path: "/storage", icon: Server },
  { label: "导入", path: "/ingest", icon: ArrowDownToLine },
  { label: "同步", path: "/sync", icon: RefreshCcw }
];

const routeMeta: Record<string, { label: string; caption: string }> = {
  "/assets": { label: "资产", caption: "文件与副本" },
  "/collections": { label: "集合", caption: "逻辑分类" },
  "/sync": { label: "同步", caption: "恢复与一致性" },
  "/ingest": { label: "导入", caption: "来源与规则" },
  "/storage": { label: "节点", caption: "连接的存储" },
  "/settings": { label: "设置", caption: "主题与工具" },
  "/welcome": { label: "资产库", caption: "选择入口" },
  "/media-lab": { label: "媒体实验室", caption: "诊断" },
  "/storage-test": { label: "存储测试", caption: "诊断" },
  "/removable-test": { label: "移动设备测试", caption: "诊断" },
  "/system-tasks": { label: "系统任务", caption: "跨库任务" }
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
    caption: "资产库"
  };
}

export function SidebarNav() {
  const location = useLocation();
  const { currentLibrary, isLibraryOpen } = useLibraryContext();

  return (
    <aside className="sidebar sidebar-rail">
      <div className="sidebar-inner">
        <div className="brand-panel rail-brand">
          <div className="window-dots" aria-hidden="true">
            <span />
            <span />
            <span />
          </div>
          <MareLogo className="brand-mark rail-brand-mark" />
          <div className="rail-brand-copy">
            <p className="eyebrow">Mare</p>
          </div>
        </div>

        <nav className="nav-list nav-rail" aria-label="库内导航">
          {primaryNavigationItems.map((item) => {
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

      <div className="sidebar-footer rail-footer">
        <span
          className={`rail-status-dot${isLibraryOpen ? " is-live" : ""}`}
          aria-hidden="true"
        />
        <div className="rail-footer-copy">
          <strong>{currentLibrary?.name ?? "未打开"}</strong>
          <small>{isLibraryOpen ? "当前资产库" : "选择资产库"}</small>
        </div>
      </div>
    </aside>
  );
}
