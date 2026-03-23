package store

import (
	"context"
	"fmt"
	"time"
)

func (store *Store) CreateStorageEndpoint(ctx context.Context, endpoint StorageEndpoint) error {
	_, err := store.db.ExecContext(
		ctx,
		`INSERT INTO storage_endpoints
		(id, name, endpoint_type, root_path, role_mode, identity_signature, availability_status, connection_config, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		endpoint.ID,
		endpoint.Name,
		endpoint.EndpointType,
		endpoint.RootPath,
		endpoint.RoleMode,
		endpoint.IdentitySignature,
		endpoint.AvailabilityStatus,
		endpoint.ConnectionConfig,
		endpoint.CreatedAt.UTC().Format(timeLayout),
		endpoint.UpdatedAt.UTC().Format(timeLayout),
	)
	if err != nil {
		return fmt.Errorf("insert storage endpoint: %w", err)
	}
	return nil
}

func (store *Store) GetStorageEndpointByID(ctx context.Context, id string) (StorageEndpoint, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT id, name, endpoint_type, root_path, role_mode, identity_signature, availability_status, connection_config, created_at, updated_at
		 FROM storage_endpoints WHERE id = ?`,
		id,
	)
	return scanStorageEndpoint(row)
}

func (store *Store) ListStorageEndpoints(ctx context.Context) ([]StorageEndpoint, error) {
	rows, err := store.db.QueryContext(
		ctx,
		`SELECT id, name, endpoint_type, root_path, role_mode, identity_signature, availability_status, connection_config, created_at, updated_at
		 FROM storage_endpoints ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list storage endpoints: %w", err)
	}
	defer rows.Close()

	var endpoints []StorageEndpoint
	for rows.Next() {
		endpoint, scanErr := scanStorageEndpoint(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		endpoints = append(endpoints, endpoint)
	}

	return endpoints, rows.Err()
}

func (store *Store) ListEnabledStorageEndpoints(ctx context.Context) ([]StorageEndpoint, error) {
	rows, err := store.db.QueryContext(
		ctx,
		`SELECT id, name, endpoint_type, root_path, role_mode, identity_signature, availability_status, connection_config, created_at, updated_at
		 FROM storage_endpoints
		 WHERE UPPER(availability_status) != 'DISABLED'
		 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list enabled storage endpoints: %w", err)
	}
	defer rows.Close()

	var endpoints []StorageEndpoint
	for rows.Next() {
		endpoint, scanErr := scanStorageEndpoint(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		endpoints = append(endpoints, endpoint)
	}

	return endpoints, rows.Err()
}

func (store *Store) UpdateStorageEndpoint(ctx context.Context, endpoint StorageEndpoint) error {
	_, err := store.db.ExecContext(
		ctx,
		`UPDATE storage_endpoints
		 SET name = ?, endpoint_type = ?, root_path = ?, role_mode = ?, identity_signature = ?, availability_status = ?, connection_config = ?, updated_at = ?
		 WHERE id = ?`,
		endpoint.Name,
		endpoint.EndpointType,
		endpoint.RootPath,
		endpoint.RoleMode,
		endpoint.IdentitySignature,
		endpoint.AvailabilityStatus,
		endpoint.ConnectionConfig,
		endpoint.UpdatedAt.UTC().Format(timeLayout),
		endpoint.ID,
	)
	if err != nil {
		return fmt.Errorf("update storage endpoint: %w", err)
	}
	return nil
}

func (store *Store) DeleteStorageEndpoint(ctx context.Context, id string) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM storage_endpoints WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete storage endpoint: %w", err)
	}
	return nil
}

func scanStorageEndpoint(scanner rowScanner) (StorageEndpoint, error) {
	var (
		endpoint      StorageEndpoint
		createdAtText string
		updatedAtText string
	)

	if err := scanner.Scan(
		&endpoint.ID,
		&endpoint.Name,
		&endpoint.EndpointType,
		&endpoint.RootPath,
		&endpoint.RoleMode,
		&endpoint.IdentitySignature,
		&endpoint.AvailabilityStatus,
		&endpoint.ConnectionConfig,
		&createdAtText,
		&updatedAtText,
	); err != nil {
		return StorageEndpoint{}, fmt.Errorf("scan storage endpoint: %w", err)
	}

	createdAt, err := time.Parse(timeLayout, createdAtText)
	if err != nil {
		return StorageEndpoint{}, fmt.Errorf("parse storage endpoint created_at: %w", err)
	}

	updatedAt, err := time.Parse(timeLayout, updatedAtText)
	if err != nil {
		return StorageEndpoint{}, fmt.Errorf("parse storage endpoint updated_at: %w", err)
	}

	endpoint.CreatedAt = createdAt
	endpoint.UpdatedAt = updatedAt
	return endpoint, nil
}
