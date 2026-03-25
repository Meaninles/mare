package catalog

import (
	"context"

	"mam/backend/internal/store"
)

func (service *Service) SearchAssets(
	ctx context.Context,
	searchQuery string,
	mediaType string,
	assetStatus string,
	limit int,
	offset int,
) ([]AssetRecord, error) {
	assets, err := service.store.SearchAssets(ctx, store.AssetListOptions{
		Limit:       limit,
		Offset:      offset,
		SearchQuery: searchQuery,
		MediaType:   mediaType,
		AssetStatus: assetStatus,
	})
	if err != nil {
		return nil, err
	}

	return service.buildAssetRecords(ctx, assets)
}

func (service *Service) buildAssetRecords(ctx context.Context, assets []store.Asset) ([]AssetRecord, error) {
	expectedEndpoints, _, err := service.listManagedEnabledEndpoints(ctx)
	if err != nil {
		return nil, err
	}

	allEndpoints, err := service.store.ListStorageEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	allEndpointLookup := make(map[string]store.StorageEndpoint, len(allEndpoints))
	for _, endpoint := range allEndpoints {
		allEndpointLookup[endpoint.ID] = endpoint
	}

	records := make([]AssetRecord, 0, len(assets))
	for _, asset := range assets {
		replicas, replicaErr := service.store.ListReplicasByAssetID(ctx, asset.ID)
		if replicaErr != nil {
			return nil, replicaErr
		}

		replicaRecords, availableReplicaCount, missingReplicaCount, err := service.buildAssetReplicaRecords(
			ctx,
			replicas,
			expectedEndpoints,
			allEndpointLookup,
			asset.LogicalPathKey,
		)
		if err != nil {
			return nil, err
		}

		posterRecord, err := service.buildPosterRecord(ctx, asset)
		if err != nil {
			return nil, err
		}

		audioMetadata, err := service.buildAudioMetadataRecord(ctx, asset.ID)
		if err != nil {
			return nil, err
		}

		var previewURL *string
		if candidate, err := service.selectReadableReplica(ctx, replicas); err == nil && candidate != nil {
			url := service.previewURL(asset.ID)
			previewURL = &url
		}

		record := AssetRecord{
			ID:                    asset.ID,
			LogicalPathKey:        asset.LogicalPathKey,
			CanonicalPath:         canonicalLogicalPath(asset.LogicalPathKey),
			CanonicalDirectory:    canonicalDirectoryPath(asset.LogicalPathKey),
			DisplayName:           asset.DisplayName,
			MediaType:             asset.MediaType,
			AssetStatus:           asset.AssetStatus,
			PrimaryTimestamp:      asset.PrimaryTimestamp,
			Poster:                posterRecord,
			PreviewURL:            previewURL,
			AudioMetadata:         audioMetadata,
			CreatedAt:             asset.CreatedAt,
			UpdatedAt:             asset.UpdatedAt,
			AvailableReplicaCount: availableReplicaCount,
			MissingReplicaCount:   missingReplicaCount,
			Replicas:              replicaRecords,
		}
		records = append(records, record)
		service.maybeQueueDerivedMedia(asset, replicas)
		service.maybeQueueSearchFeatures(asset, replicas)
	}

	return records, nil
}
