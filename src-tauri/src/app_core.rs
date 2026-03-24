use std::path::PathBuf;

use tracing::info;

use crate::{
    app_state::{AppBootstrap, DatabaseStatus, ModuleStatus, RegisteredLibrary},
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

    pub async fn bootstrap(&self, app_name: String) -> Result<AppBootstrap, AppError> {
        Ok(AppBootstrap {
            app_name,
            database: DatabaseStatus {
                ready: true,
                path: self.database_path().display().to_string(),
                migration_version: self.migration_version().to_string(),
            },
            modules: self.module_statuses(),
            active_library: self.database.get_active_library().await?,
            recent_libraries: self.database.list_libraries(Some(8)).await?,
        })
    }

    pub async fn list_libraries(&self) -> Result<Vec<RegisteredLibrary>, AppError> {
        self.database.list_libraries(None).await
    }

    pub async fn create_library_record(
        &self,
        path: String,
        name: Option<String>,
    ) -> Result<RegisteredLibrary, AppError> {
        self.database.create_library_record(path, name).await
    }

    pub async fn register_existing_library(
        &self,
        path: String,
        name: Option<String>,
    ) -> Result<RegisteredLibrary, AppError> {
        self.database.register_existing_library(path, name).await
    }

    pub async fn set_active_library(&self, id: String) -> Result<RegisteredLibrary, AppError> {
        self.database.set_active_library(id).await
    }

    pub async fn clear_active_library(&self) -> Result<(), AppError> {
        self.database.clear_active_library().await
    }
}
