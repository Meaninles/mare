package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (store *Store) CreateReplicaVersion(ctx context.Context, version ReplicaVersion) error {
	_, err := store.db.ExecContext(
		ctx,
		`INSERT INTO replica_versions
		(id, size, mtime, ctime, checksum_quick, checksum_full, media_signature, scan_revision, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		version.ID,
		version.Size,
		toNullableTime(version.MTime),
		toNullableTime(version.CTime),
		toNullableString(version.ChecksumQuick),
		toNullableString(version.ChecksumFull),
		toNullableString(version.MediaSignature),
		toNullableString(version.ScanRevision),
		version.CreatedAt.UTC().Format(timeLayout),
	)
	if err != nil {
		return fmt.Errorf("insert replica version: %w", err)
	}
	return nil
}

func (store *Store) CreateReplica(ctx context.Context, replica Replica) error {
	_, err := store.db.ExecContext(
		ctx,
		`INSERT INTO replicas
		(id, asset_id, endpoint_id, physical_path, replica_status, exists_flag, version_id, last_seen_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		replica.ID,
		replica.AssetID,
		replica.EndpointID,
		replica.PhysicalPath,
		replica.ReplicaStatus,
		toSQLiteBool(replica.ExistsFlag),
		toNullableString(replica.VersionID),
		toNullableTime(replica.LastSeenAt),
		replica.CreatedAt.UTC().Format(timeLayout),
		replica.UpdatedAt.UTC().Format(timeLayout),
	)
	if err != nil {
		return fmt.Errorf("insert replica: %w", err)
	}
	return nil
}

func (store *Store) GetReplicaVersionByID(ctx context.Context, id string) (ReplicaVersion, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT id, size, mtime, ctime, checksum_quick, checksum_full, media_signature, scan_revision, created_at
		 FROM replica_versions WHERE id = ?`,
		id,
	)
	return scanReplicaVersion(row)
}

func (store *Store) GetReplicaByAssetAndEndpoint(ctx context.Context, assetID, endpointID string) (Replica, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT id, asset_id, endpoint_id, physical_path, replica_status, exists_flag, version_id, last_seen_at, created_at, updated_at
		 FROM replicas WHERE asset_id = ? AND endpoint_id = ? LIMIT 1`,
		assetID,
		endpointID,
	)
	return scanReplica(row)
}

func (store *Store) UpdateReplica(ctx context.Context, replica Replica) error {
	_, err := store.db.ExecContext(
		ctx,
		`UPDATE replicas
		 SET asset_id = ?, endpoint_id = ?, physical_path = ?, replica_status = ?, exists_flag = ?, version_id = ?, last_seen_at = ?, updated_at = ?
		 WHERE id = ?`,
		replica.AssetID,
		replica.EndpointID,
		replica.PhysicalPath,
		replica.ReplicaStatus,
		toSQLiteBool(replica.ExistsFlag),
		toNullableString(replica.VersionID),
		toNullableTime(replica.LastSeenAt),
		replica.UpdatedAt.UTC().Format(timeLayout),
		replica.ID,
	)
	if err != nil {
		return fmt.Errorf("update replica: %w", err)
	}
	return nil
}

