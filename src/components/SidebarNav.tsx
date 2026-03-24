import { Archive, FolderInput, RefreshCcw, Settings2, Server } from "lucide-react";
import { NavLink, useLocation } from "react-router-dom";
import type { NavigationItem } from "../types/navigation";
import { MareLogo } from "./MareLogo";

export const primaryNavigationItems: NavigationItem[] = [
  { label: "资产库", path: "/library", icon: Archive },
  { label: "同步中心", path: "/sync", icon: RefreshCcw },
  { label: "导入中心", path: "/import", icon: FolderInput },
  { label: "存储管理", path: "/storage", icon: Server },
  { label: "设置", path: "/settings", icon: Settings2 }
];

const routeMeta: Record<string, { label: string; description: string }> = {
  "/library": { label: "资产库", description: "浏览、筛选、预览并查看资产副本状态。" },
  "/sync": { label: "同步中心", description: "补齐缺失副本，并跟踪多端一致性。" },
  "/tasks": { label: "任务与日志", description: "查看后台任务、通知提醒和结构化日志。" },
  "/import": { label: "导入中心", description: "识别移动设备，并按导入规则写入目标端点。" },
  "/storage": { label: "存储管理", description: "统一管理本地、NAS、115 和可移动存储端点。" },
  "/settings": { label: "设置", description: "外观、备份导出、恢复导入与验证。" },
  "/media-lab": { label: "媒体实验室", description: "内部媒体处理与预览诊断工具。" },
  "/storage-test": { label: "连接器测试", description: "内部存储连接器诊断工具。" },
  "/removable-test": { label: "移动设备测试", description: "内部可移动设备识别与诊断工具。" }
};

export function getRouteMeta(pathname: string) {
  if (pathname.startsWith("/library/")) {
    return {
      label: "资产详情",
      description: "查看资产预览、副本状态，并执行恢复或删除操作。"
    };
  }

  const matched = primaryNavigationItems.find((item) => pathname === item.path || pathname.startsWith(`${item.path}/`));
  if (matched) {
    return routeMeta[matched.path] ?? { label: matched.label, description: "" };
  }

  return routeMeta[pathname] ?? { label: "Mare", description: "桌面媒体资产管理。" };
}

export function SidebarNav() {
  const location = useLocation();

  return (
    <aside className="sidebar">
      <div className="sidebar-inner">
        <div className="brand-panel compact">
          <MareLogo className="brand-mark" />
          <div className="brand-copy-block">
            <p className="eyebrow">Mare</p>
            <h1>Mare</h1>
            <p className="brand-copy">在多个端点之间保存、同步、恢复和追踪你的媒体资产。</p>
          </div>
        </div>

        <nav className="nav-list" aria-label="主导航">
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
        <p className="sidebar-meta">Tauri 桌面客户端 + Go 后端</p>
        <p className="sidebar-submeta">全局搜索留在顶部，任务、恢复和通知保持贴近资产目录。</p>
      </div>
    </aside>
  );
}
