package store

import (
	"context"
	"database/sql"
	"fmt"
)

type LibrarySummary struct {
	AssetCount      int `json:"assetCount"`
	ReplicaCount    int `json:"replicaCount"`
	EndpointCount   int `json:"endpointCount"`
	ImportRuleCount int `json:"importRuleCount"`
	TaskCount       int `json:"taskCount"`
}

func (store *Store) SummarizeLibrary(ctx context.Context) (LibrarySummary, error) {
	return summarizeSQLiteDB(ctx, store.db)
}

func SummarizeSQLiteFile(ctx context.Context, path string) (LibrarySummary, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return LibrarySummary{}, fmt.Errorf("open sqlite database: %w", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(1)
	return summarizeSQLiteDB(ctx, db)
}

func summarizeSQLiteDB(ctx context.Context, db *sql.DB) (LibrarySummary, error) {
	return LibrarySummary{
		AssetCount:      countSQLiteTable(ctx, db, "assets"),
		ReplicaCount:    countSQLiteTable(ctx, db, "replicas"),
		EndpointCount:   countSQLiteTable(ctx, db, "storage_endpoints"),
		ImportRuleCount: countSQLiteTable(ctx, db, "import_rules"),
		TaskCount:       countSQLiteTable(ctx, db, "tasks"),
	}, nil
}

func countSQLiteTable(ctx context.Context, db *sql.DB, tableName string) int {
	existsRow := db.QueryRowContext(
		ctx,
		`SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		tableName,
	)

	var exists int
	if err := existsRow.Scan(&exists); err != nil || exists == 0 {
		return 0
	}

	countRow := db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(1) FROM %s`, tableName))
	var count int
	if err := countRow.Scan(&count); err != nil {
		return 0
	}

	return count
}
