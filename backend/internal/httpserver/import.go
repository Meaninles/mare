package httpserver

import (
	"encoding/json"
	"net/http"

	"mam/backend/internal/catalog"
)

func (server *Server) handleImportDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	devices, err := server.catalog.ListImportDevices(r.Context())
	if err != nil {
		server.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"devices": devices,
	})
}

func (server *Server) handleImportDeviceRoleSelection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var request catalog.SelectImportDeviceRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	result, err := server.catalog.SelectImportDeviceRole(r.Context(), request)
	if err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"result":  result,
	})
}

func (server *Server) handleImportSourceBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var request catalog.ImportSourceBrowseRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	result, err := server.catalog.BrowseImportSource(r.Context(), request)
	if err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"result":  result,
	})
}

func (server *Server) handleImportRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rules, err := server.catalog.ListImportRules(r.Context())
		if err != nil {
			server.writeJSON(w, http.StatusInternalServerError, map[string]any{
				"success": false,
				"error":   err.Error(),
			})
			return
		}

		server.writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"rules":   rules,
		})
	case http.MethodPost:
		var request catalog.SaveImportRulesRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			server.writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"error":   "invalid JSON payload",
			})
			return
		}

		rules, err := server.catalog.SaveImportRules(r.Context(), request)
		if err != nil {
			server.writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"error":   err.Error(),
			})
			return
		}

		server.writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"rules":   rules,
		})
	default:
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (server *Server) handleImportExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var request catalog.ExecuteImportRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	summary, err := server.catalog.ExecuteImport(r.Context(), request)
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": err == nil,
		"summary": summary,
		"error":   errorText(err),
	})
}
