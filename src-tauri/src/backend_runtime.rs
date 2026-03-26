use std::{
    fs::{self, OpenOptions},
    io::{Read, Write},
    net::{TcpStream, ToSocketAddrs},
    path::{Path, PathBuf},
    process::{Child, Command, Stdio},
    sync::Mutex,
    thread,
    time::{Duration, Instant},
};

#[cfg(windows)]
use std::os::windows::process::CommandExt;

use tauri::{AppHandle, Manager};
use tracing::{info, warn};

use crate::error::AppError;

const BACKEND_HOST: &str = "127.0.0.1";
const BACKEND_PORT: u16 = 8080;
const BACKEND_READY_PATH: &str = "/readyz";
const BACKEND_STARTUP_TIMEOUT: Duration = Duration::from_secs(20);
const BACKEND_READY_POLL_INTERVAL: Duration = Duration::from_millis(250);

#[cfg(windows)]
const CREATE_NO_WINDOW: u32 = 0x0800_0000;

pub struct BackendRuntime {
    child: Mutex<Option<Child>>,
}

impl BackendRuntime {
    pub fn initialize(app: &AppHandle) -> Result<Self, AppError> {
        if backend_ready()? {
            info!("backend already available on localhost, reusing existing process");
            return Ok(Self {
                child: Mutex::new(None),
            });
        }

        let paths = BackendPaths::resolve(app)?;
        let backend_data_dir = app.path().app_data_dir()?.join("backend");
        let backend_log_dir = backend_data_dir.join("logs");
        fs::create_dir_all(&backend_data_dir)?;
        fs::create_dir_all(&backend_log_dir)?;

        let stdout_log = OpenOptions::new()
            .create(true)
            .append(true)
            .open(backend_log_dir.join("server.stdout.log"))?;
        let stderr_log = stdout_log.try_clone()?;

        let mut command = Command::new(&paths.backend_executable);
        command
            .current_dir(paths.backend_workdir())
            .env("APP_ENV", "production")
            .env("HTTP_HOST", BACKEND_HOST)
            .env("HTTP_PORT", BACKEND_PORT.to_string())
            .env(
                "CATALOG_DB_PATH",
                backend_data_dir.join("mam.db").display().to_string(),
            )
            .env(
                "LOG_FILE_PATH",
                backend_log_dir.join("backend.log").display().to_string(),
            )
            .env(
                "MAM_ALIST_BINARY",
                paths.alist_executable.display().to_string(),
            )
            .env(
                "MAM_ARIA2_BINARY",
                paths.aria2_executable.display().to_string(),
            )
            .stdout(Stdio::from(stdout_log))
            .stderr(Stdio::from(stderr_log));

        if let Some(cloud115_bridge_script) = paths.cloud115_bridge_script.as_ref() {
            command.env(
                "MAM_115_BRIDGE_SCRIPT",
                cloud115_bridge_script.display().to_string(),
            );
        }
        if let Some(cloud115_python_path) = paths.cloud115_python_path.as_ref() {
            command.env(
                "MAM_115_PYTHONPATH",
                cloud115_python_path.display().to_string(),
            );
        }

        #[cfg(windows)]
        command.creation_flags(CREATE_NO_WINDOW);

        let mut child = command.spawn()?;
        wait_for_backend_ready(&mut child, BACKEND_STARTUP_TIMEOUT)?;

        info!(
            backend = %paths.backend_executable.display(),
            alist = %paths.alist_executable.display(),
            aria2 = %paths.aria2_executable.display(),
            cloud115_bridge = ?paths
                .cloud115_bridge_script
                .as_ref()
                .map(|path| path.display().to_string()),
            cloud115_pythonpath = ?paths
                .cloud115_python_path
                .as_ref()
                .map(|path| path.display().to_string()),
            "managed backend started"
        );

        Ok(Self {
            child: Mutex::new(Some(child)),
        })
    }
}

impl Drop for BackendRuntime {
    fn drop(&mut self) {
        let Ok(mut guard) = self.child.lock() else {
            return;
        };
        let Some(child) = guard.as_mut() else {
            return;
        };

        match child.try_wait() {
            Ok(Some(_)) => {
                guard.take();
            }
            Ok(None) => {
                if let Err(error) = child.kill() {
                    warn!("failed to stop managed backend: {}", error);
                }
                let _ = child.wait();
                guard.take();
            }
            Err(error) => {
                warn!("failed to query managed backend status: {}", error);
            }
        }
    }
}

struct BackendPaths {
    backend_executable: PathBuf,
    alist_executable: PathBuf,
    aria2_executable: PathBuf,
    cloud115_bridge_script: Option<PathBuf>,
    cloud115_python_path: Option<PathBuf>,
}

