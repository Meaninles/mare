mod app_core;
mod app_state;
mod bootstrap;
mod catalog;
mod config;
mod connectors;
mod database;
mod error;
mod import;
mod libraries;
mod search;
mod sync;
mod tasks;

use app_core::AppCore;
use bootstrap::get_app_bootstrap;
use libraries::{
    clear_active_library, create_library_record, delete_library_record, list_libraries,
    list_library_tasks, register_existing_library, set_active_library, update_library_record,
};
use tauri::Manager;
use tracing_subscriber::EnvFilter;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::from_default_env().add_directive(tracing::Level::INFO.into()))
        .init();

    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_fs::init())
        .setup(|app| {
            let handle = app.handle().clone();
            let app_core =
                tauri::async_runtime::block_on(async move { AppCore::initialize(&handle).await })?;

            let _ = app_core.pool();
            app.manage(app_core);
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            get_app_bootstrap,
            list_libraries,
            list_library_tasks,
            create_library_record,
            register_existing_library,
            set_active_library,
            clear_active_library,
            update_library_record,
            delete_library_record
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
