package httpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
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

func (server *Server) handleCD2FileDownload(w http.ResponseWriter, r *http.Request) {
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

	filePath := strings.TrimSpace(r.URL.Query().Get("path"))
	if filePath == "" {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "path is required",
		})
		return
	}

	entry, err := server.cd2fs.Stat(r.Context(), filePath)
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	if entry.IsDirectory {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "directories are not supported for direct download test",
		})
		return
	}

	info, err := server.cd2fs.GetDownloadURL(r.Context(), cd2fs.DownloadURLRequest{
		Path:      filePath,
		GetDirect: true,
	})
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	downloadURL, err := server.resolveCD2DownloadURL(info)
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, downloadURL, nil)
	if err != nil {
		server.writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	if userAgent := strings.TrimSpace(info.UserAgent); userAgent != "" {
		request.Header.Set("User-Agent", userAgent)
	}
	for key, value := range info.AdditionalHeader {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		request.Header.Set(key, value)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		server.writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"error":   fmt.Sprintf("download source returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body))),
		})
		return
	}

	if contentType := strings.TrimSpace(response.Header.Get("Content-Type")); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	if contentLength := strings.TrimSpace(response.Header.Get("Content-Length")); contentLength != "" {
		w.Header().Set("Content-Length", contentLength)
	}
	w.Header().Set("Content-Disposition", buildAttachmentDisposition(entry.Name))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, response.Body)
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

func (server *Server) resolveCD2DownloadURL(info cd2fs.DownloadURLInfo) (string, error) {
	if directURL := strings.TrimSpace(info.DirectURL); directURL != "" {
		return directURL, nil
	}

	downloadPath := strings.TrimSpace(info.DownloadURLPath)
	if downloadPath == "" {
		return "", fmt.Errorf("cd2 did not return a downloadable URL")
	}
	if strings.HasPrefix(downloadPath, "http://") || strings.HasPrefix(downloadPath, "https://") {
		return downloadPath, nil
	}

	if server.cd2 == nil {
		return "", fmt.Errorf("cd2 runtime is not configured")
	}
	runtimeState := server.cd2.Snapshot()
	baseURL := strings.TrimSpace(runtimeState.BaseURL)
	if baseURL == "" {
		return "", fmt.Errorf("cd2 runtime base URL is not configured")
	}

	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	resolvedPath := strings.ReplaceAll(downloadPath, "{SCHEME}", parsedBase.Scheme)
	resolvedPath = strings.ReplaceAll(resolvedPath, "{HOST}", parsedBase.Host)
	resolvedPath = strings.ReplaceAll(resolvedPath, "{PREVIEW}", "")
	if strings.HasPrefix(resolvedPath, "http://") || strings.HasPrefix(resolvedPath, "https://") {
		return resolvedPath, nil
	}
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(resolvedPath, "/"), nil
}

func buildAttachmentDisposition(fileName string) string {
	name := strings.TrimSpace(fileName)
	if name == "" {
		name = "cd2-download.bin"
	}
	safeName := strings.ReplaceAll(name, "\"", "_")
	return fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", safeName, url.PathEscape(path.Base(name)))
}
