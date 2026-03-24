CREATE TABLE IF NOT EXISTS import_rules (
    id TEXT PRIMARY KEY,
    rule_type TEXT NOT NULL,
    match_value TEXT NOT NULL,
    target_endpoint_ids TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_import_rules_rule_match
    ON import_rules(rule_type, match_value);
