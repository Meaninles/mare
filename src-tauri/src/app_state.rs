use serde::Serialize;

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ModuleStatus {
    pub name: String,
    pub ready: bool,
}

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DatabaseStatus {
    pub ready: bool,
    pub path: String,
    pub migration_version: String,
}

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct RegisteredLibrary {
    pub id: String,
    pub name: String,
    pub path: String,
    pub created_at: String,
    pub updated_at: String,
    pub last_opened_at: Option<String>,
    pub is_active: bool,
}

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct LibraryTaskRecord {
    pub library_id: String,
    pub library_name: String,
    pub library_path: String,
    pub library_is_active: bool,
    pub id: String,
    pub task_type: String,
    pub status: String,
    pub payload: String,
    pub result_summary: Option<String>,
    pub error_message: Option<String>,
    pub retry_count: i64,
    pub created_at: String,
    pub updated_at: String,
    pub started_at: Option<String>,
    pub finished_at: Option<String>,
    pub source_endpoint_id: Option<String>,
    pub source_endpoint_name: Option<String>,
    pub target_endpoint_id: Option<String>,
    pub target_endpoint_name: Option<String>,
}

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct AppBootstrap {
    pub app_name: String,
    pub database: DatabaseStatus,
    pub modules: Vec<ModuleStatus>,
    pub active_library: Option<RegisteredLibrary>,
    pub recent_libraries: Vec<RegisteredLibrary>,
}
