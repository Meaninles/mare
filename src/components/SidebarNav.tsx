import { Archive, FolderInput, RefreshCcw, Settings2, Server } from "lucide-react";
import { NavLink, useLocation } from "react-router-dom";
import { MareLogo } from "./MareLogo";
import type { NavigationItem } from "../types/navigation";

export const primaryNavigationItems: NavigationItem[] = [
  {
    label: "资产库",
    path: "/library",
    icon: Archive,
    description: "统一浏览资产、逻辑路径与多副本状态"
  },
  {
    label: "同步中心",
    path: "/sync",
    icon: RefreshCcw,
    description: "承接恢复、补齐与后续失败重试"
  },
  {
    label: "导入中心",
    path: "/import",
    icon: FolderInput,
    description: "面向移动设备与规则驱动的入库流"
  },
  {
    label: "存储管理",
    path: "/storage",
    icon: Server,
    description: "配置端点、触发扫描并校验运行状态"
  },
  {
    label: "设置",
    path: "/settings",
    icon: Settings2,
    description: "管理外观、连接策略与后续恢复能力"
  }
];

const hiddenRouteMeta: Record<string, { label: string; description: string }> = {
  "/storage-test": {
    label: "连接器验证",
    description: "用于人工验证 QNAP 与 115 连接器基础能力。"
  },
  "/removable-test": {
    label: "移动设备验证",
    description: "用于检测可移动设备识别与连接器读写能力。"
  }
};

export function getRouteMeta(pathname: string) {
  if (pathname.startsWith("/library/")) {
    return {
      label: "资产详情",
      description: "查看单个资产的基础信息、逻辑路径与全部副本状态。"
    };
  }

  const primaryMatch = primaryNavigationItems.find(
    (item) => pathname === item.path || pathname.startsWith(`${item.path}/`)
  );

  if (primaryMatch) {
    return {
      label: primaryMatch.label,
      description: primaryMatch.description ?? ""
    };
  }

  return (
    hiddenRouteMeta[pathname] ?? {
      label: "Mare",
      description: "面向多端点媒体资产管理的桌面客户端。"
    }
  );
}

export function SidebarNav() {
  const location = useLocation();

  return (
    <aside className="sidebar">
      <div className="sidebar-inner">
        <div className="brand-panel">
          <MareLogo className="brand-mark" />
          <div className="brand-copy-block">
            <p className="eyebrow">Desktop Client</p>
            <h1>Mare</h1>
            <p className="brand-copy">
              以客户端方式统一管理本地、NAS、网盘与移动设备里的媒体资产、同步关系与恢复入口。
            </p>
          </div>
        </div>

        <nav className="nav-list" aria-label="Primary navigation">
          {primaryNavigationItems.map((item) => {
            const Icon = item.icon;
            const isActive =
              location.pathname === item.path || location.pathname.startsWith(`${item.path}/`);

            return (
              <NavLink key={item.path} to={item.path} className={`nav-item${isActive ? " active" : ""}`}>
                <span className="nav-item-icon">
                  <Icon size={18} strokeWidth={1.8} />
                </span>
                <span className="nav-item-copy">
                  <strong>{item.label}</strong>
                  <span>{item.description}</span>
                </span>
              </NavLink>
            );
          })}
        </nav>
      </div>

      <div className="sidebar-footer">
        <p className="eyebrow">Runtime</p>
        <p className="sidebar-meta">Mare Client + Go Services</p>
        <p className="sidebar-submeta">
          A restrained desktop client for archive, ingest, sync, recovery and future catalog intelligence.
        </p>
      </div>
    </aside>
  );
}
