use std::{
    collections::BTreeMap,
    fmt::{Display, Formatter},
    str::FromStr,
};

use async_trait::async_trait;
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq, Hash)]
#[serde(rename_all = "snake_case")]
pub enum UploadProviderKind {
    P115,
    Custom,
}

impl UploadProviderKind {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::P115 => "p115",
            Self::Custom => "custom",
        }
    }
}

impl Display for UploadProviderKind {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        f.write_str(self.as_str())
    }
}

impl FromStr for UploadProviderKind {
    type Err = String;

    fn from_str(value: &str) -> Result<Self, Self::Err> {
        match value.trim().to_lowercase().as_str() {
            "p115" => Ok(Self::P115),
            "custom" => Ok(Self::Custom),
            other => Err(format!("invalid upload provider kind: {other}")),
        }
    }
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq, Hash)]
#[serde(rename_all = "snake_case")]
pub enum UploadJobStatus {
    Queued,
    Preparing,
    Uploading,
    Paused,
    Retrying,
    Recovering,
    Failed,
    Succeeded,
    Canceled,
}

impl UploadJobStatus {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Queued => "queued",
            Self::Preparing => "preparing",
            Self::Uploading => "uploading",
            Self::Paused => "paused",
            Self::Retrying => "retrying",
            Self::Recovering => "recovering",
            Self::Failed => "failed",
            Self::Succeeded => "succeeded",
            Self::Canceled => "canceled",
        }
    }

    pub fn is_terminal(self) -> bool {
        matches!(self, Self::Failed | Self::Succeeded | Self::Canceled)
    }

    pub fn is_active(self) -> bool {
        matches!(
            self,
            Self::Preparing | Self::Uploading | Self::Retrying | Self::Recovering
        )
    }

    pub fn can_transition_to(self, next: Self) -> bool {
        if self == next {
            return true;
        }

        match self {
            Self::Queued => matches!(
                next,
                Self::Preparing | Self::Paused | Self::Canceled | Self::Failed
            ),
            Self::Preparing => matches!(
                next,
                Self::Queued
                    | Self::Uploading
                    | Self::Paused
                    | Self::Retrying
                    | Self::Recovering
                    | Self::Failed
                    | Self::Canceled
            ),
            Self::Uploading => matches!(
                next,
                Self::Queued
                    | Self::Paused
                    | Self::Retrying
                    | Self::Recovering
                    | Self::Failed
                    | Self::Succeeded
                    | Self::Canceled
            ),
            Self::Paused => matches!(
                next,
                Self::Queued | Self::Preparing | Self::Failed | Self::Canceled
            ),
            Self::Retrying => matches!(
                next,
                Self::Queued
                    | Self::Preparing
                    | Self::Uploading
                    | Self::Paused
                    | Self::Recovering
                    | Self::Failed
                    | Self::Canceled
            ),
            Self::Recovering => {
                matches!(
                    next,
                    Self::Queued | Self::Preparing | Self::Retrying | Self::Failed | Self::Canceled
                )
            }
            Self::Failed => matches!(next, Self::Queued | Self::Preparing | Self::Canceled),
            Self::Succeeded | Self::Canceled => false,
        }
    }
}

impl Display for UploadJobStatus {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        f.write_str(self.as_str())
    }
}

impl FromStr for UploadJobStatus {
    type Err = String;

    fn from_str(value: &str) -> Result<Self, Self::Err> {
        match value.trim().to_lowercase().as_str() {
            "queued" => Ok(Self::Queued),
            "preparing" => Ok(Self::Preparing),
            "uploading" => Ok(Self::Uploading),
            "paused" => Ok(Self::Paused),
            "retrying" => Ok(Self::Retrying),
            "recovering" => Ok(Self::Recovering),
            "failed" => Ok(Self::Failed),
            "succeeded" => Ok(Self::Succeeded),
            "canceled" => Ok(Self::Canceled),
            other => Err(format!("invalid upload job status: {other}")),
        }
    }
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq, Hash)]
#[serde(rename_all = "snake_case")]
pub enum UploadPartStatus {
    Pending,
    Uploaded,
    Skipped,
    Failed,
}

impl UploadPartStatus {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Pending => "pending",
            Self::Uploaded => "uploaded",
            Self::Skipped => "skipped",
            Self::Failed => "failed",
        }
    }
}

impl Display for UploadPartStatus {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        f.write_str(self.as_str())
    }
}

impl FromStr for UploadPartStatus {
    type Err = String;

