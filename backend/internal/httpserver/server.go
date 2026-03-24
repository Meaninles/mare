package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"mam/backend/internal/catalog"
	"mam/backend/internal/config"
	"mam/backend/internal/platform"
)

type Server struct {
	config  config.Config
	system  platform.SystemState
	catalog *catalog.Service
	http    *http.Server
}

func New(cfg config.Config, system platform.SystemState, catalogService *catalog.Service) *Server {
	server := &Server{
		config:  cfg,
		system:  system,
		catalog: catalogService,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", server.handleHealth)
	mux.HandleFunc("/readyz", server.handleReady)
	mux.HandleFunc("/api/v1/system/bootstrap", server.handleBootstrap)
	mux.HandleFunc("/api/v1/system/logs", server.handleSystemLogs)
	mux.HandleFunc("/api/v1/settings/backup/export", server.handleSettingsBackupExport)
	mux.HandleFunc("/api/v1/settings/backup/import", server.handleSettingsBackupImport)
	mux.HandleFunc("/api/v1/catalog/endpoints", server.handleCatalogEndpoints)
	mux.HandleFunc("/api/v1/catalog/endpoints/", server.handleCatalogEndpointResource)
	mux.HandleFunc("/api/v1/catalog/assets", server.handleCatalogAssets)
	mux.HandleFunc("/api/v1/catalog/assets/", server.handleCatalogAssetResource)
	mux.HandleFunc("/api/v1/catalog/replicas/delete", server.handleCatalogDeleteReplica)
	mux.HandleFunc("/api/v1/catalog/tasks", server.handleCatalogTasks)
	mux.HandleFunc("/api/v1/catalog/scans/full", server.handleCatalogFullScan)
	mux.HandleFunc("/api/v1/catalog/scans/endpoint", server.handleCatalogEndpointScan)
	mux.HandleFunc("/api/v1/catalog/sync/overview", server.handleCatalogSyncOverview)
	mux.HandleFunc("/api/v1/catalog/sync/restore", server.handleCatalogRestoreAsset)
	mux.HandleFunc("/api/v1/catalog/sync/restore/batch", server.handleCatalogRestoreBatch)
	mux.HandleFunc("/api/v1/catalog/tasks/retry", server.handleCatalogRetryTask)
	mux.HandleFunc("/api/v1/catalog/sync/tasks/retry", server.handleCatalogRetrySyncTask)
	mux.HandleFunc("/api/v1/import/devices", server.handleImportDevices)
	mux.HandleFunc("/api/v1/import/devices/role", server.handleImportDeviceRoleSelection)
	mux.HandleFunc("/api/v1/import/sources/browse", server.handleImportSourceBrowse)
	mux.HandleFunc("/api/v1/import/rules", server.handleImportRules)
	mux.HandleFunc("/api/v1/import/execute", server.handleImportExecute)
	mux.HandleFunc("/api/v1/tools/media/video/analyze", server.handleVideoMediaToolAnalyze)
	mux.HandleFunc("/api/v1/tools/media/audio/analyze", server.handleAudioMediaToolAnalyze)
	mux.HandleFunc("/api/v1/tools/connectors/qnap/test", server.handleQNAPTest)
	mux.HandleFunc("/api/v1/tools/connectors/cloud115/test", server.handleCloud115Test)
	mux.HandleFunc("/api/v1/tools/connectors/cloud115/qrcode/start", server.handleCloud115QRCodeStart)
	mux.HandleFunc("/api/v1/tools/connectors/cloud115/qrcode/poll", server.handleCloud115QRCodePoll)
	mux.HandleFunc("/api/v1/tools/connectors/removable/devices", server.handleRemovableDevices)
	mux.HandleFunc("/api/v1/tools/connectors/removable/test", server.handleRemovableTest)

	server.http = &http.Server{
		Addr:         cfg.HTTPAddress(),
		Handler:      server.corsMiddleware(server.loggingMiddleware(mux)),
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
	}

	return server
}

func (server *Server) Start(_ context.Context) error {
	go func() {
		if err := server.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server stopped unexpectedly", "error", err)
		}
	}()

	time.Sleep(120 * time.Millisecond)
	return nil
}

func (server *Server) Shutdown(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, server.config.HTTPShutdownTimeout)
	defer cancel()

	return server.http.Shutdown(shutdownCtx)
}

func (server *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(recorder, r)
		slog.Info("http request served",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.statusCode,
			"duration", time.Since(startedAt).String(),
		)
	})
}

func (server *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (recorder *statusRecorder) WriteHeader(statusCode int) {
	recorder.statusCode = statusCode
	recorder.ResponseWriter.WriteHeader(statusCode)
}