func (store *Store) ListReplicasByAssetID(ctx context.Context, assetID string) ([]Replica, error) {
	rows, err := store.db.QueryContext(
		ctx,
		`SELECT id, asset_id, endpoint_id, physical_path, replica_status, exists_flag, version_id, last_seen_at, created_at, updated_at
		 FROM replicas WHERE asset_id = ? ORDER BY created_at DESC`,
		assetID,
	)
	if err != nil {
		return nil, fmt.Errorf("list replicas by asset: %w", err)
	}
	defer rows.Close()

	var replicas []Replica
	for rows.Next() {
		replica, scanErr := scanReplica(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		replicas = append(replicas, replica)
	}

	return replicas, rows.Err()
}

func (store *Store) ListReplicasByEndpointID(ctx context.Context, endpointID string) ([]Replica, error) {
	rows, err := store.db.QueryContext(
		ctx,
		`SELECT id, asset_id, endpoint_id, physical_path, replica_status, exists_flag, version_id, last_seen_at, created_at, updated_at
		 FROM replicas WHERE endpoint_id = ? ORDER BY created_at DESC`,
		endpointID,
	)
	if err != nil {
		return nil, fmt.Errorf("list replicas by endpoint: %w", err)
	}
	defer rows.Close()

	var replicas []Replica
	for rows.Next() {
		replica, scanErr := scanReplica(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		replicas = append(replicas, replica)
	}

	return replicas, rows.Err()
}

func (store *Store) DeleteReplicasByEndpointID(ctx context.Context, endpointID string) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM replicas WHERE endpoint_id = ?`, endpointID); err != nil {
		return fmt.Errorf("delete replicas by endpoint: %w", err)
	}
	return nil
}

func scanReplicaVersion(scanner rowScanner) (ReplicaVersion, error) {
	var (
		version        ReplicaVersion
		mtimeText      sql.NullString
		ctimeText      sql.NullString
		checksumQuick  sql.NullString
		checksumFull   sql.NullString
		mediaSignature sql.NullString
		scanRevision   sql.NullString
		createdAtText  string
	)

	if err := scanner.Scan(
		&version.ID,
		&version.Size,
		&mtimeText,
		&ctimeText,
		&checksumQuick,
		&checksumFull,
		&mediaSignature,
		&scanRevision,
		&createdAtText,
	); err != nil {
		return ReplicaVersion{}, fmt.Errorf("scan replica version: %w", err)
	}

	mtime, err := parseNullableTime(mtimeText)
	if err != nil {
		return ReplicaVersion{}, fmt.Errorf("parse replica version mtime: %w", err)
	}

	ctime, err := parseNullableTime(ctimeText)
	if err != nil {
		return ReplicaVersion{}, fmt.Errorf("parse replica version ctime: %w", err)
	}

	createdAt, err := time.Parse(timeLayout, createdAtText)
	if err != nil {
		return ReplicaVersion{}, fmt.Errorf("parse replica version created_at: %w", err)
	}

	version.MTime = mtime
	version.CTime = ctime
	version.ChecksumQuick = parseNullableString(checksumQuick)
	version.ChecksumFull = parseNullableString(checksumFull)
	version.MediaSignature = parseNullableString(mediaSignature)
	version.ScanRevision = parseNullableString(scanRevision)
	version.CreatedAt = createdAt

	return version, nil
}

func scanReplica(scanner rowScanner) (Replica, error) {
	var (
		replica        Replica
		existsFlag     int
		versionID      sql.NullString
		lastSeenAtText sql.NullString
		createdAtText  string
		updatedAtText  string
	)

	if err := scanner.Scan(
		&replica.ID,
		&replica.AssetID,
		&replica.EndpointID,
		&replica.PhysicalPath,
		&replica.ReplicaStatus,
		&existsFlag,
		&versionID,
		&lastSeenAtText,
		&createdAtText,
		&updatedAtText,
	); err != nil {
		return Replica{}, fmt.Errorf("scan replica: %w", err)
	}

	lastSeenAt, err := parseNullableTime(lastSeenAtText)
	if err != nil {
		return Replica{}, fmt.Errorf("parse replica last_seen_at: %w", err)
	}

	createdAt, err := time.Parse(timeLayout, createdAtText)
	if err != nil {
		return Replica{}, fmt.Errorf("parse replica created_at: %w", err)
	}

	updatedAt, err := time.Parse(timeLayout, updatedAtText)
	if err != nil {
		return Replica{}, fmt.Errorf("parse replica updated_at: %w", err)
	}

	replica.ExistsFlag = existsFlag == 1
	replica.VersionID = parseNullableString(versionID)
	replica.LastSeenAt = lastSeenAt
	replica.CreatedAt = createdAt
	replica.UpdatedAt = updatedAt

	return replica, nil
}
