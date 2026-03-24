package httpserver

import (
	"encoding/json"
	"net/http"

	"mam/backend/internal/catalog"
	"mam/backend/internal/platform"
)

func (server *Server) handleSettingsBackupExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var request catalog.ExportBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	bundle, err := server.catalog.ExportSettingsBackup(r.Context(), server.config, request)
	if err != nil {
		server.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"bundle":  bundle,
	})
}

func (server *Server) handleSettingsBackupImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var request catalog.ImportBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	summary, err := server.catalog.ImportSettingsBackup(r.Context(), request)
	if err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
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

func (server *Server) handleSystemLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	limit := queryInt(r, "limit", 50)
	level := r.URL.Query().Get("level")
	entries, err := platform.ReadRecentLogEntries(server.config.LogFilePath, limit, level)
	if err != nil {
		server.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"logFilePath": server.config.LogFilePath,
		"limit":       limit,
		"entries":     entries,
	})
}
