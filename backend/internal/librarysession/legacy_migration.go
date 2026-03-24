package librarysession

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mam/backend/internal/store"
)

const legacyMigrationManifestVersion = 1

type LegacyCatalogStatus struct {
	Available            bool                  `json:"available"`
	SourcePath           string                `json:"sourcePath,omitempty"`
	TargetPath           string                `json:"targetPath,omitempty"`
	ManifestPath         string                `json:"manifestPath,omitempty"`
	TargetExists         bool                  `json:"targetExists"`
	SuggestedLibraryName string                `json:"suggestedLibraryName,omitempty"`
	SourceSummary        *store.LibrarySummary `json:"sourceSummary,omitempty"`
	Reason               string                `json:"reason,omitempty"`
}

type LegacyCatalogMigrationResult struct {
	SourcePath           string               `json:"sourcePath"`
	TargetPath           string               `json:"targetPath"`
	ManifestPath         string               `json:"manifestPath"`
	SuggestedLibraryName string               `json:"suggestedLibraryName"`
	SourcePreserved      bool                 `json:"sourcePreserved"`
	CountsMatch          bool                 `json:"countsMatch"`
	SourceSummary        store.LibrarySummary `json:"sourceSummary"`
	TargetSummary        store.LibrarySummary `json:"targetSummary"`
	MigratedAt           time.Time            `json:"migratedAt"`
	Library              Status               `json:"library"`
}

type legacyMigrationManifest struct {
	Version              int                  `json:"version"`
	MigratedAt           time.Time            `json:"migratedAt"`
	SourcePath           string               `json:"sourcePath"`
	TargetPath           string               `json:"targetPath"`
	SuggestedLibraryName string               `json:"suggestedLibraryName"`
	SourcePreserved      bool                 `json:"sourcePreserved"`
	CountsMatch          bool                 `json:"countsMatch"`
	SourceSummary        store.LibrarySummary `json:"sourceSummary"`
	TargetSummary        store.LibrarySummary `json:"targetSummary"`
}

func (manager *Manager) LegacyCatalogStatus(ctx context.Context, legacySourcePath string) (LegacyCatalogStatus, error) {
	sourcePath, err := resolveLegacyCatalogPath(legacySourcePath)
	if err != nil {
		return LegacyCatalogStatus{
			Available: false,
			Reason:    err.Error(),
		}, nil
	}

	targetPath := deriveMigratedLibraryPath(sourcePath)
	status := LegacyCatalogStatus{
		SourcePath:           sourcePath,
		TargetPath:           targetPath,
		ManifestPath:         deriveMigrationManifestPath(targetPath),
		SuggestedLibraryName: suggestedLibraryNameFromPath(targetPath),
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			status.Reason = "legacy catalog file does not exist"
			return status, nil
		}
		return LegacyCatalogStatus{}, fmt.Errorf("stat legacy catalog: %w", err)
	}
	if info.IsDir() {
		status.Reason = "legacy catalog path points to a directory"
		return status, nil
	}

	status.Available = true
	if _, err := os.Stat(targetPath); err == nil {
		status.TargetExists = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return LegacyCatalogStatus{}, fmt.Errorf("stat migrated target: %w", err)
	}

	summary, err := store.SummarizeSQLiteFile(ctx, sourcePath)
	if err != nil {
		return LegacyCatalogStatus{}, fmt.Errorf("summarize legacy catalog: %w", err)
	}
	status.SourceSummary = &summary
	return status, nil
}

