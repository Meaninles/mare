import { Navigate, Outlet, Route, Routes } from "react-router-dom";
import { LoaderCircle } from "lucide-react";
import { useLibraryContext } from "./context/LibraryContext";
import { AppShell } from "./layouts/AppShell";
import { StandaloneShell } from "./layouts/StandaloneShell";
import { CollectionsPage } from "./pages/CollectionsPage";
import { ImportCenterPage } from "./pages/ImportCenterPage";
import { LibraryPage } from "./pages/LibraryPage";
import { MediaLabPage } from "./pages/MediaLabPage";
import { RemovableTesterPage } from "./pages/RemovableTesterPage";
import { SettingsPage } from "./pages/SettingsPage";
import { StorageTesterPage } from "./pages/StorageTesterPage";
import { StoragePage } from "./pages/StoragePage";
import { SyncCenterPage } from "./pages/SyncCenterPage";
import { TaskCenterPage } from "./pages/TaskCenterPage";
import { WelcomePage } from "./pages/WelcomePage";

export function App() {
  return (
    <Routes>
      <Route element={<StandaloneShell />}>
        <Route index element={<Navigate to="/welcome" replace />} />
        <Route path="/welcome" element={<WelcomePage />} />
        <Route path="/settings" element={<SettingsPage />} />
      </Route>

      <Route element={<RequireLibrary />}>
        <Route element={<AppShell />}>
          <Route path="/assets" element={<LibraryPage />} />
          <Route path="/collections" element={<CollectionsPage />} />
          <Route path="/sync" element={<SyncCenterPage />} />
          <Route path="/tasks" element={<TaskCenterPage />} />
          <Route path="/ingest" element={<ImportCenterPage />} />
          <Route path="/storage" element={<StoragePage />} />
          <Route path="/media-lab" element={<MediaLabPage />} />
          <Route path="/storage-test" element={<StorageTesterPage />} />
          <Route path="/removable-test" element={<RemovableTesterPage />} />
        </Route>
      </Route>

      <Route path="/library" element={<Navigate to="/assets" replace />} />
      <Route path="/import" element={<Navigate to="/ingest" replace />} />
      <Route path="*" element={<Navigate to="/welcome" replace />} />
    </Routes>
  );
}

function RequireLibrary() {
  const { isInitializing, isLibraryOpen } = useLibraryContext();

  if (isInitializing) {
    return (
      <section className="page-stack route-pending-shell">
        <article className="detail-card empty-state">
          <LoaderCircle size={24} className="spin" />
          <div>
            <h4>正在检查当前资产库会话</h4>
            <p>只有挂载了资产库之后，才会进入库内页面。</p>
          </div>
        </article>
      </section>
    );
  }

  if (!isLibraryOpen) {
    return <Navigate to="/welcome" replace />;
  }

  return <Outlet />;
}
