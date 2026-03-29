package app

import (
	"context"
	"log/slog"

	"mam/backend/internal/catalog"
	"mam/backend/internal/buildinfo"
	cd2auth "mam/backend/internal/cd2/auth"
	cd2client "mam/backend/internal/cd2/client"
	cd2cloudapi "mam/backend/internal/cd2/cloudapi"
	cd2fs "mam/backend/internal/cd2/fs"
	cd2runtime "mam/backend/internal/cd2/runtime"
	cd2transfers "mam/backend/internal/cd2/transfers"
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

	cd2RuntimeManager := cd2runtime.NewManager(cd2runtime.ConfigFromApp(cfg))
	cd2ClientManager := cd2client.NewManager(cd2client.ConfigFromApp(cfg))
	cd2AuthService, err := cd2auth.NewService(cd2auth.ConfigFromApp(cfg), cd2ClientManager)
	if err != nil {
		return nil, err
	}
	cd2CloudAPIService := cd2cloudapi.NewService(cd2ClientManager)
	cd2FileService := cd2fs.NewService(cd2ClientManager)
	cd2TransferService := cd2transfers.NewService(cd2ClientManager)
	if err := cd2AuthService.Bootstrap(); err != nil {
		slog.Warn("bootstrap cd2 auth profile failed", "error", err)
	}

	cd2State := cd2RuntimeManager.Probe(context.Background())
	cd2ClientState := cd2ClientManager.Probe(context.Background())

	session := librarysession.NewManager(cfg.AppName, cfg.FFmpegPath, catalog.WithCD2FSService(cd2FileService))
	system := platform.NewSystemState(
		cfg,
		buildinfo.Get(),
		platform.DefaultModules(cd2State.Ready, cd2ClientState.Ready),
		platform.NewDependencySnapshot(cfg),
		platform.DatabaseState{
			Driver:           "sqlite",
			Path:             "",
			Ready:            false,
			MigrationVersion: "unloaded",
		},
		cd2State,
		cd2ClientState,
	)
	server := httpserver.New(cfg, system, session, cd2RuntimeManager, cd2ClientManager, cd2AuthService, cd2CloudAPIService, cd2FileService, cd2TransferService)

	slog.Info(
		"cd2 runtime probed",
		"enabled", cd2State.Enabled,
		"mode", cd2State.Mode,
		"baseUrl", cd2State.BaseURL,
		"ready", cd2State.Ready,
		"productName", cd2State.ProductName,
		"versionCheckStatus", cd2State.VersionCheckStatus,
		"error", cd2State.LastError,
	)
	slog.Info(
		"cd2 gRPC client probed",
		"enabled", cd2ClientState.Enabled,
		"target", cd2ClientState.Target,
		"publicReady", cd2ClientState.PublicReady,
		"authReady", cd2ClientState.AuthReady,
		"ready", cd2ClientState.Ready,
		"authMode", cd2ClientState.ActiveAuthMode,
		"tokenSource", cd2ClientState.TokenSource,
		"protoVersion", cd2ClientState.ProtoVersion,
		"error", cd2ClientState.LastError,
	)

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
