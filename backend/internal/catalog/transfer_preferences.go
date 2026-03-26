package catalog

import (
	"context"
	"database/sql"
	"time"

	"mam/backend/internal/store"
)

func normalizeTransferPreferencesRecord(preferences store.TransferPreferences) store.TransferPreferences {
	if preferences.UploadConcurrency <= 0 {
		preferences.UploadConcurrency = defaultUploadConcurrency
	}
	if preferences.DownloadConcurrency <= 0 {
		preferences.DownloadConcurrency = defaultDownloadConcurrency
	}
	if preferences.UploadConcurrency > maxTransferConcurrency {
		preferences.UploadConcurrency = maxTransferConcurrency
	}
	if preferences.DownloadConcurrency > maxTransferConcurrency {
		preferences.DownloadConcurrency = maxTransferConcurrency
	}

	now := time.Now().UTC()
	if preferences.CreatedAt.IsZero() {
		preferences.CreatedAt = now
	}
	if preferences.UpdatedAt.IsZero() {
		preferences.UpdatedAt = now
	}
	return preferences
}

func toTransferPreferencesRecord(preferences store.TransferPreferences) TransferPreferences {
	normalized := normalizeTransferPreferencesRecord(preferences)
	return TransferPreferences{
		UploadConcurrency:   normalized.UploadConcurrency,
		DownloadConcurrency: normalized.DownloadConcurrency,
		UpdatedAt:           normalized.UpdatedAt,
	}
}

func (service *Service) GetTransferPreferences(ctx context.Context) (TransferPreferences, error) {
	preferences, err := service.store.GetTransferPreferences(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			defaults := normalizeTransferPreferencesRecord(store.TransferPreferences{})
			if upsertErr := service.store.UpsertTransferPreferences(ctx, defaults); upsertErr != nil {
				return TransferPreferences{}, upsertErr
			}
			return toTransferPreferencesRecord(defaults), nil
		}
		return TransferPreferences{}, err
	}
	return toTransferPreferencesRecord(preferences), nil
}

func (service *Service) UpdateTransferPreferences(ctx context.Context, request UpdateTransferPreferencesRequest) (TransferPreferences, error) {
	current, err := service.store.GetTransferPreferences(ctx)
	if err != nil && err != sql.ErrNoRows {
		return TransferPreferences{}, err
	}
	if err == sql.ErrNoRows {
		current = store.TransferPreferences{}
	}

	current.UploadConcurrency = request.UploadConcurrency
	current.DownloadConcurrency = request.DownloadConcurrency
	current.UpdatedAt = time.Now().UTC()
	current = normalizeTransferPreferencesRecord(current)
	if err := service.store.UpsertTransferPreferences(ctx, current); err != nil {
		return TransferPreferences{}, err
	}
	return toTransferPreferencesRecord(current), nil
}