func (manager *Manager) MigrateLegacyCatalog(
	ctx context.Context,
	legacySourcePath string,
	targetPath string,
	libraryName string,
) (LegacyCatalogMigrationResult, error) {
	sourcePath, err := resolveLegacyCatalogPath(legacySourcePath)
	if err != nil {
		return LegacyCatalogMigrationResult{}, err
	}

	if strings.EqualFold(strings.TrimSpace(filepath.Ext(sourcePath)), defaultLibraryExtension) {
		return LegacyCatalogMigrationResult{}, errors.New("legacy source already points to a library file")
	}

	normalizedTarget, err := normalizeMigrationTargetPath(targetPath, sourcePath)
	if err != nil {
		return LegacyCatalogMigrationResult{}, err
	}
	if samePath(sourcePath, normalizedTarget) {
		return LegacyCatalogMigrationResult{}, errors.New("migration target must be different from the legacy source path")
	}

	sourceSummary, err := store.SummarizeSQLiteFile(ctx, sourcePath)
	if err != nil {
		return LegacyCatalogMigrationResult{}, fmt.Errorf("summarize legacy catalog: %w", err)
	}

	if _, err := os.Stat(normalizedTarget); err == nil {
		return LegacyCatalogMigrationResult{}, fmt.Errorf("target library already exists: %s", normalizedTarget)
	} else if !errors.Is(err, os.ErrNotExist) {
		return LegacyCatalogMigrationResult{}, fmt.Errorf("stat target library: %w", err)
	}

	tempTarget := normalizedTarget + ".tmp"
	_ = os.Remove(tempTarget)
	defer func() {
		_ = os.Remove(tempTarget)
	}()

	if err := snapshotLegacyCatalog(ctx, sourcePath, tempTarget); err != nil {
		return LegacyCatalogMigrationResult{}, err
	}

	migratedStore, err := store.OpenSQLiteStore(tempTarget)
	if err != nil {
		return LegacyCatalogMigrationResult{}, fmt.Errorf("open migrated library snapshot: %w", err)
	}

	if err := applyMigratedLibraryMetadata(ctx, migratedStore, normalizedTarget, libraryName); err != nil {
		_ = migratedStore.Close()
		return LegacyCatalogMigrationResult{}, err
	}

	targetSummary, err := migratedStore.SummarizeLibrary(ctx)
	if err != nil {
		_ = migratedStore.Close()
		return LegacyCatalogMigrationResult{}, fmt.Errorf("summarize migrated library snapshot: %w", err)
	}
	if err := migratedStore.Close(); err != nil {
		return LegacyCatalogMigrationResult{}, fmt.Errorf("close migrated library snapshot: %w", err)
	}

	countsMatch := sourceSummary == targetSummary
	if !countsMatch {
		return LegacyCatalogMigrationResult{}, fmt.Errorf(
			"legacy migration validation failed: source=%+v target=%+v",
			sourceSummary,
			targetSummary,
		)
	}

	if err := os.MkdirAll(filepath.Dir(normalizedTarget), 0o755); err != nil {
		return LegacyCatalogMigrationResult{}, fmt.Errorf("create target library directory: %w", err)
	}
	if err := os.Rename(tempTarget, normalizedTarget); err != nil {
		return LegacyCatalogMigrationResult{}, fmt.Errorf("promote migrated library snapshot: %w", err)
	}

	nextRuntime, err := manager.buildRuntime(normalizedTarget, false)
	if err != nil {
		return LegacyCatalogMigrationResult{}, fmt.Errorf("open migrated library: %w", err)
	}
	manager.swapRuntime(nextRuntime)

	result := LegacyCatalogMigrationResult{
		SourcePath:           sourcePath,
		TargetPath:           normalizedTarget,
		ManifestPath:         deriveMigrationManifestPath(normalizedTarget),
		SuggestedLibraryName: defaultString(strings.TrimSpace(libraryName), nextRuntime.metadata.LibraryName),
		SourcePreserved:      true,
		CountsMatch:          true,
		SourceSummary:        sourceSummary,
		TargetSummary:        targetSummary,
		MigratedAt:           time.Now().UTC(),
		Library:              statusFromRuntime(nextRuntime),
	}

	if err := writeLegacyMigrationManifest(result); err != nil {
		slog.Warn(
			"failed to write legacy migration manifest",
			"targetPath", result.TargetPath,
			"manifestPath", result.ManifestPath,
			"error", err,
		)
	}

	return result, nil
}

func resolveLegacyCatalogPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("legacy catalog path is required")
	}

	resolved, err := filepath.Abs(filepath.Clean(trimmed))
	if err != nil {
		return "", fmt.Errorf("resolve legacy catalog path: %w", err)
	}
	return resolved, nil
}

