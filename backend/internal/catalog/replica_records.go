package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"mam/backend/internal/store"
)

func (service *Service) listManagedEnabledEndpoints(ctx context.Context) ([]store.StorageEndpoint, map[string]store.StorageEndpoint, error) {
	endpoints, err := service.store.ListEnabledStorageEndpoints(ctx)
	if err != nil {
		return nil, nil, err
	}

	managedEndpoints := make([]store.StorageEndpoint, 0, len(endpoints))
	endpointLookup := make(map[string]store.StorageEndpoint, len(endpoints))
	for _, endpoint := range endpoints {
		if !strings.EqualFold(strings.TrimSpace(endpoint.RoleMode), defaultRoleMode) {
			continue
		}
		managedEndpoints = append(managedEndpoints, endpoint)
		endpointLookup[endpoint.ID] = endpoint
	}

	return managedEndpoints, endpointLookup, nil
}

func (service *Service) buildAssetReplicaRecords(
	ctx context.Context,
	replicas []store.Replica,
	expectedEndpoints []store.StorageEndpoint,
) ([]ReplicaRecord, int, int, error) {
	replicaRecords := make([]ReplicaRecord, 0, len(replicas)+len(expectedEndpoints))
	representedEndpointIDs := make(map[string]struct{}, len(replicas))
	availableReplicaCount := 0
	missingReplicaCount := 0

	for _, replica := range replicas {
		representedEndpointIDs[replica.EndpointID] = struct{}{}
		versionRecord, _, _, err := service.buildVersionRecord(ctx, replica.VersionID)
		if err != nil {
			return nil, 0, 0, err
		}

		if replica.ExistsFlag {
			availableReplicaCount++
		} else {
			missingReplicaCount++
		}

		replicaRecords = append(replicaRecords, ReplicaRecord{
			ID:            replica.ID,
			EndpointID:    replica.EndpointID,
			PhysicalPath:  replica.PhysicalPath,
			ReplicaStatus: replica.ReplicaStatus,
			ExistsFlag:    replica.ExistsFlag,
			LastSeenAt:    replica.LastSeenAt,
			Version:       versionRecord,
		})
	}

	for _, endpoint := range expectedEndpoints {
		if _, ok := representedEndpointIDs[endpoint.ID]; ok {
			continue
		}

		missingReplicaCount++
		replicaRecords = append(replicaRecords, ReplicaRecord{
			ID:            syntheticReplicaID(endpoint.ID),
			EndpointID:    endpoint.ID,
			PhysicalPath:  "",
			ReplicaStatus: string(ReplicaStatusMissing),
			ExistsFlag:    false,
		})
	}

	return replicaRecords, availableReplicaCount, missingReplicaCount, nil
}

func (service *Service) buildSyncReplicaRecords(
	ctx context.Context,
	replicas []store.Replica,
	expectedEndpoints []store.StorageEndpoint,
	endpointLookup map[string]store.StorageEndpoint,
) ([]SyncReplicaRecord, int, int, error) {
	replicaRecords := make([]SyncReplicaRecord, 0, len(replicas)+len(expectedEndpoints))
	representedEndpointIDs := make(map[string]struct{}, len(replicas))
	availableReplicaCount := 0
	missingReplicaCount := 0

	for _, replica := range replicas {
		representedEndpointIDs[replica.EndpointID] = struct{}{}
		versionRecord, _, _, err := service.buildVersionRecord(ctx, replica.VersionID)
		if err != nil {
			return nil, 0, 0, err
		}

		if replica.ExistsFlag {
			availableReplicaCount++
		} else {
			missingReplicaCount++
		}

		endpointName := replica.EndpointID
		if endpoint, ok := endpointLookup[replica.EndpointID]; ok {
			endpointName = endpoint.Name
		}

		replicaRecords = append(replicaRecords, SyncReplicaRecord{
			ID:            replica.ID,
			EndpointID:    replica.EndpointID,
			EndpointName:  endpointName,
			PhysicalPath:  replica.PhysicalPath,
			ReplicaStatus: replica.ReplicaStatus,
			ExistsFlag:    replica.ExistsFlag,
			LastSeenAt:    replica.LastSeenAt,
			Version:       versionRecord,
		})
	}

	for _, endpoint := range expectedEndpoints {
		if _, ok := representedEndpointIDs[endpoint.ID]; ok {
			continue
		}

		missingReplicaCount++
		replicaRecords = append(replicaRecords, SyncReplicaRecord{
			ID:            syntheticReplicaID(endpoint.ID),
			EndpointID:    endpoint.ID,
			EndpointName:  endpoint.Name,
			PhysicalPath:  "",
			ReplicaStatus: string(ReplicaStatusMissing),
			ExistsFlag:    false,
		})
	}

	return replicaRecords, availableReplicaCount, missingReplicaCount, nil
}

func (service *Service) buildVersionRecord(
	ctx context.Context,
	versionID *string,
) (*AssetVersionRecord, *int64, *time.Time, error) {
	if versionID == nil {
		return nil, nil, nil, nil
	}

	version, err := service.store.GetReplicaVersionByID(ctx, *versionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, err
	}

	size := version.Size
	mtime := cloneTimePointer(version.MTime)
	return &AssetVersionRecord{
		ID:           version.ID,
		Size:         version.Size,
		MTime:        version.MTime,
		CreatedAt:    version.CreatedAt,
		ScanRevision: version.ScanRevision,
	}, &size, mtime, nil
}

func syntheticReplicaID(endpointID string) string {
	return fmt.Sprintf("missing:%s", endpointID)
}
