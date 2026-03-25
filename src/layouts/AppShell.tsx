import { useMemo, useState } from "react";
import { Outlet } from "react-router-dom";
import { SidebarNav } from "../components/SidebarNav";
import { TaskCenterDrawer } from "../components/TaskCenterDrawer";
import { useLibraryContext } from "../context/LibraryContext";
import { useCatalogTasks } from "../hooks/useCatalog";
import { useImportDevices } from "../hooks/useImport";
import { useRemovableNoticeState } from "../hooks/useRemovableNoticeState";
import { getTaskSummary, getVisibleTasks } from "../lib/task-center";

export type AppShellOutletContext = {
  openTaskCenter: () => void;
  notificationCount: number;
  hasNotificationAlert: boolean;
};

export function AppShell() {
  const { currentLibraryId } = useLibraryContext();
  const tasksQuery = useCatalogTasks(500);
  const devicesQuery = useImportDevices();
  const [taskCenterOpen, setTaskCenterOpen] = useState(false);

  const taskSummary = useMemo(() => getTaskSummary(getVisibleTasks(tasksQuery.data ?? [])), [tasksQuery.data]);
  const removableNotices = useRemovableNoticeState(devicesQuery.data ?? [], currentLibraryId);
  const notificationCount = taskSummary.failed + removableNotices.unreadCount;
  const hasNotificationAlert = notificationCount > 0 || taskSummary.failed > 0;

  return (
    <div className="app-frame">
      <SidebarNav />

      <div className="content-shell content-shell-refined library-shell">
        <main className="content-panel">
          <Outlet
            context={
              {
                openTaskCenter: () => setTaskCenterOpen(true),
                notificationCount,
                hasNotificationAlert
              } satisfies AppShellOutletContext
            }
          />
        </main>
      </div>

      <TaskCenterDrawer open={taskCenterOpen} onClose={() => setTaskCenterOpen(false)} />
    </div>
  );
}
