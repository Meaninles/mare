use std::{path::PathBuf, str::FromStr};

use sqlx::{
    migrate::Migrator,
    sqlite::{SqliteConnectOptions, SqliteJournalMode, SqlitePoolOptions, SqliteSynchronous},
    ConnectOptions, Row, SqlitePool,
};
use tauri::{AppHandle, Manager};

use crate::error::AppError;

static MIGRATOR: Migrator = sqlx::migrate!("./migrations");

pub struct DatabaseManager {
    pool: SqlitePool,
    path: PathBuf,
    migration_version: String,
}

impl DatabaseManager {
    pub async fn initialize(app: &AppHandle) -> Result<Self, AppError> {
        let app_data_dir = app.path().app_data_dir()?;
        std::fs::create_dir_all(&app_data_dir)?;

        let database_dir = app_data_dir.join("catalog");
        std::fs::create_dir_all(&database_dir)?;

        let database_path = database_dir.join("mam.sqlite3");
        let options = SqliteConnectOptions::from_str(&format!("sqlite:{}", database_path.display()))?
            .create_if_missing(true)
            .journal_mode(SqliteJournalMode::Wal)
            .synchronous(SqliteSynchronous::Normal)
            .disable_statement_logging();

        let pool = SqlitePoolOptions::new()
            .max_connections(1)
            .connect_with(options)
            .await?;

        MIGRATOR.run(&pool).await?;

        let migration_version = sqlx::query("SELECT value FROM app_metadata WHERE key = 'schema_version' LIMIT 1")
            .fetch_optional(&pool)
            .await?
            .and_then(|row| row.try_get::<String, _>("value").ok())
            .unwrap_or_else(|| "unknown".to_string());

        Ok(Self {
            pool,
            path: database_path,
            migration_version,
        })
    }

    pub fn pool(&self) -> &SqlitePool {
        &self.pool
    }

    pub fn path(&self) -> &PathBuf {
        &self.path
    }

    pub fn migration_version(&self) -> &str {
        &self.migration_version
    }
}
