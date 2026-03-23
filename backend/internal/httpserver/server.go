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
	config config.Config
	system platform.SystemState
	catalog *catalog.Service
	http   *http.Server
}

func New(cfg config.Config, system platform.SystemState, catalogService *catalog.Service) *Server {
	server := &Server{
		config: cfg,
		system: system,
		catalog: catalogService,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", server.handleHealth)
	mux.HandleFunc("/readyz", server.handleReady)
	mux.HandleFunc("/api/v1/system/bootstrap", server.handleBootstrap)
	mux.HandleFunc("/api/v1/catalog/endpoints", server.handleCatalogEndpoints)
	mux.HandleFunc("/api/v1/catalog/assets", server.handleCatalogAssets)
	mux.HandleFunc("/api/v1/catalog/assets/", server.handleCatalogAssetResource)
	mux.HandleFunc("/api/v1/catalog/tasks", server.handleCatalogTasks)
	mux.HandleFunc("/api/v1/catalog/scans/full", server.handleCatalogFullScan)
	mux.HandleFunc("/api/v1/catalog/scans/endpoint", server.handleCatalogEndpointScan)
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
		next.ServeHTTP(w, r)
		slog.Info("http request served", "method", r.Method, "path", r.URL.Path, "duration", time.Since(startedAt).String())
	})
}

func (server *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
