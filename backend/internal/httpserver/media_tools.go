package httpserver

import (
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"mam/backend/internal/catalog"
)

const mediaToolUploadLimit = 512 << 20

func (server *Server) handleVideoMediaToolAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	file, fileHeader, overrides, err := parseMediaToolUpload(r)
	if err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	defer file.Close()

	analysis, err := server.catalog.AnalyzeUploadedVideo(r.Context(), fileHeader.Filename, file, overrides)
	if err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"analysis": analysis,
	})
}

func (server *Server) handleAudioMediaToolAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	file, fileHeader, overrides, err := parseMediaToolUpload(r)
	if err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	defer file.Close()

	analysis, err := server.catalog.AnalyzeUploadedAudio(r.Context(), fileHeader.Filename, file, overrides)
	if err != nil {
		server.writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	server.writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"analysis": analysis,
	})
}

func parseMediaToolUpload(r *http.Request) (multipartFile multipartFileCloser, fileHeader *multipart.FileHeader, overrides catalog.MediaToolOverrides, err error) {
	if err = r.ParseMultipartForm(mediaToolUploadLimit); err != nil {
		return nil, nil, overrides, err
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, nil, overrides, err
	}

	overrides = catalog.MediaToolOverrides{
		FFmpegPath:  strings.TrimSpace(r.FormValue("ffmpegPath")),
		FFprobePath: strings.TrimSpace(r.FormValue("ffprobePath")),
	}

	return file, header, overrides, nil
}

type multipartFileCloser interface {
	Close() error
	io.Reader
}
