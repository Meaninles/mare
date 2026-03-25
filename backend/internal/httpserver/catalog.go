package httpserver

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"mam/backend/internal/catalog"
)

type endpointScanRequest struct {
	EndpointID string `json:"endpointId"`
}

type retrySyncTaskRequest struct {
	TaskID string `json:"taskId"`
}

func (server *Server) handleCatalogEndpoints(w http.ResponseWriter, r *http.Request) {
	catalogService, ok := server.requireCatalog(w)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		records, err := catalogService.ListEndpoints(r.Context())
		if err != nil {
			server.writeJSON(w, http.StatusInternalServerError, map[string]any{
				"success": false,
				"error":   err.Error(),
			})
			return
		}

		server.writeJSON(w, http.StatusOK, map[string]any{
			"success":   true,
			"endpoints": records,
		})
	case http.MethodPost:
		var request catalog.RegisterEndpointRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			server.writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"error":   "invalid JSON payload",
			})
			return
		}

		record, err := catalogService.RegisterEndpoint(r.Context(), request)
		if err != nil {
			server.writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"error":   err.Error(),
			})
			return
		}

		server.writeJSON(w, http.StatusOK, map[string]any{
			"success":  true,
			"endpoint": record,
		})
	default:
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (server *Server) handleCatalogEndpointResource(w http.ResponseWriter, r *http.Request) {
	catalogService, ok := server.requireCatalog(w)
	if !ok {
		return
	}

	endpointID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/catalog/endpoints/"))
	if endpointID == "" || strings.Contains(endpointID, "/") {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var request catalog.UpdateEndpointRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			server.writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"error":   "invalid JSON payload",
			})
			return
		}

		record, err := catalogService.UpdateEndpoint(r.Context(), endpointID, request)
		if err != nil {
			server.writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"error":   err.Error(),
			})
			return
		}

		server.writeJSON(w, http.StatusOK, map[string]any{
			"success":  true,
			"endpoint": record,
		})
	case http.MethodDelete:
		summary, err := catalogService.DeleteEndpoint(r.Context(), endpointID)
		if err != nil {
			server.writeJSON(w, http.StatusInternalServerError, map[string]any{
				"success": false,
				"error":   err.Error(),
			})
			return
		}

		server.writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"summary": summary,
		})
	default:
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (server *Server) handleCatalogAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	catalogService, ok := server.requireCatalog(w)
	if !ok {
		return
	}

	limit := queryInt(r, "limit", 200)
	offset := queryInt(r, "offset", 0)
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	mediaType := strings.TrimSpace(r.URL.Query().Get("mediaType"))
	assetStatus := strings.TrimSpace(r.URL.Query().Get("status"))

	var (
		records []catalog.AssetRecord
		err     error
	)
	if searchQuery != "" || mediaType != "" || assetStatus != "" {
		records, err = catalogService.SearchAssets(r.Context(), searchQuery, mediaType, assetStatus, limit, offset)
	} else {
		records, err = catalogService.ListAssets(r.Context(), limit, offset)
	}
	if err != nil {
		server.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"assets":  records,
	})
}

func (server *Server) handleCatalogDeleteReplica(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	catalogService, ok := server.requireCatalog(w)
	if !ok {
		return
	}

	var request catalog.DeleteReplicaRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	summary, err := catalogService.DeleteReplica(r.Context(), request)
	if err != nil {
		server.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"summary": summary,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"summary": summary,
	})
}

func (server *Server) handleCatalogUnifiedSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	catalogService, ok := server.requireCatalog(w)
	if !ok {
		return
	}

	limit := queryInt(r, "limit", 30)
	offset := queryInt(r, "offset", 0)
	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	mediaType := strings.TrimSpace(r.URL.Query().Get("mediaType"))
	assetStatus := strings.TrimSpace(r.URL.Query().Get("status"))

	response, err := catalogService.SearchLibrary(r.Context(), searchQuery, mediaType, assetStatus, limit, offset)
	if err != nil {
		server.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"query":    response.Query,
		"results":  response.Results,
		"warnings": response.Warnings,
	})
}

