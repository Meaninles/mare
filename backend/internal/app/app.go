package app

import (
	"context"
	"log/slog"

	"mam/backend/internal/buildinfo"
	"mam/backend/internal/config"
	"mam/backend/internal/httpserver"
	"mam/backend/internal/librarysession"
	"mam/backend/internal/platform"
)

type App struct {
	config  config.Config
	server  *httpserver.Server
	session *librarysession.Manager
}

func New(_ context.Context) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	logger := platform.NewLogger(cfg.AppEnv, cfg.LogFilePath)
	slog.SetDefault(logger)

	session := librarysession.NewManager(cfg.AppName, cfg.FFmpegPath)
	system := platform.NewSystemState(
		cfg,
		buildinfo.Get(),
		platform.DefaultModules(),
		platform.NewDependencySnapshot(cfg),
		platform.DatabaseState{
			Driver:           "sqlite",
			Path:             "",
			Ready:            false,
			MigrationVersion: "unloaded",
		},
	)
	server := httpserver.New(cfg, system, session)

	return &App{
		config:  cfg,
		server:  server,
		session: session,
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

	_, _ = app.session.CloseLibrary(context.Background())
	return nil
}
