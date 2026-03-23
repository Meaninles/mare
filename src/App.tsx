import { Navigate, Route, Routes } from "react-router-dom";
import { AppShell } from "./layouts/AppShell";
import { ImportCenterPage } from "./pages/ImportCenterPage";
import { LibraryPage } from "./pages/LibraryPage";
import { RemovableTesterPage } from "./pages/RemovableTesterPage";
import { SettingsPage } from "./pages/SettingsPage";
import { StorageTesterPage } from "./pages/StorageTesterPage";
import { StoragePage } from "./pages/StoragePage";
import { SyncCenterPage } from "./pages/SyncCenterPage";

export function App() {
  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<Navigate to="/library" replace />} />
        <Route path="/library" element={<LibraryPage />} />
        <Route path="/sync" element={<SyncCenterPage />} />
        <Route path="/import" element={<ImportCenterPage />} />
        <Route path="/storage" element={<StoragePage />} />
        <Route path="/storage-test" element={<StorageTesterPage />} />
        <Route path="/removable-test" element={<RemovableTesterPage />} />
        <Route path="/settings" element={<SettingsPage />} />
      </Route>
    </Routes>
  );
}
