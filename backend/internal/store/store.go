package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type Store struct {
	db               *sql.DB
	migrationVersion string
}

func NewSQLiteStore(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("catalog db path is required")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	store := &Store{db: db}
	if err := store.runMigrations(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (store *Store) Close() error {
	return store.db.Close()
}

func (store *Store) MigrationVersion() string {
	return store.migrationVersion
}

func (store *Store) runMigrations(ctx context.Context) error {
	if _, err := store.db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("enable sqlite foreign keys: %w", err)
	}

	if _, err := store.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}

	sort.Strings(names)

	appliedVersions, err := store.listAppliedMigrationVersions(ctx)
	if err != nil {
		return err
	}

	for _, name := range names {
		if _, alreadyApplied := appliedVersions[name]; alreadyApplied {
			store.migrationVersion = name
			continue
		}

		contents, readErr := migrationFS.ReadFile("migrations/" + name)
		if readErr != nil {
			return fmt.Errorf("read migration %s: %w", name, readErr)
		}

		tx, beginErr := store.db.BeginTx(ctx, nil)
		if beginErr != nil {
			return fmt.Errorf("begin migration %s: %w", name, beginErr)
		}

		if _, execErr := tx.ExecContext(ctx, string(contents)); execErr != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, execErr)
		}

		if _, insertErr := tx.ExecContext(
			ctx,
			"INSERT OR REPLACE INTO schema_migrations(version, applied_at) VALUES (?, ?)",
			name,
			time.Now().UTC().Format(timeLayout),
		); insertErr != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, insertErr)
		}

		if commitErr := tx.Commit(); commitErr != nil {
			return fmt.Errorf("commit migration %s: %w", name, commitErr)
		}

		store.migrationVersion = name
	}

	if store.migrationVersion == "" {
		store.migrationVersion = "none"
	}

	return nil
}

func (store *Store) listAppliedMigrationVersions(ctx context.Context) (map[string]struct{}, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()

	appliedVersions := make(map[string]struct{})
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan applied migration version: %w", err)
		}
		appliedVersions[version] = struct{}{}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}

	return appliedVersions, nil
}
