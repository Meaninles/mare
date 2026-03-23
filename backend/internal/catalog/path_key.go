package catalog

import (
	"errors"
	"path"
	"strings"
)

// NormalizeLogicalPathKey converts a connector-specific file path into a stable logical path key.
//
// Rules:
// - rootPath differences are removed when physicalPath is rooted under rootPath.
// - both "\" and "/" are treated as path separators.
// - redundant "." and ".." segments are collapsed.
// - leading/trailing separators are removed.
// - the final key is lower-cased so Windows/NAS casing differences do not fork assets.
func NormalizeLogicalPathKey(rootPath, physicalPath string) (string, error) {
	root := canonicalizePath(rootPath)
	physical := canonicalizePath(physicalPath)
	if physical == "" {
		return "", errors.New("physical path is required")
	}

	candidate := physical
	if root != "" {
		rootLower := strings.ToLower(root)
		physicalLower := strings.ToLower(physical)
		switch {
		case physicalLower == rootLower:
			return "", errors.New("physical path points to endpoint root")
		case strings.HasPrefix(physicalLower, rootLower+"/"):
			candidate = physical[len(root)+1:]
		}
	}

	candidate = strings.TrimPrefix(candidate, "/")
	candidate = canonicalizePath(candidate)
	candidate = strings.TrimPrefix(candidate, "/")
	if candidate == "" {
		return "", errors.New("logical path key is empty")
	}

	return strings.ToLower(candidate), nil
}

func canonicalizePath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	normalized := strings.ReplaceAll(trimmed, `\`, "/")
	cleaned := path.Clean(normalized)
	if cleaned == "." {
		return ""
	}

	return strings.TrimSuffix(cleaned, "/")
}
