#[allow(dead_code)]
mod domain;
#[allow(dead_code)]
mod store;

use crate::app_state::ModuleStatus;

#[allow(unused_imports)]
pub use domain::{
    ensure_status_transition, AbortUploadRequest, CompleteUploadRequest, CompleteUploadResponse,
    CreateUploadSessionRequest, CreateUploadSessionResponse, ProviderHealth,
    QueryUploadedPartsRequest, QueryUploadedPartsResponse, UploadError, UploadErrorCode,
    UploadEvent, UploadEventKind, UploadJobStatus, UploadPartDescriptor, UploadPartRequest,
    UploadPartResponse, UploadPartStatus, UploadProvider, UploadProviderKind, UploadRetryPolicy,
    UploadStateTransitionError,
};
#[allow(unused_imports)]
pub use store::{
    NewUploadJob, NewUploadSession, UploadAttemptRecord, UploadAttemptStatus, UploadErrorSnapshot,
    UploadJobRecord, UploadPartCheckpoint, UploadResumeSnapshot, UploadSessionRecord, UploadStore,
    UpsertUploadPartCheckpoint,
};

pub struct UploadModule;

impl UploadModule {
    pub fn new() -> Self {
        Self
    }

    pub fn status(&self) -> ModuleStatus {
        ModuleStatus {
            name: "upload".to_string(),
            ready: true,
        }
    }
}
