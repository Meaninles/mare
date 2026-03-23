use crate::app_state::ModuleStatus;

pub struct CatalogModule;

impl CatalogModule {
    pub fn new() -> Self {
        Self
    }

    pub fn status(&self) -> ModuleStatus {
        ModuleStatus {
            name: "catalog".to_string(),
            ready: true,
        }
    }
}
