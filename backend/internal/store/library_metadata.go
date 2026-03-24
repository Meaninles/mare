package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (store *Store) GetLibraryMetadata(ctx context.Context) (LibraryMetadata, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT library_id, library_name, file_extension, schema_family, created_at, updated_at
		 FROM library_metadata
		 WHERE id = 1`,
	)

	var (
		metadata      LibraryMetadata
		createdAtText string
		updatedAtText string
	)

	if err := row.Scan(
		&metadata.LibraryID,
		&metadata.LibraryName,
		&metadata.FileExtension,
		&metadata.SchemaFamily,
		&createdAtText,
		&updatedAtText,
	); err != nil {
		if err == sql.ErrNoRows {
			return LibraryMetadata{}, fmt.Errorf("library metadata not initialized: %w", err)
		}
		return LibraryMetadata{}, fmt.Errorf("scan library metadata: %w", err)
	}

	createdAt, err := time.Parse(timeLayout, createdAtText)
	if err != nil {
		return LibraryMetadata{}, fmt.Errorf("parse library metadata created_at: %w", err)
	}

	updatedAt, err := time.Parse(timeLayout, updatedAtText)
	if err != nil {
		return LibraryMetadata{}, fmt.Errorf("parse library metadata updated_at: %w", err)
	}

	metadata.CreatedAt = createdAt
	metadata.UpdatedAt = updatedAt
	return metadata, nil
}

func (store *Store) UpsertLibraryMetadata(ctx context.Context, metadata LibraryMetadata) error {
	_, err := store.db.ExecContext(
		ctx,
		`INSERT INTO library_metadata (
			id,
			library_id,
			library_name,
			file_extension,
			schema_family,
			created_at,
			updated_at
		)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			library_id = excluded.library_id,
			library_name = excluded.library_name,
			file_extension = excluded.file_extension,
			schema_family = excluded.schema_family,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at`,
		metadata.LibraryID,
		metadata.LibraryName,
		metadata.FileExtension,
		metadata.SchemaFamily,
		metadata.CreatedAt.UTC().Format(timeLayout),
		metadata.UpdatedAt.UTC().Format(timeLayout),
	)
	if err != nil {
		return fmt.Errorf("upsert library metadata: %w", err)
	}
	return nil
}
