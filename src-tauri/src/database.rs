use std::{
    path::{Path, PathBuf},
    str::FromStr,
};

use chrono::Utc;
use sqlx::{
    migrate::Migrator,
    sqlite::{SqliteConnectOptions, SqliteJournalMode, SqlitePoolOptions, SqliteSynchronous},
    ConnectOptions, Row, SqlitePool,
};
use tauri::{AppHandle, Manager};

use crate::app_state::RegisteredLibrary;
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
        let options =
            SqliteConnectOptions::from_str(&format!("sqlite:{}", database_path.display()))?
                .create_if_missing(true)
                .journal_mode(SqliteJournalMode::Wal)
                .synchronous(SqliteSynchronous::Normal)
                .disable_statement_logging();

        let pool = SqlitePoolOptions::new()
            .max_connections(1)
            .connect_with(options)
            .await?;

        MIGRATOR.run(&pool).await?;

        let migration_version =
            sqlx::query("SELECT value FROM app_metadata WHERE key = 'schema_version' LIMIT 1")
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

    pub async fn list_libraries(
        &self,
        limit: Option<i64>,
    ) -> Result<Vec<RegisteredLibrary>, AppError> {
        let active_id = self.active_library_id().await?;
        let rows = if let Some(limit) = limit {
            sqlx::query(
                r#"
                SELECT id, name, path, created_at, updated_at, last_opened_at, is_pinned
                FROM libraries
                ORDER BY is_pinned DESC, COALESCE(last_opened_at, updated_at) DESC, updated_at DESC, name COLLATE NOCASE ASC
                LIMIT ?1
                "#,
            )
            .bind(limit)
            .fetch_all(&self.pool)
            .await?
        } else {
            sqlx::query(
                r#"
                SELECT id, name, path, created_at, updated_at, last_opened_at, is_pinned
                FROM libraries
                ORDER BY is_pinned DESC, COALESCE(last_opened_at, updated_at) DESC, updated_at DESC, name COLLATE NOCASE ASC
                "#,
            )
            .fetch_all(&self.pool)
            .await?
        };

        rows.into_iter()
            .map(|row| self.row_to_library(row, active_id.as_deref()))
            .collect()
    }

    pub async fn get_active_library(&self) -> Result<Option<RegisteredLibrary>, AppError> {
        let active_id = self.active_library_id().await?;
        let Some(active_id) = active_id else {
            return Ok(None);
        };

        let row = sqlx::query(
            r#"
            SELECT id, name, path, created_at, updated_at, last_opened_at, is_pinned
            FROM libraries
            WHERE id = ?1
            LIMIT 1
            "#,
        )
        .bind(&active_id)
        .fetch_optional(&self.pool)
        .await?;

        row.map(|record| self.row_to_library(record, Some(active_id.as_str())))
            .transpose()
    }

    pub async fn create_library_record(
        &self,
        path: String,
        name: Option<String>,
    ) -> Result<RegisteredLibrary, AppError> {
        self.upsert_library_record(path, name, true).await
    }

    pub async fn register_existing_library(
        &self,
        path: String,
        name: Option<String>,
    ) -> Result<RegisteredLibrary, AppError> {
        self.upsert_library_record(path, name, true).await
    }

    pub async fn set_active_library(&self, id: String) -> Result<RegisteredLibrary, AppError> {
        let now = now_text();
        let exists = sqlx::query_scalar::<_, i64>("SELECT COUNT(1) FROM libraries WHERE id = ?1")
            .bind(&id)
            .fetch_one(&self.pool)
            .await?;

        if exists == 0 {
            return Err(AppError::Message(format!("library not found: {id}")));
        }

        sqlx::query("UPDATE libraries SET last_opened_at = ?1, updated_at = ?1 WHERE id = ?2")
            .bind(&now)
            .bind(&id)
            .execute(&self.pool)
            .await?;

        self.store_active_library_id(Some(&id)).await?;

        self.get_active_library()
            .await?
            .ok_or_else(|| AppError::Message(format!("library not found after activation: {id}")))
    }

    pub async fn clear_active_library(&self) -> Result<(), AppError> {
        self.store_active_library_id(None).await
    }

    pub async fn update_library_record(
        &self,
        id: String,
        path: String,
        name: Option<String>,
    ) -> Result<RegisteredLibrary, AppError> {
        let normalized_path = normalize_library_path(&path)?;
        let resolved_name = resolve_library_name(name.as_deref(), &normalized_path)?;
        let now = now_text();

        let exists = sqlx::query_scalar::<_, i64>("SELECT COUNT(1) FROM libraries WHERE id = ?1")
            .bind(&id)
            .fetch_one(&self.pool)
            .await?;

        if exists == 0 {
            return Err(AppError::Message(format!("library not found: {id}")));
        }

        sqlx::query(
            r#"
            UPDATE libraries
            SET name = ?1, path = ?2, updated_at = ?3
            WHERE id = ?4
            "#,
        )
        .bind(&resolved_name)
        .bind(&normalized_path)
        .bind(&now)
        .bind(&id)
        .execute(&self.pool)
        .await?;

        let active_id = self.active_library_id().await?;
        let row = sqlx::query(
            r#"
            SELECT id, name, path, created_at, updated_at, last_opened_at, is_pinned
            FROM libraries
            WHERE id = ?1
            LIMIT 1
            "#,
        )
        .bind(&id)
        .fetch_one(&self.pool)
        .await?;

        self.row_to_library(row, active_id.as_deref())
    }

    pub async fn set_library_pinned(
        &self,
        id: String,
        pinned: bool,
    ) -> Result<RegisteredLibrary, AppError> {
        let exists = sqlx::query_scalar::<_, i64>("SELECT COUNT(1) FROM libraries WHERE id = ?1")
            .bind(&id)
            .fetch_one(&self.pool)
            .await?;

        if exists == 0 {
            return Err(AppError::Message(format!("library not found: {id}")));
        }

        sqlx::query(
            r#"
            UPDATE libraries
            SET is_pinned = ?1
            WHERE id = ?2
            "#,
        )
        .bind(if pinned { 1_i64 } else { 0_i64 })
        .bind(&id)
        .execute(&self.pool)
        .await?;

        let active_id = self.active_library_id().await?;
        let row = sqlx::query(
            r#"
            SELECT id, name, path, created_at, updated_at, last_opened_at, is_pinned
            FROM libraries
            WHERE id = ?1
            LIMIT 1
            "#,
        )
        .bind(&id)
        .fetch_one(&self.pool)
        .await?;

        self.row_to_library(row, active_id.as_deref())
    }

    pub async fn delete_library_record(&self, id: String) -> Result<(), AppError> {
        let active_id = self.active_library_id().await?;

        let result = sqlx::query("DELETE FROM libraries WHERE id = ?1")
            .bind(&id)
            .execute(&self.pool)
            .await?;

        if result.rows_affected() == 0 {
            return Err(AppError::Message(format!("library not found: {id}")));
        }

        if active_id.as_deref() == Some(id.as_str()) {
            self.store_active_library_id(None).await?;
        }

        Ok(())
    }

    async fn upsert_library_record(
        &self,
        path: String,
        name: Option<String>,
        activate: bool,
    ) -> Result<RegisteredLibrary, AppError> {
        let normalized_path = normalize_library_path(&path)?;
        let resolved_name = resolve_library_name(name.as_deref(), &normalized_path)?;
        let now = now_text();

        let existing = sqlx::query(
            r#"
            SELECT id, name, path, created_at, updated_at, last_opened_at, is_pinned
            FROM libraries
            WHERE path = ?1
            LIMIT 1
            "#,
        )
        .bind(&normalized_path)
        .fetch_optional(&self.pool)
        .await?;

        let library_id = if let Some(existing) = existing {
            let id = existing.try_get::<String, _>("id")?;
            sqlx::query(
                r#"
                UPDATE libraries
                SET name = ?1, updated_at = ?2, last_opened_at = ?2
                WHERE id = ?3
                "#,
            )
            .bind(&resolved_name)
            .bind(&now)
            .bind(&id)
            .execute(&self.pool)
            .await?;
            id
        } else {
            let id = uuid::Uuid::new_v4().to_string();
            sqlx::query(
                r#"
                INSERT INTO libraries (id, name, path, created_at, updated_at, last_opened_at, is_pinned)
                VALUES (?1, ?2, ?3, ?4, ?4, ?4, 0)
                "#,
            )
            .bind(&id)
            .bind(&resolved_name)
            .bind(&normalized_path)
            .bind(&now)
            .execute(&self.pool)
            .await?;
            id
        };

        if activate {
            self.store_active_library_id(Some(&library_id)).await?;
        }

        let active_id = self.active_library_id().await?;
        let row = sqlx::query(
            r#"
            SELECT id, name, path, created_at, updated_at, last_opened_at, is_pinned
            FROM libraries
            WHERE id = ?1
            LIMIT 1
            "#,
        )
        .bind(&library_id)
        .fetch_one(&self.pool)
        .await?;

        self.row_to_library(row, active_id.as_deref())
    }

    async fn active_library_id(&self) -> Result<Option<String>, AppError> {
        let value =
            sqlx::query("SELECT value FROM app_metadata WHERE key = 'active_library_id' LIMIT 1")
                .fetch_optional(&self.pool)
                .await?;

        Ok(value.and_then(|row| row.try_get::<String, _>("value").ok()))
    }

    async fn store_active_library_id(&self, id: Option<&str>) -> Result<(), AppError> {
        match id {
            Some(id) => {
                sqlx::query(
                    r#"
                    INSERT INTO app_metadata (key, value, updated_at)
                    VALUES ('active_library_id', ?1, CURRENT_TIMESTAMP)
                    ON CONFLICT(key) DO UPDATE SET
                        value = excluded.value,
                        updated_at = CURRENT_TIMESTAMP
                    "#,
                )
                .bind(id)
                .execute(&self.pool)
                .await?;
            }
            None => {
                sqlx::query("DELETE FROM app_metadata WHERE key = 'active_library_id'")
                    .execute(&self.pool)
                    .await?;
            }
        }

        Ok(())
    }

    fn row_to_library(
        &self,
        row: sqlx::sqlite::SqliteRow,
        active_id: Option<&str>,
    ) -> Result<RegisteredLibrary, AppError> {
        let id = row.try_get::<String, _>("id")?;
        Ok(RegisteredLibrary {
            is_active: active_id == Some(id.as_str()),
            id,
            name: row.try_get("name")?,
            path: row.try_get("path")?,
            created_at: row.try_get("created_at")?,
            updated_at: row.try_get("updated_at")?,
            last_opened_at: row.try_get("last_opened_at")?,
            is_pinned: row.try_get::<i64, _>("is_pinned")? != 0,
        })
    }
}

fn now_text() -> String {
    Utc::now().to_rfc3339()
}

fn normalize_library_path(path: &str) -> Result<String, AppError> {
    let trimmed = path.trim();
    if trimmed.is_empty() {
        return Err(AppError::Message("library path is required".to_string()));
    }

    let normalized = PathBuf::from(trimmed);
    let as_string = normalized.to_string_lossy().trim().to_string();
    if as_string.is_empty() {
        return Err(AppError::Message("library path is required".to_string()));
    }

    Ok(as_string)
}

fn resolve_library_name(name: Option<&str>, path: &str) -> Result<String, AppError> {
    if let Some(name) = name {
        let trimmed = name.trim();
        if !trimmed.is_empty() {
            return Ok(trimmed.to_string());
        }
    }

    let library_path = Path::new(path);
    if let Some(stem) = library_path.file_stem().and_then(|value| value.to_str()) {
        let trimmed = stem.trim();
        if !trimmed.is_empty() {
            return Ok(trimmed.to_string());
        }
    }

    Err(AppError::Message("library name is required".to_string()))
}
