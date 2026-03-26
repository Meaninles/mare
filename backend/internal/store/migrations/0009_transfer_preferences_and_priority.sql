ALTER TABLE tasks ADD COLUMN priority INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS transfer_preferences (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    upload_concurrency INTEGER NOT NULL DEFAULT 1,
    download_concurrency INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
