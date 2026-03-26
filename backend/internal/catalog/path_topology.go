package catalog

import (
	"fmt"
	pathpkg "path"
	"path/filepath"
	"strings"

	"mam/backend/internal/connectors"
	"mam/backend/internal/store"
)

const importSourceRoleMode = "IMPORT_SOURCE"

func normalizeEndpointRoleMode(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", defaultRoleMode:
		return defaultRoleMode
	case importSourceRoleMode:
		return importSourceRoleMode
	default:
		return ""
	}
}

func isManagedEndpoint(endpoint store.StorageEndpoint) bool {
	return normalizeEndpointRoleMode(endpoint.RoleMode) == defaultRoleMode
}

func canonicalLogicalPath(value string) string {
	return strings.TrimPrefix(canonicalizePath(value), "/")
}

func canonicalDirectoryPath(value string) string {
	logicalPath := canonicalLogicalPath(value)
	if logicalPath == "" {
		return ""
	}

	directory := pathpkg.Dir(logicalPath)
	if directory == "." {
		return ""
	}

	return strings.TrimPrefix(directory, "/")
}

func deriveReplicaRelativePath(endpoint store.StorageEndpoint, physicalPath string, fallbackLogicalPath string) string {
	if strings.Contains(strings.TrimSpace(physicalPath), "://") {
		return canonicalLogicalPath(fallbackLogicalPath)
	}

	if relativePath, err := NormalizeLogicalPathKey(endpoint.RootPath, physicalPath); err == nil {
		return relativePath
	}

	return canonicalLogicalPath(fallbackLogicalPath)
}

func matchesCanonicalLogicalPath(endpoint store.StorageEndpoint, physicalPath string, logicalPath string) bool {
	return strings.EqualFold(
		deriveReplicaRelativePath(endpoint, physicalPath, logicalPath),
		canonicalLogicalPath(logicalPath),
	)
}

func resolveReplicaDirectoryPath(endpoint store.StorageEndpoint, logicalPath string) string {
	directory := canonicalDirectoryPath(logicalPath)
	rootPath := strings.TrimSpace(endpoint.RootPath)

	switch normalizeEndpointType(endpoint.EndpointType) {
	case string(connectors.EndpointTypeCloud115):
		if rootPath == "" {
			return directory
		}
		if directory == "" {
			return fmt.Sprintf("%s:/", rootPath)
		}
		return fmt.Sprintf("%s:/%s", rootPath, directory)
	case string(connectors.EndpointTypeAList), string(connectors.EndpointTypeNetwork):
		if rootPath == "" {
			if directory == "" {
				return "/"
			}
			return "/" + strings.TrimPrefix(directory, "/")
		}
		if directory == "" {
			return canonicalizePath(rootPath)
		}
		return canonicalizePath(pathpkg.Join(rootPath, directory))
	default:
		if rootPath == "" {
			return filepath.FromSlash(directory)
		}
		if directory == "" {
			return filepath.Clean(rootPath)
		}
		return filepath.Clean(filepath.Join(rootPath, filepath.FromSlash(directory)))
	}
}

func canonicalReplicaPhysicalPath(endpoint store.StorageEndpoint, logicalPath string) string {
	relativePath := canonicalLogicalPath(logicalPath)
	rootPath := strings.TrimSpace(endpoint.RootPath)

	switch normalizeEndpointType(endpoint.EndpointType) {
	case string(connectors.EndpointTypeCloud115):
		if rootPath == "" {
			return relativePath
		}
		if relativePath == "" {
			return fmt.Sprintf("%s:/", rootPath)
		}
		return fmt.Sprintf("%s:/%s", rootPath, relativePath)
	case string(connectors.EndpointTypeAList), string(connectors.EndpointTypeNetwork):
		if rootPath == "" {
			if relativePath == "" {
				return "/"
			}
			return "/" + strings.TrimPrefix(relativePath, "/")
		}
		if relativePath == "" {
			return canonicalizePath(rootPath)
		}
		return canonicalizePath(pathpkg.Join(rootPath, relativePath))
	default:
		if rootPath == "" {
			return filepath.FromSlash(relativePath)
		}
		if relativePath == "" {
			return filepath.Clean(rootPath)
		}
		return filepath.Clean(filepath.Join(rootPath, filepath.FromSlash(relativePath)))
	}
}
