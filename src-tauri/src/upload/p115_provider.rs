use std::{
    collections::BTreeMap,
    env, fs,
    net::TcpListener,
    path::PathBuf,
    process::{Child, Command, Stdio},
    sync::{Arc, Mutex},
    time::{Duration, Instant},
};

use async_trait::async_trait;
use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine as _};
use reqwest::StatusCode;
use serde::{de::DeserializeOwned, Deserialize, Serialize};
use serde_json::Value;
use tokio::time::sleep;
use tracing::{info, warn};

use crate::upload::{
    AbortUploadRequest, CompleteUploadRequest, CompleteUploadResponse, CreateUploadSessionRequest,
    CreateUploadSessionResponse, ProviderHealth, QueryUploadedPartsRequest,
    QueryUploadedPartsResponse, UploadError, UploadErrorCode, UploadPartDescriptor,
    UploadPartRequest, UploadPartResponse, UploadProvider, UploadProviderKind,
};

const SIDECAR_TOKEN_HEADER: &str = "X-MAM-Sidecar-Token";
const SIDECAR_HEALTH_PATH: &str = "/healthz";
const SIDECAR_OPEN_PATH: &str = "/v1/upload/open";
const SIDECAR_LIST_PARTS_PATH: &str = "/v1/upload/list-parts";
const SIDECAR_UPLOAD_PARTS_PATH: &str = "/v1/upload/upload-parts";
const SIDECAR_COMPLETE_PATH: &str = "/v1/upload/complete";
const SIDECAR_ABORT_PATH: &str = "/v1/upload/abort";

const ENV_SIDECAR_URL: &str = "MAM_P115_UPLOAD_SIDECAR_URL";
const ENV_SIDECAR_TOKEN: &str = "MAM_P115_UPLOAD_SIDECAR_TOKEN";
const ENV_SIDECAR_SCRIPT: &str = "MAM_P115_UPLOAD_SIDECAR_SCRIPT";
const ENV_SIDECAR_STATE_DIR: &str = "MAM_P115_UPLOAD_SIDECAR_STATE_DIR";
const ENV_SIDECAR_HOST: &str = "MAM_P115_UPLOAD_SIDECAR_HOST";
const ENV_SIDECAR_PORT: &str = "MAM_P115_UPLOAD_SIDECAR_PORT";
const ENV_SIDECAR_MOCK: &str = "MAM_115_UPLOAD_SIDECAR_MOCK";
const ENV_SIDECAR_PART_SIZE: &str = "MAM_115_UPLOAD_SIDECAR_PART_SIZE";
const ENV_PYTHON_COMMAND: &str = "MAM_PYTHON_CMD";
const ENV_PYTHONPATH: &str = "MAM_115_PYTHONPATH";

#[derive(Debug, Clone)]
pub struct P115UploadProviderConfig {
    pub sidecar_base_url: Option<String>,
    pub sidecar_token: Option<String>,
    pub sidecar_host: String,
    pub sidecar_port: Option<u16>,
    pub sidecar_state_dir: PathBuf,
    pub sidecar_script_path: Option<PathBuf>,
    pub python_command: String,
    pub python_path: Option<PathBuf>,
    pub startup_timeout: Duration,
    pub default_part_size: u64,
    pub mock_mode: bool,
}

#[derive(Debug, Clone)]
pub struct P115UploadProvider {
    runtime: Arc<P115SidecarRuntime>,
    http_client: reqwest::Client,
}

#[derive(Debug)]
struct P115SidecarRuntime {
    token: String,
    mode: P115SidecarMode,
    startup_timeout: Duration,
}

