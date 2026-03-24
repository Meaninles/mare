import {
  Archive,
  FolderInput,
  FolderTree,
  Home,
  RefreshCcw,
  Settings2,
  Server,
  Workflow
} from "lucide-react";
import { NavLink, useLocation } from "react-router-dom";
import { useLibraryContext } from "../context/LibraryContext";
import type { NavigationItem } from "../types/navigation";
import { MareLogo } from "./MareLogo";

export const primaryNavigationItems: NavigationItem[] = [
  { label: "Assets", path: "/assets", icon: Archive },
  { label: "Collections", path: "/collections", icon: FolderTree },
  { label: "Storage Nodes", path: "/storage", icon: Server },
  { label: "Import / Ingest", path: "/ingest", icon: FolderInput },
  { label: "Sync / Replicas", path: "/sync", icon: RefreshCcw },
  { label: "Tasks", path: "/tasks", icon: Workflow }
];

const secondaryNavigationItems: NavigationItem[] = [
  { label: "Welcome", path: "/welcome", icon: Home },
  { label: "Settings", path: "/settings", icon: Settings2 }
];

const routeMeta: Record<string, { label: string; description: string }> = {
  "/assets": { label: "Assets", description: "统一浏览当前资产库中的文件资产与副本状态。" },
  "/collections": { label: "Collections", description: "承接标签、集合和其他逻辑分类。" },
  "/sync": { label: "Sync / Replicas", description: "恢复缺失副本并跟踪跨端一致性。" },
  "/tasks": { label: "Tasks", description: "查看当前资产库的后台任务与通知。" },
  "/ingest": { label: "Import / Ingest", description: "将导入源文件按规则写入当前资产库的受管节点。" },
  "/storage": { label: "Storage Nodes", description: "配置服务于当前资产库的物理存储节点。" },
  "/settings": { label: "Settings", description: "应用主题、备份和诊断工具。" },
  "/welcome": { label: "Welcome", description: "新建资产库、打开已有资产库并查看最近使用列表。" },
  "/media-lab": { label: "Media Lab", description: "内部媒体处理与预览诊断工具。" },
  "/storage-test": { label: "Storage Tester", description: "内部存储连接器诊断工具。" },
  "/removable-test": { label: "Removable Tester", description: "内部可移动设备识别与诊断工具。" }
};

export function getRouteMeta(pathname: string) {
  const navigationItems = [...primaryNavigationItems, ...secondaryNavigationItems];
  const matched = navigationItems.find((item) => pathname === item.path || pathname.startsWith(`${item.path}/`));
  if (matched) {
    return routeMeta[matched.path] ?? { label: matched.label, description: "" };
  }

  return routeMeta[pathname] ?? { label: "Mare", description: "以资产库为核心的跨端文件资产管理。" };
}

export function SidebarNav() {
  const location = useLocation();
  const { currentLibrary, isLibraryOpen } = useLibraryContext();

  return (
    <aside className="sidebar">
      <div className="sidebar-inner">
        <div className="brand-panel compact">
          <MareLogo className="brand-mark" />
          <div className="brand-copy-block">
            <p className="eyebrow">Mare</p>
            <h1>Mare</h1>
            <p className="brand-copy">以资产库为核心统一管理多端文件资产与副本。</p>
          </div>
        </div>

        {currentLibrary ? (
          <article className="sidebar-footer compact">
            <p className="sidebar-meta">{currentLibrary.name}</p>
            <p className="sidebar-submeta">
              {isLibraryOpen ? "当前资产库已挂载，可在库内各视图之间切换。" : "当前资产库未挂载。"}
            </p>
          </article>
        ) : null}

        <nav className="nav-list" aria-label="库内导航">
          {primaryNavigationItems.map((item) => {
            const Icon = item.icon;
            const isActive = location.pathname === item.path || location.pathname.startsWith(`${item.path}/`);

            return (
              <NavLink key={item.path} to={item.path} className={`nav-item${isActive ? " active" : ""}`}>
                <span className="nav-item-icon">
                  <Icon size={18} strokeWidth={1.8} />
                </span>
                <span className="nav-item-copy compact">
                  <strong>{item.label}</strong>
                  <small>{routeMeta[item.path]?.description}</small>
                </span>
              </NavLink>
            );
          })}
        </nav>

        <nav className="nav-list" aria-label="应用导航">
          {secondaryNavigationItems.map((item) => {
            const Icon = item.icon;
            const isActive = location.pathname === item.path || location.pathname.startsWith(`${item.path}/`);

            return (
              <NavLink key={item.path} to={item.path} className={`nav-item${isActive ? " active" : ""}`}>
                <span className="nav-item-icon">
                  <Icon size={18} strokeWidth={1.8} />
                </span>
                <span className="nav-item-copy compact">
                  <strong>{item.label}</strong>
                  <small>{routeMeta[item.path]?.description}</small>
                </span>
              </NavLink>
            );
          })}
        </nav>
      </div>

      <div className="sidebar-footer compact">
        <p className="sidebar-meta">Library-Scoped UI</p>
        <p className="sidebar-submeta">
          主导航只保留资产库内页面，设置和欢迎页回到应用层。
        </p>
      </div>
    </aside>
  );
}
