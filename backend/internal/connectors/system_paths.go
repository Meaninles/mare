package connectors

import "strings"

var ignoredSystemPathSegments = map[string]struct{}{
	"$recycle.bin":              {},
	"recycler":                  {},
	"system volume information": {},
}

// ShouldIgnoreAssetPath reports whether a filesystem-like path points to
// Windows/system junk that should not participate in media scanning.
func ShouldIgnoreAssetPath(path string) bool {
	normalized := strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if normalized == "" {
		return false
	}

	for _, segment := range strings.Split(normalized, "/") {
		key := strings.ToLower(strings.TrimSpace(segment))
		if key == "" || key == "." {
			continue
		}
		if _, ignored := ignoredSystemPathSegments[key]; ignored {
			return true
		}
	}

	return false
}
