package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"mam/backend/internal/connectors"
)

type connectorTestRequest struct {
	Name               string                 `json:"name"`
	Operation          string                 `json:"operation"`
	AppType            string                 `json:"appType"`
	Path               string                 `json:"path"`
	DestinationPath    string                 `json:"destinationPath"`
	NewName            string                 `json:"newName"`
	Recursive          bool                   `json:"recursive"`
	IncludeDirectories bool                   `json:"includeDirectories"`
	MediaOnly          bool                   `json:"mediaOnly"`
	Limit              int                    `json:"limit"`
	Content            string                 `json:"content"`
	RootPath           string                 `json:"rootPath"`
	SharePath          string                 `json:"sharePath"`
	RootID             string                 `json:"rootId"`
	AccessToken        string                 `json:"accessToken"`
	Device             *connectors.DeviceInfo `json:"device"`
	QRUID              string                 `json:"qrUid"`
	QRTime             int64                  `json:"qrTime"`
	QRSign             string                 `json:"qrSign"`
}

type connectorTestResponse struct {
	Success       bool                              `json:"success"`
	Connector     string                            `json:"connector"`
	Operation     string                            `json:"operation"`
	HealthStatus  connectors.HealthStatus           `json:"healthStatus,omitempty"`
	Descriptor    *connectors.Descriptor            `json:"descriptor,omitempty"`
	Entry         *connectors.FileEntry             `json:"entry,omitempty"`
	Entries       []connectors.FileEntry            `json:"entries,omitempty"`
	Content       string                            `json:"content,omitempty"`
	QRCodeSession *connectors.Cloud115QRCodeSession `json:"qrCodeSession,omitempty"`
	Error         string                            `json:"error,omitempty"`
}

func (server *Server) handleQNAPTest(w http.ResponseWriter, r *http.Request) {
	server.handleConnectorTest(w, r, "qnap", func(request connectorTestRequest) (connectors.Connector, error) {
		return connectors.NewQNAPConnector(connectors.QNAPConfig{
			Name:      request.Name,
			SharePath: request.SharePath,
		})
	})
}

func (server *Server) handleCloud115Test(w http.ResponseWriter, r *http.Request) {
	server.handleConnectorTest(w, r, "cloud115", func(request connectorTestRequest) (connectors.Connector, error) {
		return connectors.NewCloud115Connector(connectors.Cloud115Config{
			Name:        request.Name,
			RootID:      request.RootID,
			AccessToken: request.AccessToken,
			AppType:     request.AppType,
		}, nil)
	})
}

func (server *Server) handleCloud115QRCodeStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var request connectorTestRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, connectorTestResponse{
			Success:   false,
			Connector: "cloud115",
			Operation: "qrcode_start",
			Error:     "invalid JSON payload",
		})
		return
	}

	session, err := connectors.StartCloud115QRCodeLogin(r.Context(), request.AppType)
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, connectorTestResponse{
			Success:   false,
			Connector: "cloud115",
			Operation: "qrcode_start",
			Error:     err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, connectorTestResponse{
		Success:       true,
		Connector:     "cloud115",
		Operation:     "qrcode_start",
		QRCodeSession: session,
	})
}

func (server *Server) handleCloud115QRCodePoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var request connectorTestRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, connectorTestResponse{
			Success:   false,
			Connector: "cloud115",
			Operation: "qrcode_poll",
			Error:     "invalid JSON payload",
		})
		return
	}

	session, err := connectors.PollCloud115QRCodeLogin(r.Context(), request.AppType, request.QRUID, request.QRTime, request.QRSign)
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, connectorTestResponse{
			Success:   false,
			Connector: "cloud115",
			Operation: "qrcode_poll",
			Error:     err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, connectorTestResponse{
		Success:       true,
		Connector:     "cloud115",
		Operation:     "qrcode_poll",
		QRCodeSession: session,
	})
}

func (server *Server) handleRemovableDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	devices, err := connectors.NewWindowsUSBEnumerator().ListDevices(ctx)
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
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

func (server *Server) handleRemovableTest(w http.ResponseWriter, r *http.Request) {
	server.handleConnectorTest(w, r, "removable", func(request connectorTestRequest) (connectors.Connector, error) {
		if request.Device == nil {
			return nil, errors.New("device payload is required")
		}

		return connectors.NewRemovableConnector(connectors.RemovableConfig{
			Name:   request.Name,
			Device: *request.Device,
		})
	})
}

