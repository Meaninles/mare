CREATE TABLE IF NOT EXISTS asset_transcripts (
    asset_id TEXT PRIMARY KEY,
    transcript_text TEXT NOT NULL,
    language TEXT,
    source_version_id TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(asset_id) REFERENCES assets(id) ON DELETE CASCADE,
    FOREIGN KEY(source_version_id) REFERENCES replica_versions(id)
);

CREATE TABLE IF NOT EXISTS asset_search_documents (
    id TEXT PRIMARY KEY,
    asset_id TEXT NOT NULL,
    source_kind TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(asset_id, source_kind),
    FOREIGN KEY(asset_id) REFERENCES assets(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_asset_search_documents_asset_id ON asset_search_documents(asset_id);
CREATE INDEX IF NOT EXISTS idx_asset_search_documents_source_kind ON asset_search_documents(source_kind);

CREATE VIRTUAL TABLE IF NOT EXISTS asset_search_documents_fts USING fts5 (
    asset_id UNINDEXED,
    source_kind UNINDEXED,
    content,
    content = 'asset_search_documents',
    content_rowid = 'rowid',
    tokenize = 'unicode61 remove_diacritics 2',
    prefix = '2 3 4'
);

CREATE TRIGGER IF NOT EXISTS asset_search_documents_ai
AFTER INSERT ON asset_search_documents
BEGIN
    INSERT INTO asset_search_documents_fts(rowid, asset_id, source_kind, content)
    VALUES (new.rowid, new.asset_id, new.source_kind, COALESCE(new.content, ''));
END;

CREATE TRIGGER IF NOT EXISTS asset_search_documents_ad
AFTER DELETE ON asset_search_documents
BEGIN
    INSERT INTO asset_search_documents_fts(asset_search_documents_fts, rowid, asset_id, source_kind, content)
    VALUES ('delete', old.rowid, old.asset_id, old.source_kind, COALESCE(old.content, ''));
END;

CREATE TRIGGER IF NOT EXISTS asset_search_documents_au
AFTER UPDATE ON asset_search_documents
BEGIN
    INSERT INTO asset_search_documents_fts(asset_search_documents_fts, rowid, asset_id, source_kind, content)
    VALUES ('delete', old.rowid, old.asset_id, old.source_kind, COALESCE(old.content, ''));

    INSERT INTO asset_search_documents_fts(rowid, asset_id, source_kind, content)
    VALUES (new.rowid, new.asset_id, new.source_kind, COALESCE(new.content, ''));
END;

INSERT INTO asset_search_documents_fts(rowid, asset_id, source_kind, content)
SELECT rowid, asset_id, source_kind, COALESCE(content, '')
FROM asset_search_documents;

CREATE TABLE IF NOT EXISTS asset_semantic_embeddings (
    id TEXT PRIMARY KEY,
    asset_id TEXT NOT NULL,
    feature_kind TEXT NOT NULL,
    model_name TEXT NOT NULL,
    embedding_json TEXT NOT NULL,
    source_version_id TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(asset_id, feature_kind),
    FOREIGN KEY(asset_id) REFERENCES assets(id) ON DELETE CASCADE,
    FOREIGN KEY(source_version_id) REFERENCES replica_versions(id)
);

CREATE INDEX IF NOT EXISTS idx_asset_semantic_embeddings_asset_id ON asset_semantic_embeddings(asset_id);
CREATE INDEX IF NOT EXISTS idx_asset_semantic_embeddings_feature_kind ON asset_semantic_embeddings(feature_kind);
