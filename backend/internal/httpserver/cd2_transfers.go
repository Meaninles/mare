package httpserver

import (
	"encoding/json"
	"net/http"

	cd2transfers "mam/backend/internal/cd2/transfers"
)

func (server *Server) handleCD2Transfers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2transfers == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 transfer service is not configured",
		})
		return
	}

	result, err := server.cd2transfers.List(r.Context())
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
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

func (server *Server) handleCD2TransferActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2transfers == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 transfer service is not configured",
		})
		return
	}

	var request cd2transfers.ActionRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	summary, err := server.cd2transfers.ApplyAction(r.Context(), request)
	if err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   err.Error(),
			"summary": summary,
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"summary": summary,
	})
}