func (server *Server) handleCatalogAssetResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	catalogService, ok := server.requireCatalog(w)
	if !ok {
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/catalog/assets/")
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) != 2 {
		http.NotFound(w, r)
		return
	}

	assetID := strings.TrimSpace(segments[0])
	resource := strings.TrimSpace(segments[1])
	if assetID == "" {
		http.NotFound(w, r)
		return
	}

	switch resource {
	case "poster":
		mediaResource, err := catalogService.ResolvePosterResource(r.Context(), assetID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			server.writeJSON(w, http.StatusInternalServerError, map[string]any{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		server.serveMediaFile(w, r, mediaResource)
	case "preview":
		mediaResource, err := catalogService.ResolvePreviewResource(r.Context(), assetID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				http.NotFound(w, r)
				return
			}
			server.writeJSON(w, http.StatusInternalServerError, map[string]any{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		server.serveMediaFile(w, r, mediaResource)
	case "insights":
		insights, err := catalogService.GetAssetInsights(r.Context(), assetID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			server.writeJSON(w, http.StatusInternalServerError, map[string]any{
				"success": false,
				"error":   err.Error(),
			})
			return
		}

		server.writeJSON(w, http.StatusOK, map[string]any{
			"success":  true,
			"insights": insights,
		})
	default:
		http.NotFound(w, r)
	}
}

func (server *Server) handleCatalogTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	catalogService, ok := server.requireCatalog(w)
	if !ok {
		return
	}

	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)

	tasks, err := catalogService.ListTasks(r.Context(), limit, offset)
	if err != nil {
		server.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"tasks":   tasks,
	})
}

func (server *Server) handleCatalogSyncOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	catalogService, ok := server.requireCatalog(w)
	if !ok {
		return
	}

	overview, err := catalogService.GetSyncOverview(r.Context())
	if err != nil {
		server.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"overview": overview,
	})
}

func (server *Server) handleCatalogRestoreAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	catalogService, ok := server.requireCatalog(w)
	if !ok {
		return
	}

	var request catalog.RestoreAssetRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	summary, err := catalogService.RestoreAsset(r.Context(), request)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if errors.Is(err, strconv.ErrSyntax) {
			statusCode = http.StatusBadRequest
		}
		server.writeJSON(w, statusCode, map[string]any{
			"success": false,
			"summary": summary,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"summary": summary,
	})
}

func (server *Server) handleCatalogRestoreBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	catalogService, ok := server.requireCatalog(w)
	if !ok {
		return
	}

	var request catalog.BatchRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	summary, err := catalogService.RestoreAssetsToEndpoint(r.Context(), request)
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": err == nil,
		"summary": summary,
		"error":   errorText(err),
	})
}

func (server *Server) handleCatalogRetrySyncTask(w http.ResponseWriter, r *http.Request) {
	server.handleCatalogRetryTask(w, r)
}

func (server *Server) handleCatalogRetryTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	catalogService, ok := server.requireCatalog(w)
	if !ok {
		return
	}

	var request retrySyncTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}
	if strings.TrimSpace(request.TaskID) == "" {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "taskId is required",
		})
		return
	}

	summary, err := catalogService.RetryTask(r.Context(), request.TaskID)
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": err == nil,
		"summary": summary,
		"error":   errorText(err),
	})
}

func (server *Server) handleCatalogFullScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	catalogService, ok := server.requireCatalog(w)
	if !ok {
		return
	}

	summary, err := catalogService.FullScan(r.Context())
	if err != nil {
		server.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"summary": summary,
	})
}

func (server *Server) handleCatalogEndpointScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	catalogService, ok := server.requireCatalog(w)
	if !ok {
		return
	}

	var request endpointScanRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}
	if request.EndpointID == "" {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "endpointId is required",
		})
		return
	}

	summary, err := catalogService.RescanEndpoint(r.Context(), request.EndpointID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if errors.Is(err, strconv.ErrSyntax) {
			statusCode = http.StatusBadRequest
		}
		server.writeJSON(w, statusCode, map[string]any{
			"success": false,
			"summary": summary,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"summary": summary,
	})
}

func queryInt(r *http.Request, key string, fallback int) int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (server *Server) serveMediaFile(w http.ResponseWriter, r *http.Request, resource catalog.AssetMediaResource) {
	if resource.Cleanup != nil {
		defer resource.Cleanup()
	}

	file, err := os.Open(resource.FilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		server.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	defer file.Close()

	if resource.ContentType != "" {
		w.Header().Set("Content-Type", resource.ContentType)
	}

	http.ServeContent(w, r, resource.FileName, resource.ModTime, file)
}
