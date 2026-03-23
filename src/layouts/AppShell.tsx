import { AlertCircle, LoaderCircle, Search } from "lucide-react";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { Outlet, useLocation, useNavigate } from "react-router-dom";
import { SidebarNav, getRouteMeta } from "../components/SidebarNav";
import { useTheme } from "../components/ThemeProvider";
import { useAppBootstrap } from "../hooks/useAppBootstrap";

export function AppShell() {
  const location = useLocation();
  const navigate = useNavigate();
  const bootstrapQuery = useAppBootstrap();
  const { theme } = useTheme();
  const [searchValue, setSearchValue] = useState("");

  useEffect(() => {
    const params = new URLSearchParams(location.search);
    setSearchValue(params.get("q") ?? "");
  }, [location.pathname, location.search]);

  const routeMeta = useMemo(() => {
    const params = new URLSearchParams(location.search);
    if (location.pathname === "/library" && params.has("assetId")) {
      return {
        label: "资产详情",
        description: "查看单个资产的基础信息、逻辑路径与全部副本状态。"
      };
    }

    return getRouteMeta(location.pathname);
  }, [location.pathname, location.search]);

  function handleSearchSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const params = new URLSearchParams();
    const normalizedQuery = searchValue.trim();
    if (normalizedQuery) {
      params.set("q", normalizedQuery);
    }

    navigate({
      pathname: "/library",
      search: params.toString() ? `?${params.toString()}` : ""
    });
  }

  const readyModuleCount = bootstrapQuery.data?.modules.filter((module) => module.ready).length ?? 0;
  const totalModuleCount = bootstrapQuery.data?.modules.length ?? 0;

  return (
    <div className="app-frame">
      <SidebarNav />

      <div className="content-shell">
        <header className="topbar">
          <div className="topbar-main">
            <div className="topbar-copy">
              <p className="eyebrow">Desktop Client</p>
              <h2>{routeMeta.label}</h2>
              <p>{routeMeta.description}</p>
            </div>

            <form className="shell-search" onSubmit={handleSearchSubmit}>
              <Search size={18} strokeWidth={1.8} />
              <input
                value={searchValue}
                onChange={(event) => setSearchValue(event.target.value)}
                placeholder="搜索资产名称、逻辑路径或存储端点"
                aria-label="Global asset search"
              />
            </form>
          </div>

          <div className="status-strip">
            <span className="status-pill subtle">主题 {theme === "light" ? "浅色" : "深色"}</span>

            {bootstrapQuery.isLoading ? (
              <span className="status-pill subtle">
                <LoaderCircle size={14} className="spin" />
                正在连接 Catalog 与客户端运行模块
              </span>
            ) : null}

            {bootstrapQuery.isError ? (
              <span className="status-pill danger">
                <AlertCircle size={14} />
                客户端状态读取失败
              </span>
            ) : null}

            {bootstrapQuery.data ? (
              <>
                <span className={`status-pill ${bootstrapQuery.data.database.ready ? "success" : "warning"}`}>
                  Catalog {bootstrapQuery.data.database.ready ? "已就绪" : "待检查"}
                </span>
                <span className="status-pill subtle">
                  模块 {readyModuleCount}/{totalModuleCount}
                </span>
              </>
            ) : null}
          </div>
        </header>

        <main className="content-panel">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
