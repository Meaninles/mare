package httpserver

import (
	"encoding/json"
	"net/http"

	cd2auth "mam/backend/internal/cd2/auth"
)

func (server *Server) handleCD2AuthProfile(w http.ResponseWriter, r *http.Request) {
	if server.cd2auth == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 auth service is not configured",
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		status, err := server.cd2auth.GetStatus(r.Context(), true)
		if err != nil {
			server.writeJSON(w, http.StatusInternalServerError, map[string]any{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		server.writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"auth":    status,
		})
	case http.MethodPut:
		var request cd2auth.UpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			server.writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"error":   "invalid JSON payload",
			})
			return
		}

		status, err := server.cd2auth.Configure(r.Context(), request)
		if err != nil {
			server.writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"auth":    status,
				"error":   err.Error(),
			})
			return
		}
		server.writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"auth":    status,
		})
	case http.MethodDelete:
		status, err := server.cd2auth.Clear(r.Context())
		if err != nil {
			server.writeJSON(w, http.StatusInternalServerError, map[string]any{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		server.writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"auth":    status,
		})
	default:
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (server *Server) handleCD2AuthRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2auth == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 auth service is not configured",
		})
		return
	}

	status, err := server.cd2auth.Refresh(r.Context())
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"auth":    status,
			"error":   err.Error(),
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"auth":    status,
	})
}

func (server *Server) handleCD2AuthRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2auth == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 auth service is not configured",
		})
		return
	}

	var request cd2auth.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	result, err := server.cd2auth.Register(r.Context(), request)
	if err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"result":  result,
			"error":   err.Error(),
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"result":  result,
	})
}
