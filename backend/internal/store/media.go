package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (store *Store) CreateAssetPreview(ctx context.Context, preview AssetPreview) error {
	_, err := store.db.ExecContext(
		ctx,
		`INSERT INTO asset_previews
		(id, asset_id, kind, file_path, mime_type, width, height, source_version_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		preview.ID,
		preview.AssetID,
		preview.Kind,
		preview.FilePath,
		toNullableString(preview.MIMEType),
		toNullableInt(preview.Width),
		toNullableInt(preview.Height),
		toNullableString(preview.SourceVersionID),
		preview.CreatedAt.UTC().Format(timeLayout),
		preview.UpdatedAt.UTC().Format(timeLayout),
	)
	if err != nil {
		return fmt.Errorf("insert asset preview: %w", err)
	}
	return nil
}

func (store *Store) UpdateAssetPreview(ctx context.Context, preview AssetPreview) error {
	_, err := store.db.ExecContext(
		ctx,
		`UPDATE asset_previews
		 SET file_path = ?, mime_type = ?, width = ?, height = ?, source_version_id = ?, updated_at = ?
		 WHERE id = ?`,
		preview.FilePath,
		toNullableString(preview.MIMEType),
		toNullableInt(preview.Width),
		toNullableInt(preview.Height),
		toNullableString(preview.SourceVersionID),
		preview.UpdatedAt.UTC().Format(timeLayout),
		preview.ID,
	)
	if err != nil {
		return fmt.Errorf("update asset preview: %w", err)
	}
	return nil
}

func (store *Store) GetAssetPreviewByID(ctx context.Context, id string) (AssetPreview, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT id, asset_id, kind, file_path, mime_type, width, height, source_version_id, created_at, updated_at
		 FROM asset_previews WHERE id = ?`,
		id,
	)
	return scanAssetPreview(row)
}

func (store *Store) GetAssetPreviewByAssetAndKind(ctx context.Context, assetID, kind string) (AssetPreview, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT id, asset_id, kind, file_path, mime_type, width, height, source_version_id, created_at, updated_at
		 FROM asset_previews WHERE asset_id = ? AND kind = ? LIMIT 1`,
		assetID,
		kind,
	)
	return scanAssetPreview(row)
}

func (store *Store) SaveAssetMediaMetadata(ctx context.Context, metadata AssetMediaMetadata) error {
	existing, err := store.GetAssetMediaMetadataByAssetID(ctx, metadata.AssetID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if errors.Is(err, sql.ErrNoRows) {
		_, execErr := store.db.ExecContext(
			ctx,
			`INSERT INTO asset_media_metadata
			(asset_id, duration_seconds, codec_name, sample_rate_hz, channel_count, source_version_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			metadata.AssetID,
			toNullableFloat(metadata.DurationSeconds),
			toNullableString(metadata.CodecName),
			toNullableInt(metadata.SampleRateHz),
			toNullableInt(metadata.ChannelCount),
			toNullableString(metadata.SourceVersionID),
			metadata.CreatedAt.UTC().Format(timeLayout),
			metadata.UpdatedAt.UTC().Format(timeLayout),
		)
		if execErr != nil {
			return fmt.Errorf("insert asset media metadata: %w", execErr)
		}
		return nil
	}

	existing.DurationSeconds = metadata.DurationSeconds
	existing.CodecName = metadata.CodecName
	existing.SampleRateHz = metadata.SampleRateHz
	existing.ChannelCount = metadata.ChannelCount
	existing.SourceVersionID = metadata.SourceVersionID
	existing.UpdatedAt = metadata.UpdatedAt

	_, execErr := store.db.ExecContext(
		ctx,
		`UPDATE asset_media_metadata
		 SET duration_seconds = ?, codec_name = ?, sample_rate_hz = ?, channel_count = ?, source_version_id = ?, updated_at = ?
		 WHERE asset_id = ?`,
		toNullableFloat(existing.DurationSeconds),
		toNullableString(existing.CodecName),
		toNullableInt(existing.SampleRateHz),
		toNullableInt(existing.ChannelCount),
		toNullableString(existing.SourceVersionID),
		existing.UpdatedAt.UTC().Format(timeLayout),
		existing.AssetID,
	)
	if execErr != nil {
		return fmt.Errorf("update asset media metadata: %w", execErr)
	}

	return nil
}