    fn from_str(value: &str) -> Result<Self, Self::Err> {
        match value.trim().to_lowercase().as_str() {
            "pending" => Ok(Self::Pending),
            "uploaded" => Ok(Self::Uploaded),
            "skipped" => Ok(Self::Skipped),
            "failed" => Ok(Self::Failed),
            other => Err(format!("invalid upload part status: {other}")),
        }
    }
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq, Hash)]
#[serde(rename_all = "snake_case")]
pub enum UploadErrorCode {
    Network,
    Timeout,
    AuthExpired,
    PermissionDenied,
    RateLimited,
    ProviderUnavailable,
    RemoteConflict,
    SessionExpired,
    ChecksumMismatch,
    LocalIo,
    ProcessInterrupted,
    Unknown,
}

impl UploadErrorCode {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Network => "network",
            Self::Timeout => "timeout",
            Self::AuthExpired => "auth_expired",
            Self::PermissionDenied => "permission_denied",
            Self::RateLimited => "rate_limited",
            Self::ProviderUnavailable => "provider_unavailable",
            Self::RemoteConflict => "remote_conflict",
            Self::SessionExpired => "session_expired",
            Self::ChecksumMismatch => "checksum_mismatch",
            Self::LocalIo => "local_io",
            Self::ProcessInterrupted => "process_interrupted",
            Self::Unknown => "unknown",
        }
    }

    pub fn parse_lossy(value: &str) -> Self {
        Self::from_str(value).unwrap_or(Self::Unknown)
    }
}

impl Display for UploadErrorCode {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        f.write_str(self.as_str())
    }
}

impl FromStr for UploadErrorCode {
    type Err = String;

