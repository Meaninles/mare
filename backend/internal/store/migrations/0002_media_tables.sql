CREATE TABLE IF NOT EXISTS asset_previews (
    id TEXT PRIMARY KEY,
    asset_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    file_path TEXT NOT NULL,
    mime_type TEXT,
    width INTEGER,
    height INTEGER,
    source_version_id TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(asset_id) REFERENCES assets(id),
    FOREIGN KEY(source_version_id) REFERENCES replica_versions(id),
    UNIQUE(asset_id, kind)
);

CREATE INDEX IF NOT EXISTS idx_asset_previews_asset_id ON asset_previews(asset_id);

CREATE TABLE IF NOT EXISTS asset_media_metadata (
    asset_id TEXT PRIMARY KEY,
    duration_seconds REAL,
    codec_name TEXT,
    sample_rate_hz INTEGER,
    channel_count INTEGER,
    source_version_id TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(asset_id) REFERENCES assets(id),
    FOREIGN KEY(source_version_id) REFERENCES replica_versions(id)
);
