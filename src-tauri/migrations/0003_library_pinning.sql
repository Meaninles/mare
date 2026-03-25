ALTER TABLE libraries
ADD COLUMN is_pinned INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_libraries_pinned_recent
ON libraries(is_pinned DESC, last_opened_at DESC, updated_at DESC);

INSERT INTO app_metadata (key, value, updated_at)
VALUES ('schema_version', '0003_library_pinning', CURRENT_TIMESTAMP)
ON CONFLICT(key) DO UPDATE SET
    value = excluded.value,
    updated_at = CURRENT_TIMESTAMP;
