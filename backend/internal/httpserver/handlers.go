package httpserver

import (
	"encoding/json"
	"net/http"
)

func (server *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	server.writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
	})
}

func (server *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	server.writeJSON(w, http.StatusOK, map[string]any{
		"status": "ready",
	})
}

func (server *Server) handleBootstrap(w http.ResponseWriter, _ *http.Request) {
	server.writeJSON(w, http.StatusOK, server.system.Bootstrap())
}

func (server *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
