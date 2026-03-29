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

func (server *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	databaseState := server.session.DatabaseState()
	bootstrap := server.system.Bootstrap()
	cd2State := bootstrap.CD2Runtime
	cd2ClientState := bootstrap.CD2Client
	if server.cd2 != nil {
		cd2State = server.cd2.Probe(r.Context())
	}
	if server.cd2grpc != nil {
		cd2ClientState = server.cd2grpc.Probe(r.Context())
	}

	server.writeJSON(w, http.StatusOK, server.system.BootstrapWithCD2(databaseState, cd2State, cd2ClientState))
}

func (server *Server) handleCD2RuntimeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if server.cd2 == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 runtime manager is not configured",
		})
		return
	}

	state := server.cd2.Probe(r.Context())
	statusCode := http.StatusOK
	if !state.Ready {
		statusCode = http.StatusServiceUnavailable
	}

	server.writeJSON(w, statusCode, map[string]any{
		"success": state.Ready,
		"runtime": state,
	})
}

func (server *Server) handleCD2ClientStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	if server.cd2grpc == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 gRPC client manager is not configured",
		})
		return
	}

	state := server.cd2grpc.Probe(r.Context())
	statusCode := http.StatusOK
	if !state.Ready {
		statusCode = http.StatusServiceUnavailable
	}

	server.writeJSON(w, statusCode, map[string]any{
		"success": state.Ready,
		"client":  state,
	})
}

func (server *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
