use std::{str::FromStr, sync::Arc};

use chrono::Utc;
use serde::{Deserialize, Serialize};
use sqlx::{Row, SqlitePool};

use crate::{
    error::AppError,
    upload::{
        ensure_status_transition, UploadErrorCode, UploadJobStatus, UploadPartStatus,
        UploadRetryPolicy,
    },
};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct NewUploadJob {
    pub id: String,
    pub provider_key: String,
    pub endpoint_id: Option<String>,
    pub local_path: String,
    pub remote_path: String,
    pub file_size: u64,
    pub part_size: u64,
    pub total_parts: u32,
    pub content_hash: Option<String>,
    pub metadata_json: Option<String>,
    pub retry_policy: UploadRetryPolicy,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct UploadJobRecord {
    pub id: String,
    pub provider_key: String,
    pub endpoint_id: Option<String>,
    pub local_path: String,
    pub remote_path: String,
    pub file_size: i64,
    pub part_size: i64,
    pub total_parts: i64,
    pub uploaded_bytes: i64,
    pub uploaded_parts: i64,
    pub status: UploadJobStatus,
    pub retry_count: i64,
    pub max_retries: i64,
    pub next_retry_at: Option<String>,
    pub last_error_code: Option<UploadErrorCode>,
    pub last_error_message: Option<String>,
    pub session_id: Option<String>,
    pub content_hash: Option<String>,
    pub metadata_json: String,
    pub created_at: String,
    pub updated_at: String,
    pub started_at: Option<String>,
    pub finished_at: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct UploadErrorSnapshot {
    pub code: UploadErrorCode,
    pub message: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct NewUploadSession {
    pub job_id: String,
    pub provider_key: String,
    pub provider_upload_id: String,
    pub remote_path: String,
    pub part_size: u64,
    pub total_parts: u32,
    pub extra_json: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct UploadSessionRecord {
    pub job_id: String,
    pub provider_key: String,
    pub provider_upload_id: String,
    pub remote_path: String,
    pub part_size: i64,
    pub total_parts: i64,
    pub extra_json: String,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct UpsertUploadPartCheckpoint {
    pub job_id: String,
    pub part_number: u32,
    pub start_offset: u64,
    pub end_offset: u64,
    pub part_size: u64,
    pub checksum: Option<String>,
    pub etag: Option<String>,
    pub status: UploadPartStatus,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct UploadPartCheckpoint {
    pub job_id: String,
    pub part_number: i64,
    pub start_offset: i64,
    pub end_offset: i64,
    pub part_size: i64,
    pub checksum: Option<String>,
    pub etag: Option<String>,
    pub status: UploadPartStatus,
    pub updated_at: String,
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum UploadAttemptStatus {
    Started,
    Succeeded,
    Failed,
    Canceled,
}

impl UploadAttemptStatus {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Started => "started",
            Self::Succeeded => "succeeded",
            Self::Failed => "failed",
            Self::Canceled => "canceled",
        }
    }
}

impl FromStr for UploadAttemptStatus {
    type Err = String;

    fn from_str(value: &str) -> Result<Self, Self::Err> {
        match value.trim().to_lowercase().as_str() {
            "started" => Ok(Self::Started),
            "succeeded" => Ok(Self::Succeeded),
            "failed" => Ok(Self::Failed),
            "canceled" => Ok(Self::Canceled),
            other => Err(format!("invalid upload attempt status: {other}")),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct UploadAttemptRecord {
    pub id: i64,
    pub job_id: String,
    pub attempt_no: i64,
    pub status: UploadAttemptStatus,
    pub error_code: Option<UploadErrorCode>,
    pub error_message: Option<String>,
    pub started_at: String,
    pub finished_at: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct UploadResumeSnapshot {
    pub job: UploadJobRecord,
    pub session: Option<UploadSessionRecord>,
    pub parts: Vec<UploadPartCheckpoint>,
}

#[derive(Clone)]
pub struct UploadStore {
    pool: Arc<SqlitePool>,
}

impl UploadStore {
    pub fn new(pool: &SqlitePool) -> Self {
        Self {
            pool: Arc::new(pool.clone()),
        }
    }

    pub async fn create_job(&self, payload: NewUploadJob) -> Result<UploadJobRecord, AppError> {
        let id = payload.id.trim();
        if id.is_empty() {
            return Err(AppError::Message("upload job id is required".to_string()));
        }
        if payload.provider_key.trim().is_empty() {
            return Err(AppError::Message(
                "upload provider key is required".to_string(),
            ));
        }
        if payload.local_path.trim().is_empty() {
            return Err(AppError::Message(
                "upload local path is required".to_string(),
            ));
        }
        if payload.remote_path.trim().is_empty() {
            return Err(AppError::Message(
                "upload remote path is required".to_string(),
            ));
        }
        if payload.part_size == 0 {
            return Err(AppError::Message(
                "upload part size must be > 0".to_string(),
            ));
        }

        let now = now_text();
        let retry_policy = payload.retry_policy.bounded();
        sqlx::query(
            r#"
            INSERT INTO upload_jobs (
                id,
                provider_key,
                endpoint_id,
                local_path,
                remote_path,
                file_size,
                part_size,
                total_parts,
                uploaded_bytes,
                uploaded_parts,
                status,
                retry_count,
                max_retries,
                next_retry_at,
                last_error_code,
                last_error_message,
                session_id,
                content_hash,
                metadata_json,
                created_at,
                updated_at
            )
            VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, 0, 0, ?9, 0, ?10, NULL, NULL, NULL, NULL, ?11, ?12, ?13, ?13)
            "#,
        )
        .bind(id)
        .bind(payload.provider_key.trim())
        .bind(payload.endpoint_id.as_deref())
        .bind(payload.local_path.trim())
        .bind(payload.remote_path.trim())
        .bind(u64_to_i64(payload.file_size, "upload file size")?)
        .bind(u64_to_i64(payload.part_size, "upload part size")?)
        .bind(i64::from(payload.total_parts))
        .bind(UploadJobStatus::Queued.as_str())
        .bind(i64::from(retry_policy.max_retries))
        .bind(payload.content_hash.as_deref())
        .bind(payload.metadata_json.unwrap_or_else(|| "{}".to_string()))
        .bind(now)
        .execute(self.pool.as_ref())
        .await?;

        self.get_job(id)
            .await?
            .ok_or_else(|| AppError::Message(format!("upload job inserted but not found: {id}")))
    }

    pub async fn get_job(&self, job_id: &str) -> Result<Option<UploadJobRecord>, AppError> {
        let row = sqlx::query(
            r#"
            SELECT
                id,
                provider_key,
                endpoint_id,
                local_path,
                remote_path,
                file_size,
                part_size,
                total_parts,
                uploaded_bytes,
                uploaded_parts,
                status,
                retry_count,
                max_retries,
                next_retry_at,
                last_error_code,
                last_error_message,
                session_id,
                content_hash,
                metadata_json,
                created_at,
                updated_at,
                started_at,
                finished_at
            FROM upload_jobs
            WHERE id = ?1
            LIMIT 1
            "#,
        )
        .bind(job_id)
        .fetch_optional(self.pool.as_ref())
        .await?;

        row.map(map_upload_job_row).transpose()
    }

    pub async fn transition_job_status(
        &self,
        job_id: &str,
        next_status: UploadJobStatus,
        error: Option<UploadErrorSnapshot>,
        next_retry_at: Option<&str>,
    ) -> Result<UploadJobRecord, AppError> {
        let current = self
            .get_job(job_id)
            .await?
            .ok_or_else(|| AppError::Message(format!("upload job not found: {job_id}")))?;
        ensure_status_transition(current.status, next_status)
            .map_err(|err| AppError::Message(err.to_string()))?;

        let now = now_text();
        let retry_count = if matches!(next_status, UploadJobStatus::Retrying) {
            current.retry_count + 1
        } else {
            current.retry_count
        };

        let mut started_at = current.started_at.clone();
        if matches!(next_status, UploadJobStatus::Uploading) && started_at.is_none() {
            started_at = Some(now.clone());
        }

        let finished_at = if next_status.is_terminal() {
            Some(now.clone())
        } else {
            None
        };

        let (last_error_code, last_error_message) = if let Some(value) = error {
            (Some(value.code.to_string()), Some(value.message))
        } else if matches!(
            next_status,
            UploadJobStatus::Queued
                | UploadJobStatus::Preparing
                | UploadJobStatus::Uploading
                | UploadJobStatus::Succeeded
        ) {
            (None, None)
        } else {
            (
                current.last_error_code.map(|code| code.to_string()),
                current.last_error_message.clone(),
            )
        };

        sqlx::query(
            r#"
            UPDATE upload_jobs
            SET
                status = ?1,
                retry_count = ?2,
                next_retry_at = ?3,
                last_error_code = ?4,
                last_error_message = ?5,
                started_at = ?6,
                finished_at = ?7,
                updated_at = ?8
            WHERE id = ?9
            "#,
        )
        .bind(next_status.as_str())
        .bind(retry_count)
        .bind(next_retry_at)
        .bind(last_error_code)
        .bind(last_error_message)
        .bind(started_at)
        .bind(finished_at)
        .bind(now)
        .bind(job_id)
        .execute(self.pool.as_ref())
        .await?;

        self.get_job(job_id).await?.ok_or_else(|| {
            AppError::Message(format!("upload job transition lost record: {job_id}"))
        })
    }

    pub async fn upsert_session(
        &self,
        payload: NewUploadSession,
    ) -> Result<UploadSessionRecord, AppError> {
        if payload.job_id.trim().is_empty() {
            return Err(AppError::Message(
                "upload session job id is required".to_string(),
            ));
        }
        if payload.provider_key.trim().is_empty() {
            return Err(AppError::Message(
                "upload session provider key is required".to_string(),
            ));
        }
        if payload.provider_upload_id.trim().is_empty() {
            return Err(AppError::Message(
                "upload provider upload id is required".to_string(),
            ));
        }

        let now = now_text();
        sqlx::query(
            r#"
            INSERT INTO upload_sessions (
                job_id,
                provider_key,
                provider_upload_id,
                remote_path,
                part_size,
                total_parts,
                extra_json,
                created_at,
                updated_at
            )
            VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?8)
            ON CONFLICT(job_id) DO UPDATE SET
                provider_key = excluded.provider_key,
                provider_upload_id = excluded.provider_upload_id,
                remote_path = excluded.remote_path,
                part_size = excluded.part_size,
                total_parts = excluded.total_parts,
                extra_json = excluded.extra_json,
                updated_at = excluded.updated_at
            "#,
        )
        .bind(payload.job_id.trim())
        .bind(payload.provider_key.trim())
        .bind(payload.provider_upload_id.trim())
        .bind(payload.remote_path.trim())
        .bind(u64_to_i64(payload.part_size, "upload session part size")?)
        .bind(i64::from(payload.total_parts))
        .bind(payload.extra_json.unwrap_or_else(|| "{}".to_string()))
        .bind(now)
        .execute(self.pool.as_ref())
        .await?;

        sqlx::query(
            r#"
            UPDATE upload_jobs
            SET session_id = ?1, updated_at = ?2
            WHERE id = ?3
            "#,
        )
        .bind(payload.provider_upload_id.trim())
        .bind(now_text())
        .bind(payload.job_id.trim())
        .execute(self.pool.as_ref())
        .await?;

        self.get_session(payload.job_id.trim())
            .await?
            .ok_or_else(|| {
                AppError::Message(format!(
                    "upload session upserted but not found: {}",
                    payload.job_id
                ))
            })
    }

    pub async fn get_session(&self, job_id: &str) -> Result<Option<UploadSessionRecord>, AppError> {
        let row = sqlx::query(
            r#"
            SELECT
                job_id,
                provider_key,
                provider_upload_id,
                remote_path,
                part_size,
                total_parts,
                extra_json,
                created_at,
                updated_at
            FROM upload_sessions
            WHERE job_id = ?1
            LIMIT 1
            "#,
        )
        .bind(job_id)
        .fetch_optional(self.pool.as_ref())
        .await?;

        row.map(map_upload_session_row).transpose()
    }

    pub async fn upsert_part_checkpoint(
        &self,
        payload: UpsertUploadPartCheckpoint,
    ) -> Result<UploadPartCheckpoint, AppError> {
        if payload.job_id.trim().is_empty() {
            return Err(AppError::Message(
                "upload part checkpoint job id is required".to_string(),
            ));
        }
        if payload.part_number == 0 {
            return Err(AppError::Message(
                "upload part number must start from 1".to_string(),
            ));
        }
        if payload.end_offset < payload.start_offset {
            return Err(AppError::Message(
                "upload part end offset must be >= start offset".to_string(),
            ));
        }

        let now = now_text();
        sqlx::query(
            r#"
            INSERT INTO upload_parts (
                job_id,
                part_number,
                start_offset,
                end_offset,
                part_size,
                checksum,
                etag,
                status,
                updated_at
            )
            VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
            ON CONFLICT(job_id, part_number) DO UPDATE SET
                start_offset = excluded.start_offset,
                end_offset = excluded.end_offset,
                part_size = excluded.part_size,
                checksum = excluded.checksum,
                etag = excluded.etag,
                status = excluded.status,
                updated_at = excluded.updated_at
            "#,
        )
        .bind(payload.job_id.trim())
        .bind(i64::from(payload.part_number))
        .bind(u64_to_i64(
            payload.start_offset,
            "upload part start offset",
        )?)
        .bind(u64_to_i64(payload.end_offset, "upload part end offset")?)
        .bind(u64_to_i64(payload.part_size, "upload part size")?)
        .bind(payload.checksum.as_deref())
        .bind(payload.etag.as_deref())
        .bind(payload.status.as_str())
        .bind(now)
        .execute(self.pool.as_ref())
        .await?;

        self.recalculate_job_progress(payload.job_id.trim()).await?;

        self.get_part_checkpoint(payload.job_id.trim(), payload.part_number)
            .await?
            .ok_or_else(|| {
                AppError::Message(format!(
                    "upload part checkpoint upserted but not found: {}#{}",
                    payload.job_id, payload.part_number
                ))
            })
    }

    pub async fn list_part_checkpoints(
        &self,
        job_id: &str,
    ) -> Result<Vec<UploadPartCheckpoint>, AppError> {
        let rows = sqlx::query(
            r#"
            SELECT
                job_id,
                part_number,
                start_offset,
                end_offset,
                part_size,
                checksum,
                etag,
                status,
                updated_at
            FROM upload_parts
            WHERE job_id = ?1
            ORDER BY part_number ASC
            "#,
        )
        .bind(job_id)
        .fetch_all(self.pool.as_ref())
        .await?;

        rows.into_iter().map(map_upload_part_row).collect()
    }

    pub async fn record_attempt_started(
        &self,
        job_id: &str,
        attempt_no: u32,
    ) -> Result<UploadAttemptRecord, AppError> {
        if job_id.trim().is_empty() {
            return Err(AppError::Message(
                "upload attempt job id is required".to_string(),
            ));
        }
        if attempt_no == 0 {
            return Err(AppError::Message(
                "upload attempt number must start from 1".to_string(),
            ));
        }

        let now = now_text();
        sqlx::query(
            r#"
            INSERT INTO upload_attempts (
                job_id,
                attempt_no,
                status,
                error_code,
                error_message,
                started_at,
                finished_at
            )
            VALUES (?1, ?2, ?3, NULL, NULL, ?4, NULL)
            ON CONFLICT(job_id, attempt_no) DO UPDATE SET
                status = excluded.status,
                error_code = NULL,
                error_message = NULL,
                started_at = excluded.started_at,
                finished_at = NULL
            "#,
        )
        .bind(job_id.trim())
        .bind(i64::from(attempt_no))
        .bind(UploadAttemptStatus::Started.as_str())
        .bind(now)
        .execute(self.pool.as_ref())
        .await?;

        self.get_attempt(job_id.trim(), attempt_no)
            .await?
            .ok_or_else(|| {
                AppError::Message(format!(
                    "upload attempt created but not found: {job_id}#{attempt_no}"
                ))
            })
    }

    pub async fn record_attempt_finished(
        &self,
        job_id: &str,
        attempt_no: u32,
        status: UploadAttemptStatus,
        error: Option<UploadErrorSnapshot>,
    ) -> Result<UploadAttemptRecord, AppError> {
        if !matches!(
            status,
            UploadAttemptStatus::Succeeded
                | UploadAttemptStatus::Failed
                | UploadAttemptStatus::Canceled
        ) {
            return Err(AppError::Message(
                "upload finished attempt status must be succeeded/failed/canceled".to_string(),
            ));
        }

        let now = now_text();
        let error_code = error.as_ref().map(|value| value.code.as_str().to_string());
        let error_message = error.as_ref().map(|value| value.message.trim().to_string());
        sqlx::query(
            r#"
            UPDATE upload_attempts
            SET
                status = ?1,
                error_code = ?2,
                error_message = ?3,
                finished_at = ?4
            WHERE job_id = ?5 AND attempt_no = ?6
            "#,
        )
        .bind(status.as_str())
        .bind(error_code)
        .bind(error_message)
        .bind(now)
        .bind(job_id.trim())
        .bind(i64::from(attempt_no))
        .execute(self.pool.as_ref())
        .await?;

        self.get_attempt(job_id.trim(), attempt_no)
            .await?
            .ok_or_else(|| {
                AppError::Message(format!(
                    "upload attempt not found while finishing: {job_id}#{attempt_no}"
                ))
            })
    }

    pub async fn load_resume_snapshot(
        &self,
        job_id: &str,
    ) -> Result<Option<UploadResumeSnapshot>, AppError> {
        let Some(job) = self.get_job(job_id).await? else {
            return Ok(None);
        };
        let session = self.get_session(job_id).await?;
        let parts = self.list_part_checkpoints(job_id).await?;

        Ok(Some(UploadResumeSnapshot {
            job,
            session,
            parts,
        }))
    }

    pub async fn recover_interrupted_jobs(
        &self,
        recovery_reason: &str,
    ) -> Result<Vec<UploadJobRecord>, AppError> {
        let rows = sqlx::query(
            r#"
            SELECT id
            FROM upload_jobs
            WHERE status IN ('preparing', 'uploading', 'retrying', 'recovering')
            ORDER BY created_at ASC
            "#,
        )
        .fetch_all(self.pool.as_ref())
        .await?;

        let mut recovered = Vec::with_capacity(rows.len());
        let reason = recovery_reason.trim();
        let reason_text = if reason.is_empty() {
            "任务在应用异常退出后进入恢复队列".to_string()
        } else {
            format!("任务在应用异常退出后进入恢复队列：{reason}")
        };

        for row in rows {
            let job_id = row.try_get::<String, _>("id")?;
            let _ = self
                .transition_job_status(&job_id, UploadJobStatus::Recovering, None, None)
                .await?;

            let requeued = self
                .transition_job_status(
                    &job_id,
                    UploadJobStatus::Queued,
                    Some(UploadErrorSnapshot {
                        code: UploadErrorCode::ProcessInterrupted,
                        message: reason_text.clone(),
                    }),
                    None,
                )
                .await?;

            recovered.push(requeued);
        }

        Ok(recovered)
    }

    async fn recalculate_job_progress(&self, job_id: &str) -> Result<(), AppError> {
        let row = sqlx::query(
            r#"
            SELECT
                COALESCE(SUM(CASE WHEN status = 'uploaded' THEN part_size ELSE 0 END), 0) AS uploaded_bytes,
                COALESCE(SUM(CASE WHEN status = 'uploaded' THEN 1 ELSE 0 END), 0) AS uploaded_parts
            FROM upload_parts
            WHERE job_id = ?1
            "#,
        )
        .bind(job_id)
        .fetch_one(self.pool.as_ref())
        .await?;

        let uploaded_bytes = row.try_get::<i64, _>("uploaded_bytes")?;
        let uploaded_parts = row.try_get::<i64, _>("uploaded_parts")?;
        sqlx::query(
            r#"
            UPDATE upload_jobs
            SET uploaded_bytes = ?1, uploaded_parts = ?2, updated_at = ?3
            WHERE id = ?4
            "#,
        )
        .bind(uploaded_bytes.max(0))
        .bind(uploaded_parts.max(0))
        .bind(now_text())
        .bind(job_id)
        .execute(self.pool.as_ref())
        .await?;

        Ok(())
    }

    async fn get_part_checkpoint(
        &self,
        job_id: &str,
        part_number: u32,
    ) -> Result<Option<UploadPartCheckpoint>, AppError> {
        let row = sqlx::query(
            r#"
            SELECT
                job_id,
                part_number,
                start_offset,
                end_offset,
                part_size,
                checksum,
                etag,
                status,
                updated_at
            FROM upload_parts
            WHERE job_id = ?1 AND part_number = ?2
            LIMIT 1
            "#,
        )
        .bind(job_id)
        .bind(i64::from(part_number))
        .fetch_optional(self.pool.as_ref())
        .await?;

        row.map(map_upload_part_row).transpose()
    }

    async fn get_attempt(
        &self,
        job_id: &str,
        attempt_no: u32,
    ) -> Result<Option<UploadAttemptRecord>, AppError> {
        let row = sqlx::query(
            r#"
            SELECT id, job_id, attempt_no, status, error_code, error_message, started_at, finished_at
            FROM upload_attempts
            WHERE job_id = ?1 AND attempt_no = ?2
            LIMIT 1
            "#,
        )
        .bind(job_id)
        .bind(i64::from(attempt_no))
        .fetch_optional(self.pool.as_ref())
        .await?;

        row.map(map_upload_attempt_row).transpose()
    }
}

fn map_upload_job_row(row: sqlx::sqlite::SqliteRow) -> Result<UploadJobRecord, AppError> {
    let status_text = row.try_get::<String, _>("status")?;
    let status = UploadJobStatus::from_str(&status_text).map_err(|_| {
        AppError::Message(format!("invalid upload status in database: {status_text}"))
    })?;

    let last_error_code = row
        .try_get::<Option<String>, _>("last_error_code")?
        .map(|code| UploadErrorCode::parse_lossy(&code));

    Ok(UploadJobRecord {
        id: row.try_get("id")?,
        provider_key: row.try_get("provider_key")?,
        endpoint_id: row.try_get("endpoint_id")?,
        local_path: row.try_get("local_path")?,
        remote_path: row.try_get("remote_path")?,
        file_size: row.try_get("file_size")?,
        part_size: row.try_get("part_size")?,
        total_parts: row.try_get("total_parts")?,
        uploaded_bytes: row.try_get("uploaded_bytes")?,
        uploaded_parts: row.try_get("uploaded_parts")?,
        status,
        retry_count: row.try_get("retry_count")?,
        max_retries: row.try_get("max_retries")?,
        next_retry_at: row.try_get("next_retry_at")?,
        last_error_code,
        last_error_message: row.try_get("last_error_message")?,
        session_id: row.try_get("session_id")?,
        content_hash: row.try_get("content_hash")?,
        metadata_json: row.try_get("metadata_json")?,
        created_at: row.try_get("created_at")?,
        updated_at: row.try_get("updated_at")?,
        started_at: row.try_get("started_at")?,
        finished_at: row.try_get("finished_at")?,
    })
}

fn map_upload_session_row(row: sqlx::sqlite::SqliteRow) -> Result<UploadSessionRecord, AppError> {
    Ok(UploadSessionRecord {
        job_id: row.try_get("job_id")?,
        provider_key: row.try_get("provider_key")?,
        provider_upload_id: row.try_get("provider_upload_id")?,
        remote_path: row.try_get("remote_path")?,
        part_size: row.try_get("part_size")?,
        total_parts: row.try_get("total_parts")?,
        extra_json: row.try_get("extra_json")?,
        created_at: row.try_get("created_at")?,
        updated_at: row.try_get("updated_at")?,
    })
}

fn map_upload_part_row(row: sqlx::sqlite::SqliteRow) -> Result<UploadPartCheckpoint, AppError> {
    let status_text = row.try_get::<String, _>("status")?;
    let status = UploadPartStatus::from_str(&status_text).map_err(|_| {
        AppError::Message(format!(
            "invalid upload part status in database: {status_text}"
        ))
    })?;

    Ok(UploadPartCheckpoint {
        job_id: row.try_get("job_id")?,
        part_number: row.try_get("part_number")?,
        start_offset: row.try_get("start_offset")?,
        end_offset: row.try_get("end_offset")?,
        part_size: row.try_get("part_size")?,
        checksum: row.try_get("checksum")?,
        etag: row.try_get("etag")?,
        status,
        updated_at: row.try_get("updated_at")?,
    })
}

fn map_upload_attempt_row(row: sqlx::sqlite::SqliteRow) -> Result<UploadAttemptRecord, AppError> {
    let status_text = row.try_get::<String, _>("status")?;
    let status = UploadAttemptStatus::from_str(&status_text).map_err(|_| {
        AppError::Message(format!(
            "invalid upload attempt status in database: {status_text}"
        ))
    })?;

    let error_code = row
        .try_get::<Option<String>, _>("error_code")?
        .map(|code| UploadErrorCode::parse_lossy(&code));

    Ok(UploadAttemptRecord {
        id: row.try_get("id")?,
        job_id: row.try_get("job_id")?,
        attempt_no: row.try_get("attempt_no")?,
        status,
        error_code,
        error_message: row.try_get("error_message")?,
        started_at: row.try_get("started_at")?,
        finished_at: row.try_get("finished_at")?,
    })
}

fn u64_to_i64(value: u64, field_name: &str) -> Result<i64, AppError> {
    i64::try_from(value).map_err(|_| AppError::Message(format!("{field_name} exceeds i64 range")))
}

fn now_text() -> String {
    Utc::now().to_rfc3339()
}

#[cfg(test)]
mod tests {
    use std::{path::PathBuf, str::FromStr};

    use sqlx::{
        migrate::Migrator,
        sqlite::{SqliteConnectOptions, SqlitePoolOptions},
        ConnectOptions, SqlitePool,
    };
    use uuid::Uuid;

    use super::{
        NewUploadJob, NewUploadSession, UploadAttemptStatus, UploadErrorSnapshot, UploadStore,
        UpsertUploadPartCheckpoint,
    };
    use crate::{
        error::AppError,
        upload::{UploadErrorCode, UploadJobStatus, UploadPartStatus, UploadRetryPolicy},
    };

    static TEST_MIGRATOR: Migrator = sqlx::migrate!("./migrations");

    #[tokio::test]
    async fn persists_resume_snapshot_across_updates() {
        let (store, pool) = create_test_store().await;

        let job_id = format!("job-{}", Uuid::new_v4());
        let _ = store
            .create_job(NewUploadJob {
                id: job_id.clone(),
                provider_key: "p115".to_string(),
                endpoint_id: Some("endpoint-1".to_string()),
                local_path: "D:/素材/视频01.mp4".to_string(),
                remote_path: "/视频/视频01.mp4".to_string(),
                file_size: 1024,
                part_size: 256,
                total_parts: 4,
                content_hash: None,
                metadata_json: None,
                retry_policy: UploadRetryPolicy::default(),
            })
            .await
            .expect("create upload job should succeed");

        let _ = store
            .transition_job_status(&job_id, UploadJobStatus::Preparing, None, None)
            .await
            .expect("queued -> preparing should succeed");
        let _ = store
            .transition_job_status(&job_id, UploadJobStatus::Uploading, None, None)
            .await
            .expect("preparing -> uploading should succeed");

        let session = store
            .upsert_session(NewUploadSession {
                job_id: job_id.clone(),
                provider_key: "p115".to_string(),
                provider_upload_id: "upload-115-session-1".to_string(),
                remote_path: "/视频/视频01.mp4".to_string(),
                part_size: 256,
                total_parts: 4,
                extra_json: Some("{\"bucket\":\"main\"}".to_string()),
            })
            .await
            .expect("session upsert should succeed");
        assert_eq!(session.provider_upload_id, "upload-115-session-1");

        let _ = store
            .upsert_part_checkpoint(UpsertUploadPartCheckpoint {
                job_id: job_id.clone(),
                part_number: 1,
                start_offset: 0,
                end_offset: 255,
                part_size: 256,
                checksum: None,
                etag: Some("etag-1".to_string()),
                status: UploadPartStatus::Uploaded,
            })
            .await
            .expect("part 1 checkpoint should succeed");

        let _ = store
            .upsert_part_checkpoint(UpsertUploadPartCheckpoint {
                job_id: job_id.clone(),
                part_number: 2,
                start_offset: 256,
                end_offset: 511,
                part_size: 256,
                checksum: None,
                etag: None,
                status: UploadPartStatus::Pending,
            })
            .await
            .expect("part 2 checkpoint should succeed");

        let snapshot = store
            .load_resume_snapshot(&job_id)
            .await
            .expect("load resume snapshot should succeed")
            .expect("snapshot should exist");

        assert_eq!(snapshot.job.status, UploadJobStatus::Uploading);
        assert_eq!(snapshot.job.uploaded_bytes, 256);
        assert_eq!(snapshot.job.uploaded_parts, 1);
        assert_eq!(snapshot.parts.len(), 2);
        assert_eq!(
            snapshot
                .session
                .as_ref()
                .map(|value| value.provider_key.as_str()),
            Some("p115")
        );

        pool.close().await;
    }

    #[tokio::test]
    async fn recover_interrupted_jobs_requeues_only_inflight_jobs() {
        let (store, pool) = create_test_store().await;

        let job_uploading = create_base_job(&store, "/资料/视频A.mp4")
            .await
            .expect("create uploading job");
        let job_paused = create_base_job(&store, "/资料/视频B.mp4")
            .await
            .expect("create paused job");
        let job_retrying = create_base_job(&store, "/资料/视频C.mp4")
            .await
            .expect("create retrying job");

        let _ = store
            .transition_job_status(&job_uploading, UploadJobStatus::Preparing, None, None)
            .await
            .expect("queued -> preparing for uploading job");
        let _ = store
            .transition_job_status(&job_uploading, UploadJobStatus::Uploading, None, None)
            .await
            .expect("preparing -> uploading for uploading job");

        let _ = store
            .transition_job_status(&job_paused, UploadJobStatus::Paused, None, None)
            .await
            .expect("queued -> paused for paused job");

        let _ = store
            .transition_job_status(&job_retrying, UploadJobStatus::Preparing, None, None)
            .await
            .expect("queued -> preparing for retrying job");
        let _ = store
            .transition_job_status(&job_retrying, UploadJobStatus::Retrying, None, None)
            .await
            .expect("preparing -> retrying for retrying job");

        let recovered = store
            .recover_interrupted_jobs("测试恢复")
            .await
            .expect("recover interrupted jobs should succeed");
        assert_eq!(recovered.len(), 2);
        assert!(recovered.iter().any(|job| job.id == job_uploading));
        assert!(recovered.iter().any(|job| job.id == job_retrying));
        assert!(!recovered.iter().any(|job| job.id == job_paused));

        let uploading_after = store
            .get_job(&job_uploading)
            .await
            .expect("query uploading job")
            .expect("uploading job should exist");
        let paused_after = store
            .get_job(&job_paused)
            .await
            .expect("query paused job")
            .expect("paused job should exist");
        let retrying_after = store
            .get_job(&job_retrying)
            .await
            .expect("query retrying job")
            .expect("retrying job should exist");

        assert_eq!(uploading_after.status, UploadJobStatus::Queued);
        assert_eq!(retrying_after.status, UploadJobStatus::Queued);
        assert_eq!(paused_after.status, UploadJobStatus::Paused);
        assert_eq!(
            uploading_after.last_error_code,
            Some(UploadErrorCode::ProcessInterrupted)
        );
        assert_eq!(
            retrying_after.last_error_code,
            Some(UploadErrorCode::ProcessInterrupted)
        );

        pool.close().await;
    }

    #[tokio::test]
    async fn persisted_transition_rejects_invalid_state_move() {
        let (store, pool) = create_test_store().await;

        let job_id = create_base_job(&store, "/资料/视频D.mp4")
            .await
            .expect("create base job");

        let error = store
            .transition_job_status(&job_id, UploadJobStatus::Succeeded, None, None)
            .await
            .expect_err("queued -> succeeded should be rejected");
        let message = error.to_string();
        assert!(
            message.contains("invalid upload status transition"),
            "unexpected error message: {message}"
        );

        pool.close().await;
    }

    #[tokio::test]
    async fn attempt_records_support_start_and_finish() {
        let (store, pool) = create_test_store().await;

        let job_id = create_base_job(&store, "/资料/视频E.mp4")
            .await
            .expect("create base job");

        let started = store
            .record_attempt_started(&job_id, 1)
            .await
            .expect("start attempt should succeed");
        assert_eq!(started.status, UploadAttemptStatus::Started);
        assert!(started.finished_at.is_none());

        let finished = store
            .record_attempt_finished(
                &job_id,
                1,
                UploadAttemptStatus::Failed,
                Some(UploadErrorSnapshot {
                    code: UploadErrorCode::Network,
                    message: "网络中断".to_string(),
                }),
            )
            .await
            .expect("finish attempt should succeed");
        assert_eq!(finished.status, UploadAttemptStatus::Failed);
        assert_eq!(finished.error_code, Some(UploadErrorCode::Network));
        assert!(finished.finished_at.is_some());

        pool.close().await;
    }

    async fn create_base_job(store: &UploadStore, remote_path: &str) -> Result<String, AppError> {
        let job_id = format!("job-{}", Uuid::new_v4());
        let _ = store
            .create_job(NewUploadJob {
                id: job_id.clone(),
                provider_key: "p115".to_string(),
                endpoint_id: None,
                local_path: format!("D:{remote_path}"),
                remote_path: remote_path.to_string(),
                file_size: 1024,
                part_size: 256,
                total_parts: 4,
                content_hash: None,
                metadata_json: None,
                retry_policy: UploadRetryPolicy::default(),
            })
            .await?;

        Ok(job_id)
    }

    async fn create_test_store() -> (UploadStore, SqlitePool) {
        let path = unique_test_db_path();
        let options = SqliteConnectOptions::from_str(&format!("sqlite:{}", path.display()))
            .expect("build sqlite connect options")
            .create_if_missing(true)
            .disable_statement_logging();
        let pool = SqlitePoolOptions::new()
            .max_connections(1)
            .connect_with(options)
            .await
            .expect("connect sqlite test db");

        TEST_MIGRATOR
            .run(&pool)
            .await
            .expect("run migrations for upload tests");

        (UploadStore::new(&pool), pool)
    }

    fn unique_test_db_path() -> PathBuf {
        let mut path = std::env::temp_dir();
        path.push(format!("mam-upload-tests-{}.sqlite3", Uuid::new_v4()));
        path
    }
}
