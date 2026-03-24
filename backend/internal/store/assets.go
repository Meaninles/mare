package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func (store *Store) CreateAsset(ctx context.Context, asset Asset) error {
	_, err := store.db.ExecContext(
		ctx,
		`INSERT INTO assets
		(id, logical_path_key, display_name, media_type, asset_status, primary_timestamp, primary_thumbnail_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		asset.ID,
		asset.LogicalPathKey,
		asset.DisplayName,
		asset.MediaType,
		asset.AssetStatus,
		toNullableTime(asset.PrimaryTimestamp),
		toNullableString(asset.PrimaryThumbnailID),
		asset.CreatedAt.UTC().Format(timeLayout),
		asset.UpdatedAt.UTC().Format(timeLayout),
	)
	if err != nil {
		return fmt.Errorf("insert asset: %w", err)
	}
	return nil
}

func (store *Store) GetAssetByID(ctx context.Context, id string) (Asset, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT id, logical_path_key, display_name, media_type, asset_status, primary_timestamp, primary_thumbnail_id, created_at, updated_at
		 FROM assets WHERE id = ?`,
		id,
	)
	return scanAsset(row)
}

func (store *Store) GetAssetByLogicalPathKey(ctx context.Context, logicalPathKey string) (Asset, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT id, logical_path_key, display_name, media_type, asset_status, primary_timestamp, primary_thumbnail_id, created_at, updated_at
		 FROM assets WHERE logical_path_key = ?`,
		logicalPathKey,
	)
	return scanAsset(row)
}

func (store *Store) ListAssets(ctx context.Context, limit, offset int) ([]Asset, error) {
	rows, err := store.db.QueryContext(
		ctx,
		`SELECT id, logical_path_key, display_name, media_type, asset_status, primary_timestamp, primary_thumbnail_id, created_at, updated_at
		 FROM assets
		 WHERE LOWER(COALESCE(asset_status, '')) <> 'deleted'
		 ORDER BY created_at DESC
		 LIMIT ? OFFSET ?`,
		limit,
		offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}
	defer rows.Close()

	var assets []Asset
	for rows.Next() {
		asset, scanErr := scanAsset(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		assets = append(assets, asset)
	}

	return assets, rows.Err()
}

func (store *Store) UpdateAsset(ctx context.Context, asset Asset) error {
	_, err := store.db.ExecContext(
		ctx,
		`UPDATE assets
		 SET logical_path_key = ?, display_name = ?, media_type = ?, asset_status = ?, primary_timestamp = ?, primary_thumbnail_id = ?, updated_at = ?
		 WHERE id = ?`,
		asset.LogicalPathKey,
		asset.DisplayName,
		asset.MediaType,
		asset.AssetStatus,
		toNullableTime(asset.PrimaryTimestamp),
		toNullableString(asset.PrimaryThumbnailID),
		asset.UpdatedAt.UTC().Format(timeLayout),
		asset.ID,
	)
	if err != nil {
		return fmt.Errorf("update asset: %w", err)
	}
	return nil
}

func (store *Store) UpdateAssetStatus(ctx context.Context, id, status string, updatedAt time.Time) error {
	_, err := store.db.ExecContext(
		ctx,
		`UPDATE assets SET asset_status = ?, updated_at = ? WHERE id = ?`,
		status,
		updatedAt.UTC().Format(timeLayout),
		id,
	)
	if err != nil {
		return fmt.Errorf("update asset status: %w", err)
	}
	return nil
}

func scanAsset(scanner rowScanner) (Asset, error) {
	var (
		asset                Asset
		primaryTimestampText sql.NullString
		primaryThumbnailID   sql.NullString
		createdAtText        string
		updatedAtText        string
	)

	if err := scanner.Scan(
		&asset.ID,
		&asset.LogicalPathKey,
		&asset.DisplayName,
		&asset.MediaType,
		&asset.AssetStatus,
		&primaryTimestampText,
		&primaryThumbnailID,
		&createdAtText,
		&updatedAtText,
	); err != nil {
		return Asset{}, fmt.Errorf("scan asset: %w", err)
	}

	primaryTimestamp, err := parseNullableTime(primaryTimestampText)
	if err != nil {
		return Asset{}, fmt.Errorf("parse asset primary timestamp: %w", err)
	}

	createdAt, err := time.Parse(timeLayout, createdAtText)
	if err != nil {
		return Asset{}, fmt.Errorf("parse asset created_at: %w", err)
	}

	updatedAt, err := time.Parse(timeLayout, updatedAtText)
	if err != nil {
		return Asset{}, fmt.Errorf("parse asset updated_at: %w", err)
	}

	asset.PrimaryTimestamp = primaryTimestamp
	asset.PrimaryThumbnailID = parseNullableString(primaryThumbnailID)
	asset.CreatedAt = createdAt
	asset.UpdatedAt = updatedAt
	return asset, nil
}