func (server *Server) handleConnectorTest(
	w http.ResponseWriter,
	r *http.Request,
	connectorName string,
	factory func(request connectorTestRequest) (connectors.Connector, error),
) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var request connectorTestRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, connectorTestResponse{
			Success:   false,
			Connector: connectorName,
			Error:     "invalid JSON payload",
		})
		return
	}

	connector, err := factory(request)
	if err != nil {
		server.writeJSON(w, http.StatusBadRequest, connectorTestResponse{
			Success:   false,
			Connector: connectorName,
			Operation: request.Operation,
			Error:     err.Error(),
		})
		return
	}

	response, statusCode := executeConnectorOperation(r.Context(), connectorName, connector, request)
	server.writeJSON(w, statusCode, response)
}

func executeConnectorOperation(
	ctx context.Context,
	connectorName string,
	connector connectors.Connector,
	request connectorTestRequest,
) (connectorTestResponse, int) {
	response := connectorTestResponse{
		Success:   false,
		Connector: connectorName,
		Operation: request.Operation,
		Descriptor: func() *connectors.Descriptor {
			descriptor := connector.Descriptor()
			return &descriptor
		}(),
	}

	switch request.Operation {
	case "health_check":
		status, err := connector.HealthCheck(ctx)
		if err != nil {
			response.Error = err.Error()
			return response, http.StatusBadGateway
		}
		response.Success = true
		response.HealthStatus = status
		return response, http.StatusOK
	case "list_entries":
		entries, err := connector.ListEntries(ctx, connectors.ListEntriesRequest{
			Path:               request.Path,
			Recursive:          request.Recursive,
			IncludeDirectories: request.IncludeDirectories,
			MediaOnly:          request.MediaOnly,
			Limit:              request.Limit,
		})
		if err != nil {
			response.Error = err.Error()
			return response, http.StatusBadGateway
		}
		response.Success = true
		response.Entries = entries
		return response, http.StatusOK
	case "stat_entry":
		entry, err := connector.StatEntry(ctx, request.Path)
		if err != nil {
			response.Error = err.Error()
			return response, http.StatusBadGateway
		}
		response.Success = true
		response.Entry = &entry
		return response, http.StatusOK
	case "copy_in":
		entry, err := connector.CopyIn(ctx, request.DestinationPath, strings.NewReader(request.Content))
		if err != nil {
			response.Error = err.Error()
			return response, http.StatusBadGateway
		}
		response.Success = true
		response.Entry = &entry
		return response, http.StatusOK
	case "copy_out":
		var buffer bytes.Buffer
		if err := connector.CopyOut(ctx, request.Path, &buffer); err != nil {
			response.Error = err.Error()
			return response, http.StatusBadGateway
		}
		response.Success = true
		response.Content = buffer.String()
		return response, http.StatusOK
	case "delete_entry":
		if err := connector.DeleteEntry(ctx, request.Path); err != nil {
			response.Error = err.Error()
			return response, http.StatusBadGateway
		}
		response.Success = true
		return response, http.StatusOK
	case "rename_entry":
		entry, err := connector.RenameEntry(ctx, request.Path, request.NewName)
		if err != nil {
			response.Error = err.Error()
			return response, http.StatusBadGateway
		}
		response.Success = true
		response.Entry = &entry
		return response, http.StatusOK
	case "move_entry":
		entry, err := connector.MoveEntry(ctx, request.Path, request.DestinationPath)
		if err != nil {
			response.Error = err.Error()
			return response, http.StatusBadGateway
		}
		response.Success = true
		response.Entry = &entry
		return response, http.StatusOK
	case "make_directory":
		entry, err := connector.MakeDirectory(ctx, request.Path)
		if err != nil {
			response.Error = err.Error()
			return response, http.StatusBadGateway
		}
		response.Success = true
		response.Entry = &entry
		return response, http.StatusOK
	case "read_stream":
		reader, err := connector.ReadStream(ctx, request.Path)
		if err != nil {
			response.Error = err.Error()
			return response, http.StatusBadGateway
		}
		defer reader.Close()

		body, readErr := io.ReadAll(reader)
		if readErr != nil {
			response.Error = readErr.Error()
			return response, http.StatusBadGateway
		}

		response.Success = true
		response.Content = string(body)
		return response, http.StatusOK
	default:
		response.Error = "unsupported operation"
		return response, http.StatusBadRequest
	}
}
