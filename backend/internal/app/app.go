package app

import (
	"context"
	"log/slog"
	"path/filepath"

	"mam/backend/internal/catalog"
	"mam/backend/internal/buildinfo"
	"mam/backend/internal/config"
	"mam/backend/internal/httpserver"
	"mam/backend/internal/platform"
	"mam/backend/internal/store"
)

type App struct {
	config config.Config
	server *httpserver.Server
	store  *store.Store
}

func New(_ context.Context) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	logger := platform.NewLogger(cfg.AppEnv)
	slog.SetDefault(logger)

	dataStore, err := store.NewSQLiteStore(cfg.CatalogDBPath)
	if err != nil {
		return nil, err
	}

	system := platform.NewSystemState(
		cfg,
		buildinfo.Get(),
		platform.DefaultModules(),
		platform.NewDependencySnapshot(cfg),
		platform.DatabaseState{
			Driver:           "sqlite",
			Path:             cfg.CatalogDBPath,
			Ready:            true,
			MigrationVersion: dataStore.MigrationVersion(),
		},
	)
	catalogService := catalog.NewService(dataStore, nil, catalog.MediaConfig{
		CacheRoot:  filepath.Join(filepath.Dir(cfg.CatalogDBPath), "cache", "media"),
		FFmpegPath: cfg.FFmpegPath,
	})
	server := httpserver.New(cfg, system, catalogService)

	return &App{
		config: cfg,
		server: server,
		store:  dataStore,
	}, nil
}

func (app *App) Run(ctx context.Context) error {
	slog.Info("starting backend service", "address", app.config.HTTPAddress())

	if err := app.server.Start(ctx); err != nil {
		return err
	}

	<-ctx.Done()
	slog.Info("shutdown signal received")

	if err := app.server.Shutdown(context.Background()); err != nil {
		return err
	}

	return app.store.Close()
}