#[derive(Debug)]
enum P115SidecarMode {
    External {
        base_url: String,
    },
    Managed {
        base_url: String,
        host: String,
        port: u16,
        state_dir: PathBuf,
        script_path: PathBuf,
        python_command: String,
        python_path: Option<PathBuf>,
        mock_mode: bool,
        default_part_size: u64,
        child: Mutex<Option<Child>>,
    },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct SessionTokenPayload {
    job_id: String,
    local_path: String,
    remote_path: String,
    resume_state_path: String,
    credential: String,
    app_type: String,
    root_id: String,
    part_size: u64,
    parent_id: i64,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
struct SidecarUploadRequest {
    job_id: String,
    local_path: String,
    remote_path: String,
    credential: String,
    app_type: String,
    root_id: String,
    resume_state_path: String,
    part_size: u64,
    max_parts: Option<u32>,
    parent_id: i64,
}

#[derive(Debug, Deserialize)]
struct SidecarEnvelope<T> {
    success: bool,
    data: Option<T>,
    error: Option<SidecarErrorPayload>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SidecarErrorPayload {
    code: Option<String>,
    message: Option<String>,
    retryable: Option<bool>,
    detail: Option<Value>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SidecarUploadSessionData {
    upload_id: Option<String>,
    state_path: Option<String>,
    session_created: Option<bool>,
    session_existed: Option<bool>,
    completed: Option<bool>,
    state_deleted: Option<bool>,
    parts: Option<Vec<SidecarPart>>,
    uploaded_in_call: Option<Vec<SidecarPart>>,
    progress: Option<SidecarProgress>,
    provider_response: Option<Value>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SidecarPart {
    part_number: u32,
    size: u64,
    etag: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SidecarProgress {
    file_size: u64,
    part_size: u64,
    uploaded_bytes: u64,
    uploaded_parts: u64,
    total_parts: u64,
    completed: bool,
}

impl Default for P115UploadProviderConfig {
    fn default() -> Self {
        Self::from_env()
    }
}

impl P115UploadProviderConfig {
    pub fn from_env() -> Self {
        let sidecar_base_url = env_text(ENV_SIDECAR_URL);
        let sidecar_token = env_text(ENV_SIDECAR_TOKEN);
        let sidecar_host = env_text(ENV_SIDECAR_HOST).unwrap_or_else(|| "127.0.0.1".to_string());
        let sidecar_port = env_u16(ENV_SIDECAR_PORT);
        let sidecar_state_dir =
            env_path(ENV_SIDECAR_STATE_DIR).unwrap_or_else(default_sidecar_state_dir);
        let sidecar_script_path = env_path(ENV_SIDECAR_SCRIPT).or_else(find_sidecar_script_path);
        let python_command = env_text(ENV_PYTHON_COMMAND).unwrap_or_else(detect_python_command);
        let python_path = env_path(ENV_PYTHONPATH).or_else(find_python_libs_path);
        let default_part_size = env_u64(ENV_SIDECAR_PART_SIZE).unwrap_or(10 * 1024 * 1024);
        let mock_mode = env_bool(ENV_SIDECAR_MOCK);

        Self {
            sidecar_base_url,
            sidecar_token,
            sidecar_host,
            sidecar_port,
            sidecar_state_dir,
            sidecar_script_path,
            python_command,
            python_path,
            startup_timeout: Duration::from_secs(25),
            default_part_size,
            mock_mode,
        }
    }
}

impl P115UploadProvider {
    pub fn new(config: P115UploadProviderConfig) -> Result<Self, UploadError> {
        let runtime = Arc::new(P115SidecarRuntime::from_config(config)?);
        let http_client = reqwest::Client::builder()
            .timeout(Duration::from_secs(60))
            .build()
            .map_err(|error| {
                UploadError::new(
                    UploadErrorCode::ProviderUnavailable,
                    format!("failed to initialize sidecar http client: {error}"),
                    false,
                )
            })?;

        Ok(Self {
            runtime,
            http_client,
        })
    }

    pub fn from_env() -> Result<Self, UploadError> {
        Self::new(P115UploadProviderConfig::from_env())
    }

    async fn sidecar_health_check(&self) -> Result<(), UploadError> {
        self.runtime.ensure_ready(&self.http_client).await?;
        let url = self.runtime.build_url(SIDECAR_HEALTH_PATH);
        let mut request = self.http_client.get(url);
        if let Some(token) = self.runtime.request_token() {
            request = request.header(SIDECAR_TOKEN_HEADER, token);
        }

        let response = request.send().await.map_err(map_network_error)?;
        let status = response.status();
        if !status.is_success() {
            return Err(UploadError::new(
                UploadErrorCode::ProviderUnavailable,
                format!("p115 sidecar health check failed: http {status}"),
                true,
            ));
        }
        Ok(())
    }

    async fn call_sidecar<T>(
        &self,
        path: &str,
        payload: &SidecarUploadRequest,
    ) -> Result<T, UploadError>
    where
        T: DeserializeOwned,
    {
        self.runtime.ensure_ready(&self.http_client).await?;
        let url = self.runtime.build_url(path);
        let mut request = self.http_client.post(url).json(payload);
        if let Some(token) = self.runtime.request_token() {
            request = request.header(SIDECAR_TOKEN_HEADER, token);
        }

        let response = request.send().await.map_err(map_network_error)?;
        parse_sidecar_envelope(response).await
    }
}

impl P115SidecarRuntime {
    fn from_config(config: P115UploadProviderConfig) -> Result<Self, UploadError> {
        let token = config
            .sidecar_token
            .unwrap_or_else(|| uuid::Uuid::new_v4().to_string());

        if let Some(base_url) = config.sidecar_base_url {
            return Ok(Self {
                token,
                mode: P115SidecarMode::External {
                    base_url: normalize_base_url(&base_url)?,
                },
                startup_timeout: config.startup_timeout,
            });
        }

        let script_path = config.sidecar_script_path.ok_or_else(|| {
            UploadError::new(
                UploadErrorCode::ProviderUnavailable,
                "p115 sidecar script not found; set MAM_P115_UPLOAD_SIDECAR_SCRIPT or MAM_P115_UPLOAD_SIDECAR_URL",
                false,
            )
        })?;
        if !script_path.is_file() {
            return Err(UploadError::new(
                UploadErrorCode::ProviderUnavailable,
                format!("p115 sidecar script not found: {}", script_path.display()),
                false,
            ));
        }

        fs::create_dir_all(&config.sidecar_state_dir).map_err(|error| {
            UploadError::new(
                UploadErrorCode::LocalIo,
                format!(
                    "failed to initialize p115 sidecar state dir {}: {error}",
                    config.sidecar_state_dir.display()
                ),
                false,
            )
        })?;

        let port = config.sidecar_port.unwrap_or_else(allocate_ephemeral_port);
        let base_url = format!("http://{}:{port}", config.sidecar_host);
        Ok(Self {
            token,
            mode: P115SidecarMode::Managed {
                base_url,
                host: config.sidecar_host,
                port,
                state_dir: config.sidecar_state_dir,
                script_path,
                python_command: config.python_command,
                python_path: config.python_path,
                mock_mode: config.mock_mode,
                default_part_size: config.default_part_size.max(1),
                child: Mutex::new(None),
            },
            startup_timeout: config.startup_timeout,
        })
    }

    fn build_url(&self, path: &str) -> String {
        match &self.mode {
            P115SidecarMode::External { base_url } => format!("{base_url}{path}"),
            P115SidecarMode::Managed { base_url, .. } => format!("{base_url}{path}"),
        }
    }

    fn request_token(&self) -> Option<&str> {
        if self.token.trim().is_empty() {
            None
        } else {
            Some(self.token.as_str())
        }
    }

    async fn ensure_ready(&self, http_client: &reqwest::Client) -> Result<(), UploadError> {
        match &self.mode {
            P115SidecarMode::External { .. } => {
                self.wait_for_health(http_client).await?;
                Ok(())
            }
            P115SidecarMode::Managed { .. } => {
                if self.wait_for_health(http_client).await.is_ok() {
                    return Ok(());
                }

                self.ensure_spawned()?;
                self.wait_for_health(http_client).await
            }
        }
    }

    fn ensure_spawned(&self) -> Result<(), UploadError> {
        let P115SidecarMode::Managed {
            host,
            port,
            state_dir,
            script_path,
            python_command,
            python_path,
            mock_mode,
            default_part_size,
            child,
            ..
        } = &self.mode
        else {
            return Ok(());
        };

        let mut guard = child.lock().map_err(|_| {
            UploadError::new(
                UploadErrorCode::ProcessInterrupted,
                "failed to lock sidecar process state",
                true,
            )
        })?;

        if let Some(existing) = guard.as_mut() {
            match existing.try_wait() {
                Ok(Some(_)) => {
                    *guard = None;
                }
                Ok(None) => return Ok(()),
                Err(error) => {
                    warn!("failed to inspect p115 sidecar process: {}", error);
                    *guard = None;
                }
            }
        }

        let mut command = Command::new(python_command);
        command
            .arg(script_path)
            .arg("--host")
            .arg(host)
            .arg("--port")
            .arg(port.to_string())
            .arg("--state-dir")
            .arg(state_dir)
            .arg("--token")
            .arg(&self.token)
            .arg("--part-size")
            .arg(default_part_size.to_string())
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .env("PYTHONIOENCODING", "utf-8");

        if *mock_mode {
            command.arg("--mock");
            command.env(ENV_SIDECAR_MOCK, "1");
        }

        if let Some(path) = python_path {
            if path.is_dir() {
                let existing = env::var("PYTHONPATH").ok().unwrap_or_default();
                let value = if existing.trim().is_empty() {
                    path.display().to_string()
                } else {
                    format!(
                        "{}{}{}",
                        path.display(),
                        if cfg!(windows) { ";" } else { ":" },
                        existing.trim()
                    )
                };
                command.env("PYTHONPATH", value);
            }
        }

        #[cfg(windows)]
        {
            use std::os::windows::process::CommandExt;
            const CREATE_NO_WINDOW: u32 = 0x0800_0000;
            command.creation_flags(CREATE_NO_WINDOW);
        }

        let process = command.spawn().map_err(|error| {
            UploadError::new(
                UploadErrorCode::ProviderUnavailable,
                format!(
                    "failed to start p115 sidecar using {} {}: {error}",
                    python_command,
                    script_path.display()
                ),
                true,
            )
        })?;

        info!(
            python_command = %python_command,
            script = %script_path.display(),
            host = %host,
            port = *port,
            mock_mode = *mock_mode,
            "p115 upload sidecar started"
        );

        *guard = Some(process);
        Ok(())
    }

    async fn wait_for_health(&self, http_client: &reqwest::Client) -> Result<(), UploadError> {
        let deadline = Instant::now() + self.startup_timeout;
        let mut last_error: Option<UploadError> = None;

        while Instant::now() < deadline {
            let mut request = http_client.get(self.build_url(SIDECAR_HEALTH_PATH));
            if let Some(token) = self.request_token() {
                request = request.header(SIDECAR_TOKEN_HEADER, token);
            }

            match request.send().await {
                Ok(response) if response.status().is_success() => return Ok(()),
                Ok(response) => {
                    last_error = Some(UploadError::new(
                        UploadErrorCode::ProviderUnavailable,
                        format!("p115 sidecar health check returned {}", response.status()),
                        true,
                    ));
                }
                Err(error) => {
                    last_error = Some(map_network_error(error));
                }
            }

            sleep(Duration::from_millis(200)).await;
        }

        Err(last_error.unwrap_or_else(|| {
            UploadError::new(
                UploadErrorCode::ProviderUnavailable,
                "p115 sidecar did not become ready in time",
                true,
            )
        }))
    }
}

impl Drop for P115SidecarRuntime {
    fn drop(&mut self) {
        let P115SidecarMode::Managed { child, .. } = &self.mode else {
            return;
        };
        let Ok(mut guard) = child.lock() else {
            return;
        };
        let Some(process) = guard.as_mut() else {
            return;
        };

        if let Ok(None) = process.try_wait() {
            let _ = process.kill();
            let _ = process.wait();
        }
        *guard = None;
    }
}

#[async_trait]
impl UploadProvider for P115UploadProvider {
    fn kind(&self) -> UploadProviderKind {
        UploadProviderKind::P115
    }

    async fn health_check(&self) -> Result<ProviderHealth, UploadError> {
        match self.sidecar_health_check().await {
            Ok(_) => Ok(ProviderHealth {
                available: true,
                provider: UploadProviderKind::P115,
                detail: Some("p115 sidecar is ready".to_string()),
            }),
            Err(error) => Ok(ProviderHealth {
                available: false,
                provider: UploadProviderKind::P115,
                detail: Some(error.message),
            }),
        }
    }

    async fn create_session(
        &self,
        request: CreateUploadSessionRequest,
    ) -> Result<CreateUploadSessionResponse, UploadError> {
        let context = SessionTokenPayload::from_create_request(&request)?;
        let sidecar_request = context.to_sidecar_request();
        let data: SidecarUploadSessionData = self
            .call_sidecar(SIDECAR_OPEN_PATH, &sidecar_request)
            .await?;

        let progress = data.progress.unwrap_or(SidecarProgress {
            file_size: request.file_size,
            part_size: request.part_size.max(1),
            uploaded_bytes: 0,
            uploaded_parts: 0,
            total_parts: 0,
            completed: false,
        });
        let provider_upload_id = data
            .upload_id
            .unwrap_or_else(|| format!("p115-{}", request.job_id));
        let session_id = encode_session_token(&context)?;

        let mut extra = BTreeMap::new();
        if let Some(state_path) = data.state_path.clone() {
            extra.insert("statePath".to_string(), state_path);
        }
        if let Some(created) = data.session_created {
            extra.insert("sessionCreated".to_string(), created.to_string());
        }
        if let Some(existed) = data.session_existed {
            extra.insert("sessionExisted".to_string(), existed.to_string());
        }
        if let Some(completed) = data.completed {
            extra.insert("completed".to_string(), completed.to_string());
        }
        if let Some(state_deleted) = data.state_deleted {
            extra.insert("stateDeleted".to_string(), state_deleted.to_string());
        }
        extra.insert(
            "uploadedBytes".to_string(),
            progress.uploaded_bytes.to_string(),
        );
        extra.insert(
            "uploadedParts".to_string(),
            progress.uploaded_parts.to_string(),
        );
        if let Some(provider_response) = data.provider_response {
            extra.insert(
                "providerResponse".to_string(),
                serde_json::to_string(&provider_response).unwrap_or_default(),
            );
        }

        let total_parts = u64_to_u32(progress.total_parts, "sidecar total parts")?;
        let part_size = progress.part_size.max(1);

        Ok(CreateUploadSessionResponse {
            session_id,
            provider_upload_id,
            part_size,
            total_parts,
            resume_token: data.state_path,
            extra,
        })
    }

    async fn query_uploaded_parts(
        &self,
        request: QueryUploadedPartsRequest,
    ) -> Result<QueryUploadedPartsResponse, UploadError> {
        let context = decode_session_token(&request.session_id)?;
        if context.job_id != request.job_id {
            return Err(UploadError::new(
                UploadErrorCode::SessionExpired,
                "session token does not match job id",
                false,
            ));
        }

        let sidecar_request = context.to_sidecar_request();
        let data: SidecarUploadSessionData = self
            .call_sidecar(SIDECAR_LIST_PARTS_PATH, &sidecar_request)
            .await?;

        Ok(QueryUploadedPartsResponse {
            parts: normalize_sidecar_parts(data.parts.unwrap_or_default()),
        })
    }

    async fn upload_part(
        &self,
        request: UploadPartRequest,
    ) -> Result<UploadPartResponse, UploadError> {
        let context = decode_session_token(&request.session_id)?;
        if context.job_id != request.job_id {
            return Err(UploadError::new(
                UploadErrorCode::SessionExpired,
                "session token does not match job id",
                false,
            ));
        }

        let mut sidecar_request = context.to_sidecar_request();
        sidecar_request.max_parts = Some(1);
        sidecar_request.part_size = context.part_size.max(1);
        let data: SidecarUploadSessionData = self
            .call_sidecar(SIDECAR_UPLOAD_PARTS_PATH, &sidecar_request)
            .await?;

        let uploaded_in_call = normalize_sidecar_parts(data.uploaded_in_call.unwrap_or_default());
        if let Some(first_part) = uploaded_in_call.first() {
            return Ok(UploadPartResponse {
                part_number: first_part.part_number,
                committed_size: first_part.part_size,
                etag: first_part.etag.clone(),
            });
        }

        let known_parts = normalize_sidecar_parts(data.parts.unwrap_or_default());
        if let Some(existing) = known_parts
            .into_iter()
            .find(|part| part.part_number == request.part_number)
        {
            return Ok(UploadPartResponse {
                part_number: existing.part_number,
                committed_size: existing.part_size,
                etag: existing.etag,
            });
        }

        Ok(UploadPartResponse {
            part_number: request.part_number,
            committed_size: request.bytes.len() as u64,
            etag: None,
        })
    }

    async fn complete_upload(
        &self,
        request: CompleteUploadRequest,
    ) -> Result<CompleteUploadResponse, UploadError> {
        let context = decode_session_token(&request.session_id)?;
        if context.job_id != request.job_id {
            return Err(UploadError::new(
                UploadErrorCode::SessionExpired,
                "session token does not match job id",
                false,
            ));
        }

        let sidecar_request = context.to_sidecar_request();
        let data: SidecarUploadSessionData = self
            .call_sidecar(SIDECAR_COMPLETE_PATH, &sidecar_request)
            .await?;

        let provider_response = data.provider_response.unwrap_or(Value::Null);
        let remote_file_id = extract_provider_response_text(&provider_response, &["file_id", "id"]);
        let checksum = extract_provider_response_text(&provider_response, &["sha1", "checksum"]);

        Ok(CompleteUploadResponse {
            remote_file_id,
            remote_path: context.remote_path,
            checksum,
        })
    }

    async fn abort_upload(&self, request: AbortUploadRequest) -> Result<(), UploadError> {
        let context = decode_session_token(&request.session_id)?;
        if context.job_id != request.job_id {
            return Err(UploadError::new(
                UploadErrorCode::SessionExpired,
                "session token does not match job id",
                false,
            ));
        }

        let sidecar_request = context.to_sidecar_request();
        let _: SidecarUploadSessionData = self
            .call_sidecar(SIDECAR_ABORT_PATH, &sidecar_request)
            .await?;
        Ok(())
    }
}

impl SessionTokenPayload {
    fn from_create_request(request: &CreateUploadSessionRequest) -> Result<Self, UploadError> {
        let credential = request
            .metadata
            .get("credential")
            .map(|value| value.trim().to_string())
            .filter(|value| !value.is_empty())
            .ok_or_else(|| {
                UploadError::new(
                    UploadErrorCode::AuthExpired,
                    "missing credential in create session metadata",
                    false,
                )
            })?;

        let app_type = request
            .metadata
            .get("appType")
            .map(|value| value.trim().to_string())
            .filter(|value| !value.is_empty())
            .unwrap_or_else(|| "wechatmini".to_string());
        let root_id = request
            .metadata
            .get("rootId")
            .map(|value| value.trim().to_string())
            .filter(|value| !value.is_empty())
            .unwrap_or_else(|| "0".to_string());
        let resume_state_path = request
            .metadata
            .get("resumeStatePath")
            .map(|value| value.trim().to_string())
            .unwrap_or_default();

        let parent_id = request
            .metadata
            .get("parentId")
            .and_then(|value| value.trim().parse::<i64>().ok())
            .unwrap_or(0);

        Ok(Self {
            job_id: request.job_id.clone(),
            local_path: request.local_path.clone(),
            remote_path: request.remote_path.clone(),
            resume_state_path,
            credential,
            app_type,
            root_id,
            part_size: request.part_size.max(1),
            parent_id,
        })
    }

    fn to_sidecar_request(&self) -> SidecarUploadRequest {
        SidecarUploadRequest {
            job_id: self.job_id.clone(),
            local_path: self.local_path.clone(),
            remote_path: self.remote_path.clone(),
            credential: self.credential.clone(),
            app_type: self.app_type.clone(),
            root_id: self.root_id.clone(),
            resume_state_path: self.resume_state_path.clone(),
            part_size: self.part_size.max(1),
            max_parts: None,
            parent_id: self.parent_id,
        }
    }
}

async fn parse_sidecar_envelope<T>(response: reqwest::Response) -> Result<T, UploadError>
where
    T: DeserializeOwned,
{
    let status = response.status();
    let text = response.text().await.map_err(map_network_error)?;
    let envelope = serde_json::from_str::<SidecarEnvelope<T>>(&text).map_err(|error| {
        UploadError::new(
            UploadErrorCode::ProviderUnavailable,
            format!("failed to decode sidecar response body: {error}; body={text}"),
            true,
        )
    })?;

    if !status.is_success() || !envelope.success {
        return Err(map_sidecar_error(status, envelope.error));
    }

    envelope.data.ok_or_else(|| {
        UploadError::new(
            UploadErrorCode::ProviderUnavailable,
            "sidecar response missing data field",
            true,
        )
    })
}

fn map_sidecar_error(status: StatusCode, payload: Option<SidecarErrorPayload>) -> UploadError {
    let code_text = payload
        .as_ref()
        .and_then(|value| value.code.as_ref())
        .map(|value| value.trim().to_lowercase())
        .unwrap_or_else(|| "unknown".to_string());
    let message = payload
        .as_ref()
        .and_then(|value| value.message.as_ref())
        .map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
        .unwrap_or_else(|| format!("p115 sidecar request failed with status {status}"));
    let retryable = payload
        .as_ref()
        .and_then(|value| value.retryable)
        .unwrap_or(status.is_server_error());

    let mut error = UploadError::new(
        map_sidecar_error_code(status, &code_text),
        message,
        retryable,
    );
    error.provider_code = Some(code_text);
    if let Some(detail) = payload.and_then(|value| value.detail) {
        warn!(detail = %detail, "p115 sidecar returned detail payload");
    }
    error
}

fn map_sidecar_error_code(status: StatusCode, code: &str) -> UploadErrorCode {
    match code {
        "auth_required" | "unauthorized" | "auth_expired" => UploadErrorCode::AuthExpired,
        "permission_denied" | "forbidden" => UploadErrorCode::PermissionDenied,
        "rate_limited" => UploadErrorCode::RateLimited,
        "session_not_found" | "session_expired" | "incomplete_upload" => {
            UploadErrorCode::SessionExpired
        }
        "network" => UploadErrorCode::Network,
        "timeout" => UploadErrorCode::Timeout,
        "provider_unavailable" => UploadErrorCode::ProviderUnavailable,
        "remote_conflict" => UploadErrorCode::RemoteConflict,
        _ => match status {
            StatusCode::UNAUTHORIZED => UploadErrorCode::AuthExpired,
            StatusCode::FORBIDDEN => UploadErrorCode::PermissionDenied,
            StatusCode::TOO_MANY_REQUESTS => UploadErrorCode::RateLimited,
            StatusCode::REQUEST_TIMEOUT | StatusCode::GATEWAY_TIMEOUT => UploadErrorCode::Timeout,
            StatusCode::CONFLICT | StatusCode::PRECONDITION_FAILED => {
                UploadErrorCode::RemoteConflict
            }
            StatusCode::BAD_GATEWAY | StatusCode::SERVICE_UNAVAILABLE => {
                UploadErrorCode::ProviderUnavailable
            }
            _ if status.is_server_error() => UploadErrorCode::ProviderUnavailable,
            _ => UploadErrorCode::Unknown,
        },
    }
}

fn map_network_error(error: reqwest::Error) -> UploadError {
    let code = if error.is_timeout() {
        UploadErrorCode::Timeout
    } else if error.is_connect() || error.is_request() {
        UploadErrorCode::Network
    } else {
        UploadErrorCode::ProviderUnavailable
    };
    UploadError::new(
        code,
        format!("failed to communicate with p115 sidecar: {error}"),
        true,
    )
}

fn normalize_sidecar_parts(parts: Vec<SidecarPart>) -> Vec<UploadPartDescriptor> {
    let mut normalized = parts
        .into_iter()
        .filter(|part| part.part_number > 0)
        .map(|part| UploadPartDescriptor {
            part_number: part.part_number,
            part_size: part.size,
            etag: part.etag.and_then(normalize_optional_text),
        })
        .collect::<Vec<_>>();
    normalized.sort_by_key(|item| item.part_number);
    normalized
}

fn encode_session_token(payload: &SessionTokenPayload) -> Result<String, UploadError> {
    let encoded = serde_json::to_vec(payload).map_err(|error| {
        UploadError::new(
            UploadErrorCode::Unknown,
            format!("failed to encode session token payload: {error}"),
            false,
        )
    })?;
    Ok(format!("p115:{}", URL_SAFE_NO_PAD.encode(encoded)))
}

fn decode_session_token(token: &str) -> Result<SessionTokenPayload, UploadError> {
    let encoded = token.trim().strip_prefix("p115:").ok_or_else(|| {
        UploadError::new(
            UploadErrorCode::SessionExpired,
            "invalid p115 session token format",
            false,
        )
    })?;
    let decoded = URL_SAFE_NO_PAD.decode(encoded).map_err(|error| {
        UploadError::new(
            UploadErrorCode::SessionExpired,
            format!("failed to decode p115 session token: {error}"),
            false,
        )
    })?;
    serde_json::from_slice::<SessionTokenPayload>(&decoded).map_err(|error| {
        UploadError::new(
            UploadErrorCode::SessionExpired,
            format!("failed to parse p115 session token payload: {error}"),
            false,
        )
    })
}

fn extract_provider_response_text(value: &Value, keys: &[&str]) -> Option<String> {
    for key in keys {
        if let Some(text) = value.get(*key).and_then(Value::as_str) {
            let trimmed = text.trim();
            if !trimmed.is_empty() {
                return Some(trimmed.to_string());
            }
        }
        if let Some(text) = value
            .get("data")
            .and_then(|record| record.get(*key))
            .and_then(Value::as_str)
        {
            let trimmed = text.trim();
            if !trimmed.is_empty() {
                return Some(trimmed.to_string());
            }
        }
    }
    None
}

fn normalize_optional_text(value: String) -> Option<String> {
    let trimmed = value.trim().to_string();
    if trimmed.is_empty() {
        None
    } else {
        Some(trimmed)
    }
}

fn normalize_base_url(value: &str) -> Result<String, UploadError> {
    let trimmed = value.trim().trim_end_matches('/').to_string();
    if trimmed.is_empty() {
        return Err(UploadError::new(
            UploadErrorCode::ProviderUnavailable,
            "p115 sidecar url is empty",
            false,
        ));
    }
    Ok(trimmed)
}

fn u64_to_u32(value: u64, field: &str) -> Result<u32, UploadError> {
    u32::try_from(value).map_err(|_| {
        UploadError::new(
            UploadErrorCode::Unknown,
            format!("{field} exceeds u32 range: {value}"),
            false,
        )
    })
}

fn env_text(key: &str) -> Option<String> {
    env::var(key)
        .ok()
        .map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
}

fn env_u16(key: &str) -> Option<u16> {
    env_text(key).and_then(|value| value.parse::<u16>().ok())
}

fn env_u64(key: &str) -> Option<u64> {
    env_text(key).and_then(|value| value.parse::<u64>().ok())
}

fn env_bool(key: &str) -> bool {
    env_text(key)
        .map(|value| matches!(value.to_lowercase().as_str(), "1" | "true" | "yes" | "on"))
        .unwrap_or(false)
}

fn env_path(key: &str) -> Option<PathBuf> {
    env_text(key).map(PathBuf::from)
}

fn default_sidecar_state_dir() -> PathBuf {
    if let Ok(value) = env::var("LOCALAPPDATA") {
        let path = PathBuf::from(value)
            .join("Mare")
            .join("sidecars")
            .join("p115-upload");
        return path;
    }
    std::env::temp_dir()
        .join("mare")
        .join("sidecars")
        .join("p115-upload")
}

fn detect_python_command() -> String {
    for candidate in ["python", "py"] {
        if command_exists(candidate) {
            return candidate.to_string();
        }
    }
    "python".to_string()
}

fn command_exists(command: &str) -> bool {
    Command::new(command)
        .arg("--version")
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .map(|status| status.success())
        .unwrap_or(false)
}

fn find_sidecar_script_path() -> Option<PathBuf> {
    let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    let candidates = vec![
        manifest_dir.parent().map(|value| {
            value
                .join("backend")
                .join("tools")
                .join("p115_upload_sidecar.py")
        }),
        Some(
            manifest_dir
                .join("..")
                .join("backend")
                .join("tools")
                .join("p115_upload_sidecar.py"),
        ),
        Some(
            PathBuf::from("backend")
                .join("tools")
                .join("p115_upload_sidecar.py"),
        ),
    ];

    for candidate in candidates.into_iter().flatten() {
        let path = candidate;
        if path.is_file() {
            return Some(path);
        }
    }
    None
}

fn find_python_libs_path() -> Option<PathBuf> {
    let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    let candidates = vec![
        manifest_dir
            .parent()
            .map(|value| value.join(".tools").join("pythonlibs")),
        Some(manifest_dir.join("..").join(".tools").join("pythonlibs")),
        Some(PathBuf::from(".tools").join("pythonlibs")),
    ];

    for candidate in candidates.into_iter().flatten() {
        if candidate.is_dir() {
            return Some(candidate);
        }
    }
    None
}

fn allocate_ephemeral_port() -> u16 {
    TcpListener::bind("127.0.0.1:0")
        .ok()
        .and_then(|listener| listener.local_addr().ok().map(|value| value.port()))
        .unwrap_or(19891)
}

#[cfg(test)]
mod tests {
    use std::{io::Write, process::Stdio};

    use super::*;

    fn start_mock_sidecar(token: &str) -> Option<(std::process::Child, String, PathBuf)> {
        let python_command = detect_python_command();
        if !command_exists(&python_command) {
            return None;
        }
        let script = find_sidecar_script_path()?;

        let state_dir =
            std::env::temp_dir().join(format!("mam-p115-provider-tests-{}", uuid::Uuid::new_v4()));
        let _ = fs::create_dir_all(&state_dir);
        let port = allocate_ephemeral_port();
        let base_url = format!("http://127.0.0.1:{port}");

        let mut command = Command::new(python_command);
        command
            .arg(&script)
            .arg("--host")
            .arg("127.0.0.1")
            .arg("--port")
            .arg(port.to_string())
            .arg("--state-dir")
            .arg(&state_dir)
            .arg("--token")
            .arg(token)
            .arg("--mock")
            .arg("--part-size")
            .arg("1024")
            .env("PYTHONIOENCODING", "utf-8")
            .env(ENV_SIDECAR_MOCK, "1")
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null());

        #[cfg(windows)]
        {
            use std::os::windows::process::CommandExt;
            const CREATE_NO_WINDOW: u32 = 0x0800_0000;
            command.creation_flags(CREATE_NO_WINDOW);
        }

        let child = command.spawn().ok()?;
        Some((child, base_url, state_dir))
    }

    #[tokio::test]
    #[ignore = "requires external python sidecar process; covered by backend/tools/tests"]
    async fn p115_provider_can_drive_mock_sidecar_upload_lifecycle() {
        let token = format!("token-{}", uuid::Uuid::new_v4());
        let Some((mut child, base_url, state_dir)) = start_mock_sidecar(&token) else {
            return;
        };

        let local_file_path =
            std::env::temp_dir().join(format!("mam-p115-file-{}.bin", uuid::Uuid::new_v4()));
        let mut file = fs::File::create(&local_file_path).expect("create local upload file");
        file.write_all(&vec![7_u8; 4096])
            .expect("write upload fixture");
        drop(file);

        let provider = P115UploadProvider::new(P115UploadProviderConfig {
            sidecar_base_url: Some(base_url),
            sidecar_token: Some(token),
            sidecar_host: "127.0.0.1".to_string(),
            sidecar_port: None,
            sidecar_state_dir: state_dir.clone(),
            sidecar_script_path: None,
            python_command: "python".to_string(),
            python_path: None,
            startup_timeout: Duration::from_secs(10),
            default_part_size: 1024,
            mock_mode: false,
        })
        .expect("initialize p115 provider");

        let mut metadata = BTreeMap::new();
        metadata.insert("credential".to_string(), "dummy-credential".to_string());
        metadata.insert("rootId".to_string(), "0".to_string());
        metadata.insert("appType".to_string(), "wechatmini".to_string());

        let create_response = provider
            .create_session(CreateUploadSessionRequest {
                job_id: "job-1".to_string(),
                local_path: local_file_path.display().to_string(),
                remote_path: "/测试目录/mock.bin".to_string(),
                file_size: 4096,
                part_size: 1024,
                content_hash: None,
                metadata,
            })
            .await
            .expect("create session");
        assert_eq!(create_response.total_parts, 4);

        let query_before = provider
            .query_uploaded_parts(QueryUploadedPartsRequest {
                job_id: "job-1".to_string(),
                session_id: create_response.session_id.clone(),
            })
            .await
            .expect("query parts before upload");
        assert!(query_before.parts.is_empty());

        let _ = provider
            .upload_part(UploadPartRequest {
                job_id: "job-1".to_string(),
                session_id: create_response.session_id.clone(),
                part_number: 1,
                offset: 0,
                bytes: vec![0_u8; 1024],
                checksum: None,
            })
            .await
            .expect("upload first part");

        let query_after = provider
            .query_uploaded_parts(QueryUploadedPartsRequest {
                job_id: "job-1".to_string(),
                session_id: create_response.session_id.clone(),
            })
            .await
            .expect("query parts after upload");
        assert_eq!(query_after.parts.len(), 1);

        for index in 2..=4_u32 {
            let _ = provider
                .upload_part(UploadPartRequest {
                    job_id: "job-1".to_string(),
                    session_id: create_response.session_id.clone(),
                    part_number: index,
                    offset: ((index - 1) * 1024) as u64,
                    bytes: vec![0_u8; 1024],
                    checksum: None,
                })
                .await
                .expect("upload remaining parts");
        }

        let complete_response = provider
            .complete_upload(CompleteUploadRequest {
                job_id: "job-1".to_string(),
                session_id: create_response.session_id.clone(),
            })
            .await
            .expect("complete upload");
        assert_eq!(
            complete_response.remote_path,
            "测试目录/mock.bin"
                .replace('\\', "/")
                .trim_start_matches('/')
        );

        let _ = provider
            .abort_upload(AbortUploadRequest {
                job_id: "job-1".to_string(),
                session_id: create_response.session_id.clone(),
                reason: Some("cleanup".to_string()),
            })
            .await;

        let _ = fs::remove_file(local_file_path);
        let _ = fs::remove_dir_all(&state_dir);
        let _ = child.kill();
        let _ = child.wait();
    }

    #[test]
    fn session_token_roundtrip_works() {
        let payload = SessionTokenPayload {
            job_id: "job-a".to_string(),
            local_path: "D:/a.bin".to_string(),
            remote_path: "x/y.bin".to_string(),
            resume_state_path: "D:/state/job-a.json".to_string(),
            credential: "cookie=demo".to_string(),
            app_type: "wechatmini".to_string(),
            root_id: "0".to_string(),
            part_size: 1024,
            parent_id: 0,
        };

        let token = encode_session_token(&payload).expect("encode token");
        let decoded = decode_session_token(&token).expect("decode token");
        assert_eq!(decoded.job_id, payload.job_id);
        assert_eq!(decoded.remote_path, payload.remote_path);
    }

    #[test]
    fn map_sidecar_error_code_matches_common_cases() {
        assert!(matches!(
            map_sidecar_error_code(StatusCode::UNAUTHORIZED, "auth_required"),
            UploadErrorCode::AuthExpired
        ));
        assert!(matches!(
            map_sidecar_error_code(StatusCode::TOO_MANY_REQUESTS, "rate_limited"),
            UploadErrorCode::RateLimited
        ));
        assert!(matches!(
            map_sidecar_error_code(StatusCode::CONFLICT, "session_not_found"),
            UploadErrorCode::SessionExpired
        ));
        assert!(matches!(
            map_sidecar_error_code(StatusCode::BAD_GATEWAY, "unknown"),
            UploadErrorCode::ProviderUnavailable
        ));
    }

    #[test]
    fn normalize_sidecar_parts_sorts_and_filters() {
        let parts = normalize_sidecar_parts(vec![
            SidecarPart {
                part_number: 2,
                size: 1024,
                etag: Some("etag-2".to_string()),
            },
            SidecarPart {
                part_number: 0,
                size: 999,
                etag: None,
            },
            SidecarPart {
                part_number: 1,
                size: 1024,
                etag: Some("etag-1".to_string()),
            },
        ]);

        assert_eq!(parts.len(), 2);
        assert_eq!(parts[0].part_number, 1);
        assert_eq!(parts[1].part_number, 2);
    }
}
