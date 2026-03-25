import { LibraryBig, Settings2, Workflow } from "lucide-react";
import { NavLink, Outlet } from "react-router-dom";
import { MareLogo } from "../components/MareLogo";

export function StandaloneShell() {
  return (
    <div className="app-frame">
      <aside className="sidebar sidebar-rail">
        <div className="sidebar-inner">
          <div className="brand-panel rail-brand">
            <MareLogo className="brand-mark rail-brand-mark" />
            <div className="rail-brand-copy">
              <p className="eyebrow">Mare</p>
            </div>
          </div>

          <nav className="nav-list nav-rail" aria-label="应用导航">
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

            <NavLink
              to="/settings"
              className={({ isActive }) => `nav-item rail-nav-item${isActive ? " active" : ""}`}
              title="设置"
            >
              <span className="nav-item-icon rail-nav-icon">
                <Settings2 size={18} strokeWidth={1.85} />
              </span>
              <span className="nav-item-copy rail-nav-copy">
                <strong>设置</strong>
              </span>
            </NavLink>
          </nav>
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
