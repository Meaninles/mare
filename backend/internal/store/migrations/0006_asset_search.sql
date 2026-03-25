CREATE VIRTUAL TABLE IF NOT EXISTS asset_search USING fts5 (
    asset_id UNINDEXED,
    display_name,
    logical_path_key,
    tokenize = 'unicode61 remove_diacritics 2',
    prefix = '2 3 4'
);

CREATE TRIGGER IF NOT EXISTS assets_search_ai
AFTER INSERT ON assets
BEGIN
    INSERT INTO asset_search(rowid, asset_id, display_name, logical_path_key)
    VALUES (
        new.rowid,
        new.id,
        COALESCE(new.display_name, ''),
        COALESCE(new.logical_path_key, '')
    );
END;

CREATE TRIGGER IF NOT EXISTS assets_search_ad
AFTER DELETE ON assets
BEGIN
    INSERT INTO asset_search(asset_search, rowid, asset_id, display_name, logical_path_key)
    VALUES (
        'delete',
        old.rowid,
        old.id,
        COALESCE(old.display_name, ''),
        COALESCE(old.logical_path_key, '')
    );
END;

CREATE TRIGGER IF NOT EXISTS assets_search_au
AFTER UPDATE ON assets
BEGIN
    INSERT INTO asset_search(asset_search, rowid, asset_id, display_name, logical_path_key)
    VALUES (
        'delete',
        old.rowid,
        old.id,
        COALESCE(old.display_name, ''),
        COALESCE(old.logical_path_key, '')
    );

    INSERT INTO asset_search(rowid, asset_id, display_name, logical_path_key)
    VALUES (
        new.rowid,
        new.id,
        COALESCE(new.display_name, ''),
        COALESCE(new.logical_path_key, '')
    );
END;

INSERT INTO asset_search(rowid, asset_id, display_name, logical_path_key)
SELECT
    rowid,
    id,
    COALESCE(display_name, ''),
    COALESCE(logical_path_key, '')
FROM assets;
