CREATE TABLE IF NOT EXISTS upload_jobs (
    id TEXT PRIMARY KEY,
    provider_key TEXT NOT NULL,
    endpoint_id TEXT,
    local_path TEXT NOT NULL,
    remote_path TEXT NOT NULL,
    file_size INTEGER NOT NULL CHECK(file_size >= 0),
    part_size INTEGER NOT NULL CHECK(part_size > 0),
    total_parts INTEGER NOT NULL CHECK(total_parts >= 0),
    uploaded_bytes INTEGER NOT NULL DEFAULT 0 CHECK(uploaded_bytes >= 0),
    uploaded_parts INTEGER NOT NULL DEFAULT 0 CHECK(uploaded_parts >= 0),
    status TEXT NOT NULL,
    retry_count INTEGER NOT NULL DEFAULT 0 CHECK(retry_count >= 0),
    max_retries INTEGER NOT NULL DEFAULT 8 CHECK(max_retries >= 0),
    next_retry_at TEXT,
    last_error_code TEXT,
    last_error_message TEXT,
    session_id TEXT,
    content_hash TEXT,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TEXT,
    finished_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_upload_jobs_status_updated
ON upload_jobs(status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_upload_jobs_provider_status
ON upload_jobs(provider_key, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS upload_sessions (
    job_id TEXT PRIMARY KEY,
    provider_key TEXT NOT NULL,
    provider_upload_id TEXT NOT NULL,
    remote_path TEXT NOT NULL,
    part_size INTEGER NOT NULL CHECK(part_size > 0),
    total_parts INTEGER NOT NULL CHECK(total_parts >= 0),
    extra_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(job_id) REFERENCES upload_jobs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS upload_parts (
    job_id TEXT NOT NULL,
    part_number INTEGER NOT NULL CHECK(part_number > 0),
    start_offset INTEGER NOT NULL CHECK(start_offset >= 0),
    end_offset INTEGER NOT NULL CHECK(end_offset >= start_offset),
    part_size INTEGER NOT NULL CHECK(part_size >= 0),
    checksum TEXT,
    etag TEXT,
    status TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(job_id, part_number),
    FOREIGN KEY(job_id) REFERENCES upload_jobs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_upload_parts_job_status
ON upload_parts(job_id, status, part_number);

CREATE TABLE IF NOT EXISTS upload_attempts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL,
    attempt_no INTEGER NOT NULL CHECK(attempt_no > 0),
    status TEXT NOT NULL,
    error_code TEXT,
    error_message TEXT,
    started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at TEXT,
    FOREIGN KEY(job_id) REFERENCES upload_jobs(id) ON DELETE CASCADE,
    UNIQUE(job_id, attempt_no)
);

CREATE INDEX IF NOT EXISTS idx_upload_attempts_job_attempt
ON upload_attempts(job_id, attempt_no DESC);

INSERT INTO app_metadata (key, value, updated_at)
VALUES ('schema_version', '0004_upload_jobs', CURRENT_TIMESTAMP)
ON CONFLICT(key) DO UPDATE SET
    value = excluded.value,
    updated_at = CURRENT_TIMESTAMP;
