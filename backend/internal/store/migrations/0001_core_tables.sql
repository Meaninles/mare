CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS storage_endpoints (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    endpoint_type TEXT NOT NULL,
    root_path TEXT NOT NULL,
    role_mode TEXT NOT NULL,
    identity_signature TEXT NOT NULL,
    availability_status TEXT NOT NULL,
    connection_config TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS assets (
    id TEXT PRIMARY KEY,
    logical_path_key TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    media_type TEXT NOT NULL,
    asset_status TEXT NOT NULL,
    primary_timestamp TEXT,
    primary_thumbnail_id TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS replica_versions (
    id TEXT PRIMARY KEY,
    size INTEGER NOT NULL,
    mtime TEXT,
    ctime TEXT,
    checksum_quick TEXT,
    checksum_full TEXT,
    media_signature TEXT,
    scan_revision TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS replicas (
    id TEXT PRIMARY KEY,
    asset_id TEXT NOT NULL,
    endpoint_id TEXT NOT NULL,
    physical_path TEXT NOT NULL,
    replica_status TEXT NOT NULL,
    exists_flag INTEGER NOT NULL,
    version_id TEXT,
    last_seen_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(asset_id) REFERENCES assets(id),
    FOREIGN KEY(endpoint_id) REFERENCES storage_endpoints(id),
    FOREIGN KEY(version_id) REFERENCES replica_versions(id)
);

CREATE INDEX IF NOT EXISTS idx_replicas_asset_id ON replicas(asset_id);
CREATE INDEX IF NOT EXISTS idx_replicas_endpoint_id ON replicas(endpoint_id);

CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    task_type TEXT NOT NULL,
    status TEXT NOT NULL,
    payload TEXT NOT NULL,
    result_summary TEXT,
    error_message TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
