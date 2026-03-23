use crate::app_state::ModuleStatus;

pub struct SyncModule;

impl SyncModule {
    pub fn new() -> Self {
        Self
    }

    pub fn status(&self) -> ModuleStatus {
        ModuleStatus {
            name: "sync".to_string(),
            ready: true,
        }
    }
}
