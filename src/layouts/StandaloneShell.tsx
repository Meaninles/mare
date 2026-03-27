import { LibraryBig, Settings2, Workflow } from "lucide-react";
import { NavLink, Outlet } from "react-router-dom";
import { MareLogo } from "../components/MareLogo";
import { MareWordmark } from "../components/MareWordmark";

export function StandaloneShell() {
  return (
    <div className="app-frame">
      <aside className="sidebar sidebar-rail sidebar-trident">
        <div className="sidebar-inner">
          <div className="brand-panel rail-brand sidebar-brand">
            <MareLogo className="brand-mark rail-brand-mark" />
            <div className="rail-brand-copy sidebar-brand-copy">
              <MareWordmark />
            </div>
          </div>

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
                </span>
              </NavLink>
            </nav>
          </div>
        </div>

        <div className="sidebar-footer-actions">
          <NavLink
            to="/settings"
            className={({ isActive }) => `nav-item rail-nav-item sidebar-settings-link${isActive ? " active" : ""}`}
            title="设置"
          >
            <span className="nav-item-icon rail-nav-icon">
              <Settings2 size={18} strokeWidth={1.85} />
            </span>
            <span className="nav-item-copy rail-nav-copy">
              <strong>设置</strong>
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
