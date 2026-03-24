import {
  Archive,
  FolderInput,
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
  { label: "存储节点", path: "/storage", icon: Server },
  { label: "导入", path: "/ingest", icon: FolderInput },
  { label: "同步与副本", path: "/sync", icon: RefreshCcw }
];

const routeMeta: Record<string, { label: string; description: string }> = {
  "/assets": { label: "资产", description: "统一浏览当前资产库中的文件资产与副本状态。" },
  "/collections": { label: "集合", description: "承接标签、集合和其他逻辑分类。" },
  "/sync": { label: "同步与副本", description: "恢复缺失副本并跟踪跨端一致性。" },
  "/ingest": { label: "导入", description: "将导入源文件按规则写入当前资产库的受管节点。" },
  "/storage": { label: "存储节点", description: "配置服务于当前资产库的物理存储节点。" },
  "/settings": { label: "设置", description: "应用主题、备份和诊断工具。" },
  "/welcome": { label: "欢迎页", description: "新建资产库、打开已有资产库并查看最近使用列表。" },
  "/media-lab": { label: "媒体实验室", description: "内部媒体处理与预览诊断工具。" },
  "/storage-test": { label: "存储测试器", description: "内部存储连接器诊断工具。" },
  "/removable-test": { label: "移动设备测试器", description: "内部可移动设备识别与诊断工具。" }
};

export function getRouteMeta(pathname: string) {
  const matched = primaryNavigationItems.find(
    (item) => pathname === item.path || pathname.startsWith(`${item.path}/`)
  );
  if (matched) {
    return routeMeta[matched.path] ?? { label: matched.label, description: "" };
  }

  return routeMeta[pathname] ?? {
    label: "Mare",
    description: "以资产库为核心的跨端文件资产管理。"
  };
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
      </div>

      <div className="sidebar-footer compact">
        <p className="sidebar-meta">库内界面</p>
        <p className="sidebar-submeta">左侧只保留资产库内导航，全局页面入口移到顶部角落图标。</p>
      </div>
    </aside>
  );
}
