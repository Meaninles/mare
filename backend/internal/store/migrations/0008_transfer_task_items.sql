CREATE TABLE IF NOT EXISTS transfer_task_items (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    item_index INTEGER NOT NULL,
    group_key TEXT NOT NULL DEFAULT '',
    direction TEXT NOT NULL DEFAULT '',
    source_kind TEXT NOT NULL DEFAULT '',
    source_endpoint_id TEXT NOT NULL DEFAULT '',
    source_endpoint_type TEXT NOT NULL DEFAULT '',
    source_identity_signature TEXT NOT NULL DEFAULT '',
    source_label TEXT NOT NULL DEFAULT '',
    source_path TEXT NOT NULL,
    target_endpoint_id TEXT NOT NULL DEFAULT '',
    target_endpoint_type TEXT NOT NULL DEFAULT '',
    target_label TEXT NOT NULL DEFAULT '',
    target_path TEXT NOT NULL,
    asset_id TEXT,
    logical_path_key TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    media_type TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    phase TEXT NOT NULL DEFAULT '',
    total_bytes INTEGER NOT NULL DEFAULT 0,
    staged_bytes INTEGER NOT NULL DEFAULT 0,
    committed_bytes INTEGER NOT NULL DEFAULT 0,
    progress_percent INTEGER NOT NULL DEFAULT 0,
    scan_revision TEXT NOT NULL DEFAULT '',
    staging_path TEXT NOT NULL DEFAULT '',
    target_temp_path TEXT NOT NULL DEFAULT '',
    error_message TEXT,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    FOREIGN KEY(asset_id) REFERENCES assets(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_transfer_task_items_task_id
    ON transfer_task_items(task_id, item_index ASC);

CREATE INDEX IF NOT EXISTS idx_transfer_task_items_status
    ON transfer_task_items(status);
