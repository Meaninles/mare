package store

import (
	"context"
	"fmt"
)

func (store *Store) DeleteAllStorageEndpoints(ctx context.Context) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM storage_endpoints`); err != nil {
		return fmt.Errorf("delete all storage endpoints: %w", err)
	}
	return nil
}

func (store *Store) ClearCatalogSnapshot(ctx context.Context) error {
	statements := []string{
		`DELETE FROM asset_media_metadata`,
		`DELETE FROM asset_previews`,
		`DELETE FROM replicas`,
		`DELETE FROM assets`,
		`DELETE FROM replica_versions`,
	}
	for _, statement := range statements {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("clear catalog snapshot: %w", err)
		}
	}
	return nil
}

func (store *Store) ClearTasks(ctx context.Context) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM tasks`); err != nil {
		return fmt.Errorf("clear tasks: %w", err)
	}
	return nil
}
