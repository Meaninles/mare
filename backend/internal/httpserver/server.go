package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"mam/backend/internal/catalog"
	cd2auth "mam/backend/internal/cd2/auth"
	cd2client "mam/backend/internal/cd2/client"
	cd2cloudapi "mam/backend/internal/cd2/cloudapi"
	cd2fs "mam/backend/internal/cd2/fs"
	cd2runtime "mam/backend/internal/cd2/runtime"
	cd2transfers "mam/backend/internal/cd2/transfers"
	"mam/backend/internal/config"
	"mam/backend/internal/librarysession"
	"mam/backend/internal/platform"
)

type Server struct {
	config       config.Config
	system       platform.SystemState
	session      *librarysession.Manager
	cd2          *cd2runtime.Manager
	cd2grpc      *cd2client.Manager
	cd2auth      *cd2auth.Service
	cd2cloud     *cd2cloudapi.Service
	cd2fs        *cd2fs.Service
	cd2transfers *cd2transfers.Service
	http         *http.Server
}

func New(cfg config.Config, system platform.SystemState, session *librarysession.Manager, cd2 *cd2runtime.Manager, cd2grpc *cd2client.Manager, cd2auth *cd2auth.Service, cd2cloud *cd2cloudapi.Service, cd2fsService *cd2fs.Service, cd2transferService *cd2transfers.Service) *Server {
	server := &Server{
		config:       cfg,
		system:       system,
		session:      session,
		cd2:          cd2,
		cd2grpc:      cd2grpc,
		cd2auth:      cd2auth,
		cd2cloud:     cd2cloud,
		cd2fs:        cd2fsService,
		cd2transfers: cd2transferService,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", server.handleHealth)
	mux.HandleFunc("/readyz", server.handleReady)
	mux.HandleFunc("/api/v1/system/bootstrap", server.handleBootstrap)
	mux.HandleFunc("/api/v1/cd2/runtime/status", server.handleCD2RuntimeStatus)
	mux.HandleFunc("/api/v1/cd2/client/status", server.handleCD2ClientStatus)
	mux.HandleFunc("/api/v1/cd2/auth/profile", server.handleCD2AuthProfile)
	mux.HandleFunc("/api/v1/cd2/auth/refresh", server.handleCD2AuthRefresh)
	mux.HandleFunc("/api/v1/cd2/auth/register", server.handleCD2AuthRegister)
	mux.HandleFunc("/api/v1/cd2/cloud-accounts", server.handleCD2CloudAccounts)
	mux.HandleFunc("/api/v1/cd2/cloud-accounts/", server.handleCD2CloudAccountResource)
	mux.HandleFunc("/api/v1/cd2/cloud-accounts/config", server.handleCD2CloudAccountConfig)
	mux.HandleFunc("/api/v1/cd2/cloud-accounts/115/cookie-import", server.handleCD2115CookieImport)
	mux.HandleFunc("/api/v1/cd2/cloud-accounts/115/qrcode/start", server.handleCD2115QRCodeStart)
	mux.HandleFunc("/api/v1/cd2/cloud-accounts/115open/qrcode/start", server.handleCD2115OpenQRCodeStart)
	mux.HandleFunc("/api/v1/cd2/cloud-accounts/115/qrcode/sessions/", server.handleCD2115QRCodeSession)
	mux.HandleFunc("/api/v1/cd2/files/list", server.handleCD2FileList)
	mux.HandleFunc("/api/v1/cd2/files/search", server.handleCD2FileSearch)
	mux.HandleFunc("/api/v1/cd2/files/stat", server.handleCD2FileStat)
	mux.HandleFunc("/api/v1/cd2/files/detail", server.handleCD2FileDetail)
	mux.HandleFunc("/api/v1/cd2/files/download-url", server.handleCD2FileDownloadURL)
	mux.HandleFunc("/api/v1/cd2/files/folders", server.handleCD2CreateFolder)
	mux.HandleFunc("/api/v1/cd2/files/rename", server.handleCD2RenameFile)
	mux.HandleFunc("/api/v1/cd2/files/move", server.handleCD2MoveFiles)
	mux.HandleFunc("/api/v1/cd2/files/copy", server.handleCD2CopyFiles)
	mux.HandleFunc("/api/v1/cd2/files/delete", server.handleCD2DeleteFiles)
	mux.HandleFunc("/api/v1/cd2/files/upload", server.handleCD2UploadFiles)
	mux.HandleFunc("/api/v1/cd2/transfers", server.handleCD2Transfers)
	mux.HandleFunc("/api/v1/cd2/transfers/actions", server.handleCD2TransferActions)
	mux.HandleFunc("/api/v1/libraries/current", server.handleLibraryCurrent)
	mux.HandleFunc("/api/v1/libraries/create", server.handleLibraryCreate)
	mux.HandleFunc("/api/v1/libraries/open", server.handleLibraryOpen)
	mux.HandleFunc("/api/v1/libraries/close", server.handleLibraryClose)
	mux.HandleFunc("/api/v1/libraries/legacy/status", server.handleLegacyLibraryStatus)
	mux.HandleFunc("/api/v1/libraries/legacy/migrate", server.handleLegacyLibraryMigrate)
	mux.HandleFunc("/api/v1/system/logs", server.handleSystemLogs)
	mux.HandleFunc("/api/v1/settings/transfers", server.handleTransferSettings)
	mux.HandleFunc("/api/v1/settings/backup/export", server.handleSettingsBackupExport)
	mux.HandleFunc("/api/v1/settings/backup/import", server.handleSettingsBackupImport)
	mux.HandleFunc("/api/v1/catalog/endpoints", server.handleCatalogEndpoints)
	mux.HandleFunc("/api/v1/catalog/endpoints/", server.handleCatalogEndpointResource)
	mux.HandleFunc("/api/v1/catalog/assets", server.handleCatalogAssets)
	mux.HandleFunc("/api/v1/catalog/assets/", server.handleCatalogAssetResource)
	mux.HandleFunc("/api/v1/catalog/search", server.handleCatalogUnifiedSearch)
	mux.HandleFunc("/api/v1/catalog/replicas/delete", server.handleCatalogDeleteReplica)
	mux.HandleFunc("/api/v1/catalog/tasks", server.handleCatalogTasks)
	mux.HandleFunc("/api/v1/catalog/transfers", server.handleCatalogTransfers)
	mux.HandleFunc("/api/v1/catalog/transfers/", server.handleCatalogTransferResource)
	mux.HandleFunc("/api/v1/catalog/transfers/pause", server.handleCatalogTransferPause)
	mux.HandleFunc("/api/v1/catalog/transfers/cancel", server.handleCatalogTransferCancel)
	mux.HandleFunc("/api/v1/catalog/transfers/resume", server.handleCatalogTransferResume)
	mux.HandleFunc("/api/v1/catalog/transfers/delete", server.handleCatalogTransferDelete)
	mux.HandleFunc("/api/v1/catalog/transfers/prioritize", server.handleCatalogTransferPrioritize)
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
	mux.HandleFunc("/api/v1/tools/connectors/network-storage/test", server.handleNetworkStorageTest)
	mux.HandleFunc("/api/v1/tools/network-storage/115/qrcode/start", server.handleNetworkStorage115QRCodeStart)
	mux.HandleFunc("/api/v1/tools/network-storage/115/qrcode/poll", server.handleNetworkStorage115QRCodePoll)
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

	if err := server.http.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if server.cd2grpc != nil {
		if err := server.cd2grpc.Close(); err != nil {
			slog.Warn("close cd2 gRPC client failed", "error", err)
		}
	}
	if server.cd2transfers != nil {
		server.cd2transfers.Close()
	}
	return nil
}

func (server *Server) requireCatalog(w http.ResponseWriter) (*catalog.Service, bool) {
	catalogService, err := server.session.Catalog()
	if err == nil {
		return catalogService, true
	}

	statusCode := http.StatusConflict
	server.writeJSON(w, statusCode, map[string]any{
		"success": false,
		"error":   err.Error(),
		"library": server.session.Status(),
	})
	return nil, false
}

func (server *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(recorder, r)
		log := slog.Info
		if shouldSuppressInfoRequestLog(r, recorder.statusCode) {
			log = slog.Debug
		}
		log("http request served",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.statusCode,
			"duration", time.Since(startedAt).String(),
		)
	})
}

func shouldSuppressInfoRequestLog(r *http.Request, statusCode int) bool {
	if r == nil {
		return false
	}
	if statusCode >= http.StatusBadRequest &&
		!(r.Method == http.MethodGet && r.URL.Path == "/api/v1/import/devices" && statusCode == http.StatusConflict) {
		return false
	}

	path := r.URL.Path
	if r.Method == http.MethodGet && (path == "/api/v1/catalog/tasks" || path == "/api/v1/catalog/transfers") {
		return true
	}
	if r.Method == http.MethodGet && path == "/api/v1/import/devices" && statusCode == http.StatusConflict {
		return true
	}
	return false
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
