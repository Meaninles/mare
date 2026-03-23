use crate::app_state::ModuleStatus;

pub struct SearchModule;

impl SearchModule {
    pub fn new() -> Self {
        Self
    }

    pub fn status(&self) -> ModuleStatus {
        ModuleStatus {
            name: "search".to_string(),
            ready: true,
        }
    }
}
