use crate::app_state::ModuleStatus;

pub struct ImportModule;

impl ImportModule {
    pub fn new() -> Self {
        Self
    }

    pub fn status(&self) -> ModuleStatus {
        ModuleStatus {
            name: "import".to_string(),
            ready: true,
        }
    }
}
