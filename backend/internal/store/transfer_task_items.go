package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (store *Store) CreateTransferTaskItems(ctx context.Context, items []TransferTaskItem) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transfer item transaction: %w", err)
	}

	for _, item := range items {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO transfer_task_items (
				id,
				task_id,
				item_index,
				group_key,
				direction,
				source_kind,
				source_endpoint_id,
				source_endpoint_type,
				source_identity_signature,
				source_label,
				source_path,
				target_endpoint_id,
				target_endpoint_type,
				target_label,
				target_path,
				asset_id,
				logical_path_key,
				display_name,
				media_type,
				status,
				phase,
				total_bytes,
				staged_bytes,
				committed_bytes,
				progress_percent,
				scan_revision,
				staging_path,
				target_temp_path,
				error_message,
				metadata_json,
				created_at,
				updated_at,
				started_at,
				finished_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			item.ID,
			item.TaskID,
			item.ItemIndex,
			item.GroupKey,
			item.Direction,
			item.SourceKind,
			item.SourceEndpointID,
			item.SourceEndpointType,
			item.SourceIdentitySignature,
			item.SourceLabel,
			item.SourcePath,
			item.TargetEndpointID,
			item.TargetEndpointType,
			item.TargetLabel,
			item.TargetPath,
			toNullableString(item.AssetID),
			item.LogicalPathKey,
			item.DisplayName,
			item.MediaType,
			item.Status,
			item.Phase,
			item.TotalBytes,
			item.StagedBytes,
			item.CommittedBytes,
			item.ProgressPercent,
			item.ScanRevision,
			item.StagingPath,
			item.TargetTempPath,
			toNullableString(item.ErrorMessage),
			item.MetadataJSON,
			item.CreatedAt.UTC().Format(timeLayout),
			item.UpdatedAt.UTC().Format(timeLayout),
			toNullableTime(item.StartedAt),
			toNullableTime(item.FinishedAt),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert transfer task item: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transfer item transaction: %w", err)
	}

	return nil
}

