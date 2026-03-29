package httpserver

import (
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"

	cd2fs "mam/backend/internal/cd2/fs"
)

func (server *Server) handleCD2FileList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2fs == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 file service is not configured",
		})
		return
	}

	entries, path, err := server.cd2fs.List(r.Context(), r.URL.Query().Get("path"), parseBoolValue(r.URL.Query().Get("forceRefresh")))
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"currentPath": path,
		"entries":     entries,
	})
}

func (server *Server) handleCD2FileSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2fs == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 file service is not configured",
		})
		return
	}

	entries, path, err := server.cd2fs.Search(r.Context(), cd2fs.SearchRequest{
		Path:          r.URL.Query().Get("path"),
		Query:         r.URL.Query().Get("query"),
		ForceRefresh:  parseBoolValue(r.URL.Query().Get("forceRefresh")),
		FuzzyMatch:    parseBoolValue(r.URL.Query().Get("fuzzyMatch")),
		ContentSearch: parseBoolValue(r.URL.Query().Get("contentSearch")),
	})
	if err != nil {
		statusCode := http.StatusBadGateway
		if strings.Contains(err.Error(), "不能为空") {
			statusCode = http.StatusBadRequest
		}
		server.writeJSON(w, statusCode, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"currentPath": path,
		"entries":     entries,
	})
}

func (server *Server) handleCD2FileStat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2fs == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 file service is not configured",
		})
		return
	}

	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "path is required",
		})
		return
	}

	entry, err := server.cd2fs.Stat(r.Context(), path)
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"entry":   entry,
	})
}

func (server *Server) handleCD2FileDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2fs == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 file service is not configured",
		})
		return
	}

	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "path is required",
		})
		return
	}

	detail, err := server.cd2fs.GetDetailProperties(r.Context(), path, parseBoolValue(r.URL.Query().Get("forceRefresh")))
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"detail":  detail,
	})
}

func (server *Server) handleCD2FileDownloadURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2fs == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 file service is not configured",
		})
		return
	}

	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "path is required",
		})
		return
	}

	info, err := server.cd2fs.GetDownloadURL(r.Context(), cd2fs.DownloadURLRequest{
		Path:      path,
		Preview:   parseBoolValue(r.URL.Query().Get("preview")),
		LazyRead:  parseBoolValue(r.URL.Query().Get("lazyRead")),
		GetDirect: parseBoolValue(r.URL.Query().Get("getDirect")),
	})
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"info":    info,
	})
}

func (server *Server) handleCD2CreateFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2fs == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 file service is not configured",
		})
		return
	}

	var request cd2fs.CreateFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	entry, result, err := server.cd2fs.CreateFolder(r.Context(), request)
	if err != nil {
		statusCode := http.StatusBadGateway
		if strings.Contains(err.Error(), "不能为空") {
			statusCode = http.StatusBadRequest
		}
		server.writeJSON(w, statusCode, map[string]any{
			"success": false,
			"error":   err.Error(),
			"result":  result,
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"entry":   entry,
		"result":  result,
	})
}

func (server *Server) handleCD2RenameFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2fs == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 file service is not configured",
		})
		return
	}

	var request cd2fs.RenameRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	result, err := server.cd2fs.Rename(r.Context(), request)
	if err != nil {
		statusCode := http.StatusBadGateway
		if strings.Contains(err.Error(), "不能为空") || strings.Contains(err.Error(), "根目录") {
			statusCode = http.StatusBadRequest
		}
		server.writeJSON(w, statusCode, map[string]any{
			"success": false,
			"error":   err.Error(),
			"result":  result,
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"result":  result,
	})
}

func (server *Server) handleCD2MoveFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2fs == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 file service is not configured",
		})
		return
	}

	var request cd2fs.MoveRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	result, err := server.cd2fs.Move(r.Context(), request)
	if err != nil {
		statusCode := http.StatusBadGateway
		if strings.Contains(err.Error(), "至少要选择") || strings.Contains(err.Error(), "冲突策略") {
			statusCode = http.StatusBadRequest
		}
		server.writeJSON(w, statusCode, map[string]any{
			"success": false,
			"error":   err.Error(),
			"result":  result,
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"result":  result,
	})
}

func (server *Server) handleCD2CopyFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2fs == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 file service is not configured",
		})
		return
	}

	var request cd2fs.CopyRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	result, err := server.cd2fs.Copy(r.Context(), request)
	if err != nil {
		statusCode := http.StatusBadGateway
		if strings.Contains(err.Error(), "至少要选择") || strings.Contains(err.Error(), "冲突策略") {
			statusCode = http.StatusBadRequest
		}
		server.writeJSON(w, statusCode, map[string]any{
			"success": false,
			"error":   err.Error(),
			"result":  result,
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"result":  result,
	})
}

func (server *Server) handleCD2DeleteFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2fs == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 file service is not configured",
		})
		return
	}

	var request cd2fs.DeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid JSON payload",
		})
		return
	}

	result, err := server.cd2fs.Delete(r.Context(), request)
	if err != nil {
		statusCode := http.StatusBadGateway
		if strings.Contains(err.Error(), "至少要选择") {
			statusCode = http.StatusBadRequest
		}
		server.writeJSON(w, statusCode, map[string]any{
			"success": false,
			"error":   err.Error(),
			"result":  result,
		})
		return
	}
	server.writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"result":  result,
	})
}

func (server *Server) handleCD2UploadFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if server.cd2fs == nil {
		server.writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"error":   "cd2 file service is not configured",
		})
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid multipart form",
		})
		return
	}

	parentPath := strings.TrimSpace(r.FormValue("parentPath"))
	if parentPath == "" {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "parentPath is required",
		})
		return
	}

	fileHeaders := normalizeMultipartFileHeaders(r)
	if len(fileHeaders) == 0 {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "files are required",
		})
		return
	}

	uploaded := make([]cd2fs.UploadResult, 0, len(fileHeaders))
	for _, header := range fileHeaders {
		file, err := header.Open()
		if err != nil {
			server.writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"error":   err.Error(),
			})
			return
		}

		result, uploadErr := server.cd2fs.Upload(r.Context(), parentPath, header.Filename, file)
		_ = file.Close()
		if uploadErr != nil {
			server.writeJSON(w, http.StatusBadGateway, map[string]any{
				"success":  false,
				"error":    uploadErr.Error(),
				"uploaded": uploaded,
			})
			return
		}
		uploaded = append(uploaded, result)
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"parentPath": parentPath,
		"uploaded":   uploaded,
	})
}

func parseBoolValue(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "true" || normalized == "1" || normalized == "yes" || normalized == "on"
}

func normalizeMultipartFileHeaders(r *http.Request) []*multipart.FileHeader {
	if r == nil || r.MultipartForm == nil {
		return nil
	}
	headers := append([]*multipart.FileHeader(nil), r.MultipartForm.File["files"]...)
	if len(headers) == 0 {
		headers = append(headers, r.MultipartForm.File["file"]...)
	}
	return headers
}