func normalizeMigrationTargetPath(targetPath string, sourcePath string) (string, error) {
	if strings.TrimSpace(targetPath) == "" {
		return deriveMigratedLibraryPath(sourcePath), nil
	}

	normalized, err := normalizeLibraryPath(targetPath, true)
	if err != nil {
		return "", err
	}

	resolved, err := filepath.Abs(normalized)
	if err != nil {
		return "", fmt.Errorf("resolve target library path: %w", err)
	}
	return resolved, nil
}

func deriveMigratedLibraryPath(sourcePath string) string {
	dir := filepath.Dir(sourcePath)
	sourceName := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
	switch strings.ToLower(strings.TrimSpace(sourceName)) {
	case "", "mam", "catalog":
		return filepath.Join(dir, "default-library"+defaultLibraryExtension)
	default:
		return filepath.Join(dir, sourceName+defaultLibraryExtension)
	}
}

func deriveMigrationManifestPath(targetPath string) string {
	return strings.TrimSuffix(targetPath, filepath.Ext(targetPath)) + ".migration.json"
}

func suggestedLibraryNameFromPath(path string) string {
	return libraryNameFromPath(path)
}

func snapshotLegacyCatalog(ctx context.Context, sourcePath string, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create migration temp directory: %w", err)
	}

	db, err := sql.Open("sqlite", sourcePath)
	if err == nil {
		db.SetMaxOpenConns(1)
		escapedTarget := "'" + strings.ReplaceAll(targetPath, "'", "''") + "'"
		if _, execErr := db.ExecContext(ctx, "VACUUM INTO "+escapedTarget); execErr == nil {
			_ = db.Close()
			return nil
		}
		_ = db.Close()
	}

	return copyLegacyCatalogFile(sourcePath, targetPath)
}

func copyLegacyCatalogFile(sourcePath string, targetPath string) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open legacy catalog source: %w", err)
	}
	defer sourceFile.Close()

	targetFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("create migration temp target: %w", err)
	}
	defer targetFile.Close()

	if _, err := targetFile.ReadFrom(sourceFile); err != nil {
		return fmt.Errorf("copy legacy catalog source: %w", err)
	}
	return nil
}

func applyMigratedLibraryMetadata(ctx context.Context, migratedStore *store.Store, targetPath string, libraryName string) error {
	metadata, err := migratedStore.GetLibraryMetadata(ctx)
	if err != nil {
		return fmt.Errorf("read migrated library metadata: %w", err)
	}

	resolvedName := defaultString(strings.TrimSpace(libraryName), metadata.LibraryName)
	if resolvedName == "" {
		resolvedName = suggestedLibraryNameFromPath(targetPath)
	}

	metadata.LibraryName = resolvedName
	metadata.FileExtension = defaultString(strings.TrimSpace(filepath.Ext(targetPath)), defaultLibraryExtension)
	metadata.SchemaFamily = defaultString(strings.TrimSpace(metadata.SchemaFamily), defaultLibrarySchemaFamily)
	metadata.UpdatedAt = time.Now().UTC()

	if err := migratedStore.UpsertLibraryMetadata(ctx, metadata); err != nil {
		return fmt.Errorf("update migrated library metadata: %w", err)
	}
	return nil
}

func writeLegacyMigrationManifest(result LegacyCatalogMigrationResult) error {
	manifest := legacyMigrationManifest{
		Version:              legacyMigrationManifestVersion,
		MigratedAt:           result.MigratedAt,
		SourcePath:           result.SourcePath,
		TargetPath:           result.TargetPath,
		SuggestedLibraryName: result.SuggestedLibraryName,
		SourcePreserved:      result.SourcePreserved,
		CountsMatch:          result.CountsMatch,
		SourceSummary:        result.SourceSummary,
		TargetSummary:        result.TargetSummary,
	}

	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal legacy migration manifest: %w", err)
	}

	if err := os.WriteFile(result.ManifestPath, payload, 0o644); err != nil {
		return fmt.Errorf("write legacy migration manifest: %w", err)
	}
	return nil
}