func (store *Store) GetTransferTaskItemByID(ctx context.Context, id string) (TransferTaskItem, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT
			id,
			task_id,
			item_index,
			group_key,
			direction,
			source_kind,
			source_endpoint_id,
			source_endpoint_type,
			source_identity_signature,
			source_label,
			source_path,
			target_endpoint_id,
			target_endpoint_type,
			target_label,
			target_path,
			asset_id,
			logical_path_key,
			display_name,
			media_type,
			status,
			phase,
			total_bytes,
			staged_bytes,
			committed_bytes,
			progress_percent,
			scan_revision,
			staging_path,
			target_temp_path,
			error_message,
			metadata_json,
			created_at,
			updated_at,
			started_at,
			finished_at
		FROM transfer_task_items
		WHERE id = ?`,
		id,
	)
	return scanTransferTaskItem(row)
}

func (store *Store) ListTransferTaskItemsByTaskID(ctx context.Context, taskID string) ([]TransferTaskItem, error) {
	rows, err := store.db.QueryContext(
		ctx,
		`SELECT
			id,
			task_id,
			item_index,
			group_key,
			direction,
			source_kind,
			source_endpoint_id,
			source_endpoint_type,
			source_identity_signature,
			source_label,
			source_path,
			target_endpoint_id,
			target_endpoint_type,
			target_label,
			target_path,
			asset_id,
			logical_path_key,
			display_name,
			media_type,
			status,
			phase,
			total_bytes,
			staged_bytes,
			committed_bytes,
			progress_percent,
			scan_revision,
			staging_path,
			target_temp_path,
			error_message,
			metadata_json,
			created_at,
			updated_at,
			started_at,
			finished_at
		FROM transfer_task_items
		WHERE task_id = ?
		ORDER BY item_index ASC, created_at ASC`,
		taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("list transfer task items: %w", err)
	}
	defer rows.Close()

	items := make([]TransferTaskItem, 0)
	for rows.Next() {
		item, scanErr := scanTransferTaskItem(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (store *Store) UpdateTransferTaskItem(ctx context.Context, item TransferTaskItem) error {
	_, err := store.db.ExecContext(
		ctx,
		`UPDATE transfer_task_items
		 SET
			group_key = ?,
			direction = ?,
			source_kind = ?,
			source_endpoint_id = ?,
			source_endpoint_type = ?,
			source_identity_signature = ?,
			source_label = ?,
			source_path = ?,
			target_endpoint_id = ?,
			target_endpoint_type = ?,
			target_label = ?,
			target_path = ?,
			asset_id = ?,
			logical_path_key = ?,
			display_name = ?,
			media_type = ?,
			status = ?,
			phase = ?,
			total_bytes = ?,
			staged_bytes = ?,
			committed_bytes = ?,
			progress_percent = ?,
			scan_revision = ?,
			staging_path = ?,
			target_temp_path = ?,
			error_message = ?,
			metadata_json = ?,
			updated_at = ?,
			started_at = ?,
			finished_at = ?
		 WHERE id = ?`,
		item.GroupKey,
		item.Direction,
		item.SourceKind,
		item.SourceEndpointID,
		item.SourceEndpointType,
		item.SourceIdentitySignature,
		item.SourceLabel,
		item.SourcePath,
		item.TargetEndpointID,
		item.TargetEndpointType,
		item.TargetLabel,
		item.TargetPath,
		toNullableString(item.AssetID),
		item.LogicalPathKey,
		item.DisplayName,
		item.MediaType,
		item.Status,
		item.Phase,
		item.TotalBytes,
		item.StagedBytes,
		item.CommittedBytes,
		item.ProgressPercent,
		item.ScanRevision,
		item.StagingPath,
		item.TargetTempPath,
		toNullableString(item.ErrorMessage),
		item.MetadataJSON,
		item.UpdatedAt.UTC().Format(timeLayout),
		toNullableTime(item.StartedAt),
		toNullableTime(item.FinishedAt),
		item.ID,
	)
	if err != nil {
		return fmt.Errorf("update transfer task item: %w", err)
	}

	return nil
}

func scanTransferTaskItem(scanner rowScanner) (TransferTaskItem, error) {
	var (
		item                   TransferTaskItem
		assetIDText            sql.NullString
		errorMessageText       sql.NullString
		createdAtText          string
		updatedAtText          string
		startedAtText          sql.NullString
		finishedAtText         sql.NullString
	)

	if err := scanner.Scan(
		&item.ID,
		&item.TaskID,
		&item.ItemIndex,
		&item.GroupKey,
		&item.Direction,
		&item.SourceKind,
		&item.SourceEndpointID,
		&item.SourceEndpointType,
		&item.SourceIdentitySignature,
		&item.SourceLabel,
		&item.SourcePath,
		&item.TargetEndpointID,
		&item.TargetEndpointType,
		&item.TargetLabel,
		&item.TargetPath,
		&assetIDText,
		&item.LogicalPathKey,
		&item.DisplayName,
		&item.MediaType,
		&item.Status,
		&item.Phase,
		&item.TotalBytes,
		&item.StagedBytes,
		&item.CommittedBytes,
		&item.ProgressPercent,
		&item.ScanRevision,
		&item.StagingPath,
		&item.TargetTempPath,
		&errorMessageText,
		&item.MetadataJSON,
		&createdAtText,
		&updatedAtText,
		&startedAtText,
		&finishedAtText,
	); err != nil {
		return TransferTaskItem{}, fmt.Errorf("scan transfer task item: %w", err)
	}

	createdAt, err := time.Parse(timeLayout, createdAtText)
	if err != nil {
		return TransferTaskItem{}, fmt.Errorf("parse transfer task item created_at: %w", err)
	}

	updatedAt, err := time.Parse(timeLayout, updatedAtText)
	if err != nil {
		return TransferTaskItem{}, fmt.Errorf("parse transfer task item updated_at: %w", err)
	}

	startedAt, err := parseNullableTime(startedAtText)
	if err != nil {
		return TransferTaskItem{}, fmt.Errorf("parse transfer task item started_at: %w", err)
	}

	finishedAt, err := parseNullableTime(finishedAtText)
	if err != nil {
		return TransferTaskItem{}, fmt.Errorf("parse transfer task item finished_at: %w", err)
	}

	item.AssetID = parseNullableString(assetIDText)
	item.ErrorMessage = parseNullableString(errorMessageText)
	item.CreatedAt = createdAt
	item.UpdatedAt = updatedAt
	item.StartedAt = startedAt
	item.FinishedAt = finishedAt
	return item, nil
}
