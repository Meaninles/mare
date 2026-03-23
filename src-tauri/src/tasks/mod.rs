use crate::app_state::ModuleStatus;

pub struct TasksModule;

impl TasksModule {
    pub fn new() -> Self {
        Self
    }

    pub fn status(&self) -> ModuleStatus {
        ModuleStatus {
            name: "tasks".to_string(),
            ready: true,
        }
    }
}
