use tauri::State;

use crate::{app_core::AppCore, app_state::RegisteredLibrary, error::AppError};

#[tauri::command]
pub async fn list_libraries(core: State<'_, AppCore>) -> Result<Vec<RegisteredLibrary>, AppError> {
    core.list_libraries().await
}

#[tauri::command]
pub async fn create_library_record(
    path: String,
    name: Option<String>,
    core: State<'_, AppCore>,
) -> Result<RegisteredLibrary, AppError> {
    core.create_library_record(path, name).await
}

#[tauri::command]
pub async fn register_existing_library(
    path: String,
    name: Option<String>,
    core: State<'_, AppCore>,
) -> Result<RegisteredLibrary, AppError> {
    core.register_existing_library(path, name).await
}

#[tauri::command]
pub async fn set_active_library(
    id: String,
    core: State<'_, AppCore>,
) -> Result<RegisteredLibrary, AppError> {
    core.set_active_library(id).await
}

#[tauri::command]
pub async fn clear_active_library(core: State<'_, AppCore>) -> Result<(), AppError> {
    core.clear_active_library().await
}
