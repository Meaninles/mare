use std::{collections::HashMap, path::PathBuf, str::FromStr};

use serde_json::Value;
use sqlx::{
    sqlite::{SqliteConnectOptions, SqlitePoolOptions},
    ConnectOptions, Row,
};
use tracing::info;

use crate::{
    app_state::{AppBootstrap, DatabaseStatus, LibraryTaskRecord, ModuleStatus, RegisteredLibrary},
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

    pub async fn update_library_record(
        &self,
        id: String,
        path: String,
        name: Option<String>,
    ) -> Result<RegisteredLibrary, AppError> {
        self.database.update_library_record(id, path, name).await
    }

    pub async fn set_library_pinned(
        &self,
        id: String,
        pinned: bool,
    ) -> Result<RegisteredLibrary, AppError> {
        self.database.set_library_pinned(id, pinned).await
    }

    pub async fn delete_library_record(&self, id: String) -> Result<(), AppError> {
        self.database.delete_library_record(id).await
    }

    pub async fn list_library_tasks(
        &self,
        limit_per_library: Option<i64>,
    ) -> Result<Vec<LibraryTaskRecord>, AppError> {
        let libraries = self.database.list_libraries(None).await?;
        let mut tasks = Vec::new();

        for library in libraries {
            if !std::path::Path::new(&library.path).exists() {
                continue;
            }

            let options = SqliteConnectOptions::from_str(&format!("sqlite:{}", library.path))?
                .create_if_missing(false)
                .read_only(true)
                .disable_statement_logging();

            let pool = match SqlitePoolOptions::new()
                .max_connections(1)
                .connect_with(options)
                .await
            {
                Ok(pool) => pool,
                Err(_) => continue,
            };

            let endpoint_names = read_library_endpoint_names(&pool).await.unwrap_or_default();
            let rows = match if let Some(per_library_limit) = limit_per_library {
                sqlx::query(
                    r#"
                    SELECT id, task_type, status, payload, result_summary, error_message, retry_count, created_at, updated_at, started_at, finished_at
                    FROM tasks
                    ORDER BY COALESCE(finished_at, updated_at, created_at) DESC
                    LIMIT ?1
                    "#,
                )
                .bind(per_library_limit.clamp(1, 5000))
                .fetch_all(&pool)
                .await
            } else {
                sqlx::query(
                    r#"
                    SELECT id, task_type, status, payload, result_summary, error_message, retry_count, created_at, updated_at, started_at, finished_at
                    FROM tasks
                    ORDER BY COALESCE(finished_at, updated_at, created_at) DESC
                    "#,
                )
                .fetch_all(&pool)
                .await
            } {
                Ok(rows) => rows,
                Err(_) => {
                    pool.close().await;
                    continue;
                }
            };

            for row in rows {
                let payload = row.try_get::<String, _>("payload")?;
                let result_summary = row.try_get::<Option<String>, _>("result_summary")?;
                let payload_json = serde_json::from_str::<Value>(&payload).ok();
                let result_json = result_summary
                    .as_ref()
                    .and_then(|value| serde_json::from_str::<Value>(value).ok());

                let source_endpoint_id =
                    find_json_string(&result_json, "sourceEndpointId").or_else(|| find_json_string(&payload_json, "sourceEndpointId"));
                let target_endpoint_id =
                    find_json_string(&result_json, "targetEndpointId").or_else(|| find_json_string(&payload_json, "targetEndpointId"));
                let source_endpoint_name = find_json_string(&result_json, "sourceEndpointName").or_else(|| {
                    source_endpoint_id
                        .as_ref()
                        .and_then(|value| endpoint_names.get(value))
                        .cloned()
                });
                let target_endpoint_name = find_json_string(&result_json, "targetEndpointName").or_else(|| {
                    target_endpoint_id
                        .as_ref()
                        .and_then(|value| endpoint_names.get(value))
                        .cloned()
                });

                tasks.push(LibraryTaskRecord {
                    library_id: library.id.clone(),
                    library_name: library.name.clone(),
                    library_path: library.path.clone(),
                    library_is_active: library.is_active,
                    id: row.try_get("id")?,
                    task_type: row.try_get("task_type")?,
                    status: row.try_get("status")?,
                    payload,
                    result_summary,
                    error_message: row.try_get("error_message")?,
                    retry_count: row.try_get("retry_count")?,
                    created_at: row.try_get("created_at")?,
                    updated_at: row.try_get("updated_at")?,
                    started_at: row.try_get("started_at")?,
                    finished_at: row.try_get("finished_at")?,
                    source_endpoint_id,
                    source_endpoint_name,
                    target_endpoint_id,
                    target_endpoint_name,
                });
            }

            pool.close().await;
        }

        tasks.sort_by(|left, right| {
            let left_key = left
                .finished_at
                .as_ref()
                .or(left.started_at.as_ref())
                .unwrap_or(&left.updated_at);
            let right_key = right
                .finished_at
                .as_ref()
                .or(right.started_at.as_ref())
                .unwrap_or(&right.updated_at);
            right_key.cmp(left_key)
        });

        Ok(tasks)
    }
}

async fn read_library_endpoint_names(
    pool: &sqlx::SqlitePool,
) -> Result<HashMap<String, String>, AppError> {
    let rows = sqlx::query("SELECT id, name FROM storage_endpoints")
        .fetch_all(pool)
        .await?;

    let mut values = HashMap::with_capacity(rows.len());
    for row in rows {
        values.insert(row.try_get("id")?, row.try_get("name")?);
    }
    Ok(values)
}

fn find_json_string(value: &Option<Value>, key: &str) -> Option<String> {
    value
        .as_ref()
        .and_then(|record| record.get(key))
        .and_then(|field| field.as_str())
        .map(|field| field.trim().to_string())
        .filter(|field| !field.is_empty())
}
