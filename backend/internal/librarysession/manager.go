package librarysession

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"mam/backend/internal/catalog"
	"mam/backend/internal/credentials"
	"mam/backend/internal/platform"
	"mam/backend/internal/store"
)

const (
	statusUnloaded             = "unloaded"
	statusLoaded               = "loaded"
	defaultLibraryExtension    = ".maredb"
	defaultLibrarySchemaFamily = "mare-library-v1"
)

var ErrLibraryNotOpen = errors.New("library is not open")

type Status struct {
	Status           string     `json:"status"`
	Ready            bool       `json:"ready"`
	LibraryID        string     `json:"libraryId,omitempty"`
	Path             string     `json:"path,omitempty"`
	Name             string     `json:"name,omitempty"`
	FileExtension    string     `json:"fileExtension,omitempty"`
	SchemaFamily     string     `json:"schemaFamily,omitempty"`
	MigrationVersion string     `json:"migrationVersion,omitempty"`
	CacheRoot        string     `json:"cacheRoot,omitempty"`
	LocalStateRoot   string     `json:"localStateRoot,omitempty"`
	OpenedAt         *time.Time `json:"openedAt,omitempty"`
}

type runtime struct {
	path           string
	name           string
	cacheRoot      string
	localStateRoot string
	openedAt       time.Time
	metadata       store.LibraryMetadata
	store          *store.Store
	catalog        *catalog.Service
}

type Manager struct {
	mu              sync.RWMutex
	appName         string
	ffmpegPath      string
	credentialVault *credentials.Vault
	current         *runtime
}

func NewManager(appName, ffmpegPath string) *Manager {
	vault, err := credentials.NewVault("")
	if err != nil {
		slog.Warn("failed to initialize credential vault", "error", err)
	}

	return &Manager{
		appName:         appName,
		ffmpegPath:      ffmpegPath,
		credentialVault: vault,
	}
}

func (manager *Manager) Status() Status {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	return statusFromRuntime(manager.current)
}

func (manager *Manager) DatabaseState() platform.DatabaseState {
	status := manager.Status()
	return platform.DatabaseState{
		Driver:           "sqlite",
		Path:             status.Path,
		Ready:            status.Ready,
		MigrationVersion: defaultString(status.MigrationVersion, statusUnloaded),
	}
}

func (manager *Manager) Catalog() (*catalog.Service, error) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	if manager.current == nil || manager.current.catalog == nil {
		return nil, ErrLibraryNotOpen
	}

	return manager.current.catalog, nil
}

func (manager *Manager) CreateLibrary(_ context.Context, path string) (Status, error) {
	normalizedPath, err := normalizeLibraryPath(path, true)
	if err != nil {
		return Status{}, err
	}

	nextRuntime, err := manager.buildRuntime(normalizedPath, true)
	if err != nil {
		return Status{}, err
	}

	manager.swapRuntime(nextRuntime)
	status := statusFromRuntime(nextRuntime)
	slog.Info("library created", "path", status.Path, "cacheRoot", status.CacheRoot)
	return status, nil
}

func (manager *Manager) OpenLibrary(_ context.Context, path string) (Status, error) {
	normalizedPath, err := normalizeLibraryPath(path, false)
	if err != nil {
		return Status{}, err
	}

	manager.mu.RLock()
	current := manager.current
	manager.mu.RUnlock()
	if current != nil && samePath(current.path, normalizedPath) {
		return statusFromRuntime(current), nil
	}

	nextRuntime, err := manager.buildRuntime(normalizedPath, false)
	if err != nil {
		return Status{}, err
	}

	manager.swapRuntime(nextRuntime)
	status := statusFromRuntime(nextRuntime)
	slog.Info("library opened", "path", status.Path, "cacheRoot", status.CacheRoot)
	return status, nil
}

func (manager *Manager) CloseLibrary(_ context.Context) (Status, error) {
	manager.mu.Lock()
	previous := manager.current
	manager.current = nil
	manager.mu.Unlock()

	if previous != nil {
		previous.catalog.Close()
		if err := previous.store.Close(); err != nil {
			slog.Warn("failed to close library store", "path", previous.path, "error", err)
		}
		slog.Info("library closed", "path", previous.path)
	}

	return Status{
		Status: statusUnloaded,
		Ready:  false,
	}, nil
}

