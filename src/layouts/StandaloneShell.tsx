import { ChevronsUpDown, LibraryBig, Settings2, Workflow } from "lucide-react";
import { NavLink, Outlet } from "react-router-dom";
import { MareLogo } from "../components/MareLogo";

export function StandaloneShell() {
  return (
    <div className="app-frame">
      <aside className="sidebar sidebar-rail sidebar-trident">
        <div className="sidebar-inner">
          <div className="brand-panel rail-brand sidebar-brand">
            <MareLogo className="brand-mark rail-brand-mark" />
            <div className="rail-brand-copy sidebar-brand-copy">
              <strong>Mare</strong>
              <span>媒体资产台</span>
            </div>
          </div>

          <NavLink to="/welcome" className="sidebar-workspace-switch" title="应用导航">
            <span className="sidebar-workspace-icon">
              <LibraryBig size={15} strokeWidth={1.9} />
            </span>
            <span className="sidebar-workspace-copy">
              <small>当前入口</small>
              <strong>桌面工作台</strong>
            </span>
            <ChevronsUpDown size={15} strokeWidth={1.9} />
          </NavLink>

          <div className="sidebar-nav-group">
            <p className="sidebar-section-label">功能导航</p>

            <nav className="nav-list nav-rail sidebar-nav-list" aria-label="应用导航">
              <NavLink
                to="/welcome"
                className={({ isActive }) => `nav-item rail-nav-item${isActive ? " active" : ""}`}
                title="资产库"
              >
                <span className="nav-item-icon rail-nav-icon">
                  <LibraryBig size={18} strokeWidth={1.85} />
                </span>
                <span className="nav-item-copy rail-nav-copy">
                  <strong>资产库</strong>
                  <span>选择与管理入口</span>
                </span>
              </NavLink>

              <NavLink
                to="/system-tasks"
                className={({ isActive }) => `nav-item rail-nav-item${isActive ? " active" : ""}`}
                title="任务"
              >
                <span className="nav-item-icon rail-nav-icon">
                  <Workflow size={18} strokeWidth={1.85} />
                </span>
                <span className="nav-item-copy rail-nav-copy">
                  <strong>任务</strong>
                  <span>当前资产库队列</span>
                </span>
              </NavLink>
            </nav>
          </div>
        </div>

        <div className="sidebar-footer-actions">
          <NavLink to="/settings" className={({ isActive }) => `nav-item rail-nav-item sidebar-settings-link${isActive ? " active" : ""}`} title="设置">
            <span className="nav-item-icon rail-nav-icon">
              <Settings2 size={18} strokeWidth={1.85} />
            </span>
            <span className="nav-item-copy rail-nav-copy">
              <strong>设置</strong>
              <span>主题与偏好</span>
            </span>
          </NavLink>
        </div>
      </aside>

      <div className="content-shell content-shell-refined standalone-shell">
        <main className="content-panel">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
