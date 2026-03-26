package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (store *Store) GetTransferPreferences(ctx context.Context) (TransferPreferences, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT upload_concurrency, download_concurrency, created_at, updated_at
		 FROM transfer_preferences
		 WHERE id = 1`,
	)

	var (
		preferences         TransferPreferences
		createdAtText       string
		updatedAtText       string
	)

	if err := row.Scan(
		&preferences.UploadConcurrency,
		&preferences.DownloadConcurrency,
		&createdAtText,
		&updatedAtText,
	); err != nil {
		if err == sql.ErrNoRows {
			return TransferPreferences{}, sql.ErrNoRows
		}
		return TransferPreferences{}, fmt.Errorf("scan transfer preferences: %w", err)
	}

	createdAt, err := time.Parse(timeLayout, createdAtText)
	if err != nil {
		return TransferPreferences{}, fmt.Errorf("parse transfer preferences created_at: %w", err)
	}
	updatedAt, err := time.Parse(timeLayout, updatedAtText)
	if err != nil {
		return TransferPreferences{}, fmt.Errorf("parse transfer preferences updated_at: %w", err)
	}

	preferences.CreatedAt = createdAt
	preferences.UpdatedAt = updatedAt
	return preferences, nil
}

func (store *Store) UpsertTransferPreferences(ctx context.Context, preferences TransferPreferences) error {
	_, err := store.db.ExecContext(
		ctx,
		`INSERT INTO transfer_preferences (
			id,
			upload_concurrency,
			download_concurrency,
			created_at,
			updated_at
		)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			upload_concurrency = excluded.upload_concurrency,
			download_concurrency = excluded.download_concurrency,
			updated_at = excluded.updated_at`,
		preferences.UploadConcurrency,
		preferences.DownloadConcurrency,
		preferences.CreatedAt.UTC().Format(timeLayout),
		preferences.UpdatedAt.UTC().Format(timeLayout),
	)
	if err != nil {
		return fmt.Errorf("upsert transfer preferences: %w", err)
	}
	return nil
}
