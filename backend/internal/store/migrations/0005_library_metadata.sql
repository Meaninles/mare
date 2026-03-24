CREATE TABLE IF NOT EXISTS library_metadata (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    library_id TEXT NOT NULL,
    library_name TEXT NOT NULL,
    file_extension TEXT NOT NULL DEFAULT '.maredb',
    schema_family TEXT NOT NULL DEFAULT 'mare-library-v1',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

INSERT OR IGNORE INTO library_metadata (
    id,
    library_id,
    library_name,
    file_extension,
    schema_family,
    created_at,
    updated_at
)
VALUES (
    1,
    lower(hex(randomblob(16))),
    'Untitled Library',
    '.maredb',
    'mare-library-v1',
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now'),
    strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
);

ALTER TABLE storage_endpoints
    ADD COLUMN credential_ref TEXT NOT NULL DEFAULT '';

ALTER TABLE storage_endpoints
    ADD COLUMN credential_hint TEXT NOT NULL DEFAULT '';
