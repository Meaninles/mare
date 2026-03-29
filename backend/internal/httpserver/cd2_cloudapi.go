package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"

	cd2cloudapi "mam/backend/internal/cd2/cloudapi"
)

func (server *Server) handleCD2CloudAccounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2cloud == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 cloud api service is not configured",
		})
		return
	}

	accounts, err := server.cd2cloud.ListAccounts(r.Context())
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"accounts": accounts,
	})
}

func (server *Server) handleCD2CloudAccountResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.NotFound(w, r)
		return
	}
	if server.cd2cloud == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 cloud api service is not configured",
		})
		return
	}

	trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/cd2/cloud-accounts/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		http.NotFound(w, r)
		return
	}

	request := cd2cloudapi.RemoveAccountRequest{
		CloudName:       strings.TrimSpace(parts[0]),
		UserName:        strings.TrimSpace(parts[1]),
		PermanentRemove: strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("permanent")), "true") || strings.TrimSpace(r.URL.Query().Get("permanent")) == "1",
	}
	if err := server.cd2cloud.RemoveAccount(r.Context(), request); err != nil {
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
	})
}

func (server *Server) handleCD2CloudAccountConfig(w http.ResponseWriter, r *http.Request) {
	if server.cd2cloud == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 cloud api service is not configured",
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		cloudName := strings.TrimSpace(r.URL.Query().Get("cloudName"))
		userName := strings.TrimSpace(r.URL.Query().Get("userName"))
		config, err := server.cd2cloud.GetAccountConfig(r.Context(), cloudName, userName)
		if err != nil {
			server.writeJSON(w, http.StatusBadGateway, map[string]any{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		server.writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"config":  config,
		})
	case http.MethodPut:
		var request cd2cloudapi.UpdateAccountConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			server.writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"error":   "invalid JSON payload",
			})
			return
		}

		config, err := server.cd2cloud.UpdateAccountConfig(r.Context(), request)
		if err != nil {
			server.writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		server.writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"config":  config,
		})
	default:
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (server *Server) handleCD2115CookieImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2cloud == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 cloud api service is not configured",
		})
		return
	}

	var request cd2cloudapi.Import115CookieRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	result, err := server.cd2cloud.Import115Cookie(r.Context(), request)
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

func (server *Server) handleCD2115QRCodeStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2cloud == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 cloud api service is not configured",
		})
		return
	}

	var request cd2cloudapi.Start115QRCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil && err.Error() != "EOF" {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	session, err := server.cd2cloud.Start115QRCode(r.Context(), request)
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"session": session,
	})
}

func (server *Server) handleCD2115OpenQRCodeStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2cloud == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 cloud api service is not configured",
		})
		return
	}

	var request cd2cloudapi.Start115QRCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil && err.Error() != "EOF" {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	session, err := server.cd2cloud.Start115OpenQRCode(r.Context(), request)
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"session": session,
	})
}

func (server *Server) handleCD2115QRCodeSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	if server.cd2cloud == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 cloud api service is not configured",
		})
		return
	}

	sessionID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/cd2/cloud-accounts/115/qrcode/sessions/"))
	if sessionID == "" || strings.Contains(sessionID, "/") {
		http.NotFound(w, r)
		return
	}

	session, err := server.cd2cloud.GetQRCodeSession(sessionID)
	if err != nil {
		server.writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"session": session,
	})
}