    fn from_str(value: &str) -> Result<Self, Self::Err> {
        match value.trim().to_lowercase().as_str() {
            "network" => Ok(Self::Network),
            "timeout" => Ok(Self::Timeout),
            "auth_expired" => Ok(Self::AuthExpired),
            "permission_denied" => Ok(Self::PermissionDenied),
            "rate_limited" => Ok(Self::RateLimited),
            "provider_unavailable" => Ok(Self::ProviderUnavailable),
            "remote_conflict" => Ok(Self::RemoteConflict),
            "session_expired" => Ok(Self::SessionExpired),
            "checksum_mismatch" => Ok(Self::ChecksumMismatch),
            "local_io" => Ok(Self::LocalIo),
            "process_interrupted" => Ok(Self::ProcessInterrupted),
            "unknown" => Ok(Self::Unknown),
            other => Err(format!("invalid upload error code: {other}")),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct UploadProgress {
    pub bytes_uploaded: u64,
    pub bytes_total: u64,
    pub parts_uploaded: u32,
    pub parts_total: u32,
    pub updated_at: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum UploadEventKind {
    Accepted,
    SessionCreated,
    PartUploaded,
    ProgressUpdated,
    RetryScheduled,
    Paused,
    Resumed,
    Recovered,
    Failed,
    Completed,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct UploadEvent {
    pub job_id: String,
    pub kind: UploadEventKind,
    pub status: UploadJobStatus,
    pub message: Option<String>,
    pub progress: Option<UploadProgress>,
    pub retry_count: u32,
    pub emitted_at: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "camelCase")]
pub struct UploadRetryPolicy {
    pub max_retries: u32,
    pub base_delay_ms: u64,
    pub max_delay_ms: u64,
    pub jitter_ratio: f32,
}

impl Default for UploadRetryPolicy {
    fn default() -> Self {
        Self {
            max_retries: 8,
            base_delay_ms: 1_000,
            max_delay_ms: 120_000,
            jitter_ratio: 0.15,
        }
    }
}

impl UploadRetryPolicy {
    pub fn bounded(mut self) -> Self {
        if self.max_retries > 100 {
            self.max_retries = 100;
        }
        if self.base_delay_ms == 0 {
            self.base_delay_ms = 500;
        }
        if self.max_delay_ms < self.base_delay_ms {
            self.max_delay_ms = self.base_delay_ms;
        }
        if !(0.0..=1.0).contains(&self.jitter_ratio) {
            self.jitter_ratio = 0.15;
        }
        self
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct UploadError {
    pub code: UploadErrorCode,
    pub message: String,
    pub retryable: bool,
    pub provider_code: Option<String>,
}

impl UploadError {
    pub fn new(code: UploadErrorCode, message: impl Into<String>, retryable: bool) -> Self {
        Self {
            code,
            message: message.into(),
            retryable,
            provider_code: None,
        }
    }
}

impl Display for UploadError {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "upload provider error [{}] {}",
            self.code.as_str(),
            self.message
        )
    }
}

impl std::error::Error for UploadError {}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct CreateUploadSessionRequest {
    pub job_id: String,
    pub local_path: String,
    pub remote_path: String,
    pub file_size: u64,
    pub part_size: u64,
    pub content_hash: Option<String>,
    pub metadata: BTreeMap<String, String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct CreateUploadSessionResponse {
    pub session_id: String,
    pub provider_upload_id: String,
    pub part_size: u64,
    pub total_parts: u32,
    pub resume_token: Option<String>,
    pub extra: BTreeMap<String, String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct QueryUploadedPartsRequest {
    pub job_id: String,
    pub session_id: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct UploadPartDescriptor {
    pub part_number: u32,
    pub part_size: u64,
    pub etag: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct QueryUploadedPartsResponse {
    pub parts: Vec<UploadPartDescriptor>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct UploadPartRequest {
    pub job_id: String,
    pub session_id: String,
    pub part_number: u32,
    pub offset: u64,
    pub bytes: Vec<u8>,
    pub checksum: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct UploadPartResponse {
    pub part_number: u32,
    pub committed_size: u64,
    pub etag: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct CompleteUploadRequest {
    pub job_id: String,
    pub session_id: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct CompleteUploadResponse {
    pub remote_file_id: Option<String>,
    pub remote_path: String,
    pub checksum: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct AbortUploadRequest {
    pub job_id: String,
    pub session_id: String,
    pub reason: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct ProviderHealth {
    pub available: bool,
    pub provider: UploadProviderKind,
    pub detail: Option<String>,
}

#[async_trait]
pub trait UploadProvider: Send + Sync {
    fn kind(&self) -> UploadProviderKind;

    async fn health_check(&self) -> Result<ProviderHealth, UploadError>;

    async fn create_session(
        &self,
        request: CreateUploadSessionRequest,
    ) -> Result<CreateUploadSessionResponse, UploadError>;

    async fn query_uploaded_parts(
        &self,
        request: QueryUploadedPartsRequest,
    ) -> Result<QueryUploadedPartsResponse, UploadError>;

    async fn upload_part(
        &self,
        request: UploadPartRequest,
    ) -> Result<UploadPartResponse, UploadError>;

    async fn complete_upload(
        &self,
        request: CompleteUploadRequest,
    ) -> Result<CompleteUploadResponse, UploadError>;

    async fn abort_upload(&self, request: AbortUploadRequest) -> Result<(), UploadError>;
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase")]
pub struct UploadStateTransitionError {
    pub from: UploadJobStatus,
    pub to: UploadJobStatus,
}

impl Display for UploadStateTransitionError {
    fn fmt(&self, f: &mut Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "invalid upload status transition: {} -> {}",
            self.from.as_str(),
            self.to.as_str()
        )
    }
}

impl std::error::Error for UploadStateTransitionError {}

pub fn ensure_status_transition(
    from: UploadJobStatus,
    to: UploadJobStatus,
) -> Result<(), UploadStateTransitionError> {
    if from.can_transition_to(to) {
        return Ok(());
    }

    Err(UploadStateTransitionError { from, to })
}

#[cfg(test)]
mod tests {
    use super::{ensure_status_transition, UploadJobStatus};

    #[test]
    fn upload_job_status_transition_rules_are_enforced() {
        assert!(UploadJobStatus::Queued.can_transition_to(UploadJobStatus::Preparing));
        assert!(UploadJobStatus::Uploading.can_transition_to(UploadJobStatus::Retrying));
        assert!(UploadJobStatus::Uploading.can_transition_to(UploadJobStatus::Succeeded));
        assert!(UploadJobStatus::Recovering.can_transition_to(UploadJobStatus::Queued));
        assert!(!UploadJobStatus::Succeeded.can_transition_to(UploadJobStatus::Queued));
        assert!(!UploadJobStatus::Canceled.can_transition_to(UploadJobStatus::Preparing));
    }

    #[test]
    fn ensure_status_transition_reports_invalid_path() {
        let error =
            ensure_status_transition(UploadJobStatus::Succeeded, UploadJobStatus::Uploading)
                .expect_err("succeeded should not be allowed to move back to uploading");
        assert_eq!(error.from, UploadJobStatus::Succeeded);
        assert_eq!(error.to, UploadJobStatus::Uploading);
    }
}
