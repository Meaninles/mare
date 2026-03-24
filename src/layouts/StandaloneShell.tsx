import { FolderOpen, Folders, LibraryBig, Settings2, Workflow } from "lucide-react";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useLibraryContext } from "../context/LibraryContext";
import { MareLogo } from "../components/MareLogo";

export function StandaloneShell() {
  const navigate = useNavigate();
  const { currentLibrary, currentLibrarySession, isLibraryOpen } = useLibraryContext();

  return (
    <div className="app-frame">
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

          <nav className="nav-list nav-rail" aria-label="应用导航">
            <NavLink to="/welcome" className={({ isActive }) => `nav-item rail-nav-item${isActive ? " active" : ""}`} title="资产库">
              <span className="nav-item-icon rail-nav-icon">
                <LibraryBig size={18} strokeWidth={1.85} />
              </span>
              <span className="nav-item-copy rail-nav-copy">
                <strong>资产库</strong>
              </span>
            </NavLink>

            <NavLink to="/library-manager" className={({ isActive }) => `nav-item rail-nav-item${isActive ? " active" : ""}`} title="管理">
              <span className="nav-item-icon rail-nav-icon">
                <Folders size={18} strokeWidth={1.85} />
              </span>
              <span className="nav-item-copy rail-nav-copy">
                <strong>管理</strong>
              </span>
            </NavLink>

            <NavLink to="/system-tasks" className={({ isActive }) => `nav-item rail-nav-item${isActive ? " active" : ""}`} title="任务">
              <span className="nav-item-icon rail-nav-icon">
                <Workflow size={18} strokeWidth={1.85} />
              </span>
              <span className="nav-item-copy rail-nav-copy">
                <strong>任务</strong>
              </span>
            </NavLink>

            <NavLink to="/settings" className={({ isActive }) => `nav-item rail-nav-item${isActive ? " active" : ""}`} title="设置">
              <span className="nav-item-icon rail-nav-icon">
                <Settings2 size={18} strokeWidth={1.85} />
              </span>
              <span className="nav-item-copy rail-nav-copy">
                <strong>设置</strong>
              </span>
            </NavLink>
          </nav>
        </div>

        <div className="sidebar-footer rail-footer">
          <span
            className={`rail-status-dot${isLibraryOpen ? " is-live" : ""}`}
            aria-hidden="true"
          />
          <div className="rail-footer-copy">
            <strong>{currentLibrary?.name ?? "未打开"}</strong>
            <small>{isLibraryOpen ? "当前资产库" : "入口页"}</small>
          </div>
        </div>
      </aside>

      <div className="content-shell content-shell-refined standalone-shell">
        {currentLibrary ? (
          <div className="standalone-shell-bar">
            <div className="status-pill subtle toolbar-pill">
              <LibraryBig size={14} />
              {currentLibrarySession?.name ?? currentLibrary.name}
            </div>

            {isLibraryOpen ? (
              <button
                type="button"
                className="ghost-button inline-button"
                onClick={() => navigate("/assets")}
              >
                <FolderOpen size={16} />
                进入资产
              </button>
            ) : null}
          </div>
        ) : null}

        <main className="content-panel">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