impl BackendPaths {
    fn resolve(app: &AppHandle) -> Result<Self, AppError> {
        let resource_root = app.path().resource_dir().ok();
        let workspace_root = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .parent()
            .map(Path::to_path_buf)
            .ok_or_else(|| AppError::Message("unable to resolve workspace root".into()))?;

        Ok(Self {
            backend_executable: resolve_existing_path(
                &[
                    resource_root
                        .as_ref()
                        .map(|root| root.join("backend").join("server.exe")),
                    Some(workspace_root.join("backend").join("server.exe")),
                ],
                "backend server executable",
            )?,
            alist_executable: resolve_existing_path(
                &[
                    resource_root
                        .as_ref()
                        .map(|root| root.join("runtime").join("alist").join("alist.exe")),
                    Some(
                        workspace_root
                            .join(".tools")
                            .join("runtime")
                            .join("alist")
                            .join("extracted")
                            .join("alist.exe"),
                    ),
                ],
                "alist executable",
            )?,
            aria2_executable: resolve_existing_path(
                &[
                    resource_root
                        .as_ref()
                        .map(|root| root.join("runtime").join("aria2").join("aria2c.exe")),
                    Some(
                        workspace_root
                            .join(".tools")
                            .join("runtime")
                            .join("aria2")
                            .join("extracted")
                            .join("aria2-1.37.0-win-64bit-build1")
                            .join("aria2c.exe"),
                    ),
                ],
                "aria2 executable",
            )?,
            cloud115_bridge_script: resolve_optional_file_path(&[
                resource_root.as_ref().map(|root| {
                    root.join("backend")
                        .join("tools")
                        .join("cloud115_bridge.py")
                }),
                Some(
                    workspace_root
                        .join("backend")
                        .join("tools")
                        .join("cloud115_bridge.py"),
                ),
            ]),
            cloud115_python_path: resolve_optional_dir_path(&[
                resource_root.as_ref().map(|root| root.join("pythonlibs")),
                Some(workspace_root.join(".tools").join("pythonlibs")),
            ]),
        })
    }

    fn backend_workdir(&self) -> PathBuf {
        self.backend_executable
            .parent()
            .map(Path::to_path_buf)
            .unwrap_or_else(|| PathBuf::from("."))
    }
}

fn resolve_existing_path(candidates: &[Option<PathBuf>], label: &str) -> Result<PathBuf, AppError> {
    for candidate in candidates {
        let Some(candidate) = candidate.as_ref() else {
            continue;
        };
        if candidate.is_file() {
            return Ok(candidate.clone());
        }
    }

    let searched = candidates
        .iter()
        .filter_map(|candidate| candidate.as_ref())
        .map(|candidate| candidate.display().to_string())
        .collect::<Vec<_>>()
        .join(", ");
    Err(AppError::Message(format!(
        "unable to locate {} (checked: {})",
        label, searched
    )))
}

fn resolve_optional_file_path(candidates: &[Option<PathBuf>]) -> Option<PathBuf> {
    for candidate in candidates {
        let Some(candidate) = candidate.as_ref() else {
            continue;
        };
        if candidate.is_file() {
            return Some(candidate.clone());
        }
    }
    None
}

fn resolve_optional_dir_path(candidates: &[Option<PathBuf>]) -> Option<PathBuf> {
    for candidate in candidates {
        let Some(candidate) = candidate.as_ref() else {
            continue;
        };
        if candidate.is_dir() {
            return Some(candidate.clone());
        }
    }
    None
}

fn wait_for_backend_ready(child: &mut Child, timeout: Duration) -> Result<(), AppError> {
    let deadline = Instant::now() + timeout;
    while Instant::now() < deadline {
        if backend_ready()? {
            return Ok(());
        }

        if let Some(status) = child.try_wait()? {
            return Err(AppError::Message(format!(
                "backend exited before becoming ready: {}",
                status
            )));
        }

        thread::sleep(BACKEND_READY_POLL_INTERVAL);
    }

    Err(AppError::Message(format!(
        "backend did not become ready within {} seconds",
        timeout.as_secs()
    )))
}

fn backend_ready() -> Result<bool, AppError> {
    let address = (BACKEND_HOST, BACKEND_PORT)
        .to_socket_addrs()?
        .next()
        .ok_or_else(|| AppError::Message("unable to resolve backend address".into()))?;

    let mut stream = match TcpStream::connect_timeout(&address, Duration::from_millis(500)) {
        Ok(stream) => stream,
        Err(_) => return Ok(false),
    };

    stream.set_read_timeout(Some(Duration::from_secs(1)))?;
    stream.set_write_timeout(Some(Duration::from_secs(1)))?;
    stream.write_all(
        format!(
            "GET {} HTTP/1.1\r\nHost: {}:{}\r\nConnection: close\r\n\r\n",
            BACKEND_READY_PATH, BACKEND_HOST, BACKEND_PORT
        )
        .as_bytes(),
    )?;

    let mut response = String::new();
    stream.read_to_string(&mut response)?;
    Ok(response.contains("\"status\":\"ready\""))
}
