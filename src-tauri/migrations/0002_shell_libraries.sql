CREATE TABLE IF NOT EXISTS libraries (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    path TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_opened_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_libraries_last_opened_at
ON libraries(last_opened_at DESC, updated_at DESC);

INSERT INTO app_metadata (key, value, updated_at)
VALUES ('schema_version', '0002_shell_libraries', CURRENT_TIMESTAMP)
ON CONFLICT(key) DO UPDATE SET
    value = excluded.value,
    updated_at = CURRENT_TIMESTAMP;
