import { FolderOpen, LibraryBig, Settings2, Workflow } from "lucide-react";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useLibraryContext } from "../context/LibraryContext";
import { MareLogo } from "../components/MareLogo";

export function StandaloneShell() {
  const navigate = useNavigate();
  const { currentLibrary, currentLibrarySession, isLibraryOpen } = useLibraryContext();

  return (
    <div className="app-frame">
      <aside className="sidebar">
        <div className="sidebar-inner">
          <div className="brand-panel compact">
            <MareLogo className="brand-mark" />
            <div className="brand-copy-block">
              <p className="eyebrow">Mare</p>
              <h1>Mare</h1>
              <p className="brand-copy">
                先选择或创建资产库，再进入统一的文件资产管理。
              </p>
            </div>
          </div>

          <nav className="nav-list" aria-label="应用导航">
            <NavLink to="/welcome" className={({ isActive }) => `nav-item${isActive ? " active" : ""}`}>
              <span className="nav-item-icon">
                <LibraryBig size={18} strokeWidth={1.8} />
              </span>
              <span className="nav-item-copy compact">
                <strong>资产库入口</strong>
                <small>新建、打开和查看最近使用的资产库。</small>
              </span>
            </NavLink>

            <NavLink to="/system-tasks" className={({ isActive }) => `nav-item${isActive ? " active" : ""}`}>
              <span className="nav-item-icon">
                <Workflow size={18} strokeWidth={1.8} />
              </span>
              <span className="nav-item-copy compact">
                <strong>传输任务</strong>
                <small>按资产库查看所有同步任务，并预留下载任务类别。</small>
              </span>
            </NavLink>

            <NavLink to="/settings" className={({ isActive }) => `nav-item${isActive ? " active" : ""}`}>
              <span className="nav-item-icon">
                <Settings2 size={18} strokeWidth={1.8} />
              </span>
              <span className="nav-item-copy compact">
                <strong>应用设置</strong>
                <small>主题、备份和诊断工具保持在应用层。</small>
              </span>
            </NavLink>
          </nav>

          {currentLibrary ? (
            <article className="sidebar-footer compact">
              <p className="sidebar-meta">{currentLibrary.name}</p>
              <p className="sidebar-submeta">
                {isLibraryOpen && currentLibrarySession?.ready
                  ? `当前已打开：${currentLibrarySession.name ?? currentLibrary.name}`
                  : "当前没有已挂载的资产库会话。"}
              </p>

              {isLibraryOpen ? (
                <button type="button" className="ghost-button inline-button" onClick={() => navigate("/assets")}>
                  <FolderOpen size={16} />
                  进入资产视图
                </button>
              ) : null}
            </article>
          ) : null}
        </div>

        <div className="sidebar-footer compact">
          <p className="sidebar-meta">应用壳</p>
          <p className="sidebar-submeta">
            这里不直接加载资产内容，只有选定资产库后才会进入库内页面。
          </p>
        </div>
      </aside>

      <div className="content-shell">
        <main className="content-panel">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
