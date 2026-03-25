package connectors

import (
	"path/filepath"
	"strings"
)

var imageExtensions = map[string]struct{}{
	".jpg": {}, ".jpeg": {}, ".png": {}, ".webp": {}, ".bmp": {}, ".gif": {}, ".tif": {}, ".tiff": {}, ".heic": {}, ".raw": {}, ".dng": {},
}

var videoExtensions = map[string]struct{}{
	".mp4": {}, ".mov": {}, ".mkv": {}, ".avi": {}, ".m4v": {}, ".wmv": {}, ".flv": {}, ".webm": {}, ".mts": {}, ".m2ts": {},
}

var audioExtensions = map[string]struct{}{
	".mp3": {}, ".wav": {}, ".flac": {}, ".aac": {}, ".m4a": {}, ".ogg": {}, ".wma": {}, ".aiff": {}, ".opus": {},
}

func DetectMediaType(path string, isDir bool) MediaType {
	if isDir {
		return MediaTypeUnknown
	}
	if ShouldIgnoreAssetPath(path) {
		return MediaTypeUnknown
	}

	extension := strings.ToLower(filepath.Ext(path))

	if _, ok := imageExtensions[extension]; ok {
		return MediaTypeImage
	}
	if _, ok := videoExtensions[extension]; ok {
		return MediaTypeVideo
	}
	if _, ok := audioExtensions[extension]; ok {
		return MediaTypeAudio
	}

	return MediaTypeUnknown
}