func (manager *Manager) buildRuntime(path string, create bool) (*runtime, error) {
	var (
		dataStore *store.Store
		err       error
	)

	if create {
		dataStore, err = store.CreateSQLiteStore(path)
	} else {
		dataStore, err = store.OpenSQLiteStore(path)
	}
	if err != nil {
		return nil, err
	}

	metadata, err := ensureLibraryMetadata(dataStore, path, create)
	if err != nil {
		_ = dataStore.Close()
		return nil, err
	}

	localStateRoot := manager.deriveLocalStateRoot(metadata)
	cacheRoot := filepath.Join(localStateRoot, "cache", "media")
	options := []catalog.ServiceOption{catalog.MediaConfig{
		CacheRoot:  cacheRoot,
		FFmpegPath: manager.ffmpegPath,
	}}
	if manager.credentialVault != nil {
		options = append(options, catalog.WithCredentialVault(manager.credentialVault))
	}

	catalogService := catalog.NewService(dataStore, nil, options...)
	if err := catalogService.MigrateEndpointCredentials(context.Background()); err != nil {
		_ = dataStore.Close()
		return nil, err
	}
	if err := catalogService.Start(context.Background()); err != nil {
		_ = dataStore.Close()
		return nil, err
	}

	return &runtime{
		path:           path,
		name:           metadata.LibraryName,
		cacheRoot:      cacheRoot,
		localStateRoot: localStateRoot,
		openedAt:       time.Now().UTC(),
		metadata:       metadata,
		store:          dataStore,
		catalog:        catalogService,
	}, nil
}

func (manager *Manager) swapRuntime(next *runtime) {
	manager.mu.Lock()
	previous := manager.current
	manager.current = next
	manager.mu.Unlock()

	if previous != nil {
		previous.catalog.Close()
		if err := previous.store.Close(); err != nil {
			slog.Warn("failed to close previous library store", "path", previous.path, "error", err)
		}
	}
}

func statusFromRuntime(current *runtime) Status {
	if current == nil {
		return Status{
			Status: statusUnloaded,
			Ready:  false,
		}
	}

	openedAt := current.openedAt
	return Status{
		Status:           statusLoaded,
		Ready:            true,
		LibraryID:        current.metadata.LibraryID,
		Path:             current.path,
		Name:             current.name,
		FileExtension:    current.metadata.FileExtension,
		SchemaFamily:     current.metadata.SchemaFamily,
		MigrationVersion: current.store.MigrationVersion(),
		CacheRoot:        current.cacheRoot,
		LocalStateRoot:   current.localStateRoot,
		OpenedAt:         &openedAt,
	}
}

func normalizeLibraryPath(path string, create bool) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("library path is required")
	}

	normalized := filepath.Clean(trimmed)
	if filepath.Ext(normalized) == "" {
		normalized += defaultLibraryExtension
	}

	if !create {
		info, err := os.Stat(normalized)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("library file does not exist: %s", normalized)
			}
			return "", fmt.Errorf("stat library file: %w", err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("library path points to a directory: %s", normalized)
		}
	}

	return normalized, nil
}

func libraryNameFromPath(path string) string {
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	name = strings.TrimSpace(name)
	if name == "" {
		return "library"
	}
	return name
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func samePath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func ensureLibraryMetadata(dataStore *store.Store, libraryPath string, create bool) (store.LibraryMetadata, error) {
	metadata, err := dataStore.GetLibraryMetadata(context.Background())
	if err != nil {
		return store.LibraryMetadata{}, err
	}

	now := time.Now().UTC()
	updated := false
	if strings.TrimSpace(metadata.LibraryID) == "" {
		metadata.LibraryID = uuid.NewString()
		updated = true
	}

	if create || strings.TrimSpace(metadata.LibraryName) == "" || strings.EqualFold(strings.TrimSpace(metadata.LibraryName), "Untitled Library") {
		desiredName := libraryNameFromPath(libraryPath)
		if desiredName != "" && metadata.LibraryName != desiredName {
			metadata.LibraryName = desiredName
			updated = true
		}
	}

	fileExtension := strings.TrimSpace(filepath.Ext(libraryPath))
	if fileExtension == "" {
		fileExtension = defaultLibraryExtension
	}
	if strings.TrimSpace(metadata.FileExtension) == "" || metadata.FileExtension != fileExtension {
		metadata.FileExtension = fileExtension
		updated = true
	}

	if strings.TrimSpace(metadata.SchemaFamily) == "" {
		metadata.SchemaFamily = defaultLibrarySchemaFamily
		updated = true
	}

	if metadata.CreatedAt.IsZero() {
		metadata.CreatedAt = now
		updated = true
	}

	if metadata.UpdatedAt.IsZero() || updated {
		metadata.UpdatedAt = now
	}

	if updated {
		if err := dataStore.UpsertLibraryMetadata(context.Background(), metadata); err != nil {
			return store.LibraryMetadata{}, err
		}
	}

	return metadata, nil
}

func (manager *Manager) deriveLocalStateRoot(metadata store.LibraryMetadata) string {
	libraryKey := sanitizePathSegment(defaultString(strings.TrimSpace(metadata.LibraryID), strings.TrimSpace(metadata.LibraryName)))
	if libraryKey == "" {
		libraryKey = "library"
	}

	cacheRoot, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(cacheRoot) == "" {
		return filepath.Join(".", "data", "libraries", libraryKey)
	}

	appKey := sanitizePathSegment(defaultString(manager.appName, "mam"))
	if appKey == "" {
		appKey = "mam"
	}

	return filepath.Join(cacheRoot, appKey, "libraries", libraryKey)
}

func sanitizePathSegment(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}

	nonPortable := regexp.MustCompile(`[^a-z0-9._-]+`)
	return strings.Trim(nonPortable.ReplaceAllString(trimmed, "-"), "-")
}
