use std::path::PathBuf;

use tracing::info;

use crate::{
    app_state::ModuleStatus,
    catalog::CatalogModule,
    config::ConfigModule,
    connectors::ConnectorsModule,
    database::DatabaseManager,
    error::AppError,
    import::ImportModule,
    search::SearchModule,
    sync::SyncModule,
    tasks::TasksModule,
};

pub struct AppCore {
    database: DatabaseManager,
    modules: Vec<ModuleStatus>,
}

impl AppCore {
    pub async fn initialize(app: &tauri::AppHandle) -> Result<Self, AppError> {
        let database = DatabaseManager::initialize(app).await?;
        let module_statuses = vec![
            CatalogModule::new().status(),
            SyncModule::new().status(),
            ImportModule::new().status(),
            SearchModule::new().status(),
            ConnectorsModule::new().status(),
            TasksModule::new().status(),
            ConfigModule::new().status(),
        ];

        for module in &module_statuses {
            info!(module = %module.name, ready = module.ready, "module initialized");
        }

        info!(database = %database.path().display(), "database initialized");

        Ok(Self {
            database,
            modules: module_statuses,
        })
    }

    pub fn module_statuses(&self) -> Vec<ModuleStatus> {
        self.modules.clone()
    }

    pub fn database_path(&self) -> &PathBuf {
        self.database.path()
    }

    pub fn migration_version(&self) -> &str {
        self.database.migration_version()
    }

    pub fn pool(&self) -> &sqlx::SqlitePool {
        self.database.pool()
    }
}
