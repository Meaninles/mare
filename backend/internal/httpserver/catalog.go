package httpserver

import (
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

func (server *Server) handleCatalogEndpoints(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		records, err := server.catalog.ListEndpoints(r.Context())
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

		record, err := server.catalog.RegisterEndpoint(r.Context(), request)
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

func (server *Server) handleCatalogAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	limit := queryInt(r, "limit", 200)
	offset := queryInt(r, "offset", 0)

	records, err := server.catalog.ListAssets(r.Context(), limit, offset)
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

func (server *Server) handleCatalogAssetResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
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
		mediaResource, err := server.catalog.ResolvePosterResource(r.Context(), assetID)
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
		mediaResource, err := server.catalog.ResolvePreviewResource(r.Context(), assetID)
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
	default:
		http.NotFound(w, r)
	}
}

func (server *Server) handleCatalogTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)

	tasks, err := server.catalog.ListTasks(r.Context(), limit, offset)
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

func (server *Server) handleCatalogFullScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	summary, err := server.catalog.FullScan(r.Context())
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

	summary, err := server.catalog.RescanEndpoint(r.Context(), request.EndpointID)
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
