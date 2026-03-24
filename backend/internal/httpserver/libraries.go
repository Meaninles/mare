package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
)

type libraryPathRequest struct {
	Path string `json:"path"`
}

type legacyLibraryMigrateRequest struct {
	TargetPath  string `json:"targetPath"`
	LibraryName string `json:"libraryName"`
}

func (server *Server) handleLibraryCurrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"library": server.session.Status(),
	})
}

func (server *Server) handleLibraryCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var request libraryPathRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}
	if strings.TrimSpace(request.Path) == "" {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "path is required",
		})
		return
	}

	status, err := server.session.CreateLibrary(r.Context(), request.Path)
	if err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"library": status,
	})
}

func (server *Server) handleLibraryOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var request libraryPathRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}
	if strings.TrimSpace(request.Path) == "" {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "path is required",
		})
		return
	}

	status, err := server.session.OpenLibrary(r.Context(), request.Path)
	if err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"library": status,
	})
}

func (server *Server) handleLibraryClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	status, err := server.session.CloseLibrary(r.Context())
	if err != nil {
		server.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"library": status,
	})
}

func (server *Server) handleLegacyLibraryStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	status, err := server.session.LegacyCatalogStatus(r.Context(), server.config.CatalogDBPath)
	if err != nil {
		server.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"legacy":  status,
	})
}

func (server *Server) handleLegacyLibraryMigrate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var request legacyLibraryMigrateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	result, err := server.session.MigrateLegacyCatalog(
		r.Context(),
		server.config.CatalogDBPath,
		request.TargetPath,
		request.LibraryName,
	)
	if err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"migration": result,
		"library":   result.Library,
	})
}
