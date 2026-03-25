import { Folders, LibraryBig, Settings2, Workflow } from "lucide-react";
import { NavLink, Outlet } from "react-router-dom";
import { MareLogo } from "../components/MareLogo";
import { useLibraryContext } from "../context/LibraryContext";

export function StandaloneShell() {
  const { currentLibrary, currentLibrarySession, isLibraryOpen } = useLibraryContext();

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
              to="/library-manager"
              className={({ isActive }) => `nav-item rail-nav-item${isActive ? " active" : ""}`}
              title="管理"
            >
              <span className="nav-item-icon rail-nav-icon">
                <Folders size={18} strokeWidth={1.85} />
              </span>
              <span className="nav-item-copy rail-nav-copy">
                <strong>管理</strong>
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
      </aside>

      <div className="content-shell content-shell-refined standalone-shell">
        <div className="standalone-shell-bar">
          <div className="standalone-shell-actions">
            {currentLibrary ? (
              <span
                className={`status-pill subtle toolbar-pill shell-context-pill${
                  isLibraryOpen ? " is-live" : ""
                }`}
                title={currentLibrarySession?.name ?? currentLibrary.name}
              >
                <span
                  className={`rail-status-dot shell-status-dot${isLibraryOpen ? " is-live" : ""}`}
                  aria-hidden="true"
                />
                <LibraryBig size={14} />
                <span>{currentLibrarySession?.name ?? currentLibrary.name}</span>
              </span>
            ) : null}

            <NavLink
              to="/settings"
              className="ghost-button icon-button shell-action-button shell-settings-button"
              title="设置"
              aria-label="设置"
            >
              <Settings2 size={18} strokeWidth={1.9} />
            </NavLink>
          </div>
        </div>

        <main className="content-panel">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