func (store *Store) GetAssetMediaMetadataByAssetID(ctx context.Context, assetID string) (AssetMediaMetadata, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT asset_id, duration_seconds, codec_name, sample_rate_hz, channel_count, source_version_id, created_at, updated_at
		 FROM asset_media_metadata WHERE asset_id = ?`,
		assetID,
	)
	return scanAssetMediaMetadata(row)
}

func scanAssetPreview(scanner rowScanner) (AssetPreview, error) {
	var (
		preview             AssetPreview
		mimeTypeText        sql.NullString
		widthValue          sql.NullInt64
		heightValue         sql.NullInt64
		sourceVersionIDText sql.NullString
		createdAtText       string
		updatedAtText       string
	)

	if err := scanner.Scan(
		&preview.ID,
		&preview.AssetID,
		&preview.Kind,
		&preview.FilePath,
		&mimeTypeText,
		&widthValue,
		&heightValue,
		&sourceVersionIDText,
		&createdAtText,
		&updatedAtText,
	); err != nil {
		return AssetPreview{}, fmt.Errorf("scan asset preview: %w", err)
	}

	createdAt, err := time.Parse(timeLayout, createdAtText)
	if err != nil {
		return AssetPreview{}, fmt.Errorf("parse asset preview created_at: %w", err)
	}

	updatedAt, err := time.Parse(timeLayout, updatedAtText)
	if err != nil {
		return AssetPreview{}, fmt.Errorf("parse asset preview updated_at: %w", err)
	}

	preview.MIMEType = parseNullableString(mimeTypeText)
	preview.Width = parseNullableInt(widthValue)
	preview.Height = parseNullableInt(heightValue)
	preview.SourceVersionID = parseNullableString(sourceVersionIDText)
	preview.CreatedAt = createdAt
	preview.UpdatedAt = updatedAt

	return preview, nil
}

func scanAssetMediaMetadata(scanner rowScanner) (AssetMediaMetadata, error) {
	var (
		metadata            AssetMediaMetadata
		durationValue       sql.NullFloat64
		codecNameText       sql.NullString
		sampleRateValue     sql.NullInt64
		channelCountValue   sql.NullInt64
		sourceVersionIDText sql.NullString
		createdAtText       string
		updatedAtText       string
	)

	if err := scanner.Scan(
		&metadata.AssetID,
		&durationValue,
		&codecNameText,
		&sampleRateValue,
		&channelCountValue,
		&sourceVersionIDText,
		&createdAtText,
		&updatedAtText,
	); err != nil {
		return AssetMediaMetadata{}, fmt.Errorf("scan asset media metadata: %w", err)
	}

	createdAt, err := time.Parse(timeLayout, createdAtText)
	if err != nil {
		return AssetMediaMetadata{}, fmt.Errorf("parse asset media metadata created_at: %w", err)
	}

	updatedAt, err := time.Parse(timeLayout, updatedAtText)
	if err != nil {
		return AssetMediaMetadata{}, fmt.Errorf("parse asset media metadata updated_at: %w", err)
	}

	metadata.DurationSeconds = parseNullableFloat(durationValue)
	metadata.CodecName = parseNullableString(codecNameText)
	metadata.SampleRateHz = parseNullableInt(sampleRateValue)
	metadata.ChannelCount = parseNullableInt(channelCountValue)
	metadata.SourceVersionID = parseNullableString(sourceVersionIDText)
	metadata.CreatedAt = createdAt
	metadata.UpdatedAt = updatedAt

	return metadata, nil
}
