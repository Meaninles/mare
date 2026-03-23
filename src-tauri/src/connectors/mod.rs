use crate::app_state::ModuleStatus;

pub struct ConnectorsModule;

impl ConnectorsModule {
    pub fn new() -> Self {
        Self
    }

    pub fn status(&self) -> ModuleStatus {
        ModuleStatus {
            name: "connectors".to_string(),
            ready: true,
        }
    }
}
