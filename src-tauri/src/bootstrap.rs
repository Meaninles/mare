use tauri::{AppHandle, State};

use crate::{
    app_core::AppCore,
    app_state::{AppBootstrap, DatabaseStatus},
    error::AppError,
};

#[tauri::command]
pub async fn get_app_bootstrap(app: AppHandle, core: State<'_, AppCore>) -> Result<AppBootstrap, AppError> {
    let app_name = app
        .config()
        .product_name
        .clone()
        .unwrap_or_else(|| "Mare".to_string());

    Ok(AppBootstrap {
        app_name,
        database: DatabaseStatus {
            ready: true,
            path: core.database_path().display().to_string(),
            migration_version: core.migration_version().to_string(),
        },
        modules: core.module_statuses(),
    })
}
