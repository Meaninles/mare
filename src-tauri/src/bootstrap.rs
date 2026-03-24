use tauri::{AppHandle, State};

use crate::{app_core::AppCore, app_state::AppBootstrap, error::AppError};

#[tauri::command]
pub async fn get_app_bootstrap(
    app: AppHandle,
    core: State<'_, AppCore>,
) -> Result<AppBootstrap, AppError> {
    let app_name = app
        .config()
        .product_name
        .clone()
        .unwrap_or_else(|| "Mare".to_string());

    core.bootstrap(app_name).await
}
