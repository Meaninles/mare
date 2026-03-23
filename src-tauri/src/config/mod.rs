use crate::app_state::ModuleStatus;

pub struct ConfigModule;

impl ConfigModule {
    pub fn new() -> Self {
        Self
    }

    pub fn status(&self) -> ModuleStatus {
        ModuleStatus {
            name: "config".to_string(),
            ready: true,
        }
    }
}
